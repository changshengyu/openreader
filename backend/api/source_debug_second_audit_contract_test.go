package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"openreader/backend/engine"
	"openreader/backend/models"
)

type sourceDebugStreamContractEvent struct {
	Name string
	Data map[string]any
}

func TestSourceDebugStreamRunsFixedBaselineChainWithRuntimeAndBoundary(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	const upstream = "https://source-debug-chain.example"
	source := sourceDebugContractSource(t, upstream)
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	requests := make([]string, 0, 5)
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		requests = append(requests, request.URL.RequestURI())
		mu.Unlock()
		body := ""
		switch request.URL.Path {
		case "/search":
			if request.URL.Query().Get("q") != "链路" {
				t.Fatalf("search keyword = %q, want 链路", request.URL.Query().Get("q"))
			}
			body = `<article class="book"><span class="name">搜索候选</span><a href="/book/1">详情</a></article>`
		case "/book/1":
			body = `<h1 class="detail-title">详情书名</h1><a class="toc" href="/toc/1">目录</a>`
		case "/toc/1":
			body = `<article class="chapter"><span class="chapter-name">第一章</span><a href="/chapter/1">阅读</a></article>` +
				`<article class="chapter"><span class="chapter-name">第二章</span><a href="/chapter/2">阅读</a></article>`
		case "/chapter/1":
			body = `<main class="content">正文占位</main><a class="next" href="/chapter/2">下一页</a>`
		case "/chapter/2":
			return nil, errors.New("debug content crossed the adjacent chapter boundary")
		default:
			return nil, fmt.Errorf("unexpected source debug request %s", request.URL.String())
		}
		return sourceDebugHTTPResponse(request, body), nil
	})})
	defer restore()

	response := callSourceDebugStream(t, router, token, source.ID, `{"keyword":"链路"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("source debug stream = %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("source debug content type = %q", contentType)
	}
	events := decodeSourceDebugStreamContract(t, response.Body.String())
	wantStages := []string{
		"search:start", "search:success",
		"book_info:start", "book_info:success",
		"toc:start", "toc:success",
		"content:start", "content:success",
	}
	if got := sourceDebugStageStatuses(events); fmt.Sprint(got) != fmt.Sprint(wantStages) {
		t.Fatalf("source debug stages = %v, want %v\n%s", got, wantStages, response.Body.String())
	}
	terminal := requireSingleSourceDebugTerminal(t, events, "end")
	wantContent := "/toc/1:/chapter/1:详情书名:第一章"
	if got := intValue(terminal.Data["contentLength"]); got != utf8.RuneCountInString(wantContent) {
		t.Fatalf("contentLength = %d, want %d; Book/Chapter runtime was not carried", got, utf8.RuneCountInString(wantContent))
	}
	if strings.Contains(response.Body.String(), wantContent) || strings.Contains(response.Body.String(), "/chapter/1:/") {
		t.Fatalf("debug stream leaked parser variable values: %s", response.Body.String())
	}

	mu.Lock()
	gotRequests := append([]string(nil), requests...)
	mu.Unlock()
	wantRequests := []string{"/search?q=%E9%93%BE%E8%B7%AF", "/book/1", "/toc/1", "/chapter/1"}
	if fmt.Sprint(gotRequests) != fmt.Sprint(wantRequests) {
		t.Fatalf("source debug requests = %v, want %v", gotRequests, wantRequests)
	}
	assertNoSourceDebugFailureRows(t, server, source.ID)
}

func TestSourceDebugStreamDispatchesFixedBaselineEntryModes(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	const upstream = "https://source-debug-modes.example"
	source := sourceDebugModeSource(t, upstream)
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		body := ""
		switch {
		case path == "/search":
			body = `<article class="book"><span class="name">搜索书</span><a href="/book/search">详情</a></article>`
		case path == "/explore":
			body = `<article class="explore-book"><span class="explore-name">发现书</span><a href="/book/explore">详情</a></article>`
		case strings.HasPrefix(path, "/book/"):
			key := strings.TrimPrefix(path, "/book/")
			body = `<h1 class="detail-title">` + key + `书</h1><a class="toc" href="/toc/` + key + `">目录</a>`
		case strings.HasPrefix(path, "/toc/"):
			key := strings.TrimPrefix(path, "/toc/")
			body = `<article class="chapter"><span class="chapter-name">第一章</span><a href="/chapter/` + key + `/1">阅读</a></article>`
		case path == "/toc-direct":
			body = `<article class="chapter"><span class="chapter-name">第一章</span><a href="/chapter/toc-direct/1">阅读</a></article>`
		case strings.HasPrefix(path, "/chapter/") || path == "/content-direct":
			body = `<main class="content">正文</main>`
		default:
			return nil, fmt.Errorf("unexpected source debug mode request %s", request.URL.String())
		}
		return sourceDebugHTTPResponse(request, body), nil
	})})
	defer restore()

	tests := []struct {
		name    string
		keyword string
		stages  []string
	}{
		{name: "search", keyword: "普通搜索", stages: []string{"search", "book_info", "toc", "content"}},
		{name: "absolute detail URL", keyword: upstream + "/book/direct", stages: []string{"book_info", "toc", "content"}},
		{name: "explore", keyword: "分类::" + upstream + "/explore", stages: []string{"explore", "book_info", "toc", "content"}},
		{name: "direct toc", keyword: "++" + upstream + "/toc-direct", stages: []string{"toc", "content"}},
		{name: "direct content", keyword: "--" + upstream + "/content-direct", stages: []string{"content"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"keyword": test.keyword})
			if err != nil {
				t.Fatal(err)
			}
			response := callSourceDebugStream(t, router, token, source.ID, string(body))
			if response.Code != http.StatusOK {
				t.Fatalf("source debug mode = %d: %s", response.Code, response.Body.String())
			}
			events := decodeSourceDebugStreamContract(t, response.Body.String())
			got := successfulSourceDebugStages(events)
			if fmt.Sprint(got) != fmt.Sprint(test.stages) {
				t.Fatalf("successful stages = %v, want %v\n%s", got, test.stages, response.Body.String())
			}
			requireSingleSourceDebugTerminal(t, events, "end")
		})
	}
	assertNoSourceDebugFailureRows(t, server, source.ID)
}

func TestSourceDebugStreamDefaultsBlankKeywordAndEndsEmptySearch(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	const upstream = "https://source-debug-default.example"
	source := sourceDebugModeSource(t, upstream)
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/search" || request.URL.Query().Get("q") != "我的" {
			return nil, fmt.Errorf("default debug request = %s, want /search?q=我的", request.URL.String())
		}
		return sourceDebugHTTPResponse(request, `<html></html>`), nil
	})})
	defer restore()

	response := callSourceDebugStream(t, router, token, source.ID, `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("default source debug = %d: %s", response.Code, response.Body.String())
	}
	events := decodeSourceDebugStreamContract(t, response.Body.String())
	if got := sourceDebugStageStatuses(events); fmt.Sprint(got) != fmt.Sprint([]string{"search:start", "search:empty"}) {
		t.Fatalf("empty search stages = %v: %s", got, response.Body.String())
	}
	requireSingleSourceDebugTerminal(t, events, "end")
	assertNoSourceDebugFailureRows(t, server, source.ID)
}

