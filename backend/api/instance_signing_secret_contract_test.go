package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
	"openreader/backend/services/authsession"
	"openreader/backend/services/backup"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

// When OPENREADER_JWT_SECRET is blank or a published default, nothing may be
// signed with it: the server keeps a random secret of its own in the data
// directory and signs sessions and resource capabilities with that instead.

const (
	instanceSecretFileName        = "instance-secret"
	instanceSecretCustomSecret    = "instance-secret-contract-custom-secret"
	epubCapabilityPurpose         = "openreader:epub-resource:v1"
	cbzCapabilityPurpose          = "openreader:cbz-resource:v1"
	audioCapabilityPurpose        = "openreader:audio-resource:v1"
	chapterImageCapabilityPurpose = "openreader:chapter-image:v1"
	umdCoverCapabilityPurpose     = "openreader:umd-cover:v1"
	coverCapabilityPurpose        = "openreader:cover-image:v1"
)

var instanceSecretFormat = regexp.MustCompile(`^[0-9a-f]{64}\n$`)

// publishedSigningValues lists what a server configured with configured must
// never sign with: the configured value itself and both published defaults.
func publishedSigningValues(configured string) []string {
	values := []string{publishedCurrentJWTSecret, publishedPreviousJWTSecret}
	if configured != publishedCurrentJWTSecret && configured != publishedPreviousJWTSecret {
		values = append([]string{configured}, values...)
	}
	return values
}

func instanceSecretTestConfig(t *testing.T, secret string) config.Config {
	t.Helper()
	return config.Config{
		DataDir:       t.TempDir(),
		CacheDir:      t.TempDir(),
		LibraryDir:    t.TempDir(),
		DatabasePath:  filepath.Join(t.TempDir(), "test.db"),
		JWTSecret:     secret,
		LocalStoreDir: filepath.Join(t.TempDir(), "localStore"),
	}
}

// startInstanceSecretServer starts a server on cfg's existing directories and
// database, so calling it twice with one cfg is a restart.
func startInstanceSecretServer(t *testing.T, cfg config.Config) (*gin.Engine, *Server, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	registerLegacyBookSourceFixtureBridge(t, database)
	router := gin.New()
	server := RegisterRoutes(router, cfg, database, readersync.NewHub(), scheduler.New(database, 1),
		backup.New(database, filepath.Join(cfg.DataDir, "webdav"), cfg))
	httpServer := httptest.NewServer(router)
	t.Cleanup(httpServer.Close)
	return router, server, httpServer
}

// readInstanceSecret returns the secret in dataDir after checking that it is a
// private regular file holding 64 lowercase hexadecimal characters.
func readInstanceSecret(t *testing.T, dataDir string) (string, os.FileInfo) {
	t.Helper()
	path := filepath.Join(dataDir, instanceSecretFileName)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("data/%s was not created: %v", instanceSecretFileName, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("data/%s is not a regular file: %v", instanceSecretFileName, info.Mode())
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("data/%s mode = %o, want 600", instanceSecretFileName, perm)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !instanceSecretFormat.Match(content) {
		t.Fatalf("data/%s is not 64 lowercase hexadecimal characters and a newline (%d bytes)", instanceSecretFileName, len(content))
	}
	return strings.TrimSuffix(string(content), "\n"), info
}

// signingInstanceSecret reads the instance secret without stopping the test,
// so a server that never created one still reports what it signed with.
func signingInstanceSecret(t *testing.T, dataDir string) string {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(dataDir, instanceSecretFileName)); err != nil {
		t.Errorf("data/%s was not created: %v", instanceSecretFileName, err)
		return ""
	}
	secret, _ := readInstanceSecret(t, dataDir)
	return secret
}

func writeInstanceSecret(t *testing.T, dataDir string, mode os.FileMode) string {
	t.Helper()
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	secret := hex.EncodeToString(random)
	path := filepath.Join(dataDir, instanceSecretFileName)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return secret
}

func requireNoInstanceSecretStage(t *testing.T, dataDir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dataDir, ".instance-secret*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("instance secret staging files remain: %v", matches)
	}
}

