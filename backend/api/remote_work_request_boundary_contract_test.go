package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	contractRemoteSearchBodyBytes  = 64 << 10
	contractRemoteControlBodyBytes = 16 << 10
	contractRemoteSearchSourceIDs  = 5000
	contractRemoteHealthSources    = 300
)

type remoteWorkRouteFixture struct {
	name           string
	path           string
	body           string
	maxBytes       int
	malformedError string
}

func TestRemoteWorkJSONBoundaryRejectsDeclaredAndChunkedOverflow(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	source, book := createRemoteWorkBoundaryFixtures(t, server, user.ID)

	var requests atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return remoteWorkHTTPResponse(request, `<html><main class="content">正文</main></html>`), nil
	})})
	defer restore()

	for _, route := range remoteWorkBoundaryRoutes(source.ID, book.ID) {
		for _, chunked := range []bool{false, true} {
			transport := "declared"
			if chunked {
				transport = "chunked"
			}
			t.Run(route.name+"/"+transport, func(t *testing.T) {
				before := requests.Load()
				body := padRemoteWorkObject(route.body, route.maxBytes+1)
				response := performRemoteWorkRequest(router, auth, route.path, body, chunked, nil)
				assertRemoteWorkError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
				if requests.Load() != before {
					t.Fatalf("overflow %s started remote work", route.name)
				}
			})
		}
	}
}

func TestRemoteWorkJSONBoundaryRejectsTrailingDocuments(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	source, book := createRemoteWorkBoundaryFixtures(t, server, user.ID)

	var requests atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return remoteWorkHTTPResponse(request, `<html><main class="content">正文</main></html>`), nil
	})})
	defer restore()

	for _, route := range remoteWorkBoundaryRoutes(source.ID, book.ID) {
		for _, suffix := range []struct {
			name string
			body string
		}{
			{name: "second-json", body: `{"ignored":true}`},
			{name: "garbage", body: `garbage`},
		} {
			t.Run(route.name+"/"+suffix.name, func(t *testing.T) {
				before := requests.Load()
				response := performRemoteWorkRequest(router, auth, route.path, route.body+suffix.body, false, nil)
				assertRemoteWorkError(t, response, http.StatusBadRequest, route.malformedError)
				if requests.Load() != before {
					t.Fatalf("malformed %s started remote work", route.name)
				}
			})
		}
	}

	response := performRemoteWorkRequest(router, auth, "/api/sources/batch-test", "null", false, nil)
	assertRemoteWorkError(t, response, http.StatusBadRequest, "invalid batch test payload")
}

func TestRemoteSearchBoundaryRejectsCardinalityAndKeywordBeforeWork(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	source := createRemoteWorkSearchSources(t, server, 1)[0]

	var requests atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return remoteWorkHTTPResponse(request, `<html></html>`), nil
	})})
	defer restore()

	ids := make([]string, contractRemoteSearchSourceIDs+1)
	for index := range ids {
		ids[index] = strconv.FormatUint(uint64(source.ID), 10)
	}
	body := `{"keyword":"边界","sourceIds":[` + strings.Join(ids, ",") + `],"lastIndex":-1,"searchSize":1}`
	response := performRemoteWorkRequest(router, auth, "/api/search", body, false, nil)
	assertRemoteWorkError(t, response, http.StatusBadRequest, "too many sources")
	if requests.Load() != 0 {
		t.Fatalf("5,001 source IDs started %d remote requests", requests.Load())
	}

	body = fmt.Sprintf(`{"keyword":%q,"sourceIds":[%d],"page":1}`, strings.Repeat("k", 1025), source.ID)
	response = performRemoteWorkRequest(router, auth, "/api/search", body, false, nil)
	assertRemoteWorkError(t, response, http.StatusBadRequest, "keyword is required")
	if requests.Load() != 0 {
		t.Fatalf("oversized keyword started %d remote requests", requests.Load())
	}
}

func TestRemoteSearchStopsAfterEightStableSourceWindows(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	sources := createRemoteWorkSearchSources(t, server, 10)

	var requests atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return remoteWorkHTTPResponse(request, `<html></html>`), nil
	})})
	defer restore()

	response := performRemoteWorkRequest(router, auth, "/api/search", remoteWorkSearchBody(sources, 1), false, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("bounded search = %d: %s", response.Code, response.Body.String())
	}
	var result searchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 8 || result.LastIndex != 7 || !result.HasMore {
		t.Fatalf("search work boundary requests=%d response=%+v, want 8/lastIndex=7/hasMore", requests.Load(), result)
	}
}

