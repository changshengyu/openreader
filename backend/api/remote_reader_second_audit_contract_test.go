package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/remotereader"
)

func TestRemoteReaderCreateRejectsOversizedPayloadBeforeSourceWork(t *testing.T) {
	router, _ := setupTestServer(t)
	token := registerLifecycleToken(t, router, "remotereaderpayload")
	var requests atomic.Int32
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<h1>unexpected</h1>`)), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	payload, err := json.Marshal(map[string]any{
		"title":    "oversized",
		"bookUrl":  "https://remote-reader-payload.test/book",
		"sourceId": 1,
		"intro":    strings.Repeat("x", 64*1024),
	})
	if err != nil {
		t.Fatal(err)
	}
	validPrefix := `{"title":"oversized trailing body","bookUrl":"https://remote-reader-payload.test/book","sourceId":1}`
	for _, test := range []struct {
		name          string
		body          string
		contentLength int64
	}{
		{name: "oversized JSON field", body: string(payload), contentLength: int64(len(payload))},
		{name: "chunked trailing data", body: validPrefix + strings.Repeat(" ", 64*1024), contentLength: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/reader/remote-sessions", strings.NewReader(test.body))
			request.ContentLength = test.contentLength
			request.Header.Set("Authorization", token)
			request.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, request)
			if writer.Code != http.StatusRequestEntityTooLarge || !strings.Contains(writer.Body.String(), "remote reader payload too large") {
				t.Fatalf("oversized create = %d %s, want safe 413", writer.Code, writer.Body.String())
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("oversized create made %d source requests", requests.Load())
	}
}

func TestRemoteReaderCreateValidatesRequiredFieldsAndVariablesBeforeSourceWork(t *testing.T) {
	router, _ := setupTestServer(t)
	token := registerLifecycleToken(t, router, "remotereadervalidation")
	var requests atomic.Int32
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("validation must stop before transport")
	})})
	defer restore()

	for _, test := range []struct {
		name string
		body string
		code string
	}{
		{name: "missing title", body: `{"bookUrl":"https://remote-reader-validation.test/book","sourceId":1}`},
		{name: "invalid variable", body: `{"title":"invalid variable","bookUrl":"https://remote-reader-validation.test/book","sourceId":1,"variable":"{\"bad\":1}"}`, code: "source_rule_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/reader/remote-sessions", strings.NewReader(test.body))
			request.Header.Set("Authorization", token)
			request.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			router.ServeHTTP(writer, request)
			if writer.Code != http.StatusBadRequest {
				t.Fatalf("validation = %d %s, want 400", writer.Code, writer.Body.String())
			}
			if test.code != "" && !strings.Contains(writer.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("validation response missing code %q: %s", test.code, writer.Body.String())
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid creates made %d source requests", requests.Load())
	}
}

func TestRemoteReaderErrorsAreRedactedAndFailureCacheIsTyped(t *testing.T) {
	router, server := setupTestServer(t)
	username := "remotereadererrors"
	token := registerLifecycleToken(t, router, username)
	userID := sourceFailureUserID(t, server, username)

	requestSource := models.BookSource{Name: "request error", BaseURL: "https://remote-reader-errors.test", Charset: "utf-8", Enabled: true}
	if err := requestSource.SetRules(models.BookSourceRule{ContentRule: ".content|text"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&requestSource).Error; err != nil {
		t.Fatal(err)
	}
	requestSession, err := server.remoteReaders.Create(userID, requestSource, models.Book{Title: "request error"}, []models.Chapter{{Index: 0, Title: "one", URL: requestSource.BaseURL + "/chapter?token=query-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream https://alice:password@remote-reader-errors.test/chapter?token=query-secret header cookie-secret proxy proxy-secret")
	})})
	request := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+requestSession.ID+"/chapters/0/content", nil)
	request.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, request)
	restore()
	assertSourceErrorResponse(t, writer, http.StatusBadGateway, "failed to load chapter content", "source_request_failed", "content")
	for _, secret := range []string{"alice", "password", "query-secret", "cookie-secret", "proxy-secret", "token="} {
		if strings.Contains(writer.Body.String(), secret) {
			t.Fatalf("remote reader error leaked %q: %s", secret, writer.Body.String())
		}
	}
	var requestFailures int64
	if err := server.db.Model(&models.SourceFailure{}).Where("user_id = ? AND source_id = ?", userID, requestSource.ID).Count(&requestFailures).Error; err != nil {
		t.Fatal(err)
	}
	if requestFailures != 1 {
		t.Fatalf("typed request failure rows = %d, want 1", requestFailures)
	}

	ruleSource := models.BookSource{Name: "rule error", BaseURL: "https://remote-reader-rule-errors.test", Charset: "utf-8", Enabled: true}
	if err := ruleSource.SetRules(models.BookSourceRule{ContentRule: "{{raw-rule-secret}}"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&ruleSource).Error; err != nil {
		t.Fatal(err)
	}
	ruleSession, err := server.remoteReaders.Create(userID, ruleSource, models.Book{Title: "rule error"}, []models.Chapter{{Index: 0, Title: "one", URL: ruleSource.BaseURL + "/chapter"}})
	if err != nil {
		t.Fatal(err)
	}
	restore = engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<main class="content">body-secret</main>`)), Header: make(http.Header), Request: request}, nil
	})})
	ruleRequest := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+ruleSession.ID+"/chapters/0/content", nil)
	ruleRequest.Header.Set("Authorization", token)
	ruleWriter := httptest.NewRecorder()
	router.ServeHTTP(ruleWriter, ruleRequest)
	restore()
	assertSourceErrorResponse(t, ruleWriter, http.StatusBadGateway, "failed to load chapter content", "source_rule_unsupported", "content")
	for _, secret := range []string{"raw-rule-secret", "body-secret", "{{"} {
		if strings.Contains(ruleWriter.Body.String(), secret) {
			t.Fatalf("remote reader rule error leaked %q: %s", secret, ruleWriter.Body.String())
		}
	}
	var ruleFailures int64
	if err := server.db.Model(&models.SourceFailure{}).Where("user_id = ? AND source_id = ?", userID, ruleSource.ID).Count(&ruleFailures).Error; err != nil {
		t.Fatal(err)
	}
	if ruleFailures != 0 {
		t.Fatalf("local rule error created %d failure rows", ruleFailures)
	}
}

