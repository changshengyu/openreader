package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestShelfLastCheckTimeIsIndependentFromReadingOrderAndOnlyAdvancesForNewChapters(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	chapterCount := 2
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			chapters := make([]string, 0, chapterCount)
			for index := 0; index < chapterCount; index++ {
				chapters = append(chapters, fmt.Sprintf(`{"title":"第%d章","url":"/chapter/%d"}`, index+1, index+1))
			}
			body := fmt.Sprintf(`{"book":{"name":"更新时间合同","chapters":[%s]}}`, strings.Join(chapters, ","))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "更新时间源", BaseURL: "https://last-check.test", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoInitRule: "$.book",
		BookInfoNameRule: "$.name",
		ChapterListRule:  "$.book.chapters[*]",
		ChapterNameRule:  "$.title",
		ChapterURLRule:   "$.url",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	oldCheck := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC).UnixMilli()
	book := models.Book{
		UserID: 1, SourceID: source.ID, Title: "更新时间合同", URL: "https://last-check.test/book",
		LastChapter: "第1章", ChapterCount: 1, LastCheckTime: oldCheck, CanUpdate: true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第1章", URL: "https://last-check.test/chapter/1"}).Error; err != nil {
		t.Fatal(err)
	}

	refresh := func() {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh", nil)
		request.Header.Set("Authorization", token)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, request)
		if writer.Code != http.StatusOK {
			t.Fatalf("refresh: code=%d body=%s", writer.Code, writer.Body.String())
		}
	}

	refresh()
	var grown models.Book
	if err := server.db.First(&grown, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grown.LastCheckTime <= oldCheck {
		t.Fatalf("new chapters did not advance lastCheckTime: old=%d book=%+v", oldCheck, grown)
	}

	firstCheck := grown.LastCheckTime
	refresh()
	var unchanged models.Book
	if err := server.db.First(&unchanged, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.LastCheckTime != firstCheck {
		t.Fatalf("same-size refresh changed lastCheckTime: before=%d after=%d", firstCheck, unchanged.LastCheckTime)
	}
	unchanged.CreatedAt = time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	unchanged.UpdatedAt = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if orderAt := shelfOrderAt(unchanged, nil); !orderAt.Equal(unchanged.CreatedAt) {
		t.Fatalf("metadata updatedAt changed upstream reading order: got=%s created=%s updated=%s", orderAt, unchanged.CreatedAt, unchanged.UpdatedAt)
	}

	progress := models.ReadingProgress{
		UserID: 1, BookID: book.ID, ChapterIndex: 1, ChapterTitle: "第2章",
		UpdatedAt: time.Now().Add(time.Hour),
	}
	if err := server.db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	item := server.bookShelfListItem(1, unchanged)
	if item.LastCheckTime != firstCheck {
		t.Fatalf("reading order changed the visible book update time: %+v", item)
	}
	if item.Progress == nil || !item.ShelfOrderAt.Equal(progress.UpdatedAt) {
		t.Fatalf("last-reading shelf order was not preserved: %+v", item)
	}
}

func TestReaderDevRestorePreservesShelfLastCheckTime(t *testing.T) {
	_, server := setupTestServer(t)
	want := time.Date(2024, time.December, 3, 2, 1, 0, 0, time.UTC).UnixMilli()
	data := []byte(fmt.Sprintf(`[{"name":"恢复更新时间","bookUrl":"local://last-check","lastChapter":"终章","chapterCount":3,"lastCheckTime":%d}]`, want))
	books, _, err := server.restoreBookshelfFromData(data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if books != 1 {
		t.Fatalf("restored books = %d, want 1", books)
	}
	var restored models.Book
	if err := server.db.Where("user_id = ? AND title = ?", 1, "恢复更新时间").First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.LastCheckTime != want {
		t.Fatalf("restored lastCheckTime = %d, want %d", restored.LastCheckTime, want)
	}
}
