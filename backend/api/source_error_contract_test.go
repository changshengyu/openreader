package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestSourceRequestErrorsAreStructuredAndRedacted(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	source := sourceErrorContractSource(t)
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	restore := engine.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream https://alice:supersecret@source-errors.example/data?token=private-token failed")
	})})
	defer restore()

	t.Run("single paged search", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"keyword":"测试","sourceIds":[`+strconv.FormatUint(uint64(source.ID), 10)+`],"page":1}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		assertSourceErrorResponse(t, writer, http.StatusBadGateway, "failed to search source", "source_request_failed", "search")
	})

	t.Run("explore", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/explore/"+strconv.FormatUint(uint64(source.ID), 10), nil)
		req.Header.Set("Authorization", token)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		assertSourceErrorResponse(t, writer, http.StatusBadRequest, "failed to explore source", "source_request_failed", "explore")
	})

	t.Run("source debug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/test", strings.NewReader(`{"keyword":"测试"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		assertSourceErrorResponse(t, writer, http.StatusOK, "failed to request book source", "source_request_failed", "search")
	})

	t.Run("add remote book", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(`{"title":"测试书","bookUrl":"https://source-errors.example/book/1","sourceId":`+strconv.FormatUint(uint64(source.ID), 10)+`}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		assertSourceErrorResponse(t, writer, http.StatusBadRequest, "failed to fetch chapters", "source_request_failed", "book_info")
	})
}

func TestChapterContentRuleFailureHasStructuredCode(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	source := models.BookSource{Name: "正文规则错误源", BaseURL: "https://content-errors.example", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "规则错误书", URL: source.BaseURL + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: source.BaseURL + "/chapter/1"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
	req.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	assertSourceErrorResponse(t, writer, http.StatusBadGateway, "failed to load chapter content", "source_rule_invalid", "content")
}

func sourceErrorContractSource(t *testing.T) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: "结构化错误源", BaseURL: "https://source-errors.example", Charset: "utf-8", Enabled: true, EnabledExplore: boolPointer(true)}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "https://source-errors.example/search?q={keyword}",
		ExploreURL:   "https://source-errors.example/explore",
		BookListRule: ".book",
		BookNameRule: ".name|text",
		BookURLRule:  "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	return source
}

func assertSourceErrorResponse(t *testing.T, writer *httptest.ResponseRecorder, status int, message, code, stage string) {
	t.Helper()
	if writer.Code != status {
		t.Fatalf("status = %d, want %d: %s", writer.Code, status, writer.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(writer.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v: %s", err, writer.Body.String())
	}
	if payload["error"] != message || payload["code"] != code || payload["stage"] != stage {
		t.Fatalf("source error payload = %#v, want error=%q code=%q stage=%q", payload, message, code, stage)
	}
	body := writer.Body.String()
	for _, forbidden := range []string{"alice", "supersecret", "private-token", "token="} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("source error leaked %q: %s", forbidden, body)
		}
	}
	if payload["error"] == "" {
		t.Fatal("source error must remain visible to legacy clients")
	}
}
