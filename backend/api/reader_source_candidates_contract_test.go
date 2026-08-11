package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestSourceCandidatesAvailableSeedsWithoutRemoteRequestAndIsolatesOwner(t *testing.T) {
	router, server := setupTestServer(t)
	ownerToken := registerLifecycleToken(t, router, "candidateavailableowner")
	otherToken := registerLifecycleToken(t, router, "candidateavailableother")
	owner := sourceCandidateContractUser(t, server, "candidateavailableowner")
	source := sourceCandidateContractSource(t, server, "当前来源", "默认", "https://available.example")
	book := models.Book{
		UserID:      owner.ID,
		SourceID:    source.ID,
		Type:        source.SourceType,
		Title:       "历史候选书",
		Author:      "历史作者",
		URL:         "https://available.example/current",
		LastChapter: "第十章",
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	remoteCalls := 0
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		remoteCalls++
		return nil, errors.New("available mode must not make a remote request")
	})})
	defer restoreHTTPClient()

	response := sourceCandidateContractRequest(t, router, ownerToken, book.ID, "")
	if response.Code != http.StatusOK {
		t.Fatalf("available source candidates: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var rows []sourceCandidateContractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Current || rows[0].BookURL != book.URL || rows[0].SourceID != source.ID {
		t.Fatalf("historical current candidate was not seeded: %+v", rows)
	}
	if remoteCalls != 0 {
		t.Fatalf("available mode made %d remote requests", remoteCalls)
	}

	var stored []models.BookSourceCandidate
	if err := server.db.Where("user_id = ? AND book_id = ?", owner.ID, book.ID).Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].BookURL != book.URL {
		t.Fatalf("unexpected persisted candidates: %+v", stored)
	}

	foreign := sourceCandidateContractRequest(t, router, otherToken, book.ID, "mode=available")
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign candidate read: expected 404, got %d: %s", foreign.Code, foreign.Body.String())
	}
	var foreignCount int64
	other := sourceCandidateContractUser(t, server, "candidateavailableother")
	if err := server.db.Model(&models.BookSourceCandidate{}).Where("user_id = ?", other.ID).Count(&foreignCount).Error; err != nil {
		t.Fatal(err)
	}
	if foreignCount != 0 {
		t.Fatalf("foreign read created %d candidate rows", foreignCount)
	}
}

