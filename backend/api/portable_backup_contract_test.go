package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
	"openreader/backend/services/localbook"
)

func TestPortableBackupRestoresLocalArchiveAndLogicalReferences(t *testing.T) {
	_, source := setupTestServer(t)
	sourceUser := models.User{Username: "portable-source", PasswordHash: "hash"}
	if err := source.db.Create(&sourceUser).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: sourceUser.ID, Name: "可移植分类"}
	if err := source.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	book, err := localbook.NewImporter(source.cfg, source.db).Import(localbook.ImportRequest{
		UserID: sourceUser.ID, UserName: sourceUser.Username, FileName: "portable.txt", Extension: ".txt",
		Data: []byte("第一章 起始正文\n第一章 第二段"), Title: "可移植 TXT", Author: "备份作者", CategoryID: &category.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := source.db.Create(&models.ReadingProgress{UserID: sourceUser.ID, BookID: book.ID, ChapterIndex: 0, Offset: 3, ChapterTitle: "第一章"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := source.db.Create(&models.Bookmark{UserID: sourceUser.ID, BookID: book.ID, ChapterIndex: 0, Offset: 4, Title: "可移植书签"}).Error; err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(source.cfg.LibraryDir, book.OriginalFile)
	originalData, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(originalData)
	backupDir := filepath.Join(source.cfg.DataDir, "webdav", "users", sourceUser.Username)
	portablePath, count, err := source.backupSvc.RunPortableForUser(sourceUser.ID, sourceUser.Username, backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("source portable book count = %d", count)
	}

	_, destination := setupTestServer(t)
	destinationUser := models.User{Username: "portable-destination", PasswordHash: "hash"}
	if err := destination.db.Create(&destinationUser).Error; err != nil {
		t.Fatal(err)
	}
	result, err := destination.restorePortableBackupFile(portablePath, destinationUser.ID, destinationUser.Username)
	if err != nil {
		t.Fatal(err)
	}
	if result["localBooks"] != 1 || result["books"] != 1 || result["progress"] != 1 || result["bookmarks"] != 1 {
		t.Fatalf("portable restore result = %#v", result)
	}

	var restored models.Book
	if err := destination.db.Where("user_id = ? AND url = ?", destinationUser.ID, book.URL).First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.SourceID != 0 || restored.LibraryPath == "" || !strings.HasPrefix(restored.LibraryPath, filepath.Join("data", destinationUser.Username)+string(filepath.Separator)) {
		t.Fatalf("restored local book path = %+v", restored)
	}
	restoredData, err := os.ReadFile(filepath.Join(destination.cfg.LibraryDir, restored.OriginalFile))
	if err != nil {
		t.Fatal(err)
	}
	gotHash := sha256.Sum256(restoredData)
	if gotHash != wantHash {
		t.Fatalf("restored archive hash = %x, want %x", gotHash, wantHash)
	}
	var chapter models.Chapter
	if err := destination.db.Where("book_id = ?", restored.ID).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if chapter.CachePath == "" {
		t.Fatalf("portable restore did not rebuild a local chapter cache: %+v", chapter)
	}
	if _, err := os.Stat(filepath.Join(destination.cfg.LibraryDir, restored.LibraryPath, chapter.CachePath)); err != nil {
		t.Fatalf("restored local chapter cache is missing: %v", err)
	}
	var progress models.ReadingProgress
	if err := destination.db.Where("user_id = ? AND book_id = ?", destinationUser.ID, restored.ID).First(&progress).Error; err != nil || progress.Offset != 3 {
		t.Fatalf("restored progress = %+v, err=%v", progress, err)
	}
	var bookmark models.Bookmark
	if err := destination.db.Where("user_id = ? AND book_id = ?", destinationUser.ID, restored.ID).First(&bookmark).Error; err != nil || bookmark.Title != "可移植书签" {
		t.Fatalf("restored bookmark = %+v, err=%v", bookmark, err)
	}
}

func TestPortableBackupRejectsConflictingLocalIdentityBeforeMutation(t *testing.T) {
	_, source := setupTestServer(t)
	sourceUser := models.User{Username: "portable-conflict-source", PasswordHash: "hash"}
	if err := source.db.Create(&sourceUser).Error; err != nil {
		t.Fatal(err)
	}
	book, err := localbook.NewImporter(source.cfg, source.db).Import(localbook.ImportRequest{
		UserID: sourceUser.ID, UserName: sourceUser.Username, FileName: "source.txt", Extension: ".txt",
		Data: []byte("源 archive"), Title: "源书", Author: "作者",
	})
	if err != nil {
		t.Fatal(err)
	}
	portablePath, _, err := source.backupSvc.RunPortableForUser(sourceUser.ID, sourceUser.Username, filepath.Join(source.cfg.DataDir, "webdav", "users", sourceUser.Username))
	if err != nil {
		t.Fatal(err)
	}

	_, destination := setupTestServer(t)
	destinationUser := models.User{Username: "portable-conflict-destination", PasswordHash: "hash"}
	if err := destination.db.Create(&destinationUser).Error; err != nil {
		t.Fatal(err)
	}
	conflict, err := localbook.NewImporter(destination.cfg, destination.db).Import(localbook.ImportRequest{
		UserID: destinationUser.ID, UserName: destinationUser.Username, FileName: "different.txt", Extension: ".txt",
		Data: []byte("不同 archive"), Title: "目标已有书", Author: "作者",
	})
	if err != nil {
		t.Fatal(err)
	}
	if conflict.URL != book.URL {
		t.Fatalf("fixture must create matching local identity: source=%q target=%q", book.URL, conflict.URL)
	}
	conflictPath := filepath.Join(destination.cfg.LibraryDir, conflict.OriginalFile)
	before, err := os.ReadFile(conflictPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := destination.restorePortableBackupFile(portablePath, destinationUser.ID, destinationUser.Username); !errors.Is(err, errPortableBackupConflict) {
		t.Fatalf("portable conflict error = %v, want identity conflict", err)
	}
	after, err := os.ReadFile(conflictPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("conflicting archive changed: after=%q err=%v", after, err)
	}
	var count int64
	if err := destination.db.Model(&models.Book{}).Where("user_id = ?", destinationUser.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("conflict must not create/overwrite shelf rows: count=%d err=%v", count, err)
	}
}

func TestPortableBackupTriggerAndGenericUploadRestoreAPI(t *testing.T) {
	sourceRouter, source := setupTestServer(t)
	sourceToken := authHeader(t, sourceRouter)
	var sourceUser models.User
	if err := source.db.Where("username = ?", "testuser").First(&sourceUser).Error; err != nil {
		t.Fatal(err)
	}
	book, err := localbook.NewImporter(source.cfg, source.db).Import(localbook.ImportRequest{
		UserID: sourceUser.ID, UserName: sourceUser.Username, FileName: "api-portable.txt", Extension: ".txt",
		Data: []byte("API portable archive"), Title: "API 可移植书", Author: "API 作者",
	})
	if err != nil {
		t.Fatal(err)
	}
	trigger := httptest.NewRequest(http.MethodPost, "/api/backup/portable/trigger", nil)
	trigger.Header.Set("Authorization", sourceToken)
	triggerWriter := httptest.NewRecorder()
	sourceRouter.ServeHTTP(triggerWriter, trigger)
	if triggerWriter.Code != http.StatusOK {
		t.Fatalf("portable trigger: expected 200, got %d: %s", triggerWriter.Code, triggerWriter.Body.String())
	}
	var triggered struct {
		Name       string `json:"name"`
		Format     string `json:"format"`
		LocalBooks int    `json:"localBooks"`
	}
	if err := json.Unmarshal(triggerWriter.Body.Bytes(), &triggered); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(triggered.Name, "portable_backup_") || triggered.Format != "openreader-portable-v2" || triggered.LocalBooks != 1 {
		t.Fatalf("portable trigger payload = %+v", triggered)
	}
	list := httptest.NewRequest(http.MethodGet, "/api/backup/list", nil)
	list.Header.Set("Authorization", sourceToken)
	listWriter := httptest.NewRecorder()
	sourceRouter.ServeHTTP(listWriter, list)
	if listWriter.Code != http.StatusOK || !strings.Contains(listWriter.Body.String(), triggered.Name) || !strings.Contains(listWriter.Body.String(), "openreader-portable-v2") {
		t.Fatalf("portable backup list: %d %s", listWriter.Code, listWriter.Body.String())
	}
	download := httptest.NewRequest(http.MethodGet, "/api/backup/download/"+triggered.Name, nil)
	download.Header.Set("Authorization", sourceToken)
	downloadWriter := httptest.NewRecorder()
	sourceRouter.ServeHTTP(downloadWriter, download)
	if downloadWriter.Code != http.StatusOK || downloadWriter.Body.Len() == 0 {
		t.Fatalf("portable backup download: %d %s", downloadWriter.Code, downloadWriter.Body.String())
	}

	destinationRouter, destination := setupTestServer(t)
	destinationToken := authHeader(t, destinationRouter)
	upload := multipartBackupRestoreRequest(t, triggered.Name, downloadWriter.Body.Bytes())
	upload.Header.Set("Authorization", destinationToken)
	uploadWriter := httptest.NewRecorder()
	destinationRouter.ServeHTTP(uploadWriter, upload)
	if uploadWriter.Code != http.StatusOK || !strings.Contains(uploadWriter.Body.String(), `"localBooks":1`) {
		t.Fatalf("generic portable upload restore: %d %s", uploadWriter.Code, uploadWriter.Body.String())
	}
	var destinationUser models.User
	if err := destination.db.Where("username = ?", "testuser").First(&destinationUser).Error; err != nil {
		t.Fatal(err)
	}
	var restored models.Book
	if err := destination.db.Where("user_id = ? AND url = ?", destinationUser.ID, book.URL).First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination.cfg.LibraryDir, restored.OriginalFile)); err != nil {
		t.Fatalf("generic upload restore did not promote archive: %v", err)
	}
}

func TestPortableBackupRejectsBadManifestBeforeShelfMutation(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "portable-invalid-manifest", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	manifest := `{"format":"openreader-portable-backup","version":1,"books":[{"bookUrl":"local://book_1","title":"损坏校验","extension":".txt","entry":"local-books/b0001/original.txt","size":5,"sha256":"0000000000000000000000000000000000000000000000000000000000000000"}]}`
	archive := makeBackupRestoreZIP(t, map[string]string{
		"openreader-portable-v1.json":    manifest,
		"bookshelf.json":                 `[{"title":"损坏校验","url":"local://book_1","sourceId":0}]`,
		"local-books/b0001/original.txt": "wrong",
	})
	path := filepath.Join(t.TempDir(), "invalid-portable.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := server.restorePortableBackupFile(path, user.ID, user.Username); !errors.Is(err, errInvalidPortableBackup) {
		t.Fatalf("bad portable manifest error = %v", err)
	}
	var books int64
	if err := server.db.Model(&models.Book{}).Where("user_id = ?", user.ID).Count(&books).Error; err != nil || books != 0 {
		t.Fatalf("bad portable manifest must not create shelf rows: count=%d err=%v", books, err)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LibraryDir, "data", user.Username)); !os.IsNotExist(err) {
		t.Fatalf("bad portable manifest must not create a library archive root: %v", err)
	}
}

func TestPortableBackupUsesItsOwnArchiveBudgetDuringRestore(t *testing.T) {
	_, source := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxImportBytes = 8
	})
	sourceUser := models.User{Username: "portable-budget-source", PasswordHash: "hash"}
	if err := source.db.Create(&sourceUser).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: sourceUser.ID, SourceID: 0, URL: "local://portable-budget", Title: "Portable budget",
		Author: "OpenReader", LibraryPath: filepath.Join("data", sourceUser.Username, "portable-budget"),
		OriginalFile: filepath.Join("data", sourceUser.Username, "portable-budget", "budget.txt"),
	}
	if err := source.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(source.cfg.LibraryDir, book.OriginalFile)
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o700); err != nil {
		t.Fatal(err)
	}
	archiveData := []byte("第一章\nportable archive is larger than the ordinary upload cap")
	if err := os.WriteFile(archivePath, archiveData, 0o600); err != nil {
		t.Fatal(err)
	}
	portablePath, _, err := source.backupSvc.RunPortableForUser(sourceUser.ID, sourceUser.Username, filepath.Join(source.cfg.DataDir, "webdav", "users", sourceUser.Username))
	if err != nil {
		t.Fatal(err)
	}

	_, destination := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxImportBytes = 8
	})
	destinationUser := models.User{Username: "portable-budget-destination", PasswordHash: "hash"}
	if err := destination.db.Create(&destinationUser).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := destination.restorePortableBackupFile(portablePath, destinationUser.ID, destinationUser.Username); err != nil {
		t.Fatalf("portable restore must use its independently bounded archive cap: %v", err)
	}
	var restored models.Book
	if err := destination.db.Where("user_id = ? AND url = ?", destinationUser.ID, book.URL).First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination.cfg.LibraryDir, restored.OriginalFile))
	if err != nil || string(got) != string(archiveData) {
		t.Fatalf("portable archive restored with wrong bytes: %q, err=%v", got, err)
	}
}
