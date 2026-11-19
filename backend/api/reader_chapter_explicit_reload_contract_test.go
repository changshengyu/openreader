package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestReaderExplicitChapterReloadReturnsFreshContentAndPreservesServerCache(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapterreload")
	sourceContent, sourceOffline, fetches := "original content", false, 0
	t.Cleanup(engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		fetches++
		if sourceOffline {
			return nil, io.ErrUnexpectedEOF
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`<main class="content">%s</main><span class="token">%s token</span>`,
				sourceContent, sourceContent,
			))),
			Header:  make(http.Header),
			Request: request,
		}, nil
	})}))

	get := func(query string) *httptest.ResponseRecorder {
		return requestExplicitChapterReload(fixture, context.Background(), fixture.auth, query)
	}
	assertReaderReloadContent(t, get(""), "original content")
	_, previous := loadReaderChapterContentLifecycleState(t, fixture)
	sourceContent = "updated content"
	assertReaderReloadContent(t, get(""), "original content")
	assertReaderReloadContent(t, get("?refresh=0"), "original content")

	otherUser := registerLifecycleToken(t, fixture.router.(*gin.Engine), "chapterreloadother")
	if response := requestExplicitChapterReload(fixture, context.Background(), otherUser, "?refresh=1"); response.Code != http.StatusNotFound {
		t.Fatalf("other user's reload response = %d %s, want 404", response.Code, response.Body.String())
	}
	otherFixture := fixture
	otherFixture.auth = otherUser
	otherFixture.user = lifecycleUser(t, fixture.server, "chapterreloadother")
	otherFixture.book = models.Book{
		UserID: otherFixture.user.ID, SourceID: fixture.source.ID, URL: fixture.book.URL, Title: fixture.book.Title,
	}
	if err := fixture.server.db.Create(&otherFixture.book).Error; err != nil {
		t.Fatal(err)
	}
	otherFixture.chapter = models.Chapter{
		BookID: otherFixture.book.ID, Title: fixture.chapter.Title, URL: fixture.chapter.URL, CachePath: previous.CachePath,
	}
	if err := fixture.server.db.Create(&otherFixture.chapter).Error; err != nil {
		t.Fatal(err)
	}
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(otherFixture, context.Background()), "original content")
	if fetches != 1 {
		t.Fatalf("initial read, cached reads and other user reload made %d source requests, want 1", fetches)
	}

	assertReaderReloadContent(t, get("?refresh=1"), "updated content")
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(otherFixture, context.Background()), "original content")
	if fetches != 2 {
		t.Fatalf("successful explicit reload made %d total source requests, want 2", fetches)
	}
	sourceOffline = true
	assertReaderReloadContent(t, get(""), "original content")
	if response := get("?refresh=1"); response.Code != http.StatusBadGateway {
		t.Fatalf("offline source reload response = %d %s, want 502", response.Code, response.Body.String())
	}
	assertReaderReloadContent(t, get(""), "original content")

	sourceOffline, sourceContent = false, "  "
	if response := get("?refresh=1"); response.Code != http.StatusBadGateway {
		t.Fatalf("empty source reload response = %d %s, want 502", response.Code, response.Body.String())
	}
	assertReaderReloadContent(t, get(""), "original content")
	_, chapter := loadReaderChapterContentLifecycleState(t, fixture)
	if chapter.CachePath != previous.CachePath || chapter.Variable != previous.Variable {
		t.Fatalf("explicit reloads changed chapter state: cache=%q variable=%q", chapter.CachePath, chapter.Variable)
	}
	if cached, err := engine.ReadChapterCache(fixture.server.cfg.CacheDir, chapter.CachePath); err != nil || cached != "original content" {
		t.Fatalf("server cache after explicit reloads = %q, %v, want original content", cached, err)
	}
	if fetches != 4 {
		t.Fatalf("all requests made %d source requests, want 4", fetches)
	}
}

