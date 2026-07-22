package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestFixedBaselineBackupExportsUpstreamAliases(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "fixed-backup-user", PasswordHash: "hash", CanEditSources: true, CanAccessStore: true}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "固定基线书源", BaseURL: "https://fixed-backup.example", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{SearchURL: "/search?q={{key}}", BookListRule: ".book", BookNameRule: "h2", ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: user.ID, SourceID: source.ID, Title: "固定基线书", Author: "作者",
		URL: source.BaseURL + "/book/1", LastChapter: "第十章", ChapterCount: 10,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, time.July, 22, 9, 30, 0, 0, time.UTC)
	if err := server.db.Create(&models.Bookmark{
		UserID: user.ID, BookID: book.ID, ChapterIndex: 2, Offset: 18,
		Title: "第三章", Excerpt: "段落正文", Note: "书签备注", CreatedAt: createdAt, UpdatedAt: createdAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReplaceRule{
		UserID: user.ID, Name: "固定规则", Pattern: "旧", Replacement: "新", Scope: "*", Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	path, err := server.backupSvc.RunNowForUser(user.ID, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	entries := readFixedBaselineBackupEntries(t, path)
	for _, name := range []string{"bookSource.json", "bookshelf.json", "bookmark.json", "bookmarks.json", "replaceRule.json", "replaceRules.json"} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("generated backup missing %s; entries=%v", name, fixedBaselineEntryNames(entries))
		}
	}
	if !strings.Contains(string(entries["bookSource.json"]), `"bookSourceName"`) || strings.Contains(string(entries["bookSource.json"]), `"baseUrl"`) {
		t.Fatalf("bookSource.json must use the upstream source encoder: %s", entries["bookSource.json"])
	}
	bookshelf := string(entries["bookshelf.json"])
	for _, field := range []string{`"name"`, `"bookUrl"`, `"originName"`, `"latestChapterTitle"`, `"totalChapterNum"`, `"durChapterIndex"`} {
		if !strings.Contains(bookshelf, field) {
			t.Fatalf("bookshelf.json missing upstream field %s: %s", field, bookshelf)
		}
	}
	bookmark := string(entries["bookmark.json"])
	for _, field := range []string{`"time"`, `"bookName"`, `"bookAuthor"`, `"chapterPos"`, `"bookText"`, `"content"`} {
		if !strings.Contains(bookmark, field) {
			t.Fatalf("bookmark.json missing upstream field %s: %s", field, bookmark)
		}
	}
}

func TestFixedBaselineRestoreAcceptsSingularBookmarkAndReplaceRule(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "fixed-restore-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "上游书", Author: "上游作者", URL: "https://fixed-restore.example/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 4, Title: "第五章"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookmark.json": `[{
			"time":1784696400000,"bookName":"上游书","bookAuthor":"上游作者",
			"chapterIndex":4,"chapterPos":26,"chapterName":"第五章","bookText":"当前段落","content":"备注"
		}]`,
		"replaceRule.json": `[{
			"id":1784696400000,"name":"上游规则","group":"净化","pattern":"旧词",
			"replacement":"新词","scope":"*","isEnabled":true,"isRegex":false,"order":7
		}]`,
	})
	result, err := server.restoreLegadoBackupData(archive, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result["bookmarks"] != 1 || result["replaceRules"] != 1 {
		t.Fatalf("singular upstream artifacts were not restored: %#v", result)
	}
	var bookmark models.Bookmark
	if err := server.db.Where("user_id = ?", user.ID).First(&bookmark).Error; err != nil {
		t.Fatal(err)
	}
	if bookmark.BookID != book.ID || bookmark.ChapterID != chapter.ID || bookmark.Offset != 26 || bookmark.Excerpt != "当前段落" || bookmark.Note != "备注" {
		t.Fatalf("unexpected upstream bookmark mapping: %+v", bookmark)
	}
	var rule models.ReplaceRule
	if err := server.db.Where("user_id = ?", user.ID).First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if rule.Name != "上游规则" || rule.Group != "净化" || rule.Order != 7 || rule.Pattern != "旧词" || !rule.Enabled {
		t.Fatalf("unexpected upstream replacement mapping: %+v", rule)
	}
}

func TestFixedBaselineRestoreDeduplicatesGeneratedAliases(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "alias-restore-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "别名书", URL: "https://alias.example/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	created := "2026-07-22T09:30:00Z"
	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookmark.json":     `[{"time":1784696400000,"bookName":"别名书","chapterIndex":0,"chapterPos":2,"bookText":"旧格式"}]`,
		"bookmarks.json":    `[{"bookTitle":"别名书","bookUrl":"https://alias.example/book","chapterIndex":0,"offset":2,"excerpt":"丰富格式","createdAt":"` + created + `","updatedAt":"` + created + `"}]`,
		"replaceRule.json":  `[{"name":"同一规则","pattern":"旧","replacement":"新","scope":"*","isEnabled":true,"isRegex":false,"order":0}]`,
		"replaceRules.json": `[{"name":"同一规则","pattern":"旧","replacement":"新","scope":"*","enabled":true,"isRegex":false,"createdAt":"` + created + `","updatedAt":"` + created + `"}]`,
	})
	result, err := server.restoreLegadoBackupData(archive, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result["bookmarks"] != 1 || result["replaceRules"] != 1 {
		t.Fatalf("generated aliases must execute once: %#v", result)
	}
	var bookmarkCount, ruleCount int64
	if err := server.db.Model(&models.Bookmark{}).Where("user_id = ?", user.ID).Count(&bookmarkCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&models.ReplaceRule{}).Where("user_id = ?", user.ID).Count(&ruleCount).Error; err != nil {
		t.Fatal(err)
	}
	if bookmarkCount != 1 || ruleCount != 1 {
		t.Fatalf("alias restore duplicated rows: bookmarks=%d rules=%d", bookmarkCount, ruleCount)
	}
}

func TestFixedBaselineRestoreNeverReusesArchiveSourceID(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "source-id-restore-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	foreign := models.BookSource{Name: "无关书源", BaseURL: "https://unrelated-source.example", Enabled: true}
	if err := server.db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookshelf.json": `[{"name":"不能串源的书","bookUrl":"https://archive-book.example/book","sourceId":` + fmt.Sprint(foreign.ID) + `}]`,
	})
	if _, err := server.restoreLegadoBackupData(archive, user.ID); err != nil {
		t.Fatal(err)
	}
	var book models.Book
	if err := server.db.Where("user_id = ? AND title = ?", user.ID, "不能串源的书").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.SourceID != 0 {
		t.Fatalf("archive-local source id was reused against this database: %d", book.SourceID)
	}
}

func TestFixedBaselineRestoreSkipsGlobalSourcesWithoutEditPermission(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth := registerStorageTestUser(t, router, "restoremember")
	if err := server.db.Model(&models.User{}).Where("username = ?", "restoremember").Updates(map[string]any{
		"can_access_webdav": true,
		"can_edit_sources":  false,
	}).Error; err != nil {
		t.Fatal(err)
	}
	var user models.User
	if err := server.db.Where("username = ?", "restoremember").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookSource.json":   `[{"bookSourceName":"forbidden-source","bookSourceUrl":"https://forbidden.example","enabled":true}]`,
		"userSettings.json": `[{"key":"shelf","value":"{\"view\":\"list\"}"}]`,
	})
	req := multipartBackupRestoreRequest(t, "backup.zip", archive)
	req.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("personal restore should continue: %d %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["sourcesSkipped"] != true || result["sources"] != float64(0) || result["settings"] != float64(1) {
		t.Fatalf("restore must report source permission skip and personal success: %#v", result)
	}
	var sources int64
	if err := server.db.Model(&models.BookSource{}).Where("name = ?", "forbidden-source").Count(&sources).Error; err != nil {
		t.Fatal(err)
	}
	if sources != 0 {
		t.Fatalf("source-edit permission was bypassed: %d", sources)
	}
	var settings int64
	if err := server.db.Model(&models.UserSetting{}).Where("user_id = ? AND key = ?", user.ID, "shelf").Count(&settings).Error; err != nil {
		t.Fatal(err)
	}
	if settings != 1 {
		t.Fatalf("personal setting was not restored: %d", settings)
	}
}

func TestFixedBaselineRestoreContentFailureRollsBackEarlierArtifacts(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "rollback-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"categories.json": `[ {"name":"must-rollback","sortOrder":1} ]`,
		"bookmarks.json":  `{"not":"an array"}`,
	})
	if _, err := server.restoreLegadoBackupData(archive, user.ID); !errors.Is(err, errInvalidBackupArchive) {
		t.Fatalf("malformed supported artifact error = %v, want invalid backup", err)
	}
	var count int64
	if err := server.db.Model(&models.Category{}).Where("user_id = ? AND name = ?", user.ID, "must-rollback").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("earlier category survived failed logical restore: %d", count)
	}
}

func TestFixedBaselineRestoreFieldTypeFailurePreventsMutation(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "decode-rollback-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"categories.json": `[ {"name":"must-not-start","sortOrder":1} ]`,
		"bookmark.json":   `[{"bookName":"坏字段书签","chapterIndex":"not-a-number"}]`,
	})
	if _, err := server.restoreLegadoBackupData(archive, user.ID); !errors.Is(err, errInvalidBackupArchive) {
		t.Fatalf("wrong field type error = %v, want invalid backup", err)
	}
	var count int64
	if err := server.db.Model(&models.Category{}).Where("user_id = ? AND name = ?", user.ID, "must-not-start").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("decode failure started the restore transaction: %d", count)
	}
}

func TestFixedBaselineRestoreDatabaseFailureRollsBackEarlierArtifacts(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "database-rollback-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	callbackName := "test:fixed-restore-write-failure"
	if err := server.db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "replace_rules" {
			tx.AddError(errors.New("injected replace-rule write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.db.Callback().Create().Remove(callbackName) })

	archive := makeBackupRestoreZIP(t, map[string]string{
		"categories.json":  `[{"name":"must-rollback-after-write-error","sortOrder":1}]`,
		"replaceRule.json": `[{"name":"写失败规则","pattern":"旧","replacement":"新","isEnabled":true}]`,
	})
	if _, err := server.restoreLegadoBackupData(archive, user.ID); !errors.Is(err, errBackupRestorePersistence) {
		t.Fatalf("database restore error = %v, want persistence failure", err)
	}
	var count int64
	if err := server.db.Model(&models.Category{}).Where("user_id = ? AND name = ?", user.ID, "must-rollback-after-write-error").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("earlier category survived database rollback: %d", count)
	}
}

func TestFixedBaselineRestoreKeepsNewestReadingProgress(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "progress-merge-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "进度合并书", URL: "https://progress-merge.example/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	currentTime := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	current := models.ReadingProgress{
		UserID: user.ID, BookID: book.ID, ChapterIndex: 9, Offset: 90,
		ChapterTitle: "第十章", Mode: "scroll", UpdatedAt: currentTime,
	}
	if err := server.db.Create(&current).Error; err != nil {
		t.Fatal(err)
	}

	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookshelf.json":       `[{"name":"进度合并书","bookUrl":"https://progress-merge.example/book","durChapterIndex":2,"durChapterPos":20,"durChapterTitle":"第三章","durChapterTime":1784692800000}]`,
		"readingProgress.json": `[{"bookTitle":"进度合并书","bookUrl":"https://progress-merge.example/book","chapterIndex":12,"offset":120,"chapterTitle":"第十三章","updatedAt":"2026-07-22T13:00:00Z"}]`,
	})
	result, err := server.restoreLegadoBackupData(archive, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result["progress"] != 1 {
		t.Fatalf("only the newer progress artifact should be applied: %#v", result)
	}
	var restored models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.ChapterIndex != 12 || restored.Offset != 120 || restored.ChapterTitle != "第十三章" {
		t.Fatalf("progress merge regressed or ignored the newer artifact: %+v", restored)
	}
	if !restored.UpdatedAt.Equal(time.Date(2026, time.July, 22, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("restored progress timestamp was not preserved: %s", restored.UpdatedAt)
	}
}

func TestFixedBaselineBackupQueryFailureLeavesNoVisibleArchive(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "failed-backup-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Callback().Query().Before("gorm:query").Register("test:fixed-backup-query-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "book_sources" {
			tx.AddError(errors.New("injected source query failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.backupSvc.RunNowForUser(user.ID, user.Username); err == nil {
		t.Fatal("backup query failure must propagate")
	}
	root := filepath.Join(server.cfg.DataDir, "webdav", "users", user.Username)
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "backup_") || strings.HasPrefix(entry.Name(), ".backup-") {
			t.Fatalf("failed generation left a visible/temporary archive: %s", entry.Name())
		}
	}
}

func TestExplicitUserSettingForceOverwritesOnlyAfterConfirmationIntent(t *testing.T) {
	router, _ := setupTestServer(t)
	auth := authHeader(t, router)
	first := putSettingContract(t, router, auth, `{"value":{"marker":"first"},"baseUpdatedAt":""}`)
	firstUpdatedAt, _ := first["updatedAt"].(string)
	second := putSettingContract(t, router, auth, `{"value":{"marker":"second"},"baseUpdatedAt":"`+firstUpdatedAt+`"}`)
	secondUpdatedAt, _ := second["updatedAt"].(string)

	staleReq := httptest.NewRequest(http.MethodPut, "/api/settings/search", strings.NewReader(`{"value":{"marker":"stale"},"baseUpdatedAt":"`+firstUpdatedAt+`"}`))
	staleReq.Header.Set("Authorization", auth)
	staleReq.Header.Set("Content-Type", "application/json")
	staleResponse := httptest.NewRecorder()
	router.ServeHTTP(staleResponse, staleReq)
	if staleResponse.Header().Get("X-OpenReader-Setting-Conflict") != "1" {
		t.Fatalf("ordinary stale write must retain CAS: %d %s", staleResponse.Code, staleResponse.Body.String())
	}

	forced := putSettingContract(t, router, auth, `{"value":{"marker":"forced"},"baseUpdatedAt":"`+firstUpdatedAt+`","force":true}`)
	value, _ := forced["value"].(map[string]any)
	if value["marker"] != "forced" || forced["updatedAt"] == secondUpdatedAt {
		t.Fatalf("confirmed force save did not persist the current terminal: %#v", forced)
	}
}

func putSettingContract(t *testing.T, handler http.Handler, auth string, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/settings/search", strings.NewReader(body))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("save setting: %d %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func readFixedBaselineBackupEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entries := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		entries[file.Name] = data
	}
	return entries
}

func fixedBaselineEntryNames(entries map[string][]byte) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	return names
}