func jwtVerifiesWith(token, secret string) bool {
	parsed, err := jwt.Parse(token, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithoutClaimsValidation())
	return err == nil && parsed.Valid
}

func requireSessionSignedOnlyByInstanceSecret(t *testing.T, label, token, instance string, published []string) {
	t.Helper()
	for _, value := range published {
		if jwtVerifiesWith(token, value) {
			t.Errorf("%s verifies with a published or blank secret (%q)", label, value)
		}
	}
	if !jwtVerifiesWith(token, instance) {
		t.Errorf("%s is not signed with the instance secret", label)
	}
}

// capabilitySignature reproduces the HMAC capabilities: a purpose key derived
// from the secret signs the base64url payload.
func capabilitySignature(secret, purpose, payload string) string {
	derivation := hmac.New(sha256.New, []byte(secret))
	_, _ = derivation.Write([]byte(purpose))
	mac := hmac.New(sha256.New, derivation.Sum(nil))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func requireCapabilitySignedOnlyByInstanceSecret(t *testing.T, label, capability, purpose, instance string, published []string) {
	t.Helper()
	parts := strings.Split(capability, ".")
	if len(parts) != 2 {
		t.Fatalf("%s capability has %d parts", label, len(parts))
	}
	for _, value := range published {
		if capabilitySignature(value, purpose, parts[0]) == parts[1] {
			t.Errorf("%s capability is signed with a published or blank secret (%q)", label, value)
		}
	}
	if capabilitySignature(instance, purpose, parts[0]) != parts[1] {
		t.Errorf("%s capability is not signed with the instance secret", label)
	}
}

func resignCapability(capability, secret, purpose string) string {
	payload := strings.Split(capability, ".")[0]
	return payload + "." + capabilitySignature(secret, purpose, payload)
}

func coverCapabilityAEAD(t *testing.T, secret string) cipher.AEAD {
	t.Helper()
	key := sha256.Sum256([]byte(coverCapabilityPurpose + "\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return aead
}

func coverCapabilityOpens(t *testing.T, token, secret string) bool {
	t.Helper()
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "v1."))
	if err != nil {
		t.Fatalf("cover capability is not base64url: %v", err)
	}
	aead := coverCapabilityAEAD(t, secret)
	if len(sealed) <= aead.NonceSize() {
		return false
	}
	_, err = aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(coverCapabilityPurpose))
	return err == nil
}

func sealCoverCapability(t *testing.T, secret string, userID uint, rawURL string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"v": 1,
		"p": coverCapabilityPurpose,
		"u": userID,
		"r": rawURL,
		"e": time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	aead := coverCapabilityAEAD(t, secret)
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := aead.Seal(nonce, nonce, payload, []byte(coverCapabilityPurpose))
	return "v1." + base64.RawURLEncoding.EncodeToString(sealed)
}

func capabilityResponse(router *gin.Engine, method, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(method, target, nil))
	return response
}

func instanceSecretAccount(t *testing.T, server *Server, userID uint) models.User {
	t.Helper()
	var user models.User
	if err := server.db.First(&user, userID).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

// createInstanceSecretCBZBook stores a local CBZ book for user and returns its
// first page's capability URL.
func createInstanceSecretCBZBook(t *testing.T, server *Server, user models.User, directory string) string {
	t.Helper()
	bookRoot := filepath.Join(server.cfg.LibraryDir, directory)
	if err := os.MkdirAll(bookRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bookRoot, "book.cbz"), testCBZArchive(t, "instance secret cbz page"), 0o600); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       user.ID,
		Title:        "instance secret cbz " + directory,
		OriginalFile: filepath.ToSlash(filepath.Join(directory, "book.cbz")),
		LibraryPath:  directory,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "page", ResourcePath: "pages/001.jpg"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	prepared, err := server.cbzReader.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	return prepared.ResourceURL
}

