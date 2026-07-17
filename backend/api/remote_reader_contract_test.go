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

func TestRemoteReaderSessionIsEphemeralUserBoundAndLoadsContent(t *testing.T) {
	router, server := setupTestServer(t)
	ownerToken := registerLifecycleToken(t, router, "remotereaderowner")
	otherToken := registerLifecycleToken(t, router, "remotereaderother")

	const upstream = "https://remote-reader-contract.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := `<main><h1 class="title">解析后的书名</h1><span class="author">解析作者</span><div class="chapter"><span class="chapter-title">第一章</span><a href="/chapter/1">阅读</a></div></main>`
			if request.URL.Path == "/chapter/1" {
				body = `<main class="content">临时阅读正文</main>`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "临时阅读契约源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoNameRule:   ".title",
		BookInfoAuthorRule: ".author",
		ChapterListRule:    ".chapter",
		ChapterNameRule:    ".chapter-title|text",
		ChapterURLRule:     "a|attr:href",
		ContentRule:        ".content",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	requestBody := `{"title":"搜索标题","author":"搜索作者","bookUrl":"` + upstream + `/book","sourceId":` + strconv.FormatUint(uint64(source.ID), 10) + `}`
	request := httptest.NewRequest(http.MethodPost, "/api/reader/remote-sessions", strings.NewReader(requestBody))
	request.Header.Set("Authorization", ownerToken)
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, request)
	if writer.Code != http.StatusCreated {
		t.Fatalf("create remote reader session: expected 201, got %d: %s", writer.Code, writer.Body.String())
	}
	if writer.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create remote reader session: expected Cache-Control no-store, got %q", writer.Header().Get("Cache-Control"))
	}

	var created struct {
		ID       string           `json:"id"`
		Book     models.Book      `json:"book"`
		Chapters []models.Chapter `json:"chapters"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Book.ID != 0 || created.Book.Title != "搜索标题" || len(created.Chapters) != 1 {
		t.Fatalf("unexpected ephemeral session response: %+v", created)
	}

	assertRemoteReaderRuntimeLeavesNoShelfRows(t, server)

	readRequest := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+created.ID+"/chapters/0/content", nil)
	readRequest.Header.Set("Authorization", ownerToken)
	readWriter := httptest.NewRecorder()
	router.ServeHTTP(readWriter, readRequest)
	if readWriter.Code != http.StatusOK || !strings.Contains(readWriter.Body.String(), "临时阅读正文") {
		t.Fatalf("load remote session content: code=%d body=%s", readWriter.Code, readWriter.Body.String())
	}
	if readWriter.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("load remote session content: expected Cache-Control no-store, got %q", readWriter.Header().Get("Cache-Control"))
	}
	assertRemoteReaderRuntimeLeavesNoShelfRows(t, server)

	foreignRequest := httptest.NewRequest(http.MethodGet, "/api/reader/remote-sessions/"+created.ID, nil)
	foreignRequest.Header.Set("Authorization", otherToken)
	foreignWriter := httptest.NewRecorder()
	router.ServeHTTP(foreignWriter, foreignRequest)
	if foreignWriter.Code != http.StatusNotFound {
		t.Fatalf("foreign remote reader session: expected 404, got %d: %s", foreignWriter.Code, foreignWriter.Body.String())
	}
}

func assertRemoteReaderRuntimeLeavesNoShelfRows(t *testing.T, server *Server) {
	t.Helper()
	for _, model := range []any{&models.Book{}, &models.Chapter{}, &models.ReadingProgress{}, &models.Bookmark{}} {
		var count int64
		if err := server.db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("temporary remote reader must not persist %T rows, got %d", model, count)
		}
	}
}
