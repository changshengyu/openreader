package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestRemoteBookInfoTOCCancellationReachesUpstreamAndHasNoBusinessSideEffects(t *testing.T) {
	for _, fixture := range []struct {
		name        string
		username    string
		requestPath func(remoteBookLifecycleFixture) string
		requestBody func(remoteBookLifecycleFixture) string
	}{
		{
			name:     "temporary session creation",
			username: "cancelremote1",
			requestPath: func(remoteBookLifecycleFixture) string {
				return "/api/reader/remote-sessions"
			},
			requestBody: func(fixture remoteBookLifecycleFixture) string {
				return `{"title":"临时会话","bookUrl":"` + fixture.source.BaseURL + `/book","sourceId":` + strconv.FormatUint(uint64(fixture.source.ID), 10) + `}`
			},
		},
		{
			name:     "explicit remote refresh",
			username: "cancelremote2",
			requestPath: func(fixture remoteBookLifecycleFixture) string {
				return "/api/books/" + strconv.FormatUint(uint64(fixture.book.ID), 10) + "/refresh"
			},
			requestBody: func(remoteBookLifecycleFixture) string { return "" },
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			scenario := newRemoteBookLifecycleFixture(t, fixture.username)
			started, upstreamCanceled, release := installBlockingRemoteBookLifecycleClient(t)

			ctx, cancel := context.WithCancel(context.Background())
			request := httptest.NewRequest(
				http.MethodPost,
				fixture.requestPath(scenario),
				strings.NewReader(fixture.requestBody(scenario)),
			).WithContext(ctx)
			request.Header.Set("Authorization", scenario.auth)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				scenario.router.ServeHTTP(response, request)
				close(done)
			}()

			waitRemoteBookLifecycleSignal(t, started, release, "request did not reach the remote transport")
			cancel()
			canceled := false
			select {
			case <-upstreamCanceled:
				canceled = true
			case <-time.After(300 * time.Millisecond):
				close(release)
			}
			if canceled {
				close(release)
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("handler did not finish after caller cancellation")
			}

			if !canceled {
				t.Error("caller cancellation did not cancel the upstream BookInfo/TOC request")
			}
			if response.Body.Len() != 0 {
				t.Errorf("caller cancellation wrote a business response: %d %s", response.Code, response.Body.String())
			}
			var failureCount int64
			if err := scenario.server.db.Model(&models.SourceFailure{}).
				Where("user_id = ? AND source_id = ?", scenario.userID, scenario.source.ID).
				Count(&failureCount).Error; err != nil {
				t.Fatal(err)
			}
			if failureCount != 0 {
				t.Errorf("caller cancellation recorded %d source failures", failureCount)
			}

			if scenario.book.ID != 0 {
				var current models.Book
				if err := scenario.server.db.First(&current, scenario.book.ID).Error; err != nil {
					t.Fatal(err)
				}
				if current.Title != scenario.book.Title || current.ChapterCount != scenario.book.ChapterCount || current.LastCheckTime != scenario.book.LastCheckTime {
					t.Errorf("canceled refresh mutated book: got %+v want title=%q count=%d lastCheckTime=%d", current, scenario.book.Title, scenario.book.ChapterCount, scenario.book.LastCheckTime)
				}
				var chapters []models.Chapter
				if err := scenario.server.db.Where("book_id = ?", scenario.book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
					t.Fatal(err)
				}
				if len(chapters) != 1 || chapters[0].URL != scenario.chapter.URL {
					t.Errorf("canceled refresh replaced catalogue: %+v", chapters)
				}
			}
		})
	}
}