func TestRemoteSearchSuppressedSourcesConsumeStableWindows(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	sources := createRemoteWorkSearchSources(t, server, 10)

	for _, source := range sources[:8] {
		failure := models.SourceFailure{
			UserID:    user.ID,
			SourceID:  source.ID,
			SourceURL: source.BaseURL,
			Message:   "请求书源失败",
			FailedAt:  time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		}
		if err := server.db.Create(&failure).Error; err != nil {
			t.Fatal(err)
		}
	}

	var requests atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return remoteWorkHTTPResponse(request, `<html></html>`), nil
	})})
	defer restore()

	response := performRemoteWorkRequest(router, auth, "/api/search", remoteWorkSearchBody(sources, 1), false, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("suppressed search = %d: %s", response.Code, response.Body.String())
	}
	var result searchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 || result.LastIndex != 7 || !result.HasMore {
		t.Fatalf("suppressed ordinal boundary requests=%d response=%+v", requests.Load(), result)
	}
}

func TestRemoteSearchCapsActiveConcurrencyAtSixty(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	sources := createRemoteWorkSearchSources(t, server, 61)

	gate := make(chan struct{})
	started := make(chan struct{}, len(sources))
	var active atomic.Int64
	var maximum atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		current := active.Add(1)
		updateRemoteWorkMaximum(&maximum, current)
		started <- struct{}{}
		select {
		case <-gate:
		case <-request.Context().Done():
			active.Add(-1)
			return nil, request.Context().Err()
		}
		active.Add(-1)
		return remoteWorkHTTPResponse(request, `<html></html>`), nil
	})})
	defer restore()

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- performRemoteWorkRequest(router, auth, "/api/search", remoteWorkSearchBody(sources, 61), false, nil)
	}()
	for count := 0; count < 60; count++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(gate)
			t.Fatal("search did not start the expected bounded concurrency window")
		}
	}
	time.Sleep(50 * time.Millisecond)
	observed := maximum.Load()
	close(gate)
	select {
	case response := <-done:
		if response.Code != http.StatusOK {
			t.Fatalf("concurrency search = %d: %s", response.Code, response.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("concurrency search did not finish")
	}
	if observed > 60 {
		t.Fatalf("search started %d concurrent remote requests, want at most 60", observed)
	}
}

func TestRemoteHealthBoundaryRejectsRawCardinalityBeforeWork(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	source := createRemoteWorkSearchSources(t, server, 1)[0]

	var requests atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return remoteWorkHTTPResponse(request, `<html></html>`), nil
	})})
	defer restore()

	ids := make([]string, contractRemoteHealthSources+1)
	for index := range ids {
		ids[index] = strconv.FormatUint(uint64(source.ID), 10)
	}
	body := `{"sourceIds":[` + strings.Join(ids, ",") + `],"keyword":"边界"}`
	response := performRemoteWorkRequest(router, auth, "/api/sources/batch-test", body, false, nil)
	assertRemoteWorkError(t, response, http.StatusBadRequest, "too many sources")
	if requests.Load() != 0 {
		t.Fatalf("301 health IDs started %d remote requests", requests.Load())
	}
}

