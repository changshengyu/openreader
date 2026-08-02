package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func contentSearchContractUser(t *testing.T, server *Server) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func createContentSearchCacheChapter(t *testing.T, server *Server, bookID uint, index int, content string) models.Chapter {
	t.Helper()
	cachePath := filepath.Join("content-search-contract", strconv.Itoa(index)+".txt")
	path := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: bookID, Index: index, Title: "第" + strconv.Itoa(index+1) + "章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return chapter
}

func TestContentSearchDoesNotSkipDenseFinalChapter(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{UserID: user.ID, Title: "密集正文搜索"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		lines = append(lines, "第"+strconv.Itoa(i+1)+"段目标词")
	}
	createContentSearchCacheChapter(t, server, book.ID, 0, strings.Join(lines, "\n"))
	createContentSearchCacheChapter(t, server, book.ID, 1, "下一章目标词")

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标词")+"&paged=1&lastIndex=-1&chapterLimit=2&matchLimit=3&perChapterLimit=3",
		nil,
	)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("dense content search: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page struct {
		List      []contentMatch `json:"list"`
		LastIndex int            `json:"lastIndex"`
		HasMore   bool           `json:"hasMore"`
		Truncated bool           `json:"truncated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 12 || page.LastIndex != 0 || !page.HasMore || page.Truncated {
		t.Fatalf("the final scanned chapter must be complete before its cursor advances: %+v", page)
	}
}

func TestContentSearchReportsUnavailableRemoteChapters(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	source := models.BookSource{
		Name:    "网络不可用搜索源",
		BaseURL: "https://unavailable.example",
		Charset: "utf-8",
	}
	if err := source.SetRules(models.BookSourceRule{ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "不可用搜索书", SourceID: source.ID}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://unavailable.example/chapter"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	restoreClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return nil, errors.New("fixture chapter fetch unavailable")
		}),
	})
	defer restoreClient()

	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=1", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unavailable chapter search: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page struct {
		List                []contentMatch `json:"list"`
		Incomplete          bool           `json:"incomplete"`
		UnavailableChapters int            `json:"unavailableChapters"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 0 || !page.Incomplete || page.UnavailableChapters != 1 {
		t.Fatalf("an unavailable remote scan must not masquerade as an ordinary empty result: %+v", page)
	}
}

