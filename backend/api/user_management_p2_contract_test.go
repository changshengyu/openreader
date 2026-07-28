package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"openreader/backend/models"
)

func TestUserManagementMatchesReaderDevNewAccountContractWithoutBreakingLegacyLogin(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)

	for _, body := range []string{
		`{"username":"four","password":"password8"}`,
		`{"username":"new-user","password":"password8"}`,
		`{"username":"default","password":"password8"}`,
		`{"username":"valid1","password":"short77"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, request)
		if writer.Code != http.StatusBadRequest {
			t.Fatalf("register %s: status=%d, want 400: %s", body, writer.Code, writer.Body.String())
		}
	}

	validRegistration := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"valid1","password":"password8"}`))
	validRegistration.Header.Set("Content-Type", "application/json")
	validWriter := httptest.NewRecorder()
	router.ServeHTTP(validWriter, validRegistration)
	if validWriter.Code != http.StatusOK {
		t.Fatalf("valid registration: status=%d: %s", validWriter.Code, validWriter.Body.String())
	}

	for _, body := range []string{
		`{"username":"four","password":"password8"}`,
		`{"username":"new-user","password":"password8"}`,
		`{"username":"default","password":"password8"}`,
		`{"username":"managed5","password":"short77"}`,
	} {
		writer := adminContractRequest(router, http.MethodPost, "/api/admin/users", body, adminAuth)
		assertAdminContractError(t, writer, http.StatusBadRequest, "BAD_REQUEST")
	}

	created := adminContractRequest(router, http.MethodPost, "/api/admin/users", `{
		"username":"managed5",
		"password":"password8",
		"canAccessStore":false,
		"canAccessWebdav":true
	}`, adminAuth)
	if created.Code != http.StatusCreated {
		t.Fatalf("create managed user: %d %s", created.Code, created.Body.String())
	}
	var managed struct {
		ID              uint  `json:"id"`
		CanAccessStore  bool  `json:"canAccessStore"`
		CanAccessWebdav *bool `json:"canAccessWebdav"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &managed); err != nil {
		t.Fatalf("decode managed user: %v", err)
	}
	if managed.ID == 0 || managed.CanAccessStore || managed.CanAccessWebdav == nil || !*managed.CanAccessWebdav {
		t.Fatalf("independent storage permissions were not persisted: %+v", managed)
	}

	shortReset := adminContractRequest(router, http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(managed.ID), 10)+"/password", `{"password":"short77"}`, adminAuth)
	assertAdminContractError(t, shortReset, http.StatusBadRequest, "BAD_REQUEST")
	validReset := adminContractRequest(router, http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(managed.ID), 10)+"/password", `{"password":"changed88"}`, adminAuth)
	if validReset.Code != http.StatusOK {
		t.Fatalf("reset valid password: %d %s", validReset.Code, validReset.Body.String())
	}

	legacyHash, err := bcrypt.GenerateFromPassword([]byte("legacy7"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	legacy := models.User{Username: "old-user", PasswordHash: string(legacyHash), Role: "user"}
	if err := server.db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy account: %v", err)
	}
	legacyLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"old-user","password":"legacy7"}`))
	legacyLogin.Header.Set("Content-Type", "application/json")
	legacyWriter := httptest.NewRecorder()
	router.ServeHTTP(legacyWriter, legacyLogin)
	if legacyWriter.Code != http.StatusOK {
		t.Fatalf("legacy credentials must remain login-capable: %d %s", legacyWriter.Code, legacyWriter.Body.String())
	}
}

