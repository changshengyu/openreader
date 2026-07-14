package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"openreader/backend/config"
)

func TestLocalStoreP1E3ListingOmitsHiddenItemsAndReturnsModificationTime(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatalf("create local-store root: %v", err)
	}
	for name, body := range map[string]string{
		".private.txt": "hidden",
		"visible.txt":  "visible",
	} {
		if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local-store", nil)
	req.Header.Set("Authorization", auth)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("list local-store: expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	var response struct {
		Items []struct {
			Name         string `json:"name"`
			LastModified string `json:"lastModified"`
		} `json:"items"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].Name != "visible.txt" {
		t.Fatalf("upstream listing must omit hidden files, got %+v", response.Items)
	}
	if response.Items[0].LastModified == "" {
		t.Fatalf("upstream listing must include lastModified, got %+v", response.Items[0])
	}
}

func TestLocalStoreP1E3UploadAcceptsMultipleManagedFilesWithoutParserAdmission(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("path", "incoming"); err != nil {
		t.Fatalf("write path: %v", err)
	}
	for name, content := range map[string]string{
		"notes.bin": "ordinary managed file",
		"book.txt":  "第一章\n正文",
	} {
		part, err := form.CreateFormFile("file", name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatalf("close form: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/local-store/upload", &body)
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", form.FormDataContentType())
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusCreated {
		t.Fatalf("multi upload: expected 201, got %d: %s", writer.Code, writer.Body.String())
	}
	var response struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(response.Paths) != 2 {
		t.Fatalf("multi upload must return every saved path, got %+v", response)
	}
	for _, name := range []string{"notes.bin", "book.txt"} {
		if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, "incoming", name)); err != nil {
			t.Fatalf("stored %s: %v", name, err)
		}
	}
}

func TestLocalStoreP1E3RejectedMultiUploadPartPreservesExistingDestination(t *testing.T) {
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxImportBytes = 8
	})
	auth := authHeader(t, router)
	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatalf("create local-store root: %v", err)
	}
	existingPath := filepath.Join(server.cfg.LocalStoreDir, "existing.txt")
	if err := os.WriteFile(existingPath, []byte("previous"), 0o644); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, fixture := range []struct {
		name string
		body string
	}{
		{name: "accepted.bin", body: "ok"},
		{name: "existing.txt", body: "123456789"},
	} {
		part, err := form.CreateFormFile("file", fixture.name)
		if err != nil {
			t.Fatalf("create %s: %v", fixture.name, err)
		}
		if _, err := part.Write([]byte(fixture.body)); err != nil {
			t.Fatalf("write %s: %v", fixture.name, err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatalf("close form: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/local-store/upload", &body)
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", form.FormDataContentType())
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized multi part: expected 413, got %d: %s", writer.Code, writer.Body.String())
	}
	if content, err := os.ReadFile(existingPath); err != nil || string(content) != "previous" {
		t.Fatalf("rejected multi part must preserve existing destination, data=%q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(server.cfg.LocalStoreDir, "accepted.bin")); err != nil || string(content) != "ok" {
		t.Fatalf("accepted earlier multi part must retain its independent write, data=%q err=%v", content, err)
	}
}