func TestContentSearchMakesSafetyTruncationExplicit(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{UserID: user.ID, Title: "搜索安全上限"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	createContentSearchCacheChapter(t, server, book.ID, 0, strings.Repeat("目标\n", contentSearchMaxMatchesPerChapter+1))

	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=1&matchLimit=1", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("safety-capped content search: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page struct {
		List       []contentMatch `json:"list"`
		Incomplete bool           `json:"incomplete"`
		Truncated  bool           `json:"truncated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.List) != contentSearchMaxMatchesPerChapter || !page.Incomplete || !page.Truncated {
		t.Fatalf("a safety cap must remain visible instead of silently skipping matches: %+v", page)
	}
}

func TestContentSearchUsesRawExactOverlappingChapterText(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{UserID: user.ID, Title: "原始精确正文搜索"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	createContentSearchCacheChapter(t, server, book.ID, 0, "aaaa\n原始目标\n目 标\nAb")
	plain := false
	if err := server.db.Create(&models.ReplaceRule{
		UserID:      user.ID,
		Name:        "搜索不得套用的替换规则",
		Pattern:     "原始目标",
		Replacement: "替换后",
		Scope:       "*",
		IsRegex:     &plain,
		Enabled:     true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	search := func(query string) []contentMatch {
		t.Helper()
		request := httptest.NewRequest(
			http.MethodGet,
			"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape(query),
			nil,
		)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("search %q: expected 200, got %d: %s", query, response.Code, response.Body.String())
		}
		var matches []contentMatch
		if err := json.Unmarshal(response.Body.Bytes(), &matches); err != nil {
			t.Fatal(err)
		}
		return matches
	}

	if matches := search("原始目标"); len(matches) != 1 {
		t.Fatalf("raw text must remain searchable before Reader replacement, got %+v", matches)
	}
	if matches := search("替换后"); len(matches) != 0 {
		t.Fatalf("Reader replacement output must not become search input, got %+v", matches)
	}
	if matches := search("aa"); len(matches) != 3 {
		t.Fatalf("upstream exact search must keep overlapping positions 0,1,2, got %+v", matches)
	}
	if matches := search("目标"); len(matches) != 1 {
		t.Fatalf("punctuation/whitespace normalization must not fabricate a second result, got %+v", matches)
	}
	if matches := search("ab"); len(matches) != 0 {
		t.Fatalf("upstream exact search is case-sensitive, got %+v", matches)
	}
}

func TestContentSearchPreservesLeadingTrailingAndAllSpaceQueries(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{
		UserID: user.ID,
		Title:  "原样空白正文搜索",
		URL:    "https://raw-query.example/book",
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	createContentSearchCacheChapter(t, server, book.ID, 0, "目标\n前文 目标 后文\n一   二")

	modernSearch := func(query string) []contentMatch {
		t.Helper()
		request := httptest.NewRequest(
			http.MethodGet,
			"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape(query),
			nil,
		)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("modern raw query %q: expected 200, got %d: %s", query, response.Code, response.Body.String())
		}
		var matches []contentMatch
		if err := json.Unmarshal(response.Body.Bytes(), &matches); err != nil {
			t.Fatal(err)
		}
		return matches
	}

	for _, query := range []string{" 目标 ", "   "} {
		matches := modernSearch(query)
		if len(matches) != 1 || matches[0].Query != query {
			t.Fatalf("modern query must remain exact %q, got %+v", query, matches)
		}

		body, err := json.Marshal(legacySearchBookContentRequest{
			BookURL: book.URL,
			Keyword: query,
			LastIndex: func() *int {
				value := -1
				return &value
			}(),
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/reader3/searchBookContent", strings.NewReader(string(body)))
		request.Header.Set("Authorization", token)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("legacy raw query %q: expected 200, got %d: %s", query, response.Code, response.Body.String())
		}
		var payload struct {
			IsSuccess bool `json:"isSuccess"`
			Data      struct {
				List []legacyContentMatch `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if !payload.IsSuccess || len(payload.Data.List) != 1 || payload.Data.List[0].Query != query {
			t.Fatalf("legacy query must remain exact %q, got %+v", query, payload)
		}
	}
}

func TestContentSearchRejectsMissingConfiguredSourceBeforeScanning(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{
		UserID:   user.ID,
		Title:    "缺失书源正文搜索",
		URL:      "https://missing-source.example/book",
		SourceID: 999999,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{
		BookID: book.ID,
		Index:  0,
		Title:  "第一章",
		URL:    "https://missing-source.example/chapter",
	}).Error; err != nil {
		t.Fatal(err)
	}

	modern := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1",
		nil,
	)
	modern.Header.Set("Authorization", token)
	modernResponse := httptest.NewRecorder()
	router.ServeHTTP(modernResponse, modern)
	if modernResponse.Code != http.StatusBadRequest {
		t.Fatalf("modern missing-source search: expected 400, got %d: %s", modernResponse.Code, modernResponse.Body.String())
	}
	var modernBody map[string]any
	if err := json.Unmarshal(modernResponse.Body.Bytes(), &modernBody); err != nil {
		t.Fatal(err)
	}
	if modernBody["error"] != "未配置书源" {
		t.Fatalf("modern missing-source error = %#v", modernBody["error"])
	}

	body := `{"bookUrl":"https://missing-source.example/book","keyword":"目标","lastIndex":-1,"size":20}`
	legacy := httptest.NewRequest(http.MethodPost, "/api/reader3/searchBookContent", strings.NewReader(body))
	legacy.Header.Set("Authorization", token)
	legacy.Header.Set("Content-Type", "application/json")
	legacyResponse := httptest.NewRecorder()
	router.ServeHTTP(legacyResponse, legacy)
	if legacyResponse.Code != http.StatusOK {
		t.Fatalf("legacy missing-source search: expected 200, got %d: %s", legacyResponse.Code, legacyResponse.Body.String())
	}
	var legacyBody struct {
		IsSuccess bool   `json:"isSuccess"`
		ErrorMsg  string `json:"errorMsg"`
	}
	if err := json.Unmarshal(legacyResponse.Body.Bytes(), &legacyBody); err != nil {
		t.Fatal(err)
	}
	if legacyBody.IsSuccess || legacyBody.ErrorMsg != "未配置书源" {
		t.Fatalf("legacy missing-source response = %+v", legacyBody)
	}
}