func TestSourceDebugStreamAuthOwnershipAndDetachedContracts(t *testing.T) {
	router, server := setupTestServer(t)
	tokenA := registerLifecycleToken(t, router, "debugstreamowner")
	tokenB := registerLifecycleToken(t, router, "debugstreamother")
	owner := lifecycleUser(t, server, "debugstreamowner")

	source, err := server.bookSources.Create(owner.ID, sourceDebugModeSource(t, "https://source-debug-owner.example"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		token  string
		path   string
		status int
	}{
		{name: "missing JWT", path: fmt.Sprintf("/api/sources/%d/debug/stream", source.ID), status: http.StatusUnauthorized},
		{name: "foreign source", token: tokenB, path: fmt.Sprintf("/api/sources/%d/debug/stream", source.ID), status: http.StatusNotFound},
		{name: "invalid id", token: tokenA, path: "/api/sources/not-a-number/debug/stream", status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"keyword":"测试"}`))
			req.Header.Set("Content-Type", "application/json")
			if test.token != "" {
				req.Header.Set("Authorization", test.token)
			}
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, req)
			if writer.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", writer.Code, test.status, writer.Body.String())
			}
		})
	}

	if err := server.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND source_id = ?", owner.ID, source.ID).
		Update("detached", true).Error; err != nil {
		t.Fatal(err)
	}
	detached := callSourceDebugStream(t, router, tokenA, source.ID, `{"keyword":"测试"}`)
	if detached.Code != http.StatusNotFound {
		t.Fatalf("detached source status = %d, want 404: %s", detached.Code, detached.Body.String())
	}
}

func TestSourceDebugStreamCancellationStopsChainWithoutFailureOrEnd(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	const upstream = "https://source-debug-cancel.example"
	source := sourceDebugModeSource(t, upstream)
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, 1)
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-request.Context().Done()
		return nil, request.Context().Err()
	})})
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/sources/%d/debug/stream", source.ID), strings.NewReader(`{"keyword":"取消"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(writer, req)
		close(done)
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("source debug request did not reach the cancellable transport")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("source debug handler did not stop after cancellation")
	}
	if strings.Contains(writer.Body.String(), "event: end") {
		t.Fatalf("canceled source debug emitted a fake end: %s", writer.Body.String())
	}
	assertNoSourceDebugFailureRows(t, server, source.ID)
}

func TestLegacySourceDebugProbesNeverPopulateFailureCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New("upstream https://alice:secret@source-debug-failure.example/path?token=private failed")
	})})
	defer restore()

	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "search", path: "/test", body: `{"keyword":"测试"}`},
		{name: "toc", path: "/test-chapter", body: `{"bookUrl":"https://source-debug-failure.example/book/1"}`},
		{name: "content", path: "/test-content", body: `{"chapterUrl":"https://source-debug-failure.example/chapter/1"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := sourceDebugModeSource(t, "https://source-debug-failure.example")
			if err := server.db.Create(&source).Error; err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/sources/"+strconv.FormatUint(uint64(source.ID), 10)+test.path, strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", token)
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, req)
			if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), `"code":"source_request_failed"`) {
				t.Fatalf("legacy %s response = %d %s", test.name, writer.Code, writer.Body.String())
			}
			assertNoSourceDebugFailureRows(t, server, source.ID)
		})
	}
}

func sourceDebugContractSource(t *testing.T, baseURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: "书源调试链路", BaseURL: baseURL, Charset: "utf-8", Enabled: true, EnabledExplore: boolPointer(true)}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:          baseURL + "/search?q={keyword}",
		BookListRule:       ".book",
		BookNameRule:       `.name|text`,
		BookURLRule:        `a|attr:href`,
		BookInfoNameRule:   `@put:{"tocPath":".toc|attr:href"}.detail-title|text`,
		BookInfoAuthorRule: `@get:{missingSearchVariable}`,
		TOCURLRule:         `@get:{tocPath}`,
		ChapterListRule:    ".chapter",
		ChapterNameRule:    `@put:{"chapterPath":"a|attr:href"}.chapter-name|text`,
		ChapterURLRule:     `@get:{chapterPath}`,
		ContentRule:        `@get:{tocPath}:@get:{chapterPath}:@get:{bookName}:@get:{title}`,
		NextContentURLRule: ".next|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	return source
}

func sourceDebugModeSource(t *testing.T, baseURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: "书源调试分派", BaseURL: baseURL, Charset: "utf-8", Enabled: true, EnabledExplore: boolPointer(true)}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:           baseURL + "/search?q={keyword}",
		ExploreURL:          baseURL + "/explore",
		BookListRule:        ".book",
		BookNameRule:        ".name|text",
		BookURLRule:         "a|attr:href",
		ExploreBookListRule: ".explore-book",
		ExploreBookNameRule: ".explore-name|text",
		ExploreBookURLRule:  "a|attr:href",
		BookInfoNameRule:    ".detail-title|text",
		TOCURLRule:          ".toc|attr:href",
		ChapterListRule:     ".chapter",
		ChapterNameRule:     ".chapter-name|text",
		ChapterURLRule:      "a|attr:href",
		ContentRule:         ".content|text",
	}); err != nil {
		t.Fatal(err)
	}
	return source
}

func sourceDebugHTTPResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}
}

func callSourceDebugStream(t *testing.T, router http.Handler, token string, sourceID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/sources/%d/debug/stream", sourceID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	return writer
}

func decodeSourceDebugStreamContract(t *testing.T, body string) []sourceDebugStreamContractEvent {
	t.Helper()
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	blocks := strings.Split(normalized, "\n\n")
	events := make([]sourceDebugStreamContractEvent, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		name := "message"
		dataLines := make([]string, 0, 1)
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(dataLines) == 0 {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &data); err != nil {
			t.Fatalf("decode source debug SSE %q: %v", block, err)
		}
		events = append(events, sourceDebugStreamContractEvent{Name: name, Data: data})
	}
	return events
}

func sourceDebugStageStatuses(events []sourceDebugStreamContractEvent) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		if event.Name != "stage" {
			continue
		}
		result = append(result, fmt.Sprint(event.Data["stage"])+":"+fmt.Sprint(event.Data["status"]))
	}
	return result
}

func successfulSourceDebugStages(events []sourceDebugStreamContractEvent) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		if event.Name == "stage" && event.Data["status"] == "success" {
			result = append(result, fmt.Sprint(event.Data["stage"]))
		}
	}
	return result
}

func requireSingleSourceDebugTerminal(t *testing.T, events []sourceDebugStreamContractEvent, want string) sourceDebugStreamContractEvent {
	t.Helper()
	terminals := make([]sourceDebugStreamContractEvent, 0, 1)
	for _, event := range events {
		if event.Name == "end" || event.Name == "error" {
			terminals = append(terminals, event)
		}
	}
	if len(terminals) != 1 || terminals[0].Name != want {
		t.Fatalf("source debug terminals = %#v, want one %s", terminals, want)
	}
	return terminals[0]
}

func assertNoSourceDebugFailureRows(t *testing.T, server *Server, sourceID uint) {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.SourceFailure{}).Where("source_id = ?", sourceID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("source debug created %d source failure rows", count)
	}
}

func intValue(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	default:
		return 0
	}
}
