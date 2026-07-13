package api

import (
	"context"
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

func persistentVariableUser(t *testing.T, server *Server) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func persistedVariablesForAPITest(t *testing.T, raw string) map[string]string {
	t.Helper()
	values, err := models.SourceRuleVariableMap(raw)
	if err != nil {
		t.Fatalf("decode persisted variables %q: %v", raw, err)
	}
	return values
}

func TestRemoteBookPersistsSearchAndChapterSourceVariables(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := persistentVariableUser(t, server)

	const upstream = "https://persistent-variables.example"
	source := models.BookSource{Name: "持久变量远程源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoNameRule:   `@put:{"tocPath":".toc|attr:href"}.name|text`,
		BookInfoAuthorRule: "@get:{searchToken}",
		TOCURLRule:         "@get:{tocPath}",
		ChapterListRule:    ".chapter",
		ChapterNameRule:    `@put:{"chapterPath":".token|text"}.name|text`,
		ChapterURLRule:     "@get:{chapterPath}",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := ""
		switch request.URL.Path {
		case "/book/1":
			body = `<h1 class="name">持久变量书</h1><a class="toc" href="/toc/1">目录</a>`
		case "/toc/1":
			body = `<div class="chapter"><span class="token">/chapter/1</span><a class="name">第一章</a></div>`
		default:
			t.Fatalf("unexpected upstream request %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
	})})
	defer restoreHTTPClient()

	payload, err := json.Marshal(map[string]any{
		"title":    "搜索候选书",
		"bookUrl":  upstream + "/book/1",
		"sourceId": source.ID,
		"variable": `{"searchToken":"搜索令牌"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusCreated {
		t.Fatalf("create remote book = %d: %s", writer.Code, writer.Body.String())
	}

	var book models.Book
	if err := server.db.Where("user_id = ? AND url = ?", user.ID, upstream+"/book/1").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	bookVariables := persistedVariablesForAPITest(t, book.Variable)
	// reader-dev keeps the search-provided title unless this source explicitly
	// enables the detail-page rename rule. The variable state is independent of
	// that visible title decision.
	if book.Title != "搜索候选书" || book.Author != "搜索令牌" || bookVariables["searchToken"] != "搜索令牌" || bookVariables["tocPath"] != "/toc/1" {
		t.Fatalf("stored remote book = %+v, variables = %#v", book, bookVariables)
	}
	var chapter models.Chapter
	if err := server.db.Where("book_id = ? AND `index` = ?", book.ID, 0).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	chapterVariables := persistedVariablesForAPITest(t, chapter.Variable)
	if chapter.URL != upstream+"/chapter/1" || chapterVariables["chapterPath"] != "/chapter/1" {
		t.Fatalf("stored remote chapter = %+v, variables = %#v", chapter, chapterVariables)
	}
}

func TestRemoteChapterContentPersistsChapterSourceVariables(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	user := persistentVariableUser(t, server)

	const upstream = "https://content-variables.example"
	source := models.BookSource{Name: "正文持久变量源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule: `@put:{"contentToken":".token|text"}@get:{bookName}:@get:{title}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "正文持久书", URL: upstream + "/book/1", Variable: `{"bookToken":"书籍令牌"}`}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: upstream + "/chapter/1", Variable: `{"chapterToken":"章节令牌"}`}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<main class="content">忽略正文</main><span class="token">正文令牌</span>`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	defer restoreHTTPClient()

	content, err := server.loadChapterTextContextResult(context.Background(), book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	if content != "正文持久书:第一章" {
		t.Fatalf("content = %q", content)
	}
	var persistedBook models.Book
	var persistedChapter models.Chapter
	if err := server.db.First(&persistedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.First(&persistedChapter, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if values := persistedVariablesForAPITest(t, persistedBook.Variable); values["bookToken"] != "书籍令牌" {
		t.Fatalf("book variables changed unexpectedly: %#v", values)
	}
	if values := persistedVariablesForAPITest(t, persistedChapter.Variable); values["chapterToken"] != "章节令牌" || values["contentToken"] != "正文令牌" {
		t.Fatalf("chapter variables were not persisted: %#v", values)
	}
}

func TestSourceSemanticChangeClearsPersistentSourceVariables(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := persistentVariableUser(t, server)

	source := models.BookSource{Name: "清理持久变量源", BaseURL: "https://before.example", Charset: "utf-8", Rules: `{"bookInfoName":"h1|text"}`, Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "待清理", URL: "https://before.example/book", Variable: `{"staleBook":"x"}`}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://before.example/chapter", Variable: `{"staleChapter":"x"}`}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	updated := source
	updated.BaseURL = "https://after.example"
	body, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/sources/"+strconv.FormatUint(uint64(source.ID), 10), strings.NewReader(string(body)))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("update source = %d: %s", writer.Code, writer.Body.String())
	}
	if err := server.db.First(&book, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.First(&chapter, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if book.Variable != "" || chapter.Variable != "" {
		t.Fatalf("semantic source update must clear stale state: book=%q chapter=%q", book.Variable, chapter.Variable)
	}
}