func TestLegacyContentSearchReturnsUpstreamIndexesAndExcerptWidth(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{
		UserID: user.ID,
		Title:  "兼容搜索字段",
		URL:    "https://legacy-fields.example/book",
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	createContentSearchCacheChapter(
		t,
		server,
		book.ID,
		0,
		strings.Repeat("L", 30)+"TARGET"+strings.Repeat("R", 30),
	)

	body := `{"bookUrl":"https://legacy-fields.example/book","keyword":"TARGET","lastIndex":-1,"size":20}`
	request := httptest.NewRequest(http.MethodPost, "/api/reader3/searchBookContent", strings.NewReader(body))
	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy content search: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		IsSuccess bool `json:"isSuccess"`
		Data      struct {
			List []struct {
				ResultText          string `json:"resultText"`
				QueryIndexInResult  int    `json:"queryIndexInResult"`
				QueryIndexInChapter int    `json:"queryIndexInChapter"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.IsSuccess || len(payload.Data.List) != 1 {
		t.Fatalf("legacy content search payload = %+v", payload)
	}
	result := payload.Data.List[0]
	if result.QueryIndexInResult != 20 || result.QueryIndexInChapter != 30 {
		t.Fatalf("legacy query indexes = %+v", result)
	}
	if result.ResultText != strings.Repeat("L", 20)+"TARGET"+strings.Repeat("R", 20) {
		t.Fatalf("legacy excerpt width = %q", result.ResultText)
	}
}

func TestContentSearchStopsSchedulingRemoteChaptersAfterCancellation(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "cancel-search-user", PasswordHash: "not-used"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "取消搜索源", BaseURL: "https://search-cancel.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "取消正文搜索", SourceID: source.ID}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://search-cancel.example/1"},
		{BookID: book.ID, Index: 1, Title: "第二章", URL: "https://search-cancel.example/2"},
	}
	if err := server.db.Create(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make([]string, 0, 2)
	restoreClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.URL.Path)
			cancel()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`<main class="content">目标正文</main>`)),
				Request:    request,
			}, nil
		}),
	})
	defer restoreClient()

	scan := server.collectContentMatchesContext(ctx, book, chapters, "目标", 0, 2, 20, 20)
	if !scan.Canceled || len(requests) != 1 || len(scan.Matches) != 0 {
		t.Fatalf("cancellation must stop the search before later chapter requests: scan=%+v requests=%v", scan, requests)
	}
}

func TestLegacyContentSearchPropagatesRequestCancellation(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	source := models.BookSource{Name: "兼容接口取消搜索源", BaseURL: "https://legacy-search-cancel.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:   user.ID,
		Title:    "兼容接口取消正文搜索",
		URL:      "https://legacy-search-cancel.example/book",
		SourceID: source.ID,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://legacy-search-cancel.example/1"},
		{BookID: book.ID, Index: 1, Title: "第二章", URL: "https://legacy-search-cancel.example/2"},
	}
	if err := server.db.Create(&chapters).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make([]string, 0, 2)
	restoreClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.URL.Path)
			cancel()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`<main class="content">没有命中</main>`)),
				Request:    request,
			}, nil
		}),
	})
	defer restoreClient()

	body := `{"bookUrl":"https://legacy-search-cancel.example/book","keyword":"目标","lastIndex":-1,"size":20}`
	request := httptest.NewRequest(http.MethodPost, "/api/reader3/searchBookContent", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if len(requests) != 1 {
		t.Fatalf("legacy cancellation must stop before the next chapter request: %v", requests)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("a canceled compatibility search must not serialize false success: %s", response.Body.String())
	}
}