func TestRemoteHealthUsesFixedWorkersInsteadOfOneGoroutinePerSource(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	sources := createRemoteWorkSearchSources(t, server, 80)

	gate := make(chan struct{})
	started := make(chan struct{}, 3)
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-gate:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
		return remoteWorkHTTPResponse(request, `<html></html>`), nil
	})})
	defer restore()

	ids := remoteWorkSourceIDList(sources)
	body := `{"sourceIds":[` + strings.Join(ids, ",") + `],"keyword":"边界","concurrent":3}`
	baseline := runtime.NumGoroutine()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- performRemoteWorkRequest(router, auth, "/api/sources/batch-test", body, false, nil)
	}()
	for count := 0; count < 3; count++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(gate)
			t.Fatal("health check did not start three workers")
		}
	}
	time.Sleep(50 * time.Millisecond)
	delta := runtime.NumGoroutine() - baseline
	close(gate)
	select {
	case response := <-done:
		if response.Code != http.StatusOK {
			t.Fatalf("health worker request = %d: %s", response.Code, response.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("health worker request did not finish")
	}
	if delta > 30 {
		t.Fatalf("health check added %d goroutines for 80 sources, want a fixed worker pool", delta)
	}
}

func TestLegacySourceProbePropagatesCallerCancellation(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	source := createRemoteWorkSearchSources(t, server, 1)[0]

	started := make(chan struct{}, 1)
	transportResult := make(chan bool, 1)
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		started <- struct{}{}
		select {
		case <-request.Context().Done():
			transportResult <- true
			return nil, request.Context().Err()
		case <-time.After(500 * time.Millisecond):
			transportResult <- false
			return remoteWorkHTTPResponse(request, `<html></html>`), nil
		}
	})})
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- performRemoteWorkRequest(router, auth, "/api/sources/"+uintString(source.ID)+"/test", `{"keyword":"取消"}`, false, ctx)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("legacy source probe did not start transport")
	}
	cancel()
	canceled := <-transportResult
	select {
	case response := <-done:
		if response.Code != http.StatusOK {
			t.Fatalf("canceled legacy probe = %d: %s", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("legacy source probe did not finish after cancellation")
	}
	if !canceled {
		t.Fatal("legacy source probe did not propagate caller cancellation to transport")
	}
}

func remoteWorkBoundaryRoutes(sourceID, bookID uint) []remoteWorkRouteFixture {
	return []remoteWorkRouteFixture{
		{name: "search", path: "/api/search", body: fmt.Sprintf(`{"keyword":"边界","sourceIds":[%d],"page":1}`, sourceID), maxBytes: contractRemoteSearchBodyBytes, malformedError: "keyword is required"},
		{name: "source-test", path: "/api/sources/" + uintString(sourceID) + "/test", body: `{"keyword":"边界"}`, maxBytes: contractRemoteControlBodyBytes, malformedError: "keyword is required"},
		{name: "source-test-chapter", path: "/api/sources/" + uintString(sourceID) + "/test-chapter", body: `{"bookUrl":"https://remote-work-boundary.example/book"}`, maxBytes: contractRemoteControlBodyBytes, malformedError: "bookUrl is required"},
		{name: "source-test-content", path: "/api/sources/" + uintString(sourceID) + "/test-content", body: `{"chapterUrl":"https://remote-work-boundary.example/chapter"}`, maxBytes: contractRemoteControlBodyBytes, malformedError: "chapterUrl is required"},
		{name: "source-batch-test", path: "/api/sources/batch-test", body: fmt.Sprintf(`{"sourceIds":[%d],"keyword":"边界"}`, sourceID), maxBytes: contractRemoteControlBodyBytes, malformedError: "invalid batch test payload"},
		{name: "book-cache", path: "/api/books/" + uintString(bookID) + "/cache", body: `{"all":true}`, maxBytes: contractRemoteControlBodyBytes, malformedError: "invalid cache payload"},
		{name: "book-cache-stream", path: "/api/books/" + uintString(bookID) + "/cache/stream", body: `{"all":true}`, maxBytes: contractRemoteControlBodyBytes, malformedError: "invalid cache payload"},
	}
}

func createRemoteWorkBoundaryFixtures(t *testing.T, server *Server, userID uint) (models.BookSource, models.Book) {
	t.Helper()
	source := sourceDebugModeSource(t, "https://remote-work-boundary.example")
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: userID, SourceID: source.ID, Title: "远程工作边界书", URL: source.BaseURL + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	return source, book
}

func createRemoteWorkSearchSources(t *testing.T, server *Server, count int) []models.BookSource {
	t.Helper()
	sources := make([]models.BookSource, 0, count)
	for index := 0; index < count; index++ {
		baseURL := fmt.Sprintf("https://remote-work-search-%03d.example", index)
		source := sourceFailureTestSource(t, fmt.Sprintf("远程工作书源 %03d", index), baseURL)
		if err := server.db.Create(&source).Error; err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}
	return sources
}

func remoteWorkSearchBody(sources []models.BookSource, concurrent int) string {
	return fmt.Sprintf(
		`{"keyword":"边界","sourceIds":[%s],"concurrentCount":%d,"lastIndex":-1,"searchSize":1}`,
		strings.Join(remoteWorkSourceIDList(sources), ","),
		concurrent,
	)
}

func remoteWorkSourceIDList(sources []models.BookSource) []string {
	ids := make([]string, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, strconv.FormatUint(uint64(source.ID), 10))
	}
	return ids
}

func padRemoteWorkObject(body string, total int) string {
	prefix := strings.TrimSuffix(body, "}") + `,"padding":"`
	suffix := `"}`
	if total < len(prefix)+len(suffix) {
		panic("remote-work target body is too small")
	}
	return prefix + strings.Repeat("p", total-len(prefix)-len(suffix)) + suffix
}

func performRemoteWorkRequest(router http.Handler, auth, path, body string, chunked bool, ctx context.Context) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", auth)
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	if ctx != nil {
		request = request.WithContext(ctx)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertRemoteWorkError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response %d %q: %v", response.Code, response.Body.String(), err)
	}
	if response.Code != status || payload.Error != message {
		t.Fatalf("remote-work error = %d %q, want %d %q", response.Code, payload.Error, status, message)
	}
}

func remoteWorkHTTPResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}
}

func updateRemoteWorkMaximum(maximum *atomic.Int64, candidate int64) {
	for {
		current := maximum.Load()
		if candidate <= current || maximum.CompareAndSwap(current, candidate) {
			return
		}
	}
}
