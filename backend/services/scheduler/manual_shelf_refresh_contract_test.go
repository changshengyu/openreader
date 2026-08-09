package scheduler

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/models"
)

func newManualShelfRefreshDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := readerdb.Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	return database
}

func createManualShelfRefreshSource(t *testing.T, database *gorm.DB, userID uint, rules models.BookSourceRule) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: "手动刷新合同源", BaseURL: "https://manual-refresh.test", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(rules); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.BookSourceNamespace{UserID: userID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.UserBookSource{UserID: userID, SourceID: source.ID}).Error; err != nil {
		t.Fatal(err)
	}
	return source
}

func manualShelfRefreshHTML(chapters ...string) string {
	var body strings.Builder
	body.WriteString("<html><body>")
	for index, title := range chapters {
		fmt.Fprintf(&body, `<li class="chapter"><a href="/chapter/%d">%s</a></li>`, index, title)
	}
	body.WriteString("</body></html>")
	return body.String()
}

func TestManualShelfRefreshReplacesChangedAndShorterCatalogue(t *testing.T) {
	database := newManualShelfRefreshDatabase(t)
	source := createManualShelfRefreshSource(t, database, 1, models.BookSourceRule{
		ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href",
	})
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: schedulerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(manualShelfRefreshHTML("新第一章", "新第二章"))),
			Header:     make(http.Header), Request: request,
		}, nil
	})})
	defer restore()

	oldCheck := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	book := models.Book{UserID: 1, SourceID: source.ID, Title: "目录缩短", URL: "https://manual-refresh.test/book", LastChapter: "旧第三章", ChapterCount: 3, LastCheckTime: oldCheck, CanUpdate: true}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "旧第一章", URL: "https://manual-refresh.test/old/0"},
		{BookID: book.ID, Index: 1, Title: "旧第二章", URL: "https://manual-refresh.test/old/1"},
		{BookID: book.ID, Index: 2, Title: "旧第三章", URL: "https://manual-refresh.test/old/2"},
	}
	if err := database.Create(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{UserID: 1, BookID: book.ID, ChapterID: chapters[2].ID, ChapterIndex: 2, Offset: 17}
	bookmark := models.Bookmark{UserID: 1, BookID: book.ID, ChapterID: chapters[2].ID, ChapterIndex: 2, Offset: 9, Title: "旧末章书签"}
	if err := database.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}

	service := New(database, time.Hour)
	added, err := service.checkBook(book)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Fatalf("shorter catalogue reported %d new chapters", added)
	}
	var refreshed models.Book
	if err := database.First(&refreshed, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.ChapterCount != 2 || refreshed.LastChapter != "新第二章" || refreshed.LastCheckTime != oldCheck {
		t.Fatalf("book summary did not follow the authoritative shorter catalogue: %+v", refreshed)
	}
	var current []models.Chapter
	if err := database.Where("book_id = ?", book.ID).Order("`index` asc").Find(&current).Error; err != nil {
		t.Fatal(err)
	}
	if len(current) != 2 || current[0].Title != "新第一章" || current[1].URL != "https://manual-refresh.test/chapter/1" {
		t.Fatalf("catalogue was not replaced: %+v", current)
	}
	if err := database.First(&progress, progress.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&bookmark, bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterID != 0 || progress.ChapterIndex != 2 || bookmark.ChapterID != 0 || bookmark.ChapterIndex != 2 {
		t.Fatalf("out-of-range references were not made recoverable: progress=%+v bookmark=%+v", progress, bookmark)
	}
}

func TestManualShelfRefreshAppendsWithoutReplacingExistingChapterIdentity(t *testing.T) {
	database := newManualShelfRefreshDatabase(t)
	source := createManualShelfRefreshSource(t, database, 1, models.BookSourceRule{
		ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href",
	})
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: schedulerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(manualShelfRefreshHTML("第一章", "第二章", "第三章"))),
			Header:     make(http.Header), Request: request,
		}, nil
	})})
	defer restore()

	oldCheck := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	book := models.Book{UserID: 1, SourceID: source.ID, Title: "只追加目录", URL: "https://manual-refresh.test/book", LastChapter: "第二章", ChapterCount: 2, LastCheckTime: oldCheck, CanUpdate: true}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://manual-refresh.test/chapter/0", CachePath: "manual-refresh/one.txt"},
		{BookID: book.ID, Index: 1, Title: "第二章", URL: "https://manual-refresh.test/chapter/1", CachePath: "manual-refresh/two.txt"},
	}
	if err := database.Create(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{UserID: 1, BookID: book.ID, ChapterID: chapters[1].ID, ChapterIndex: 1, Offset: 23}
	bookmark := models.Bookmark{UserID: 1, BookID: book.ID, ChapterID: chapters[1].ID, ChapterIndex: 1, Offset: 11, Title: "第二章书签"}
	if err := database.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}

	service := New(database, time.Hour)
	result, err := service.CheckNowForUserDetailed(1)
	if err != nil {
		t.Fatal(err)
	}
	if result.NewChapters != 1 || result.Updated != 1 || len(result.UpdatedBookIDs) != 1 || len(result.ReplacedBookIDs) != 0 || len(result.SupersededCachePaths) != 0 {
		t.Fatalf("append result = %+v", result)
	}
	var current []models.Chapter
	if err := database.Where("book_id = ?", book.ID).Order("`index` asc").Find(&current).Error; err != nil {
		t.Fatal(err)
	}
	if len(current) != 3 || current[0].ID != chapters[0].ID || current[1].ID != chapters[1].ID {
		t.Fatalf("append replaced existing chapter identity: before=%+v after=%+v", chapters, current)
	}
	if current[0].CachePath != "manual-refresh/one.txt" || current[1].CachePath != "manual-refresh/two.txt" {
		t.Fatalf("append discarded existing cache paths: %+v", current)
	}
	if err := database.First(&progress, progress.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&bookmark, bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterID != chapters[1].ID || bookmark.ChapterID != chapters[1].ID {
		t.Fatalf("append rewrote stable references: progress=%+v bookmark=%+v", progress, bookmark)
	}
}