func TestRemoteReaderInvalidChapterIndexDoesNotRenewLease(t *testing.T) {
	router, server := setupTestServer(t)
	username := "remotereaderinvalidindex"
	token := registerLifecycleToken(t, router, username)
	userID := sourceFailureUserID(t, server, username)
	now := time.Date(2026, 8, 9, 11, 0, 0, 0, time.UTC)
	server.remoteReaders = remotereader.NewStore(remotereader.DefaultLimits(), func() time.Time { return now })
	session, err := server.remoteReaders.Create(userID, models.BookSource{Name: "index"}, models.Book{Title: "index"}, []models.Chapter{{Index: 0, Title: "one"}})
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(29 * time.Minute)
	invalid := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+session.ID+"/chapters/not-a-number/content", nil)
	invalid.Header.Set("Authorization", token)
	invalidWriter := httptest.NewRecorder()
	router.ServeHTTP(invalidWriter, invalid)
	if invalidWriter.Code != http.StatusBadRequest {
		t.Fatalf("invalid index = %d %s", invalidWriter.Code, invalidWriter.Body.String())
	}

	now = session.CreatedAt.Add(31 * time.Minute)
	lookup := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+session.ID, nil)
	lookup.Header.Set("Authorization", token)
	lookupWriter := httptest.NewRecorder()
	router.ServeHTTP(lookupWriter, lookup)
	if lookupWriter.Code != http.StatusGone {
		t.Fatalf("invalid index renewed lease: lookup = %d %s, want 410", lookupWriter.Code, lookupWriter.Body.String())
	}
}