func TestPublishedOrBlankJWTSecretIsReplacedByInstanceSecret(t *testing.T) {
	for _, configured := range []struct{ name, secret string }{
		{"current-default", publishedCurrentJWTSecret},
		{"previous-default", publishedPreviousJWTSecret},
		{"empty", ""},
		{"whitespace", " \t "},
	} {
		t.Run(configured.name, func(t *testing.T) {
			router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
				cfg.JWTSecret = configured.secret
			})
			instance := signingInstanceSecret(t, server.cfg.DataDir)
			requireNoInstanceSecretStage(t, server.cfg.DataDir)

			account := registerSessionContractUser(t, router, "instanceowner", "password8")
			requireSessionSignedOnlyByInstanceSecret(t, "registered session token", account.Token, instance, publishedSigningValues(configured.secret))
			login := loginSessionContractUser(t, router, "instanceowner", "password8")
			requireSessionSignedOnlyByInstanceSecret(t, "login session token", login.Token, instance, publishedSigningValues(configured.secret))
		})
	}
}

func TestInstanceSecretSignsEveryResourceCapability(t *testing.T) {
	png := chapterImagePNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer upstream.Close()

	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.JWTSecret = publishedCurrentJWTSecret
	})
	instance := signingInstanceSecret(t, server.cfg.DataDir)
	published := publishedSigningValues(publishedCurrentJWTSecret)
	user := capabilityTestUser(t, router, server)

	epubRoot := filepath.Join(server.cfg.LibraryDir, "instance-epub")
	if err := os.MkdirAll(epubRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(epubRoot, "book.epub"), testEPUBArchive(t), 0o600); err != nil {
		t.Fatal(err)
	}
	epubBook := models.Book{UserID: user.ID, Title: "instance epub", OriginalFile: "instance-epub/book.epub", LibraryPath: "instance-epub"}
	if err := server.db.Create(&epubBook).Error; err != nil {
		t.Fatal(err)
	}
	epubChapter := models.Chapter{BookID: epubBook.ID, Index: 0, Title: "one", ResourcePath: "OPS/one.xhtml"}
	if err := server.db.Create(&epubChapter).Error; err != nil {
		t.Fatal(err)
	}
	epub, err := server.epubReader.PrepareChapter(epubBook, &epubChapter)
	if err != nil {
		t.Fatal(err)
	}

	cbzURL := createInstanceSecretCBZBook(t, server, user, "instance-cbz")

	trackPath := filepath.Join(server.cfg.LibraryDir, "instance-audio", "tracks", "001.mp3")
	if err := os.MkdirAll(filepath.Dir(trackPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trackPath, []byte("instance secret audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	audioBook := models.Book{UserID: user.ID, Type: 1, Title: "instance audio", LibraryPath: "instance-audio"}
	if err := server.db.Create(&audioBook).Error; err != nil {
		t.Fatal(err)
	}
	audioChapter := models.Chapter{BookID: audioBook.ID, Index: 0, Title: "track", ResourcePath: "tracks/001.mp3"}
	if err := server.db.Create(&audioChapter).Error; err != nil {
		t.Fatal(err)
	}
	audio, err := server.audioReader.PrepareChapter(audioBook, &audioChapter, "tracks/001.mp3")
	if err != nil {
		t.Fatal(err)
	}

	source := createCacheStreamSource(t, server, upstream.URL)
	imageBook := models.Book{UserID: user.ID, SourceID: source.ID, Title: "instance image", URL: upstream.URL + "/book"}
	if err := server.db.Create(&imageBook).Error; err != nil {
		t.Fatal(err)
	}
	imageChapter := models.Chapter{BookID: imageBook.ID, Index: 0, Title: "图", URL: upstream.URL + "/chapter"}
	if err := server.db.Create(&imageChapter).Error; err != nil {
		t.Fatal(err)
	}
	imageURL := upstream.URL + "/illustration"
	content := `<img src="` + imageURL + `">`
	if _, err := server.chapterImages.CacheChapter(context.Background(), source, imageBook, imageChapter, content); err != nil {
		t.Fatal(err)
	}
	mapping, _, err := server.chapterImages.CachedImages(imageBook, imageChapter, content)
	if err != nil {
		t.Fatal(err)
	}
	imageResource := mapping[imageURL]
	if !strings.HasPrefix(imageResource, "/api/chapter-image/") {
		t.Fatalf("chapter image mapping = %v", mapping)
	}

	umdAuth := "Bearer " + loginSessionContractUser(t, router, "testuser", "test1234").Token
	umdPreview := previewLocalBookCover(t, router, umdAuth, "实例密钥封面.umd", localBookCoverUMD(t, localBookCoverJPEG))
	umdBook, _ := importLocalBookCover(t, router, server, umdAuth, "实例密钥封面.umd", umdPreview.ImportToken)
	if !strings.HasPrefix(umdBook.CoverURL, "/api/umd-cover/") {
		t.Fatalf("UMD cover URL = %q", umdBook.CoverURL)
	}

	for _, resource := range []struct {
		name, url, prefix, purpose string
	}{
		{"EPUB", epub.ResourceURL, "/api/epub-resource/", epubCapabilityPurpose},
		{"CBZ", cbzURL, "/api/cbz-resource/", cbzCapabilityPurpose},
		{"audio", audio.ResourceURL, "/api/audio-resource/", audioCapabilityPurpose},
		{"chapter image", imageResource, "/api/chapter-image/", chapterImageCapabilityPurpose},
		{"UMD cover", umdBook.CoverURL, "/api/umd-cover/", umdCoverCapabilityPurpose},
	} {
		t.Run(resource.name, func(t *testing.T) {
			var capability string
			if resource.prefix == "/api/chapter-image/" || resource.prefix == "/api/umd-cover/" {
				unescaped, err := url.PathUnescape(strings.TrimPrefix(resource.url, resource.prefix))
				if err != nil {
					t.Fatal(err)
				}
				capability = unescaped
			} else {
				capability, _ = capabilityParts(t, resource.url, resource.prefix)
			}
			requireCapabilitySignedOnlyByInstanceSecret(t, resource.name, capability, resource.purpose, instance, published)

			if response := capabilityResponse(router, http.MethodGet, resource.url); response.Code != http.StatusOK {
				t.Errorf("%s capability issued by the server: status=%d, want 200: %s", resource.name, response.Code, response.Body.String())
			}
			for _, value := range published {
				forged := strings.Replace(resource.url, capability, resignCapability(capability, value, resource.purpose), 1)
				if response := capabilityResponse(router, http.MethodGet, forged); response.Code != http.StatusForbidden {
					t.Errorf("%s capability re-signed with %q: status=%d, want 403", resource.name, value, response.Code)
				}
			}
		})
	}

	t.Run("cover", func(t *testing.T) {
		resource, err := server.coverImages.Project(user.ID, 0, "https://cover.invalid/instance-secret.png")
		if err != nil {
			t.Fatal(err)
		}
		token, err := url.PathUnescape(strings.TrimPrefix(resource, "/api/cover/"))
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range published {
			if coverCapabilityOpens(t, token, value) {
				t.Errorf("cover capability opens with a published or blank secret (%q)", value)
			}
		}
		if !coverCapabilityOpens(t, token, instance) {
			t.Error("cover capability is not sealed with the instance secret")
		}
		// An account that does not exist: a capability that passes the seal
		// check would end in 404 there, before any remote fetch.
		for _, value := range published {
			forged := sealCoverCapability(t, value, 987654, "https://cover.invalid/forged.png")
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				if response := capabilityResponse(router, method, "/api/cover/"+url.PathEscape(forged)); response.Code != http.StatusForbidden {
					t.Errorf("%s cover capability sealed with %q: status=%d, want 403", method, value, response.Code)
				}
			}
		}
	})
}

func TestInstanceSecretSurvivesRestartAndExistingFilesAreReused(t *testing.T) {
	t.Run("restart", func(t *testing.T) {
		cfg := instanceSecretTestConfig(t, publishedCurrentJWTSecret)
		router, server, _ := startInstanceSecretServer(t, cfg)
		before, beforeInfo := readInstanceSecret(t, cfg.DataDir)
		account := registerSessionContractUser(t, router, "restartowner", "password8")
		cbzURL := createInstanceSecretCBZBook(t, server, instanceSecretAccount(t, server, account.User.ID), "restart-cbz")

		restarted, _, restartedHTTP := startInstanceSecretServer(t, cfg)
		after, afterInfo := readInstanceSecret(t, cfg.DataDir)
		if after != before || !os.SameFile(beforeInfo, afterInfo) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
			t.Fatal("restart replaced or rewrote data/instance-secret")
		}
		requireNoInstanceSecretStage(t, cfg.DataDir)
		requireSessionEntriesAccepted(t, restarted, restartedHTTP, "session issued before the restart", account.Token, account.User.ID)
		if response := capabilityResponse(restarted, http.MethodGet, cbzURL); response.Code != http.StatusOK {
			t.Errorf("CBZ capability issued before the restart: status=%d, want 200", response.Code)
		}
	})

	t.Run("existing-file", func(t *testing.T) {
		cfg := instanceSecretTestConfig(t, publishedCurrentJWTSecret)
		existing := writeInstanceSecret(t, cfg.DataDir, 0o600)
		router, _, _ := startInstanceSecretServer(t, cfg)
		account := registerSessionContractUser(t, router, "existingowner", "password8")
		requireSessionSignedOnlyByInstanceSecret(t, "session token", account.Token, existing, publishedSigningValues(publishedCurrentJWTSecret))
		if current, _ := readInstanceSecret(t, cfg.DataDir); current != existing {
			t.Error("an existing instance secret was replaced")
		}
	})

	t.Run("permissive-file", func(t *testing.T) {
		cfg := instanceSecretTestConfig(t, publishedCurrentJWTSecret)
		existing := writeInstanceSecret(t, cfg.DataDir, 0o644)
		router, _, _ := startInstanceSecretServer(t, cfg)
		current, _ := readInstanceSecret(t, cfg.DataDir)
		if current != existing {
			t.Error("tightening permissions replaced the instance secret")
		}
		account := registerSessionContractUser(t, router, "permissiveowner", "password8")
		requireSessionSignedOnlyByInstanceSecret(t, "session token", account.Token, existing, publishedSigningValues(publishedCurrentJWTSecret))
	})
}

