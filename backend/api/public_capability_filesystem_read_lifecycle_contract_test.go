package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

type capabilityHeaderGateWriter struct {
	gin.ResponseWriter
	once    sync.Once
	reached chan struct{}
	release chan struct{}
}

func (w *capabilityHeaderGateWriter) Header() http.Header {
	header := w.ResponseWriter.Header()
	w.once.Do(func() {
		close(w.reached)
		<-w.release
	})
	return header
}

func runCapabilityHandlerAfterVerifiedPath(
	t *testing.T,
	requestPath string,
	params gin.Params,
	handler func(*gin.Context),
	replace func(),
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	gate := &capabilityHeaderGateWriter{
		ResponseWriter: context.Writer,
		reached:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	context.Writer = gate
	context.Request = httptest.NewRequest(http.MethodGet, requestPath, nil)
	context.Params = params

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler(context)
	}()

	select {
	case <-gate.reached:
	case <-time.After(5 * time.Second):
		close(gate.release)
		<-done
		t.Fatal("capability handler did not reach the post-verification response boundary")
	}
	replace()
	close(gate.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("capability handler did not finish after releasing the response boundary")
	}
	return recorder
}

func capabilityParts(t *testing.T, resourceURL, prefix string) (string, string) {
	t.Helper()
	parsed, err := url.Parse(resourceURL)
	if err != nil {
		t.Fatal(err)
	}
	remainder := strings.TrimPrefix(parsed.EscapedPath(), prefix)
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Fatalf("unexpected capability resource URL %q", resourceURL)
	}
	capability, err := url.PathUnescape(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	resourcePath, err := url.PathUnescape("/" + parts[1])
	if err != nil {
		t.Fatal(err)
	}
	return capability, resourcePath
}

func replaceRegularWithSymlink(t *testing.T, target, replacement string) {
	t.Helper()
	backup := target + ".verified-open"
	if err := os.Rename(target, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, target); err != nil {
		_ = os.Rename(backup, target)
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(target)
		_ = os.Rename(backup, target)
	})
}

func onlyGlobFile(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("glob %q matched %d files: %v", pattern, len(matches), matches)
	}
	return matches[0]
}

func capabilityTestUser(t *testing.T, router *gin.Engine, server *Server) models.User {
	t.Helper()
	_ = authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func TestEPUBCapabilityDoesNotServeReplacementAfterAuthorization(t *testing.T) {
	router, server := setupTestServer(t)
	user := capabilityTestUser(t, router, server)
	bookRoot := filepath.Join(server.cfg.LibraryDir, "capability-epub")
	if err := os.MkdirAll(bookRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(bookRoot, "book.epub")
	if err := os.WriteFile(sourcePath, testEPUBArchive(t), 0o600); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       user.ID,
		Title:        "capability epub",
		OriginalFile: filepath.ToSlash(filepath.Join("capability-epub", "book.epub")),
		LibraryPath:  "capability-epub",
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "one", ResourcePath: "OPS/one.xhtml"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	prepared, err := server.epubReader.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	capability, _ := capabilityParts(t, prepared.ResourceURL, "/api/epub-resource/")
	assetPath := onlyGlobFile(t, filepath.Join(bookRoot, ".epub-resources", "*", "OPS", "styles", "book.css"))
	replacement := filepath.Join(t.TempDir(), "outside.css")
	const replacementBytes = "mounted epub replacement"
	if err := os.WriteFile(replacement, []byte(replacementBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	requestPath := "/api/epub-resource/" + url.PathEscape(capability) + "/OPS/styles/book.css"
	response := runCapabilityHandlerAfterVerifiedPath(t, requestPath, gin.Params{
		{Key: "capability", Value: capability},
		{Key: "resourcePath", Value: "/OPS/styles/book.css"},
	}, server.epubResource, func() {
		replaceRegularWithSymlink(t, assetPath, replacement)
	})
	if strings.Contains(response.Body.String(), replacementBytes) {
		t.Fatalf("EPUB capability served the replacement mounted object: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestCBZCapabilityDoesNotServeReplacementAfterAuthorization(t *testing.T) {
	router, server := setupTestServer(t)
	user := capabilityTestUser(t, router, server)
	bookRoot := filepath.Join(server.cfg.LibraryDir, "capability-cbz")
	if err := os.MkdirAll(bookRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(bookRoot, "book.cbz")
	if err := os.WriteFile(sourcePath, testCBZArchive(t, "verified cbz page"), 0o600); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       user.ID,
		Title:        "capability cbz",
		OriginalFile: filepath.ToSlash(filepath.Join("capability-cbz", "book.cbz")),
		LibraryPath:  "capability-cbz",
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
	capability, resourcePath := capabilityParts(t, prepared.ResourceURL, "/api/cbz-resource/")
	assetPath := onlyGlobFile(t, filepath.Join(bookRoot, ".cbz-resources", "*", "pages", "001.jpg"))
	replacement := filepath.Join(t.TempDir(), "outside.jpg")
	const replacementBytes = "mounted cbz replacement"
	if err := os.WriteFile(replacement, []byte(replacementBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	response := runCapabilityHandlerAfterVerifiedPath(t, prepared.ResourceURL, gin.Params{
		{Key: "capability", Value: capability},
		{Key: "resourcePath", Value: resourcePath},
	}, server.cbzResource, func() {
		replaceRegularWithSymlink(t, assetPath, replacement)
	})
	if strings.Contains(response.Body.String(), replacementBytes) {
		t.Fatalf("CBZ capability served the replacement mounted object: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestAudioCapabilityDoesNotServeReplacementAfterAuthorization(t *testing.T) {
	router, server := setupTestServer(t)
	user := capabilityTestUser(t, router, server)
	bookRoot := filepath.Join(server.cfg.LibraryDir, "capability-audio")
	trackPath := filepath.Join(bookRoot, "tracks", "001.mp3")
	if err := os.MkdirAll(filepath.Dir(trackPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trackPath, []byte("verified audio bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Type: 1, Title: "capability audio", LibraryPath: "capability-audio"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "track", ResourcePath: "tracks/001.mp3"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	prepared, err := server.audioReader.PrepareChapter(book, &chapter, "tracks/001.mp3")
	if err != nil {
		t.Fatal(err)
	}
	capability, resourcePath := capabilityParts(t, prepared.ResourceURL, "/api/audio-resource/")
	resource, err := server.audioReader.OpenResource(capability, resourcePath)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(t.TempDir(), "outside.mp3")
	const replacementBytes = "mounted audio replacement"
	if err := os.WriteFile(replacement, []byte(replacementBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceRegularWithSymlink(t, trackPath, replacement)
	served, err := os.ReadFile(resource.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(served), replacementBytes) {
		t.Fatalf("audio capability authorized a path that reopened the replacement mounted object: %q", served)
	}
}
