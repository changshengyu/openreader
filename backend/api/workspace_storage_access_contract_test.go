package api

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

func registerStorageTestUser(t *testing.T, router *gin.Engine, username string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"`+username+`","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("register %s: expected 200, got %d: %s", username, w.Code, w.Body.String())
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("register %s: decode response: %v", username, err)
	}
	if response.Token == "" {
		t.Fatalf("register %s: missing token", username)
	}
	return "Bearer " + response.Token
}

func TestWorkspaceStoragePermissionsSeparateLocalStoreFromWebDAVAndBackup(t *testing.T) {
	router, server := setupTestServer(t)
	disabledAuth := registerStorageTestUser(t, router, "storedisabled")

	if err := server.db.Model(&models.User{}).Where("username = ?", "storedisabled").Update("can_access_store", false).Error; err != nil {
		t.Fatalf("disable store access: %v", err)
	}

	legacyFallbackTests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "local list", method: http.MethodGet, path: "/api/local-store"},
		{name: "local download", method: http.MethodGet, path: "/api/local-store/download?path=book.txt"},
		{name: "local create directory", method: http.MethodPost, path: "/api/local-store/directory", body: `{"path":"","name":"books"}`},
		{name: "local rename", method: http.MethodPut, path: "/api/local-store/rename", body: `{"path":"book.txt","name":"renamed.txt"}`},
		{name: "local delete", method: http.MethodDelete, path: "/api/local-store?path=book.txt"},
		{name: "local preview", method: http.MethodPost, path: "/api/local-store/import-preview", body: `{"paths":["book.txt"]}`},
		{name: "local import", method: http.MethodPost, path: "/api/local-store/import", body: `{"paths":["book.txt"]}`},
	}

	for _, tt := range legacyFallbackTests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", disabledAuth)
			if tt.body != "" && strings.HasPrefix(tt.body, "{") {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.name == "webdav move" {
				req.Header.Set("Destination", "/webdav/renamed.txt")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s: expected 403 before any storage operation, got %d: %s", tt.name, w.Code, w.Body.String())
			}
		})
	}

	if err := server.db.Model(&models.User{}).Where("username = ?", "storedisabled").Update("can_access_webdav", true).Error; err != nil {
		t.Fatalf("grant explicit WebDAV access: %v", err)
	}
	for _, tt := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "local list", method: http.MethodGet, path: "/api/local-store"},
		{name: "local create directory", method: http.MethodPost, path: "/api/local-store/directory", body: `{"path":"","name":"books"}`},
	} {
		t.Run("LocalStore remains disabled: "+tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", disabledAuth)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, req)
			if writer.Code != http.StatusForbidden {
				t.Fatalf("%s: expected 403, got %d: %s", tt.name, writer.Code, writer.Body.String())
			}
		})
	}
	for _, tt := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "WebDAV list", method: http.MethodGet, path: "/webdav/"},
		{name: "backup list", method: http.MethodGet, path: "/api/backup/list"},
	} {
		t.Run("explicit WebDAV grant: "+tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Authorization", disabledAuth)
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, req)
			if writer.Code == http.StatusForbidden {
				t.Fatalf("%s: explicit WebDAV grant must be independent: %s", tt.name, writer.Body.String())
			}
		})
	}

	if err := server.db.Model(&models.User{}).Where("username = ?", "storedisabled").Updates(map[string]any{
		"can_access_store":  true,
		"can_access_webdav": false,
	}).Error; err != nil {
		t.Fatalf("separate WebDAV and local-store permissions: %v", err)
	}
	localRequest := httptest.NewRequest(http.MethodGet, "/api/local-store", nil)
	localRequest.Header.Set("Authorization", disabledAuth)
	localWriter := httptest.NewRecorder()
	router.ServeHTTP(localWriter, localRequest)
	if localWriter.Code != http.StatusOK {
		t.Fatalf("local-store must remain enabled: %d %s", localWriter.Code, localWriter.Body.String())
	}
	for _, path := range []string{"/webdav/", "/api/backup/list"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", disabledAuth)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		if writer.Code != http.StatusForbidden {
			t.Fatalf("%s: explicit WebDAV denial must not inherit LocalStore grant, got %d: %s", path, writer.Code, writer.Body.String())
		}
	}
}