func TestInstanceSecretTransitionKeepsPublishedSessionsAndRefusesLegacyTokens(t *testing.T) {
	router, server, httpServer := setupLegacySessionServer(t, publishedCurrentJWTSecret)
	account := registerSessionContractUser(t, router, "transitionowner", "password8")
	user := instanceSecretAccount(t, server, account.User.ID)

	// Logins served by an older image: jti-backed rows signed with either
	// published default.
	for _, signer := range []string{publishedCurrentJWTSecret, publishedPreviousJWTSecret} {
		token, err := authsession.New(server.db, signer).Issue(context.Background(), user)
		if err != nil {
			t.Fatal(err)
		}
		requireSessionEntriesAccepted(t, router, httpServer, "session signed with "+signer, token, user.ID)
		logout := sessionContractJSONRequest(router, http.MethodPost, "/api/auth/logout", "", token)
		if logout.Code != http.StatusNoContent {
			t.Fatalf("logout of a session signed with %s: status=%d, want 204: %s", signer, logout.Code, logout.Body.String())
		}
		requireSessionEntriesRejected(t, router, httpServer, "logged-out session signed with "+signer, token)
	}

	for _, signer := range []string{publishedCurrentJWTSecret, publishedPreviousJWTSecret} {
		legacy := signLegacySessionToken(t, signer, user.ID, time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC))
		requireSessionEntriesRejected(t, router, httpServer, "legacy token signed with "+signer, legacy)
	}
	if count := countLegacySessionRows(t, server); count != 0 {
		t.Errorf("legacy tokens created %d legacy sessions, want 0", count)
	}

	identity := make([]byte, 32)
	if _, err := rand.Read(identity); err != nil {
		t.Fatal(err)
	}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId":      user.ID,
		"authVersion": 1,
		"jti":         base64.RawURLEncoding.EncodeToString(identity),
		"iat":         time.Now().Unix(),
	}).SignedString([]byte(publishedCurrentJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	requireSessionEntriesRejected(t, router, httpServer, "forged token with an unknown identity", forged)
}

func TestCustomJWTSecretKeepsTodaysBehaviourAndIgnoresInstanceSecret(t *testing.T) {
	t.Run("no-file", func(t *testing.T) {
		router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
			cfg.JWTSecret = instanceSecretCustomSecret
		})
		if _, err := os.Lstat(filepath.Join(server.cfg.DataDir, instanceSecretFileName)); !os.IsNotExist(err) {
			t.Errorf("a custom secret created data/%s: %v", instanceSecretFileName, err)
		}
		account := registerSessionContractUser(t, router, "customowner", "password8")
		if !jwtVerifiesWith(account.Token, instanceSecretCustomSecret) {
			t.Error("session token is not signed with the custom secret")
		}
	})

	t.Run("existing-file", func(t *testing.T) {
		cfg := instanceSecretTestConfig(t, instanceSecretCustomSecret)
		existing := writeInstanceSecret(t, cfg.DataDir, 0o644)
		path := filepath.Join(cfg.DataDir, instanceSecretFileName)
		before, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		router, server, httpServer := startInstanceSecretServer(t, cfg)
		after, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != existing+"\n" || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
			t.Error("a custom secret touched data/instance-secret")
		}

		account := registerSessionContractUser(t, router, "customexisting", "password8")
		if !jwtVerifiesWith(account.Token, instanceSecretCustomSecret) || jwtVerifiesWith(account.Token, existing) {
			t.Error("session token is not signed with the custom secret alone")
		}
		cbzURL := createInstanceSecretCBZBook(t, server, instanceSecretAccount(t, server, account.User.ID), "custom-cbz")
		capability, _ := capabilityParts(t, cbzURL, "/api/cbz-resource/")
		requireCapabilitySignedOnlyByInstanceSecret(t, "CBZ", capability, cbzCapabilityPurpose, instanceSecretCustomSecret,
			[]string{existing, publishedCurrentJWTSecret, publishedPreviousJWTSecret})

		published, err := authsession.New(server.db, publishedCurrentJWTSecret).Issue(context.Background(), instanceSecretAccount(t, server, account.User.ID))
		if err != nil {
			t.Fatal(err)
		}
		requireSessionEntriesRejected(t, router, httpServer, "session signed with a published default under a custom secret", published)
	})
}