func TestRemoteBookRefreshRejectsDeletedOrChangedSnapshotAfterFetch(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		username string
		mutate   func(*testing.T, remoteBookLifecycleFixture)
		assert   func(*testing.T, remoteBookLifecycleFixture)
	}{
		{
			name:     "deleted book",
			username: "stalebook1",
			mutate: func(t *testing.T, scenario remoteBookLifecycleFixture) {
				request := httptest.NewRequest(http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(scenario.book.ID), 10), nil)
				request.Header.Set("Authorization", scenario.auth)
				response := httptest.NewRecorder()
				scenario.router.ServeHTTP(response, request)
				if response.Code != http.StatusNoContent {
					t.Fatalf("concurrent delete = %d %s, want 204", response.Code, response.Body.String())
				}
			},
			assert: func(t *testing.T, scenario remoteBookLifecycleFixture) {
				var bookCount, chapterCount int64
				if err := scenario.server.db.Model(&models.Book{}).Where("id = ?", scenario.book.ID).Count(&bookCount).Error; err != nil {
					t.Fatal(err)
				}
				if err := scenario.server.db.Model(&models.Chapter{}).Where("book_id = ?", scenario.book.ID).Count(&chapterCount).Error; err != nil {
					t.Fatal(err)
				}
				if bookCount != 0 || chapterCount != 0 {
					t.Errorf("late refresh resurrected deleted state: books=%d chapters=%d", bookCount, chapterCount)
				}
			},
		},
		{
			name:     "changed source and metadata",
			username: "stalebook2",
			mutate: func(t *testing.T, scenario remoteBookLifecycleFixture) {
				replacement := createRemoteBookLifecycleSource(t, scenario.server, scenario.source.BaseURL+"-replacement")
				if err := scenario.server.db.Model(&models.Book{}).
					Where("id = ? AND user_id = ?", scenario.book.ID, scenario.userID).
					Updates(map[string]any{
						"source_id": replacement.ID,
						"url":       replacement.BaseURL + "/book",
						"variable":  `{"new":"state"}`,
						"title":     "用户并发编辑标题",
					}).Error; err != nil {
					t.Fatal(err)
				}
			},
			assert: func(t *testing.T, scenario remoteBookLifecycleFixture) {
				var current models.Book
				if err := scenario.server.db.First(&current, scenario.book.ID).Error; err != nil {
					t.Fatal(err)
				}
				if current.SourceID == scenario.source.ID || current.URL == scenario.book.URL || current.Variable != `{"new":"state"}` || current.Title != "用户并发编辑标题" {
					t.Errorf("late refresh overwrote concurrent book state: %+v", current)
				}
				var chapters []models.Chapter
				if err := scenario.server.db.Where("book_id = ?", scenario.book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
					t.Fatal(err)
				}
				if len(chapters) != 1 || chapters[0].URL != scenario.chapter.URL {
					t.Errorf("late refresh replaced the current catalogue: %+v", chapters)
				}
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			scenario := newRemoteBookLifecycleFixture(t, fixture.username)
			started, _, release := installBlockingRemoteBookLifecycleClient(t)
			request := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(scenario.book.ID), 10)+"/refresh", nil)
			request.Header.Set("Authorization", scenario.auth)
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				scenario.router.ServeHTTP(response, request)
				close(done)
			}()

			waitRemoteBookLifecycleSignal(t, started, release, "refresh did not reach the remote transport")
			fixture.mutate(t, scenario)
			close(release)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("refresh did not finish after releasing the upstream response")
			}
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"error":"book changed during refresh"`) {
				t.Errorf("stale refresh = %d %s, want stable 409", response.Code, response.Body.String())
			}
			fixture.assert(t, scenario)
		})
	}
}

type remoteBookLifecycleFixture struct {
	router  http.Handler
	server  *Server
	auth    string
	userID  uint
	source  models.BookSource
	book    models.Book
	chapter models.Chapter
}

func newRemoteBookLifecycleFixture(t *testing.T, username string) remoteBookLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	userID := sourceFailureUserID(t, server, username)
	source := createRemoteBookLifecycleSource(t, server, "https://"+username+".test")
	book := models.Book{
		UserID:        userID,
		SourceID:      source.ID,
		Title:         "刷新前书名",
		URL:           source.BaseURL + "/book",
		Variable:      `{"old":"state"}`,
		ChapterCount:  1,
		LastChapter:   "旧章节",
		LastCheckTime: 1700000000000,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧章节", URL: source.BaseURL + "/old-chapter"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return remoteBookLifecycleFixture{router: router, server: server, auth: auth, userID: userID, source: source, book: book, chapter: chapter}
}

func createRemoteBookLifecycleSource(t *testing.T, server *Server, baseURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: "远程生命周期源 " + baseURL, BaseURL: baseURL, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoCanRenameRule: ".rename",
		BookInfoNameRule:      ".book-title",
		BookInfoAuthorRule:    ".book-author",
		ChapterListRule:       ".chapter",
		ChapterNameRule:       ".chapter-title|text",
		ChapterURLRule:        "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	return source
}

func installBlockingRemoteBookLifecycleClient(t *testing.T) (<-chan struct{}, <-chan struct{}, chan struct{}) {
	t.Helper()
	started := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var canceledOnce sync.Once
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-request.Context().Done():
			canceledOnce.Do(func() { close(upstreamCanceled) })
			return nil, request.Context().Err()
		case <-release:
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<main>
					<span class="rename">1</span><h1 class="book-title">远端刷新标题</h1>
					<span class="book-author">远端作者</span>
					<div class="chapter"><span class="chapter-title">远端第一章</span><a href="/new-chapter">阅读</a></div>
				</main>`)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}
	})})
	t.Cleanup(restore)
	return started, upstreamCanceled, release
}

func waitRemoteBookLifecycleSignal(t *testing.T, started <-chan struct{}, release chan struct{}, message string) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal(message)
	}
}
