package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openreader/backend/models"
)

type stagedStoragePreview struct {
	Items []struct {
		Path        string `json:"path"`
		Error       string `json:"error"`
		ImportToken string `json:"importToken"`
		Book        *struct {
			ChapterCount int `json:"chapterCount"`
		} `json:"book"`
	} `json:"items"`
}

func previewStorageBook(t *testing.T, router http.Handler, auth string, endpoint string, path string) stagedStoragePreview {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"paths":["`+path+`"]}`))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("preview %s: expected 200, got %d: %s", endpoint, writer.Code, writer.Body.String())
	}
	var preview stagedStoragePreview
	if err := json.Unmarshal(writer.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview %s: %v", endpoint, err)
	}
	if len(preview.Items) != 1 || preview.Items[0].Book == nil || preview.Items[0].ImportToken == "" || !validLocalImportToken(preview.Items[0].ImportToken) {
		t.Fatalf("preview %s must return one staged book, got %+v", endpoint, preview)
	}
	return preview
}

func importStagedStorageBook(t *testing.T, router http.Handler, auth string, endpoint string, path string, importToken string, title string) models.Book {
	return importStagedStorageBookWithTOCRule(t, router, auth, endpoint, path, importToken, title, "")
}

func importStagedStorageBookWithTOCRule(t *testing.T, router http.Handler, auth string, endpoint string, path string, importToken string, title string, tocRule string) models.Book {
	t.Helper()
	body := `{"items":[{"path":"` + path + `","importToken":"` + importToken + `","title":"` + title + `","tocRule":"` + tocRule + `"}]}`
	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("import %s: expected 200, got %d: %s", endpoint, writer.Code, writer.Body.String())
	}
	var response struct {
		Imported []struct {
			Error string      `json:"error"`
			Book  models.Book `json:"book"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode import %s: %v", endpoint, err)
	}
	if len(response.Imported) != 1 || response.Imported[0].Error != "" || response.Imported[0].Book.ID == 0 {
		t.Fatalf("import %s must use the staged snapshot, got %+v", endpoint, response.Imported)
	}
	return response.Imported[0].Book
}

func reparseStagedStorageBook(t *testing.T, router http.Handler, auth string, endpoint string, path string, importToken string, tocRule string) stagedStoragePreview {
	t.Helper()
	body := `{"items":[{"path":"` + path + `","importToken":"` + importToken + `","tocRule":"` + tocRule + `"}]}`
	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("reparse %s: expected 200, got %d: %s", endpoint, writer.Code, writer.Body.String())
	}
	var preview stagedStoragePreview
	if err := json.Unmarshal(writer.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode reparse %s: %v", endpoint, err)
	}
	if len(preview.Items) != 1 {
		t.Fatalf("reparse %s must return one item, got %+v", endpoint, preview)
	}
	return preview
}

func TestLocalStorePreviewStagesSnapshotForConfirmImport(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	path := filepath.Join(server.cfg.LocalStoreDir, "snapshot.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create local-store root: %v", err)
	}
	if err := os.WriteFile(path, []byte("第一章 开始\n正文\n第二章 继续\n正文"), 0o644); err != nil {
		t.Fatalf("write local-store fixture: %v", err)
	}

	preview := previewStorageBook(t, router, auth, "/api/local-store/import-preview", "snapshot.txt")
	expectedChapters := preview.Items[0].Book.ChapterCount
	if expectedChapters < 1 {
		t.Fatalf("staged preview must have a readable catalogue: %+v", preview.Items[0].Book)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove original local-store file after preview: %v", err)
	}

	book := importStagedStorageBook(t, router, auth, "/api/local-store/import", "snapshot.txt", preview.Items[0].ImportToken, "本地快照导入")
	if book.ChapterCount != expectedChapters {
		t.Fatalf("confirm must import the preview snapshot (%d chapters), got %+v", expectedChapters, book)
	}
	dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(book.UserID), preview.Items[0].ImportToken)
	if _, err := os.Stat(dataPath); !os.IsNotExist(err) {
		t.Fatalf("consumed staged data must be removed, got %v", err)
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("consumed staged metadata must be removed, got %v", err)
	}
}

func TestWebDAVPreviewStagesSnapshotForConfirmImport(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	path := filepath.Join(server.cfg.DataDir, "webdav", "snapshot.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create WebDAV root: %v", err)
	}
	if err := os.WriteFile(path, []byte("第一章 开始\n正文\n第二章 继续\n正文"), 0o644); err != nil {
		t.Fatalf("write WebDAV fixture: %v", err)
	}

	preview := previewStorageBook(t, router, auth, "/api/webdav/import-preview", "snapshot.txt")
	expectedChapters := preview.Items[0].Book.ChapterCount
	if expectedChapters < 1 {
		t.Fatalf("staged preview must have a readable catalogue: %+v", preview.Items[0].Book)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove original WebDAV file after preview: %v", err)
	}

	book := importStagedStorageBook(t, router, auth, "/api/webdav/import", "snapshot.txt", preview.Items[0].ImportToken, "WebDAV 快照导入")
	if book.ChapterCount != expectedChapters {
		t.Fatalf("confirm must import the preview snapshot (%d chapters), got %+v", expectedChapters, book)
	}
}

