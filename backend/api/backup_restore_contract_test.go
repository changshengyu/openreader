package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestBackupArchivePreflightRejectsUnsafeAndOverBudgetStructures(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		limits  backupRestoreLimits
		wantErr error
	}{
		{
			name:    "unsafe path",
			files:   map[string]string{"../bookSource.json": `[]`},
			limits:  defaultBackupRestoreLimits(),
			wantErr: errInvalidBackupArchive,
		},
		{
			name:    "duplicate canonical path",
			files:   map[string]string{"bookSource.json": `[]`, "BOOKSOURCE.json": `[]`},
			limits:  defaultBackupRestoreLimits(),
			wantErr: errInvalidBackupArchive,
		},
		{
			name:    "too many entries",
			files:   map[string]string{"one.json": `{}`, "two.json": `{}`},
			limits:  backupRestoreLimits{MaxCompressedBytes: 1024, MaxEntries: 1, MaxEntryBytes: 1024, MaxExpandedBytes: 1024},
			wantErr: errBackupArchiveLimit,
		},
		{
			name:    "entry over budget",
			files:   map[string]string{"bookSource.json": `12345`},
			limits:  backupRestoreLimits{MaxCompressedBytes: 1024, MaxEntries: 2, MaxEntryBytes: 4, MaxExpandedBytes: 1024},
			wantErr: errBackupArchiveLimit,
		},
		{
			name:    "total over budget",
			files:   map[string]string{"one.json": `12345`, "two.json": `67890`},
			limits:  backupRestoreLimits{MaxCompressedBytes: 1024, MaxEntries: 2, MaxEntryBytes: 8, MaxExpandedBytes: 8},
			wantErr: errBackupArchiveLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newBackupRestoreArchive(makeBackupRestoreZIP(t, tt.files), tt.limits)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("preflight error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestBackupArchiveStructuralFailureDoesNotMutateUserData(t *testing.T) {
	_, server := setupTestServer(t)
	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookSource.json": `[{"name":"must-not-restore","baseUrl":"https://source.example"}]`,
		"../invalid.json": `{}`,
	})
	if _, err := server.restoreLegadoBackupData(archive, 1); !errors.Is(err, errInvalidBackupArchive) {
		t.Fatalf("unsafe restore error = %v, want %v", err, errInvalidBackupArchive)
	}
	var count int64
	if err := server.db.Model(&models.BookSource{}).Where("name = ?", "must-not-restore").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid archive must not mutate sources, got %d", count)
	}
}

func TestBackupRestoreRejectsOversizedUploadAndNonZipWebDAVTarget(t *testing.T) {
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxBackupRestoreBytes = 8
	})
	auth := authHeader(t, router)

	oversized := multipartBackupRestoreRequest(t, "backup.zip", bytes.Repeat([]byte("x"), 9))
	oversized.Header.Set("Authorization", auth)
	oversizedWriter := httptest.NewRecorder()
	router.ServeHTTP(oversizedWriter, oversized)
	if oversizedWriter.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized backup upload: expected 413, got %d: %s", oversizedWriter.Code, oversizedWriter.Body.String())
	}

	path := filepath.Join(server.cfg.DataDir, "webdav", "not-a-backup.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/restore-webdav", strings.NewReader(`{"path":"not-a-backup.txt"}`))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusBadRequest || !strings.Contains(writer.Body.String(), "backup file must be a zip archive") {
		t.Fatalf("non-zip WebDAV restore: expected safe 400, got %d: %s", writer.Code, writer.Body.String())
	}
}

func makeBackupRestoreZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func multipartBackupRestoreRequest(t *testing.T, name string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/restore-legado", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
