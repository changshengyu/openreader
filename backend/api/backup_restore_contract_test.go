package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
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

func TestBackupRoundTripsPersistentSourceVariables(t *testing.T) {
	_, sourceServer := setupTestServer(t)
	user := models.User{Username: "variable-backup-user", PasswordHash: "hash"}
	if err := sourceServer.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "变量备份书源", BaseURL: "https://backup-variables.example", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{BookInfoNameRule: "h1|text", ContentRule: ".content|text"}); err != nil {
		t.Fatal(err)
	}
	if err := sourceServer.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:   user.ID,
		SourceID: source.ID,
		Title:    "变量备份书",
		URL:      source.BaseURL + "/book/1",
		Variable: `{"bookToken":"book-backup"}`,
	}
	if err := sourceServer.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{
		BookID:   book.ID,
		Index:    0,
		Title:    "第一章",
		URL:      source.BaseURL + "/chapter/1",
		Variable: `{"chapterToken":"chapter-backup"}`,
	}
	if err := sourceServer.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	backupPath, err := sourceServer.backupSvc.RunNowForUser(user.ID, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var hasBookshelf, hasChapterVariables bool
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch file.Name {
		case "bookshelf.json":
			hasBookshelf = strings.Contains(string(contents), `"sourceName": "变量备份书源"`) && strings.Contains(string(contents), "book-backup")
		case "chapterVariables.json":
			hasChapterVariables = strings.Contains(string(contents), "chapter-backup")
		}
	}
	if !hasBookshelf || !hasChapterVariables {
		t.Fatalf("backup must include portable source variables: shelf=%v chapter=%v", hasBookshelf, hasChapterVariables)
	}

	_, destinationServer := setupTestServer(t)
	destinationUser := models.User{Username: "variable-restore-user", PasswordHash: "hash"}
	if err := destinationServer.db.Create(&destinationUser).Error; err != nil {
		t.Fatal(err)
	}
	result, err := destinationServer.restoreLegadoBackupData(archive, destinationUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result["chapterVariables"] != 1 {
		t.Fatalf("restored chapter variable count = %#v", result)
	}
	var restoredBook models.Book
	if err := destinationServer.db.Where("user_id = ? AND url = ?", destinationUser.ID, book.URL).First(&restoredBook).Error; err != nil {
		t.Fatal(err)
	}
	if restoredBook.SourceID == 0 || restoredBook.Variable != book.Variable {
		t.Fatalf("restored book source state = %+v", restoredBook)
	}
	var restoredChapter models.Chapter
	if err := destinationServer.db.Where("book_id = ? AND `index` = ?", restoredBook.ID, 0).First(&restoredChapter).Error; err != nil {
		t.Fatal(err)
	}
	if restoredChapter.Variable != chapter.Variable || restoredChapter.URL != chapter.URL {
		t.Fatalf("restored chapter source state = %+v", restoredChapter)
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