func TestLoginPersistsAndReturnsReaderDevLastLoginAt(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)

	hash, err := bcrypt.GenerateFromPassword([]byte("password8"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	user := models.User{
		Username:       "loginuser",
		PasswordHash:   string(hash),
		Role:           "user",
		CanEditSources: true,
		CanAccessStore: true,
		LastActiveAt:   previous,
	}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatalf("create login user: %v", err)
	}

	failed := adminContractRequest(
		router,
		http.MethodPost,
		"/api/auth/login",
		`{"username":"loginuser","password":"wrong-password"}`,
		"",
	)
	if failed.Code != http.StatusUnauthorized {
		t.Fatalf("failed login: status=%d, want 401: %s", failed.Code, failed.Body.String())
	}
	var unchanged models.User
	if err := server.db.First(&unchanged, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !unchanged.LastActiveAt.Equal(previous) {
		t.Fatalf("failed login changed persisted login time: got %s want %s", unchanged.LastActiveAt, previous)
	}

	success := adminContractRequest(
		router,
		http.MethodPost,
		"/api/auth/login",
		`{"username":"loginuser","password":"password8"}`,
		"",
	)
	if success.Code != http.StatusOK {
		t.Fatalf("successful login: status=%d: %s", success.Code, success.Body.String())
	}
	var loginPayload struct {
		User struct {
			LastLoginAt  time.Time `json:"lastLoginAt"`
			LastActiveAt time.Time `json:"lastActiveAt"`
		} `json:"user"`
	}
	if err := json.Unmarshal(success.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("decode login payload: %v", err)
	}
	if !loginPayload.User.LastLoginAt.After(previous) {
		t.Fatalf("login response did not advance lastLoginAt: %+v", loginPayload.User)
	}
	if !loginPayload.User.LastActiveAt.Equal(loginPayload.User.LastLoginAt) {
		t.Fatalf("legacy lastActiveAt alias diverged: %+v", loginPayload.User)
	}

	var stored models.User
	if err := server.db.First(&stored, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.LastActiveAt.Equal(loginPayload.User.LastLoginAt) {
		t.Fatalf("persisted login time diverged from response: stored=%s response=%s", stored.LastActiveAt, loginPayload.User.LastLoginAt)
	}

	listed := adminContractRequest(router, http.MethodGet, "/api/admin/users", "", adminAuth)
	if listed.Code != http.StatusOK {
		t.Fatalf("list users: status=%d: %s", listed.Code, listed.Body.String())
	}
	var users []struct {
		ID           uint      `json:"id"`
		LastLoginAt  time.Time `json:"lastLoginAt"`
		LastActiveAt time.Time `json:"lastActiveAt"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode user list: %v", err)
	}
	for _, listedUser := range users {
		if listedUser.ID != user.ID {
			continue
		}
		if !listedUser.LastLoginAt.Equal(stored.LastActiveAt) ||
			!listedUser.LastActiveAt.Equal(stored.LastActiveAt) {
			t.Fatalf("admin list login aliases diverged: %+v stored=%s", listedUser, stored.LastActiveAt)
		}
		return
	}
	t.Fatalf("login user %d missing from administrator list", user.ID)
}

func TestUserDeletionRemovesEveryPrivateRowAndWorkspaceWithoutTouchingOtherUsers(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)

	target := createUserManagementP2User(t, server, "purgeuser", time.Now())
	other := createUserManagementP2User(t, server, "keepuser", time.Now())
	populateUserManagementP2Workspace(t, server, target)
	populateUserManagementP2Workspace(t, server, other)
	targetBookID := userManagementP2BookID(t, server, target)
	if err := os.WriteFile(filepath.Join(server.cfg.DataDir, "webdav", "legacy-admin.txt"), []byte("administrator legacy data"), 0o644); err != nil {
		t.Fatalf("write legacy WebDAV fixture: %v", err)
	}

	deleted := adminContractRequest(router, http.MethodPost, "/api/admin/users/batch-delete", `{"ids":[`+strconv.FormatUint(uint64(target.ID), 10)+`]}`, adminAuth)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete user: %d %s", deleted.Code, deleted.Body.String())
	}
	assertUserManagementP2DataRemoved(t, server, target)
	assertUserManagementP2BookChaptersRemoved(t, server, targetBookID)
	assertUserManagementP2WorkspaceRemoved(t, server, target)
	assertUserManagementP2DataPresent(t, server, other)
	assertUserManagementP2WorkspacePresent(t, server, other)
	if _, err := os.Stat(filepath.Join(server.cfg.DataDir, "webdav", "legacy-admin.txt")); err != nil {
		t.Fatalf("deleting a regular user touched the administrator WebDAV root: %v", err)
	}

	inactive := createUserManagementP2User(t, server, "inactiveuser", time.Now().Add(-91*24*time.Hour))
	populateUserManagementP2Workspace(t, server, inactive)
	inactiveBookID := userManagementP2BookID(t, server, inactive)
	cleaned := adminContractRequest(router, http.MethodPost, "/api/admin/cleanup-inactive", `{}`, adminAuth)
	if cleaned.Code != http.StatusOK {
		t.Fatalf("cleanup inactive user: %d %s", cleaned.Code, cleaned.Body.String())
	}
	assertUserManagementP2DataRemoved(t, server, inactive)
	assertUserManagementP2BookChaptersRemoved(t, server, inactiveBookID)
	assertUserManagementP2WorkspaceRemoved(t, server, inactive)

	cleanupFailureUser := createUserManagementP2User(t, server, "cleanupfailureuser", time.Now())
	populateUserManagementP2Workspace(t, server, cleanupFailureUser)
	originalRemove := removeUserWorkspace
	removeUserWorkspace = func(string) error { return errors.New("private path must not reach the response") }
	t.Cleanup(func() { removeUserWorkspace = originalRemove })
	cleanupFailure := adminContractRequest(router, http.MethodPost, "/api/admin/users/batch-delete", `{"ids":[`+strconv.FormatUint(uint64(cleanupFailureUser.ID), 10)+`]}`, adminAuth)
	if cleanupFailure.Code != http.StatusOK || strings.Contains(cleanupFailure.Body.String(), "private path") {
		t.Fatalf("post-commit cleanup failure must be client-safe: %d %s", cleanupFailure.Code, cleanupFailure.Body.String())
	}
	assertUserManagementP2DataRemoved(t, server, cleanupFailureUser)
	assertUserManagementP2WorkspacePresent(t, server, cleanupFailureUser)
}

func createUserManagementP2User(t *testing.T, server *Server, username string, lastActive time.Time) models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password8"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{
		Username:       username,
		PasswordHash:   string(hash),
		Role:           "user",
		CanEditSources: true,
		CanAccessStore: true,
		LastActiveAt:   lastActive,
	}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	return user
}

func populateUserManagementP2Workspace(t *testing.T, server *Server, user models.User) {
	t.Helper()
	category := models.Category{UserID: user.ID, Name: "分类"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, CategoryID: &category.ID, Title: "书籍"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookCategory{UserID: user.ID, BookID: book.ID, CategoryID: category.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterIndex: 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Bookmark{UserID: user.ID, BookID: book.ID, Title: "书签"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReplaceRule{UserID: user.ID, Name: "规则", Pattern: "foo", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	rss := models.RSSSource{UserID: user.ID, Title: "RSS", URL: "https://example.com/" + user.Username, Enabled: true}
	if err := server.db.Create(&rss).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.RSSArticle{UserID: user.ID, SourceID: rss.ID, Title: "文章"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "reader", Value: `{}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.SourceFailure{UserID: user.ID, SourceID: 1, SourceURL: "https://example.com/source", Message: "failed", FailedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}

	for _, root := range []string{
		filepath.Join(server.cfg.DataDir, "webdav", "users", user.Username),
		filepath.Join(server.cfg.LocalStoreDir, "users", user.Username),
		filepath.Join(server.cfg.LibraryDir, "data", user.Username),
		filepath.Join(server.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(user.ID), 10)),
	} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte(user.Username), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func userManagementP2BookID(t *testing.T, server *Server, user models.User) uint {
	t.Helper()
	var book models.Book
	if err := server.db.Where("user_id = ?", user.ID).First(&book).Error; err != nil {
		t.Fatalf("find user book: %v", err)
	}
	return book.ID
}

func assertUserManagementP2DataRemoved(t *testing.T, server *Server, user models.User) {
	t.Helper()
	var userCount int64
	if err := server.db.Model(&models.User{}).Where("id = ?", user.ID).Count(&userCount).Error; err != nil {
		t.Fatalf("count user: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("user remains after deletion: %d", userCount)
	}
	for name, model := range map[string]any{
		"books":           &models.Book{},
		"categories":      &models.Category{},
		"book categories": &models.BookCategory{},
		"progress":        &models.ReadingProgress{},
		"bookmarks":       &models.Bookmark{},
		"rss sources":     &models.RSSSource{},
		"rss articles":    &models.RSSArticle{},
		"replace rules":   &models.ReplaceRule{},
		"settings":        &models.UserSetting{},
		"source failures": &models.SourceFailure{},
	} {
		var count int64
		if err := server.db.Model(model).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s remain after user deletion: %d", name, count)
		}
	}
	var chapterCount int64
	if err := server.db.Model(&models.Chapter{}).Joins("JOIN books ON books.id = chapters.book_id").Where("books.user_id = ?", user.ID).Count(&chapterCount).Error; err != nil {
		t.Fatalf("count chapters: %v", err)
	}
	if chapterCount != 0 {
		t.Fatalf("chapters remain after user deletion: %d", chapterCount)
	}
}

func assertUserManagementP2DataPresent(t *testing.T, server *Server, user models.User) {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.User{}).Where("id = ?", user.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("other user must remain: count=%d err=%v", count, err)
	}
	if err := server.db.Model(&models.BookCategory{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("other user book categories must remain: count=%d err=%v", count, err)
	}
	if err := server.db.Model(&models.SourceFailure{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("other user source failures must remain: count=%d err=%v", count, err)
	}
}

func assertUserManagementP2BookChaptersRemoved(t *testing.T, server *Server, bookID uint) {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.Chapter{}).Where("book_id = ?", bookID).Count(&count).Error; err != nil {
		t.Fatalf("count chapters: %v", err)
	}
	if count != 0 {
		t.Fatalf("chapters remain after user deletion: %d", count)
	}
}

func assertUserManagementP2WorkspaceRemoved(t *testing.T, server *Server, user models.User) {
	t.Helper()
	for _, root := range userManagementP2WorkspaceRoots(server, user) {
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("private workspace root remains %s: %v", root, err)
		}
	}
}

func assertUserManagementP2WorkspacePresent(t *testing.T, server *Server, user models.User) {
	t.Helper()
	for _, root := range userManagementP2WorkspaceRoots(server, user) {
		if _, err := os.Stat(filepath.Join(root, "owned.txt")); err != nil {
			t.Fatalf("other private workspace was touched at %s: %v", root, err)
		}
	}
}

func userManagementP2WorkspaceRoots(server *Server, user models.User) []string {
	return []string{
		filepath.Join(server.cfg.DataDir, "webdav", "users", user.Username),
		filepath.Join(server.cfg.LocalStoreDir, "users", user.Username),
		filepath.Join(server.cfg.LibraryDir, "data", user.Username),
		filepath.Join(server.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(user.ID), 10)),
	}
}