func TestManualShelfRefreshRollsBackPartialCatalogueInsert(t *testing.T) {
	database := newManualShelfRefreshDatabase(t)
	source := createManualShelfRefreshSource(t, database, 1, models.BookSourceRule{
		ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href",
	})
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: schedulerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(manualShelfRefreshHTML("第一章", "第二章", "第三章"))), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	book := models.Book{UserID: 1, SourceID: source.ID, Title: "事务失败", URL: "https://manual-refresh.test/tx", LastChapter: "第一章", ChapterCount: 1, CanUpdate: true}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://manual-refresh.test/chapter/0"}).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`CREATE TRIGGER fail_manual_refresh_insert BEFORE INSERT ON chapters WHEN NEW.book_id = %d AND NEW."index" = 2 BEGIN SELECT RAISE(ABORT, 'forced manual refresh failure'); END`, book.ID)
	if err := database.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	service := New(database, time.Hour)
	if _, err := service.checkBook(book); err == nil {
		t.Fatal("expected the injected chapter insert failure")
	}
	var current []models.Chapter
	if err := database.Where("book_id = ?", book.ID).Order("`index` asc").Find(&current).Error; err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].Index != 0 || current[0].Title != "第一章" {
		t.Fatalf("failed refresh left a partial catalogue: %+v", current)
	}
	var unchanged models.Book
	if err := database.First(&unchanged, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.ChapterCount != 1 || unchanged.LastChapter != "第一章" {
		t.Fatalf("failed refresh changed the book summary: %+v", unchanged)
	}
}

