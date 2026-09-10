package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"openreader/backend/models"
)

func TestUserAssetUploadRejectsCallerRootSymlink(t *testing.T) {
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, "assetrootupload")
	uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")
	usersRoot := filepath.Join(uploadsRoot, "users")
	if err := os.MkdirAll(usersRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(server.cfg.DataDir, "outside-upload-user")
	if err := os.MkdirAll(filepath.Join(outside, "covers"), 0o700); err != nil {
		t.Fatal(err)
	}
	callerRoot := filepath.Join(usersRoot, strconv.FormatUint(uint64(user.ID), 10))
	if err := os.Symlink(outside, callerRoot); err != nil {
		t.Fatal(err)
	}

	response := performUserAssetLifecycleUpload(t, router, auth, context.Background())
	if response.Code == http.StatusCreated {
		t.Fatalf("upload through caller-root symlink succeeded: %s", response.Body.String())
	}
	assertUserAssetLifecyclePathFree(t, response.Body.String(), server.cfg.DataDir, outside)
	assertDirectoryHasNoEntries(t, filepath.Join(outside, "covers"))
}

func TestUserAssetUploadCancellationLeavesNoFinalFile(t *testing.T) {
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, "assetuploadcancel")
	ctx, cancel := context.WithCancel(context.Background())
	userAssetUploadLifecycleTestHook = func(stage string) {
		if stage == "before_save" {
			cancel()
		}
	}
	t.Cleanup(func() { userAssetUploadLifecycleTestHook = nil })

	response := performUserAssetLifecycleUpload(t, router, auth, ctx)
	if response.Code == http.StatusCreated {
		t.Fatalf("cancelled upload succeeded: %s", response.Body.String())
	}
	assertUserAssetRootHasNoFiles(t, server, user.ID)
}