func TestRemoteReaderContentPreservesBookAndPerChapterVariables(t *testing.T) {
	router, server := setupTestServer(t)
	username := "remotereadervariables"
	token := registerLifecycleToken(t, router, username)
	userID := sourceFailureUserID(t, server, username)

	source := models.BookSource{ID: 91, Name: "variables", BaseURL: "https://remote-reader-variables.test", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{ContentRule: `@put:{"seen":".token|text"}@get:{global}:@get:{local}`}); err != nil {
		t.Fatal(err)
	}
	session, err := server.remoteReaders.Create(userID, source, models.Book{Title: "变量书", Variable: `{"global":"书籍变量"}`}, []models.Chapter{
		{Index: 0, Title: "第一章", URL: source.BaseURL + "/one", Variable: `{"local":"章节一"}`},
		{Index: 1, Title: "第二章", URL: source.BaseURL + "/two", Variable: `{"local":"章节二"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		token := "seen-one"
		if request.URL.Path == "/two" {
			token = "seen-two"
		}
		body := `<span class="token">` + token + `</span>`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	for index, want := range []string{"书籍变量:章节一", "书籍变量:章节二"} {
		request := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+session.ID+"/chapters/"+strconv.Itoa(index)+"/content", nil)
		request.Header.Set("Authorization", token)
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, request)
		if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), want) {
			t.Fatalf("chapter %d = %d %s, want %q", index, writer.Code, writer.Body.String(), want)
		}
	}

	updated, err := server.remoteReaders.Get(userID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var bookVariables map[string]string
	if err := json.Unmarshal([]byte(updated.Book.Variable), &bookVariables); err != nil {
		t.Fatal(err)
	}
	if bookVariables["global"] != "书籍变量" || len(bookVariables) != 1 {
		t.Fatalf("book variables changed during content: %#v", bookVariables)
	}
	for index, want := range []string{"seen-one", "seen-two"} {
		var chapterVariables map[string]string
		if err := json.Unmarshal([]byte(updated.Chapters[index].Variable), &chapterVariables); err != nil {
			t.Fatal(err)
		}
		if chapterVariables["local"] != "章节"+[]string{"一", "二"}[index] || chapterVariables["seen"] != want {
			t.Fatalf("chapter %d variables = %#v", index, chapterVariables)
		}
	}
}

func TestRemoteReaderCanceledContentDoesNotCommitVariablesOrFailure(t *testing.T) {
	router, server := setupTestServer(t)
	username := "remotereadercancel"
	token := registerLifecycleToken(t, router, username)
	userID := sourceFailureUserID(t, server, username)
	source := models.BookSource{ID: 92, Name: "cancel", BaseURL: "https://remote-reader-cancel.test", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{ContentRule: `@put:{"seen":".token|text"}.content|text`}); err != nil {
		t.Fatal(err)
	}
	session, err := server.remoteReaders.Create(userID, source, models.Book{Title: "取消书", Variable: `{"book":"before"}`}, []models.Chapter{{Index: 0, Title: "第一章", URL: source.BaseURL + "/one", Variable: `{"chapter":"before"}`}})
	if err != nil {
		t.Fatal(err)
	}
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})})
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+session.ID+"/chapters/0/content", nil).WithContext(ctx)
	request.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, request)
	if strings.TrimSpace(writer.Body.String()) != "" {
		t.Fatalf("canceled request returned synthetic body: %s", writer.Body.String())
	}

	updated, err := server.remoteReaders.Get(userID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Book.Variable != `{"book":"before"}` || updated.Chapters[0].Variable != `{"chapter":"before"}` {
		t.Fatalf("canceled request committed variables: book=%q chapter=%q", updated.Book.Variable, updated.Chapters[0].Variable)
	}
	var failures int64
	if err := server.db.Model(&models.SourceFailure{}).Where("user_id = ? AND source_id = ?", userID, source.ID).Count(&failures).Error; err != nil {
		t.Fatal(err)
	}
	if failures != 0 {
		t.Fatalf("canceled request recorded %d source failures", failures)
	}
}