func TestStoragePreviewStageReparsesAfterMountedSourceIsRemoved(t *testing.T) {
	tests := []struct {
		name            string
		previewEndpoint string
		importEndpoint  string
		filePath        func(*Server) string
	}{
		{
			name:            "local store",
			previewEndpoint: "/api/local-store/import-preview",
			importEndpoint:  "/api/local-store/import",
			filePath: func(server *Server) string {
				return filepath.Join(server.cfg.LocalStoreDir, "retry-rule.txt")
			},
		},
		{
			name:            "WebDAV",
			previewEndpoint: "/api/webdav/import-preview",
			importEndpoint:  "/api/webdav/import",
			filePath: func(server *Server) string {
				return filepath.Join(server.cfg.DataDir, "webdav", "retry-rule.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			path := tt.filePath(server)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create fixture root: %v", err)
			}
			if err := os.WriteFile(path, []byte("== 第一章 ==\n正文内容。"), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			initial := previewStorageBook(t, router, auth, tt.previewEndpoint, "retry-rule.txt")
			stageToken := initial.Items[0].ImportToken
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove mounted source after preview: %v", err)
			}

			failed := reparseStagedStorageBook(t, router, auth, tt.previewEndpoint, "retry-rule.txt", stageToken, `^不存在的目录$`)
			if failed.Items[0].ImportToken != stageToken || !strings.Contains(failed.Items[0].Error, "no readable chapters") {
				t.Fatalf("failed staged reparse must retain token/error, got %+v", failed.Items[0])
			}

			retry := reparseStagedStorageBook(t, router, auth, tt.previewEndpoint, "retry-rule.txt", stageToken, `^== .+ ==$`)
			if retry.Items[0].ImportToken != stageToken || retry.Items[0].Book == nil || retry.Items[0].Book.ChapterCount != 1 {
				t.Fatalf("valid staged reparse must use removed-source snapshot, got %+v", retry.Items[0])
			}

			book := importStagedStorageBookWithTOCRule(t, router, auth, tt.importEndpoint, "retry-rule.txt", stageToken, "目录重试导入", `^== .+ ==$`)
			if book.ChapterCount != 1 {
				t.Fatalf("staged reparse/import chapter count = %d, want 1", book.ChapterCount)
			}
		})
	}
}

func TestStoragePreviewTokensAreUserScopedAndExpireSafely(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	path := filepath.Join(server.cfg.LocalStoreDir, "token-scope.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create local-store root: %v", err)
	}
	if err := os.WriteFile(path, []byte("第一章 开始\n正文"), 0o644); err != nil {
		t.Fatalf("write local-store fixture: %v", err)
	}
	preview := previewStorageBook(t, router, adminAuth, "/api/local-store/import-preview", "token-scope.txt")
	token := preview.Items[0].ImportToken

	memberAuth := registerStorageTestUser(t, router, "token-other-user")
	foreign := httptest.NewRequest(http.MethodPost, "/api/local-store/import", strings.NewReader(`{"items":[{"path":"token-scope.txt","importToken":"`+token+`"}]}`))
	foreign.Header.Set("Authorization", memberAuth)
	foreign.Header.Set("Content-Type", "application/json")
	foreignWriter := httptest.NewRecorder()
	router.ServeHTTP(foreignWriter, foreign)
	if foreignWriter.Code != http.StatusOK || !strings.Contains(foreignWriter.Body.String(), "invalid or expired local import token") {
		t.Fatalf("foreign staged token must be rejected without filesystem fallback, got %d: %s", foreignWriter.Code, foreignWriter.Body.String())
	}
	var member models.User
	if err := server.db.Where("username = ?", "token-other-user").First(&member).Error; err != nil {
		t.Fatalf("load member: %v", err)
	}
	var memberBooks int64
	if err := server.db.Model(&models.Book{}).Where("user_id = ?", member.ID).Count(&memberBooks).Error; err != nil {
		t.Fatalf("count member books: %v", err)
	}
	if memberBooks != 0 {
		t.Fatalf("foreign staged token must not create a member book, got %d", memberBooks)
	}

	dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(1), token)
	encoded, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read staged metadata: %v", err)
	}
	var metadata localImportStageMetadata
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		t.Fatalf("decode staged metadata: %v", err)
	}
	metadata.CreatedAt = time.Now().Add(-localImportStageLifetime - time.Minute)
	expired, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("encode expired metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, expired, 0o600); err != nil {
		t.Fatalf("write expired metadata: %v", err)
	}

	expiredRequest := httptest.NewRequest(http.MethodPost, "/api/local-store/import", strings.NewReader(`{"items":[{"path":"token-scope.txt","importToken":"`+token+`"}]}`))
	expiredRequest.Header.Set("Authorization", adminAuth)
	expiredRequest.Header.Set("Content-Type", "application/json")
	expiredWriter := httptest.NewRecorder()
	router.ServeHTTP(expiredWriter, expiredRequest)
	if expiredWriter.Code != http.StatusOK || !strings.Contains(expiredWriter.Body.String(), "invalid or expired local import token") {
		t.Fatalf("expired staged token must be rejected, got %d: %s", expiredWriter.Code, expiredWriter.Body.String())
	}
	if _, err := os.Stat(dataPath); !os.IsNotExist(err) {
		t.Fatalf("expired staged bytes must be cleaned, got %v", err)
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("expired staged metadata must be cleaned, got %v", err)
	}
}

func TestLocalImportStageRejectsOversizedInputBeforePersistingPreview(t *testing.T) {
	_, server := setupTestServer(t)
	server.cfg.MaxImportBytes = 8
	if _, err := server.stageLocalImport(1, "oversized.txt", ".txt", []byte("123456789")); err != errLocalImportTooLarge {
		t.Fatalf("oversized staged input: expected size error, got %v", err)
	}

	path := filepath.Join(t.TempDir(), "oversized.txt")
	if err := os.WriteFile(path, []byte("123456789"), 0o600); err != nil {
		t.Fatalf("write oversized fixture: %v", err)
	}
	if _, err := server.readBoundedLocalImportFile(path); err != errLocalImportTooLarge {
		t.Fatalf("oversized mounted input: expected size error, got %v", err)
	}
}