func TestSourceCandidatesSearchUsesExactIdentityStableGroupCursorAndCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := registerLifecycleToken(t, router, "candidatesearchowner")
	owner := sourceCandidateContractUser(t, server, "candidatesearchowner")
	first := sourceCandidateContractSource(t, server, "第一来源", "优先", "https://first-candidate.example")
	second := sourceCandidateContractSource(t, server, "第二来源", "优先", "https://second-candidate.example")
	outside := sourceCandidateContractSource(t, server, "其他分组", "其他", "https://outside-candidate.example")
	book := models.Book{
		UserID:   owner.ID,
		SourceID: first.ID,
		Title:    "精确书名",
		Author:   "精确作者",
		URL:      "https://first-candidate.example/current",
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	var callsMu sync.Mutex
	calls := make([]string, 0)
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		callsMu.Lock()
		calls = append(calls, request.URL.Host)
		callsMu.Unlock()
		body := ""
		switch request.URL.Host {
		case "first-candidate.example":
			body = sourceCandidateSearchHTML(
				[]string{"精确书名", "错误书名", "精确书名"},
				[]string{"精确作者", "精确作者", "错误作者"},
				[]string{"/first-exact", "/wrong-title", "/wrong-author"},
			)
		case "second-candidate.example":
			body = sourceCandidateSearchHTML(
				[]string{"精确书名"},
				[]string{"精确作者"},
				[]string{"/second-exact"},
			)
		case "outside-candidate.example":
			body = sourceCandidateSearchHTML(
				[]string{"精确书名"},
				[]string{"精确作者"},
				[]string{"/must-not-run"},
			)
		default:
			return nil, errors.New("unexpected candidate host")
		}
		return sourceCandidateHTTPResponse(request, body), nil
	})})
	defer restoreHTTPClient()

	query := "mode=search&group=" + "%E4%BC%98%E5%85%88" + "&offset=0&limit=2&paged=1"
	response := sourceCandidateContractRequest(t, router, token, book.ID, query)
	if response.Code != http.StatusOK {
		t.Fatalf("search source candidates: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var batch struct {
		List       []sourceCandidateContractResponse `json:"list"`
		Offset     int                               `json:"offset"`
		NextOffset int                               `json:"nextOffset"`
		HasMore    bool                              `json:"hasMore"`
		Total      int                               `json:"total"`
		Searched   int                               `json:"searched"`
		Matched    int                               `json:"matched"`
		Failed     int                               `json:"failed"`
		Empty      int                               `json:"empty"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.List) != 2 || batch.List[0].SourceID != first.ID || batch.List[1].SourceID != second.ID {
		t.Fatalf("search did not retain exact matches in stable source order: %+v", batch)
	}
	if batch.List[0].BookURL != "https://first-candidate.example/first-exact" ||
		batch.List[1].BookURL != "https://second-candidate.example/second-exact" {
		t.Fatalf("search accepted a title/author mismatch: %+v", batch.List)
	}
	if batch.Offset != 0 || batch.NextOffset != 2 || batch.HasMore || batch.Total != 2 ||
		batch.Searched != 2 || batch.Matched != 2 || batch.Failed != 0 || batch.Empty != 0 {
		t.Fatalf("unexpected stable cursor metadata: %+v", batch)
	}
	callsMu.Lock()
	gotCalls := append([]string(nil), calls...)
	callsMu.Unlock()
	sort.Strings(gotCalls)
	if strings.Join(gotCalls, ",") != "first-candidate.example,second-candidate.example" {
		t.Fatalf("group search contacted unexpected sources: %v (outside id %d)", gotCalls, outside.ID)
	}

	var stored []models.BookSourceCandidate
	if err := server.db.Where("user_id = ? AND book_id = ?", owner.ID, book.ID).Order("sort_order asc").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 || stored[0].BookURL != batch.List[0].BookURL || stored[1].BookURL != batch.List[1].BookURL {
		t.Fatalf("exact search batch was not merged into the derived cache: %+v", stored)
	}
}

func TestSourceCandidatesRefreshTouchesOnlyCachedSourcesAndRetainsCurrentOnFailure(t *testing.T) {
	router, server := setupTestServer(t)
	token := registerLifecycleToken(t, router, "candidaterefreshowner")
	owner := sourceCandidateContractUser(t, server, "candidaterefreshowner")
	currentSource := sourceCandidateContractSource(t, server, "当前失败源", "默认", "https://current-fail.example")
	goodSource := sourceCandidateContractSource(t, server, "缓存成功源", "默认", "https://cached-good.example")
	uncachedSource := sourceCandidateContractSource(t, server, "未缓存源", "默认", "https://uncached.example")
	book := models.Book{
		UserID:      owner.ID,
		SourceID:    currentSource.ID,
		Title:       "刷新书名",
		Author:      "刷新作者",
		URL:         "https://current-fail.example/current",
		LastChapter: "当前章",
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	seed := []models.BookSourceCandidate{
		{
			UserID: owner.ID, BookID: book.ID, SourceID: currentSource.ID,
			SourceURL: currentSource.BaseURL, SourceName: currentSource.Name, SourceGroup: currentSource.Group,
			Title: book.Title, Author: book.Author, BookURL: book.URL, LatestChapterTitle: book.LastChapter, SortOrder: 1,
		},
		{
			UserID: owner.ID, BookID: book.ID, SourceID: goodSource.ID,
			SourceURL: goodSource.BaseURL, SourceName: goodSource.Name, SourceGroup: goodSource.Group,
			Title: book.Title, Author: book.Author, BookURL: "https://cached-good.example/old", LatestChapterTitle: "旧章", SortOrder: 2,
		},
	}
	if err := server.db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}

	var callsMu sync.Mutex
	calls := make([]string, 0)
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		callsMu.Lock()
		calls = append(calls, request.URL.Host)
		callsMu.Unlock()
		switch request.URL.Host {
		case "current-fail.example":
			return nil, errors.New("current source unavailable")
		case "cached-good.example":
			return sourceCandidateHTTPResponse(request, sourceCandidateSearchHTML(
				[]string{"刷新书名"}, []string{"刷新作者"}, []string{"/new"},
			)), nil
		case "uncached.example":
			return nil, errors.New("refresh contacted an uncached source")
		default:
			return nil, errors.New("unexpected refresh host")
		}
	})})
	defer restoreHTTPClient()

	response := sourceCandidateContractRequest(t, router, token, book.ID, "mode=refresh")
	if response.Code != http.StatusOK {
		t.Fatalf("refresh source candidates: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var rows []sourceCandidateContractResponse
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].Current || rows[0].BookURL != book.URL ||
		rows[1].BookURL != "https://cached-good.example/new" {
		t.Fatalf("refresh did not replace successes while retaining current: %+v", rows)
	}
	callsMu.Lock()
	gotCalls := append([]string(nil), calls...)
	callsMu.Unlock()
	if len(gotCalls) != 2 || strings.Contains(strings.Join(gotCalls, ","), "uncached.example") {
		t.Fatalf("refresh must touch only cached source ids, got %v (uncached id %d)", gotCalls, uncachedSource.ID)
	}

	var stored []models.BookSourceCandidate
	if err := server.db.Where("user_id = ? AND book_id = ?", owner.ID, book.ID).Order("sort_order asc").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 || stored[0].BookURL != book.URL || stored[1].BookURL != "https://cached-good.example/new" {
		t.Fatalf("refresh cache replacement is incorrect: %+v", stored)
	}

	illegal := sourceCandidateContractRequest(t, router, token, book.ID, "mode=everything")
	if illegal.Code != http.StatusBadRequest {
		t.Fatalf("unknown candidate mode: expected 400, got %d: %s", illegal.Code, illegal.Body.String())
	}
}

type sourceCandidateContractResponse struct {
	SourceID           uint   `json:"sourceId"`
	SourceName         string `json:"sourceName"`
	Group              string `json:"group"`
	Title              string `json:"title"`
	Author             string `json:"author"`
	BookURL            string `json:"bookUrl"`
	LatestChapterTitle string `json:"latestChapterTitle"`
	Current            bool   `json:"current"`
}

func sourceCandidateContractUser(t *testing.T, server *Server, username string) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func sourceCandidateContractSource(t *testing.T, server *Server, name, group, baseURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{
		Name: name, Group: group, BaseURL: baseURL, Charset: "utf-8", Enabled: true,
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:      baseURL + "/search?q={keyword}",
		BookListRule:   ".book",
		BookNameRule:   ".title|text",
		BookAuthorRule: ".author|text",
		BookURLRule:    ".link|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	return source
}

func sourceCandidateContractRequest(t *testing.T, router http.Handler, token string, bookID uint, query string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/books/" + strconv.FormatUint(uint64(bookID), 10) + "/source-candidates"
	if query != "" {
		path += "?" + query
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func sourceCandidateSearchHTML(titles, authors, urls []string) string {
	var body strings.Builder
	body.WriteString("<html><body>")
	for index := range titles {
		body.WriteString(`<div class="book"><a class="link" href="`)
		body.WriteString(urls[index])
		body.WriteString(`"><span class="title">`)
		body.WriteString(titles[index])
		body.WriteString(`</span></a><span class="author">`)
		body.WriteString(authors[index])
		body.WriteString(`</span></div>`)
	}
	body.WriteString("</body></html>")
	return body.String()
}

func sourceCandidateHTTPResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}
}