func TestReaderExplicitChapterReloadCancellationKeepsPublishedCache(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapterreloadcancel")
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(fixture, context.Background()), "old remote content")
	_, previous := loadReaderChapterContentLifecycleState(t, fixture)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reachedFetch := false
	installReaderChapterContentLifecycleHook(t, func(stage string, _ context.Context, _ models.Book, _ models.Chapter) {
		if stage == "after_remote_fetch" {
			reachedFetch = true
			cancel()
		}
	})

	response := requestExplicitChapterReload(fixture, ctx, fixture.auth, "?refresh=1")
	if !reachedFetch || response.Body.Len() != 0 {
		t.Fatalf("cancelled reload reached fetch = %t, body = %q, want a cancelled source reload", reachedFetch, response.Body.String())
	}
	_, current := loadReaderChapterContentLifecycleState(t, fixture)
	if current.CachePath != previous.CachePath || current.Variable != previous.Variable {
		t.Fatalf("cancelled reload changed chapter state: cache=%q variable=%q", current.CachePath, current.Variable)
	}
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(fixture, context.Background()), "old remote content")
}

func TestReaderExplicitChapterReloadRejectsChangedSource(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapterreloadsourcechange")
	ctx := context.Background()
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(fixture, ctx), "old remote content")
	_, previous := loadReaderChapterContentLifecycleState(t, fixture)
	installReaderChapterContentLifecycleHook(t, func(stage string, _ context.Context, _ models.Book, _ models.Chapter) {
		if stage != "after_remote_fetch" {
			return
		}
		if err := fixture.server.db.Model(&fixture.source).Update("charset", "gbk").Error; err != nil {
			t.Errorf("update source semantics: %v", err)
		}
	})

	assertReaderChapterStaleReason(t, requestExplicitChapterReload(fixture, ctx, fixture.auth, "?refresh=1"), "source")
	_, current := loadReaderChapterContentLifecycleState(t, fixture)
	if current.CachePath != previous.CachePath || current.Variable != previous.Variable {
		t.Fatalf("stale reload changed chapter state: cache=%q variable=%q", current.CachePath, current.Variable)
	}
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(fixture, ctx), "old remote content")
}

func TestReaderExplicitChapterReloadLeavesCacheReadableWhileFetching(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapterreloadconcurrent")
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(fixture, ctx), "old remote content")

	fetchStarted := make(chan struct{}, 1)
	releaseFetch := make(chan struct{})
	t.Cleanup(engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		select {
		case fetchStarted <- struct{}{}:
		default:
		}
		select {
		case <-releaseFetch:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("<main class=content>updated remote content</main>")),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}))
	reloadDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		reloadDone <- requestExplicitChapterReload(fixture, ctx, fixture.auth, "?refresh=1")
	}()
	select {
	case <-fetchStarted:
	case response := <-reloadDone:
		t.Fatalf("reload returned %d before making its source request", response.Code)
	case <-time.After(2 * time.Second):
		t.Fatal("explicit reload did not start its source request")
	}

	ordinaryDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		ordinaryDone <- performReaderChapterContentLifecycleRequest(fixture, ctx)
	}()
	var ordinary *httptest.ResponseRecorder
	select {
	case ordinary = <-ordinaryDone:
	case <-time.After(2 * time.Second):
	}
	close(releaseFetch)
	select {
	case response := <-reloadDone:
		assertReaderReloadContent(t, response, "updated remote content")
	case <-time.After(2 * time.Second):
		t.Fatal("explicit reload did not finish")
	}
	if ordinary == nil {
		t.Fatal("ordinary cached request waited behind the explicit reload")
	}
	assertReaderReloadContent(t, ordinary, "old remote content")
	assertReaderReloadContent(t, performReaderChapterContentLifecycleRequest(fixture, ctx), "old remote content")
}

func requestExplicitChapterReload(
	fixture readerChapterContentLifecycleFixture,
	ctx context.Context,
	auth, query string,
) *httptest.ResponseRecorder {
	path := fmt.Sprintf("/api/books/%d/chapters/%d/content%s", fixture.book.ID, fixture.chapter.Index, query)
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func assertReaderReloadContent(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("chapter response = %d %s, want 200", response.Code, response.Body.String())
	}
	var payload struct {
		Content string `json:"content"`
		Format  string `json:"format"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chapter response: %v", err)
	}
	if payload.Content != want || payload.Format != "text" {
		t.Fatalf("chapter response contains %q in %q format, want %q in text format", payload.Content, payload.Format, want)
	}
}