func TestManualShelfRefreshUsesPersistedBookVariablesWithoutChangingBookInfo(t *testing.T) {
	database := newManualShelfRefreshDatabase(t)
	source := createManualShelfRefreshSource(t, database, 1, models.BookSourceRule{
		TOCURLRule:      "@get:{tocPath}",
		ChapterListRule: ".chapter",
		ChapterNameRule: `@put:{"chapterPath":".path|text"}.name|text`,
		ChapterURLRule:  "@get:{chapterPath}",
	})
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: schedulerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `<h1>不得覆盖的远端书名</h1>`
		if request.URL.Path == "/toc" {
			body = `<div class="chapter"><span class="path">/content/one</span><a class="name">变量章节</a></div>`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	book := models.Book{UserID: 1, SourceID: source.ID, Title: "保留书名", Author: "保留作者", CoverURL: "keep-cover", URL: "https://manual-refresh.test/book", Variable: `{"tocPath":"/toc"}`, ChapterCount: 0, CanUpdate: true}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	service := New(database, time.Hour)
	if _, err := service.checkBook(book); err != nil {
		t.Fatal(err)
	}
	var refreshed models.Book
	if err := database.First(&refreshed, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.Title != "保留书名" || refreshed.Author != "保留作者" || refreshed.CoverURL != "keep-cover" || refreshed.Variable != `{"tocPath":"/toc"}` {
		t.Fatalf("TOC-only refresh changed BookInfo or lost variables: %+v", refreshed)
	}
	var chapter models.Chapter
	if err := database.Where("book_id = ?", book.ID).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if chapter.Title != "变量章节" || chapter.URL != "https://manual-refresh.test/content/one" || !strings.Contains(chapter.Variable, "chapterPath") {
		t.Fatalf("persisted variables did not reach the refreshed chapter: %+v", chapter)
	}
}

func TestManualShelfRefreshFetchesConcurrentlyWithUpstreamBound(t *testing.T) {
	database := newManualShelfRefreshDatabase(t)
	source := createManualShelfRefreshSource(t, database, 1, models.BookSourceRule{
		ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href",
	})
	var active int32
	var maximum int32
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: schedulerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maximum)
			if current <= observed || atomic.CompareAndSwapInt32(&maximum, observed, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(manualShelfRefreshHTML("第一章"))), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	for index := 0; index < 20; index++ {
		book := models.Book{UserID: 1, SourceID: source.ID, Title: fmt.Sprintf("并发书 %d", index), URL: fmt.Sprintf("https://manual-refresh.test/book/%d", index), CanUpdate: true}
		if err := database.Create(&book).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := New(database, time.Hour)
	service.CheckNowForUser(1)
	if maximum <= 1 || maximum > 16 {
		t.Fatalf("remote fetch concurrency = %d, want 2..16", maximum)
	}
}

func TestManualShelfRefreshRejectsAStaleBookSnapshot(t *testing.T) {
	database := newManualShelfRefreshDatabase(t)
	source := createManualShelfRefreshSource(t, database, 1, models.BookSourceRule{
		ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href",
	})
	started := make(chan struct{})
	release := make(chan struct{})
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: schedulerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(manualShelfRefreshHTML("第一章", "第二章"))), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	book := models.Book{UserID: 1, SourceID: source.ID, Title: "陈旧抓取", URL: "https://manual-refresh.test/old", LastChapter: "第一章", ChapterCount: 1, CanUpdate: true}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://manual-refresh.test/chapter/0"}).Error; err != nil {
		t.Fatal(err)
	}
	service := New(database, time.Hour)
	result := make(chan error, 1)
	go func() {
		_, err := service.checkBook(book)
		result <- err
	}()
	<-started
	if err := database.Model(&models.Book{}).Where("id = ?", book.ID).Updates(map[string]any{
		"url": "https://manual-refresh.test/new", "can_update": false,
	}).Error; err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; err == nil {
		t.Fatal("stale fetched result was accepted")
	}
	var current models.Book
	if err := database.First(&current, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.URL != "https://manual-refresh.test/new" || current.CanUpdate || current.ChapterCount != 1 {
		t.Fatalf("stale refresh overwrote the newer book state: %+v", current)
	}
	var count int64
	if err := database.Model(&models.Chapter{}).Where("book_id = ?", book.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stale refresh appended chapters: %d", count)
	}
}
