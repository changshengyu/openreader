package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestExploreRequestBoundaryRejectsUnboundedPageAndUndeclaredEntryBeforeFetch(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	source := createExploreLifecycleSource(t, server,
		"热门::https://declared-explore.example/top/{page}\n分类::/category/{page}",
	)

	var requests atomic.Int32
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			return exploreLifecycleResponse(request), nil
		}),
	})
	defer restoreHTTPClient()

	longEntry := "https://declared-explore.example/" + strings.Repeat("x", 8193)
	fixtures := []struct {
		name    string
		query   string
		message string
		secret  string
	}{
		{name: "page above limit", query: "page=100001", message: "invalid page"},
		{
			name:    "undeclared same origin",
			query:   "page=1&url=" + url.QueryEscape("https://declared-explore.example/private/{page}"),
			message: "invalid explore URL",
			secret:  "private",
		},
		{
			name:    "undeclared cross origin",
			query:   "page=1&url=" + url.QueryEscape("https://secret.example/collect/{page}"),
			message: "invalid explore URL",
			secret:  "secret.example",
		},
		{
			name: "client injected request options",
			query: "page=1&url=" + url.QueryEscape(
				`https://declared-explore.example/top/{page}, {"method":"POST","headers":{"X-Secret":"leak"}}`,
			),
			message: "invalid explore URL",
			secret:  "X-Secret",
		},
		{
			name:    "entry above byte limit",
			query:   "page=1&url=" + url.QueryEscape(longEntry),
			message: "invalid explore URL",
			secret:  longEntry[:64],
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			before := requests.Load()
			response := performExploreLifecycleRequest(
				t,
				router,
				auth,
				"/api/explore/"+strconv.FormatUint(uint64(source.ID), 10)+"?"+fixture.query,
				nil,
			)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"error":"`+fixture.message+`"`) {
				t.Fatalf("response = %d %s, want safe 400 %q", response.Code, response.Body.String(), fixture.message)
			}
			if requests.Load() != before {
				t.Fatalf("rejected request performed remote work: before=%d after=%d", before, requests.Load())
			}
			if fixture.secret != "" && strings.Contains(response.Body.String(), fixture.secret) {
				t.Fatalf("response exposed rejected entry: %s", response.Body.String())
			}
		})
	}

	missing := performExploreLifecycleRequest(
		t,
		router,
		auth,
		"/api/explore/4294967295?page=100001&url="+url.QueryEscape("https://secret.example/private"),
		nil,
	)
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"error":"source not found"`) {
		t.Fatalf("missing source priority = %d %s, want source-first 404", missing.Code, missing.Body.String())
	}
}

func TestExploreRequestBoundaryAllowsDeclaredMaximumPageAndEntryBytes(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)

	entry := "https://declared-explore.example/" + strings.Repeat("x", 8192-len("https://declared-explore.example/"))
	source := createExploreLifecycleSource(t, server, entry)
	var requested string
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = request.URL.String()
			return exploreLifecycleResponse(request), nil
		}),
	})
	defer restoreHTTPClient()

	response := performExploreLifecycleRequest(
		t,
		router,
		auth,
		"/api/explore/"+strconv.FormatUint(uint64(source.ID), 10)+"?page=100000&url="+url.QueryEscape(entry),
		nil,
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"page":100000`) {
		t.Fatalf("declared boundary response = %d %s", response.Code, response.Body.String())
	}
	if requested != entry {
		t.Fatalf("declared boundary request URL length = %d, want %d", len(requested), len(entry))
	}
}

func TestExploreRequestCancellationStopsFetchAndDoesNotCacheFailure(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	source := createExploreLifecycleSource(t, server, "https://cancel-explore.example/list/{page}")

	started := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var canceledOnce sync.Once
	var releaseOnce sync.Once
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			startedOnce.Do(func() { close(started) })
			select {
			case <-request.Context().Done():
				canceledOnce.Do(func() { close(upstreamCanceled) })
				return nil, request.Context().Err()
			case <-release:
				return nil, errors.New("late explore request failure")
			}
		}),
	})
	defer restoreHTTPClient()
	defer releaseOnce.Do(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/explore/"+strconv.FormatUint(uint64(source.ID), 10)+"?page=1",
		nil,
	).WithContext(ctx)
	request.Header.Set("Authorization", auth)
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(response, request)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		releaseOnce.Do(func() { close(release) })
		t.Fatal("explore request did not reach the remote transport")
	}
	cancel()

	canceled := false
	select {
	case <-upstreamCanceled:
		canceled = true
	case <-time.After(2 * time.Second):
		releaseOnce.Do(func() { close(release) })
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		releaseOnce.Do(func() { close(release) })
		t.Fatal("explore handler did not finish after caller cancellation")
	}

	var failureCount int64
	if err := server.db.Model(&models.SourceFailure{}).Where("source_id = ?", source.ID).Count(&failureCount).Error; err != nil {
		t.Fatal(err)
	}
	if !canceled {
		t.Error("caller cancellation did not cancel the upstream request context")
	}
	if failureCount != 0 {
		t.Errorf("caller cancellation cached %d source failures, want 0", failureCount)
	}
	if response.Body.Len() != 0 {
		t.Errorf("caller cancellation wrote a business response: %s", response.Body.String())
	}
}

func createExploreLifecycleSource(t *testing.T, server *Server, exploreURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{
		Name:     "Explore 生命周期源",
		BaseURL:  "https://declared-explore.example",
		Charset:  "utf-8",
		Enabled:  true,
		Header:   `{"Authorization":"Bearer source-secret"}`,
		LoginURL: "https://declared-explore.example/login",
	}
	if err := source.SetRules(models.BookSourceRule{
		ExploreURL:   exploreURL,
		BookListRule: ".book",
		BookNameRule: ".title|text",
		BookURLRule:  ".link|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	return source
}

func performExploreLifecycleRequest(t *testing.T, router http.Handler, auth, path string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if ctx != nil {
		request = request.WithContext(ctx)
	}
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func exploreLifecycleResponse(request *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`<div class="book"><a class="link" href="/book"><span class="title">边界书</span></a></div>`,
		)),
		Header:  make(http.Header),
		Request: request,
	}
}