func TestRawWebDAVRequiresAuthenticationBeforeFilesystemAccess(t *testing.T) {
	router, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/webdav/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated WebDAV list: expected 401, got %d: %s", w.Code, w.Body.String())
	}

	auth := authHeader(t, router)
	authorized := httptest.NewRequest(http.MethodGet, "/webdav/", nil)
	authorized.Header.Set("Authorization", auth)
	authorizedWriter := httptest.NewRecorder()
	router.ServeHTTP(authorizedWriter, authorized)
	if authorizedWriter.Code != http.StatusMultiStatus {
		t.Fatalf("authorized WebDAV list: expected 207, got %d: %s", authorizedWriter.Code, authorizedWriter.Body.String())
	}
}

func TestWorkspaceStorageUsesPrivateRootsForRegularUsersAndKeepsAdminLegacyRoot(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	memberAuth := registerStorageTestUser(t, router, "privatemember")
	var admin, member models.User
	if err := server.db.Where("username = ?", "testuser").First(&admin).Error; err != nil {
		t.Fatalf("load administrator: %v", err)
	}
	if err := server.db.Where("username = ?", "privatemember").First(&member).Error; err != nil {
		t.Fatalf("load private member: %v", err)
	}
	if err := server.db.Create(&models.UserSetting{UserID: admin.ID, Key: "backup-scope", Value: `{"owner":"administrator"}`}).Error; err != nil {
		t.Fatalf("create administrator setting: %v", err)
	}
	if err := server.db.Create(&models.UserSetting{UserID: member.ID, Key: "backup-scope", Value: `{"owner":"privatemember"}`}).Error; err != nil {
		t.Fatalf("create member setting: %v", err)
	}

	adminWrite := httptest.NewRequest(http.MethodPut, "/webdav/legacy.txt", strings.NewReader("legacy administrator data"))
	adminWrite.Header.Set("Authorization", adminAuth)
	adminWriteWriter := httptest.NewRecorder()
	router.ServeHTTP(adminWriteWriter, adminWrite)
	if adminWriteWriter.Code != http.StatusCreated {
		t.Fatalf("admin legacy WebDAV write: expected 201, got %d: %s", adminWriteWriter.Code, adminWriteWriter.Body.String())
	}
	if _, err := os.Stat(filepath.Join(server.cfg.DataDir, "webdav", "legacy.txt")); err != nil {
		t.Fatalf("legacy WebDAV root must remain usable for the administrator: %v", err)
	}

	privateDirectory := httptest.NewRequest(http.MethodPost, "/api/local-store/directory", strings.NewReader(`{"path":"","name":"private-books"}`))
	privateDirectory.Header.Set("Authorization", memberAuth)
	privateDirectory.Header.Set("Content-Type", "application/json")
	privateDirectoryWriter := httptest.NewRecorder()
	router.ServeHTTP(privateDirectoryWriter, privateDirectory)
	if privateDirectoryWriter.Code != http.StatusCreated {
		t.Fatalf("member local-store directory: expected 201, got %d: %s", privateDirectoryWriter.Code, privateDirectoryWriter.Body.String())
	}
	memberLocalRoot := filepath.Join(server.cfg.LocalStoreDir, "users", "privatemember")
	if _, err := os.Stat(filepath.Join(memberLocalRoot, "private-books")); err != nil {
		t.Fatalf("member LocalStore must use a private root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, "private-books")); !os.IsNotExist(err) {
		t.Fatalf("member LocalStore must not write into the legacy root, stat err=%v", err)
	}

	privateWrite := httptest.NewRequest(http.MethodPut, "/webdav/private.txt", strings.NewReader("private member data"))
	privateWrite.Header.Set("Authorization", memberAuth)
	privateWriteWriter := httptest.NewRecorder()
	router.ServeHTTP(privateWriteWriter, privateWrite)
	if privateWriteWriter.Code != http.StatusCreated {
		t.Fatalf("member WebDAV write: expected 201, got %d: %s", privateWriteWriter.Code, privateWriteWriter.Body.String())
	}
	memberWebDAVRoot := filepath.Join(server.cfg.DataDir, "webdav", "users", "privatemember")
	if _, err := os.Stat(filepath.Join(memberWebDAVRoot, "private.txt")); err != nil {
		t.Fatalf("member WebDAV must use a private root: %v", err)
	}

	memberReadsLegacy := httptest.NewRequest(http.MethodGet, "/webdav/legacy.txt", nil)
	memberReadsLegacy.Header.Set("Authorization", memberAuth)
	memberReadsLegacyWriter := httptest.NewRecorder()
	router.ServeHTTP(memberReadsLegacyWriter, memberReadsLegacy)
	if memberReadsLegacyWriter.Code != http.StatusNotFound {
		t.Fatalf("member must not read the administrator legacy root: expected 404, got %d: %s", memberReadsLegacyWriter.Code, memberReadsLegacyWriter.Body.String())
	}

	memberBackup := httptest.NewRequest(http.MethodPost, "/api/backup/trigger", nil)
	memberBackup.Header.Set("Authorization", memberAuth)
	memberBackupWriter := httptest.NewRecorder()
	router.ServeHTTP(memberBackupWriter, memberBackup)
	if memberBackupWriter.Code != http.StatusOK {
		t.Fatalf("member backup: expected 200, got %d: %s", memberBackupWriter.Code, memberBackupWriter.Body.String())
	}
	var backup struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(memberBackupWriter.Body.Bytes(), &backup); err != nil || backup.Name == "" {
		t.Fatalf("member backup response: %+v, err=%v", backup, err)
	}
	if _, err := os.Stat(filepath.Join(memberWebDAVRoot, backup.Name)); err != nil {
		t.Fatalf("member backup must be private: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.DataDir, "webdav", backup.Name)); !os.IsNotExist(err) {
		t.Fatalf("member backup must not be written to the admin legacy root, stat err=%v", err)
	}

	archive, err := zip.OpenReader(filepath.Join(memberWebDAVRoot, backup.Name))
	if err != nil {
		t.Fatalf("open private backup: %v", err)
	}
	defer archive.Close()
	var settings []models.UserSetting
	for _, file := range archive.File {
		if file.Name != "userSettings.json" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open user settings in backup: %v", err)
		}
		if err := json.NewDecoder(reader).Decode(&settings); err != nil {
			reader.Close()
			t.Fatalf("decode user settings in backup: %v", err)
		}
		reader.Close()
		break
	}
	if len(settings) != 1 || settings[0].UserID != member.ID || settings[0].Value != `{"owner":"privatemember"}` {
		t.Fatalf("member backup must contain only member settings, got %+v", settings)
	}
}

func TestWorkspaceStorageRejectsUnsupportedDirectImportFilesPredictably(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatalf("create local-store root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "unsupported.bin"), []byte("not a book"), 0o644); err != nil {
		t.Fatalf("write local-store fixture: %v", err)
	}
	webdavRoot := filepath.Join(server.cfg.DataDir, "webdav")
	if err := os.MkdirAll(webdavRoot, 0o755); err != nil {
		t.Fatalf("create WebDAV root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(webdavRoot, "unsupported.bin"), []byte("not a book"), 0o644); err != nil {
		t.Fatalf("write WebDAV fixture: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "local preview", path: "/api/local-store/import-preview"},
		{name: "local import", path: "/api/local-store/import"},
		{name: "WebDAV preview", path: "/api/webdav/import-preview"},
		{name: "WebDAV import", path: "/api/webdav/import"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(`{"paths":["unsupported.bin"]}`))
			req.Header.Set("Authorization", auth)
			req.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, req)
			if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), `"error":"unsupported file type"`) {
				t.Fatalf("%s: expected deterministic unsupported-file item error, got %d: %s", tt.name, writer.Code, writer.Body.String())
			}
		})
	}
}
