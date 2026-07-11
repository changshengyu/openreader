package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"openreader/backend/config"
)

func oversizedMultipartRequest(t *testing.T, endpoint string, fileName string, data []byte, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, endpoint, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestImportAndStorageUploadsRejectOversizedInputBeforeWriting(t *testing.T) {
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxImportBytes = 8
	})
	auth := authHeader(t, router)
	overLimit := []byte("123456789")

	direct := oversizedMultipartRequest(t, "/api/imports/books/preview", "oversized.txt", overLimit, nil)
	direct.Header.Set("Authorization", auth)
	directWriter := httptest.NewRecorder()
	router.ServeHTTP(directWriter, direct)
	if directWriter.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("direct preview: expected 413, got %d: %s", directWriter.Code, directWriter.Body.String())
	}

	localStorePath := filepath.Join(server.cfg.LocalStoreDir, "oversized.txt")
	if err := os.MkdirAll(filepath.Dir(localStorePath), 0o755); err != nil {
		t.Fatalf("create local-store root: %v", err)
	}
	if err := os.WriteFile(localStorePath, []byte("previous local-store file"), 0o644); err != nil {
		t.Fatalf("write existing local-store file: %v", err)
	}
	localStore := oversizedMultipartRequest(t, "/api/local-store/upload", "oversized.txt", overLimit, map[string]string{"path": ""})
	localStore.Header.Set("Authorization", auth)
	localStoreWriter := httptest.NewRecorder()
	router.ServeHTTP(localStoreWriter, localStore)
	if localStoreWriter.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("local-store upload: expected 413, got %d: %s", localStoreWriter.Code, localStoreWriter.Body.String())
	}
	if data, err := os.ReadFile(localStorePath); err != nil || string(data) != "previous local-store file" {
		t.Fatalf("oversized local-store upload must preserve the previous destination, data=%q err=%v", data, err)
	}

	webdavPath := filepath.Join(server.cfg.DataDir, "webdav", "oversized.txt")
	if err := os.MkdirAll(filepath.Dir(webdavPath), 0o755); err != nil {
		t.Fatalf("create WebDAV root: %v", err)
	}
	if err := os.WriteFile(webdavPath, []byte("previous WebDAV file"), 0o644); err != nil {
		t.Fatalf("write existing WebDAV file: %v", err)
	}
	webdav := httptest.NewRequest(http.MethodPut, "/webdav/oversized.txt", bytes.NewReader(overLimit))
	webdav.ContentLength = -1 // Exercise the bounded streaming path, not only the Content-Length fast path.
	webdav.Header.Set("Authorization", auth)
	webdav.Header.Set("Content-Type", "application/octet-stream")
	webdavWriter := httptest.NewRecorder()
	router.ServeHTTP(webdavWriter, webdav)
	if webdavWriter.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("WebDAV upload: expected 413, got %d: %s", webdavWriter.Code, webdavWriter.Body.String())
	}
	if data, err := os.ReadFile(webdavPath); err != nil || string(data) != "previous WebDAV file" {
		t.Fatalf("oversized WebDAV upload must preserve the previous destination, data=%q err=%v", data, err)
	}
}