// readBackupArchive returns every entry of a downloaded zip backup, inflated.
func readBackupArchive(t *testing.T, label string, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("%s is not a zip archive: %v", label, err)
	}
	entries := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(io.LimitReader(opened, 64<<20))
		_ = opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = content
	}
	return entries
}

func TestInstanceSecretIsNotReachableThroughStorageOrBackups(t *testing.T) {
	cfg := instanceSecretTestConfig(t, publishedCurrentJWTSecret)
	secret := writeInstanceSecret(t, cfg.DataDir, 0o600)
	router, _, _ := startInstanceSecretServer(t, cfg)
	admin := registerSessionContractUser(t, router, "vaultadmin", "password8")
	member := registerSessionContractUser(t, router, "vaultmember", "password8")
	bearer := "Bearer " + admin.Token

	// Error bodies may echo the requested path, so only listings are checked
	// for the file name; every response is checked for the secret itself.
	requireHidden := func(label string, response *httptest.ResponseRecorder) {
		t.Helper()
		if strings.Contains(response.Body.String(), secret) {
			t.Errorf("%s returned the instance secret (status %d)", label, response.Code)
		}
	}
	requireUnlisted := func(label string, response *httptest.ResponseRecorder) {
		t.Helper()
		requireHidden(label, response)
		if strings.Contains(response.Body.String(), instanceSecretFileName) {
			t.Errorf("%s listed data/%s (status %d)", label, instanceSecretFileName, response.Code)
		}
	}

	for _, token := range []string{admin.Token, member.Token} {
		for _, prefix := range []string{"/webdav", "/reader3/webdav"} {
			put := webDAVProtocolRequest(t, router, http.MethodPut, prefix+"/probe.txt", "Bearer "+token, "probe", nil)
			if put.Code != http.StatusCreated && put.Code != http.StatusNoContent {
				t.Fatalf("PUT %s/probe.txt status=%d: %s", prefix, put.Code, put.Body.String())
			}
			for _, depth := range []string{"1", "infinity"} {
				response := webDAVProtocolRequest(t, router, "PROPFIND", prefix+"/", "Bearer "+token, "", map[string]string{"Depth": depth})
				if response.Code != http.StatusMultiStatus {
					t.Fatalf("PROPFIND %s/ Depth %s status=%d", prefix, depth, response.Code)
				}
				requireUnlisted("PROPFIND "+prefix+"/ Depth "+depth, response)
			}
			for _, path := range []string{"/../instance-secret", "/..%2finstance-secret", "/%2e%2e/instance-secret", "/..%5cinstance-secret"} {
				for _, method := range []string{http.MethodGet, "PROPFIND"} {
					requireHidden(method+" "+prefix+path, webDAVProtocolRequest(t, router, method, prefix+path, "Bearer "+token, "", map[string]string{"Depth": "0"}))
				}
			}
		}
	}

	for _, path := range []string{"/uploads/../instance-secret", "/uploads/%2e%2e/instance-secret", "/uploads/..%2finstance-secret", "/uploads/users/../../instance-secret"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			requireHidden(method+" "+path, capabilityResponse(router, method, path))
		}
	}

	relative, err := filepath.Rel(cfg.LocalStoreDir, filepath.Join(cfg.DataDir, instanceSecretFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.ToSlash(relative), filepath.Join(cfg.DataDir, instanceSecretFileName), "../instance-secret"} {
		requireHidden("local store download "+path, sessionContractJSONRequest(router, http.MethodGet, "/api/local-store/download?path="+url.QueryEscape(path), "", admin.Token))
		requireUnlisted("local store list "+path, sessionContractJSONRequest(router, http.MethodGet, "/api/local-store?path="+url.QueryEscape(filepath.ToSlash(filepath.Dir(path))), "", admin.Token))
	}
	for _, name := range []string{"..%2finstance-secret", "%2e%2e%2finstance-secret", "..%5cinstance-secret"} {
		requireHidden("backup download "+name, sessionContractJSONRequest(router, http.MethodGet, "/api/backup/download/"+name, "", admin.Token))
	}

	for _, trigger := range []string{"/api/backup/trigger", "/api/backup/portable/trigger"} {
		for _, token := range []string{admin.Token, member.Token} {
			created := sessionContractJSONRequest(router, http.MethodPost, trigger, "", token)
			if created.Code != http.StatusOK {
				t.Fatalf("POST %s status=%d: %s", trigger, created.Code, created.Body.String())
			}
			var payload struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil || payload.Name == "" {
				t.Fatalf("POST %s response has no backup name: %s", trigger, created.Body.String())
			}
			download := sessionContractJSONRequest(router, http.MethodGet, "/api/backup/download/"+url.PathEscape(payload.Name), "", token)
			if download.Code != http.StatusOK {
				t.Fatalf("download %s status=%d", payload.Name, download.Code)
			}
			for name, content := range readBackupArchive(t, payload.Name, download.Body.Bytes()) {
				if strings.Contains(name, instanceSecretFileName) || bytes.Contains(content, []byte(secret)) {
					t.Errorf("backup %s entry %q carries the instance secret", payload.Name, name)
				}
			}
		}
	}
	requireUnlisted("PROPFIND /webdav/ after backups", webDAVProtocolRequest(t, router, "PROPFIND", "/webdav/", bearer, "", map[string]string{"Depth": "infinity"}))
}