func TestUserAssetDeleteRejectsCallerRootSymlink(t *testing.T) {
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, "assetrootdelete")
	uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")
	usersRoot := filepath.Join(uploadsRoot, "users")
	if err := os.MkdirAll(usersRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(server.cfg.DataDir, "outside-delete-user")
	outsideFile := filepath.Join(outside, "covers", "victim.png")
	if err := os.MkdirAll(filepath.Dir(outsideFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, readerAppearancePNG(t, 1, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	callerRoot := filepath.Join(usersRoot, strconv.FormatUint(uint64(user.ID), 10))
	if err := os.Symlink(outside, callerRoot); err != nil {
		t.Fatal(err)
	}
	assetURL := fmt.Sprintf("/uploads/users/%d/covers/victim.png", user.ID)

	response := performUserAssetLifecycleJSON(router, auth, http.MethodDelete, "/api/uploads", assetDeletePayload(assetURL), context.Background())
	if response.Code == http.StatusOK {
		t.Fatalf("delete through caller-root symlink succeeded: %s", response.Body.String())
	}
	assertUserAssetLifecyclePathFree(t, response.Body.String(), server.cfg.DataDir, outside)
	if data, err := os.ReadFile(outsideFile); err != nil || len(data) == 0 {
		t.Fatalf("unsafe delete changed outside file: bytes=%d err=%v", len(data), err)
	}
}

func TestBookCreateRejectsCoverThroughCallerRootSymlink(t *testing.T) {
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, "assetrootbook")
	uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")
	usersRoot := filepath.Join(uploadsRoot, "users")
	if err := os.MkdirAll(usersRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(server.cfg.DataDir, "outside-book-user")
	outsideFile := filepath.Join(outside, "covers", "cover.png")
	if err := os.MkdirAll(filepath.Dir(outsideFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, readerAppearancePNG(t, 1, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	callerRoot := filepath.Join(usersRoot, strconv.FormatUint(uint64(user.ID), 10))
	if err := os.Symlink(outside, callerRoot); err != nil {
		t.Fatal(err)
	}
	assetURL := fmt.Sprintf("/uploads/users/%d/covers/cover.png", user.ID)
	body := fmt.Sprintf(`{"title":"unsafe cover","customCoverUrl":%q}`, assetURL)

	response := performUserAssetLifecycleJSON(router, auth, http.MethodPost, "/api/books", body, context.Background())
	if response.Code != http.StatusBadRequest || response.Body.String() != `{"error":"invalid custom cover url"}` {
		t.Fatalf("unsafe cover create = %d %s", response.Code, response.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.Book{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unsafe cover created %d books", count)
	}
}

func TestUserSettingRejectsNewMissingManagedAssetReference(t *testing.T) {
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, "assetsettingsmissing")
	assetURL := fmt.Sprintf("/uploads/users/%d/backgrounds/missing.png", user.ID)
	body := fmt.Sprintf(`{"value":{"appearance":{"background":%q}}}`, assetURL)

	response := performUserAssetLifecycleJSON(router, auth, http.MethodPut, "/api/settings/reader", body, context.Background())
	if response.Code != http.StatusBadRequest {
		t.Fatalf("new missing managed asset setting = %d %s, want 400", response.Code, response.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.UserSetting{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected setting left %d rows", count)
	}
}

func TestUserAssetDeleteIgnoresSettingSubstringThatIsNotAReference(t *testing.T) {
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, "assetsubstring")
	assetURL, assetPath := createUserAssetLifecycleFile(t, server, user.ID, "backgrounds", "paper.png")
	value, err := json.Marshal(map[string]any{"note": "prefix " + assetURL + " suffix"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "reader", Value: string(value)}).Error; err != nil {
		t.Fatal(err)
	}

	response := performUserAssetLifecycleJSON(router, auth, http.MethodDelete, "/api/uploads", assetDeletePayload(assetURL), context.Background())
	if response.Code != http.StatusOK {
		t.Fatalf("substring-only setting blocked delete = %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(assetPath); !os.IsNotExist(err) {
		t.Fatalf("substring-only delete left asset: %v", err)
	}
}

func TestBookCoverWriteAndAssetDeleteCannotBothCommit(t *testing.T) {
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, "assetbookdeleterace")
	assetURL, assetPath := createUserAssetLifecycleFile(t, server, user.ID, "covers", "cover.png")
	book := models.Book{UserID: user.ID, Title: "race book", CanUpdate: true}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	reached := make(chan struct{})
	release := make(chan struct{})
	bookPatchWriteLifecycleTestHook = func(action string) {
		if action != "metadata" {
			return
		}
		select {
		case <-reached:
		default:
			close(reached)
		}
		<-release
	}
	t.Cleanup(func() { bookPatchWriteLifecycleTestHook = nil })

	bookDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		path := "/api/books/" + strconv.FormatUint(uint64(book.ID), 10)
		body := fmt.Sprintf(`{"customCoverUrl":%q}`, assetURL)
		bookDone <- performUserAssetLifecycleJSON(router, auth, http.MethodPut, path, body, context.Background())
	}()
	select {
	case <-reached:
	case <-time.After(3 * time.Second):
		t.Fatal("book write did not reach asset admission barrier")
	}

	deleteDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		deleteDone <- performUserAssetLifecycleJSON(router, auth, http.MethodDelete, "/api/uploads", assetDeletePayload(assetURL), context.Background())
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	bookResponse := waitUserAssetLifecycleResponse(t, bookDone, "book write")
	deleteResponse := waitUserAssetLifecycleResponse(t, deleteDone, "asset delete")

	var stored models.Book
	if err := server.db.First(&stored, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, statErr := os.Stat(assetPath)
	bookSucceeded := bookResponse.Code == http.StatusOK
	deleteSucceeded := deleteResponse.Code == http.StatusOK
	if bookSucceeded && deleteSucceeded {
		t.Fatalf("book write and delete both succeeded: book=%s delete=%s stored=%q fileErr=%v", bookResponse.Body.String(), deleteResponse.Body.String(), stored.CustomCoverURL, statErr)
	}
	if bookSucceeded {
		if deleteResponse.Code != http.StatusConflict || stored.CustomCoverURL != assetURL || statErr != nil {
			t.Fatalf("reference-first result is inconsistent: book=%d delete=%d stored=%q fileErr=%v", bookResponse.Code, deleteResponse.Code, stored.CustomCoverURL, statErr)
		}
		return
	}
	if !deleteSucceeded || bookResponse.Code != http.StatusBadRequest || stored.CustomCoverURL != "" || !os.IsNotExist(statErr) {
		t.Fatalf("delete-first result is inconsistent: book=%d %s delete=%d %s stored=%q fileErr=%v", bookResponse.Code, bookResponse.Body.String(), deleteResponse.Code, deleteResponse.Body.String(), stored.CustomCoverURL, statErr)
	}
}

func TestPortableAssetPromotionRejectsCallerRootSymlink(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "asset-portable-root", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")
	usersRoot := filepath.Join(uploadsRoot, "users")
	if err := os.MkdirAll(usersRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(server.cfg.DataDir, "outside-portable-user")
	if err := os.MkdirAll(filepath.Join(outside, "backgrounds"), 0o700); err != nil {
		t.Fatal(err)
	}
	callerRoot := filepath.Join(usersRoot, strconv.FormatUint(uint64(user.ID), 10))
	if err := os.Symlink(outside, callerRoot); err != nil {
		t.Fatal(err)
	}
	data := readerAppearancePNG(t, 1, 1)
	stage := filepath.Join(t.TempDir(), "stage.png")
	if err := os.WriteFile(stage, data, 0o600); err != nil {
		t.Fatal(err)
	}
	finalURL := fmt.Sprintf("/uploads/users/%d/backgrounds/final.png", user.ID)
	finalPath := filepath.Join(callerRoot, "backgrounds", "final.png")
	assets := []portableStagedAsset{{
		manifest: portableBackupManifestAsset{Kind: "backgrounds", Extension: ".png", Size: int64(len(data))},
		path:     stage, finalURL: finalURL, finalPath: finalPath,
	}}

	promoted, err := server.promotePortableAssets(assets, user.ID)
	if err == nil || len(promoted) != 0 {
		t.Fatalf("portable promote through caller-root symlink succeeded: promoted=%v err=%v", promoted, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "backgrounds", "final.png")); !os.IsNotExist(err) {
		t.Fatalf("portable promote wrote outside root: %v", err)
	}
}

func performUserAssetLifecycleUpload(t *testing.T, router http.Handler, auth string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := makeAssetBoundaryMultipart(t, [][2]string{{"type", "cover"}}, []assetBoundaryFilePart{{
		field: "file", filename: "cover.png", data: readerAppearancePNG(t, 1, 1),
	}})
	request := httptest.NewRequest(http.MethodPost, "/api/uploads", body).WithContext(ctx)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	cleanupParsedMultipart(t, request)
	return response
}

func performUserAssetLifecycleJSON(router http.Handler, auth, method, path, body string, ctx context.Context) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func createUserAssetLifecycleFile(t *testing.T, server *Server, userID uint, kind, name string) (string, string) {
	t.Helper()
	path := filepath.Join(server.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(userID), 10), kind, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, readerAppearancePNG(t, 1, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("/uploads/users/%d/%s/%s", userID, kind, name), path
}

func assertDirectoryHasNoEntries(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("directory %s contains entries: %v", path, entries)
	}
}

func assertUserAssetLifecyclePathFree(t *testing.T, body string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if path != "" && strings.Contains(body, path) {
			t.Fatalf("response exposed host path %q: %s", path, body)
		}
	}
}

func waitUserAssetLifecycleResponse(t *testing.T, done <-chan *httptest.ResponseRecorder, action string) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case response := <-done:
		return response
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not complete", action)
		return nil
	}
}
