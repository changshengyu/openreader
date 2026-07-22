package scheduler

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/models"
)

type schedulerRoundTripFunc func(*http.Request) (*http.Response, error)

func (function schedulerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestSchedulerAdvancesLastCheckTimeOnlyWhenItAddsChapters(t *testing.T) {
	database, err := readerdb.Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	body := `{"chapters":[{"title":"第一章","url":"/1"},{"title":"第二章","url":"/2"}]}`
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: schedulerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "定时更新时间源", BaseURL: "https://scheduler-last-check.test", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		ChapterListRule: "$.chapters[*]",
		ChapterNameRule: "$.title",
		ChapterURLRule:  "$.url",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	oldCheck := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	book := models.Book{
		UserID: 1, SourceID: source.ID, Title: "定时更新时间", URL: "https://scheduler-last-check.test/toc",
		LastChapter: "第一章", ChapterCount: 1, LastCheckTime: oldCheck, CanUpdate: true,
	}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://scheduler-last-check.test/1"}).Error; err != nil {
		t.Fatal(err)
	}

	service := New(database, time.Hour)
	count, err := service.checkBook(book)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("new chapter count = %d, want 1", count)
	}
	var grown models.Book
	if err := database.First(&grown, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grown.LastCheckTime <= oldCheck {
		t.Fatalf("scheduler did not advance lastCheckTime: %+v", grown)
	}

	firstCheck := grown.LastCheckTime
	count, err = service.checkBook(grown)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("same catalogue reported %d new chapters", count)
	}
	var unchanged models.Book
	if err := database.First(&unchanged, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.LastCheckTime != firstCheck {
		t.Fatalf("no-growth scheduler check changed lastCheckTime: before=%d after=%d", firstCheck, unchanged.LastCheckTime)
	}

	if err := database.Where("book_id = ? AND `index` = ?", book.ID, 1).Delete(&models.Chapter{}).Error; err != nil {
		t.Fatal(err)
	}
	count, err = service.checkBook(unchanged)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("catalogue repair added %d chapters, want 1", count)
	}
	var repaired models.Book
	if err := database.First(&repaired, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if repaired.LastCheckTime != firstCheck {
		t.Fatalf("repair below the persisted chapter count changed lastCheckTime: before=%d after=%d", firstCheck, repaired.LastCheckTime)
	}
}
