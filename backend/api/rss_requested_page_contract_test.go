package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

type rssRequestedPageResponse struct {
	Items    []models.RSSArticle `json:"items"`
	Page     int                 `json:"page"`
	HasMore  bool                `json:"hasMore"`
	Imported int                 `json:"imported"`
	Total    int                 `json:"total"`
}

func TestRSSRefreshFetchesOnlyTheRequestedRulePage(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	requestedBodies := make([]string, 0, 2)
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			requestedBodies = append(requestedBodies, string(body))
			title := "第一页文章"
			link := "/post/1"
			if string(body) == "page=2" {
				title = "第二页文章"
				link = "/post/2"
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<article class="entry"><a class="title" href="` + link + `">` + title + `</a></article>`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{
		UserID:       user.ID,
		Title:        "分页 RSS",
		URL:          `https://rss.example/news, {"method":"POST","body":"page=<1,2>"}`,
		RuleArticles: ".entry",
		RuleNextPage: "PAGE",
		RuleTitle:    ".title",
		RuleLink:     ".title@href",
		Enabled:      true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	refresh := func(page int) rssRequestedPageResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh?page="+strconv.Itoa(page), nil)
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("page %d: expected 200, got %d: %s", page, w.Code, w.Body.String())
		}
		var response rssRequestedPageResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	pageOne := refresh(1)
	if strings.Join(requestedBodies, ",") != "page=1" || pageOne.Page != 1 || len(pageOne.Items) != 1 || pageOne.Items[0].Title != "第一页文章" || !pageOne.HasMore {
		t.Fatalf("page one eagerly crossed the request boundary: requests=%v response=%+v", requestedBodies, pageOne)
	}
	requestedBodies = requestedBodies[:0]

	pageTwo := refresh(2)
	if strings.Join(requestedBodies, ",") != "page=2" || pageTwo.Page != 2 || len(pageTwo.Items) != 1 || pageTwo.Items[0].Title != "第二页文章" || pageTwo.HasMore {
		t.Fatalf("page two did not remain one requested transition: requests=%v response=%+v", requestedBodies, pageTwo)
	}
	requestedBodies = requestedBodies[:0]

	pageThree := refresh(3)
	if len(requestedBodies) != 0 || pageThree.Page != 3 || len(pageThree.Items) != 0 || pageThree.HasMore {
		t.Fatalf("exhausted page template must end without refetching the last descriptor: requests=%v response=%+v", requestedBodies, pageThree)
	}
}

func TestRSSStandardFeedPageAfterOneIsEmptyWithoutRemoteFetch(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	requests := 0
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<rss><channel></channel></rss>`)), Header: make(http.Header), Request: request}, nil
	})})
	defer restoreHTTPClient()

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{UserID: user.ID, Title: "标准 RSS", URL: "https://rss.example/feed.xml", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh?page=2", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response rssRequestedPageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if requests != 0 || response.Page != 2 || response.HasMore || len(response.Items) != 0 {
		t.Fatalf("standard page two must be an empty local end: requests=%d response=%+v", requests, response)
	}
}

func TestRSSRefreshRejectsSortURLOutsideOwnedSourceOptions(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	requests := 0
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<rss><channel></channel></rss>`)), Header: make(http.Header), Request: request}, nil
	})})
	defer restoreHTTPClient()

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{UserID: user.ID, Title: "受限 RSS", URL: "https://rss.example/feed.xml", SingleURL: true, Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh?sortUrl=https%3A%2F%2Foutside.example%2Ffeed", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || requests != 0 || !strings.Contains(w.Body.String(), "invalid RSS sort URL") {
		t.Fatalf("arbitrary sort URL must fail before fetch: status=%d requests=%d body=%s", w.Code, requests, w.Body.String())
	}
}

func TestRSSSourceImportIsOneSelectedBatch(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	createReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"sourceName":"旧名称","sourceUrl":"https://rss.example/shared"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", token)
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create fixture source: %d %s", createW.Code, createW.Body.String())
	}

	payload := `[
		{"sourceName":"更新名称","sourceUrl":"https://rss.example/shared"},
		{"sourceName":"新增名称","sourceUrl":"https://rss.example/new"},
		{"sourceName":"","sourceUrl":"https://rss.example/skipped"}
	]`
	importReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources/import", strings.NewReader(payload))
	importReq.Header.Set("Content-Type", "application/json")
	importReq.Header.Set("Authorization", token)
	importW := httptest.NewRecorder()
	router.ServeHTTP(importW, importReq)
	if importW.Code != http.StatusOK || !strings.Contains(importW.Body.String(), `"created":1`) || !strings.Contains(importW.Body.String(), `"updated":1`) || !strings.Contains(importW.Body.String(), `"skipped":1`) {
		t.Fatalf("batch import response: %d %s", importW.Code, importW.Body.String())
	}

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	var sources []models.RSSSource
	if err := server.db.Where("user_id = ?", user.ID).Order("id asc").Find(&sources).Error; err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].Title != "更新名称" || sources[1].Title != "新增名称" || sources[1].SingleURL {
		t.Fatalf("unexpected imported sources: %+v", sources)
	}
}
