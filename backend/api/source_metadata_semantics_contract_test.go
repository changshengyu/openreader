package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestSourceMetadataSemanticsReachShelfRefreshSourceChangeAndTemporaryReader(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	titles := map[string]string{
		"metadata-a.test": "详情甲 作者：网页",
		"metadata-b.test": "详情乙 某人 著",
	}
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			host := request.URL.Host
			title, ok := titles[host]
			if !ok {
				t.Fatalf("unexpected metadata host: %s", host)
			}
			body := fmt.Sprintf(`{"book":{"name":%q,"author":"作者：详情作者 著","intro":"第一段<br>第二段","rename":false,"chapters":[{"title":"第一章","url":"/chapter/1"}]}}`, title)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	createSource := func(name, baseURL string) models.BookSource {
		t.Helper()
		source := models.BookSource{Name: name, BaseURL: baseURL, Charset: "utf-8", Enabled: true}
		if err := source.SetRules(models.BookSourceRule{
			BookInfoInitRule:      "$.book",
			BookInfoNameRule:      "$.name",
			BookInfoAuthorRule:    "$.author",
			BookInfoIntroRule:     "$.intro",
			BookInfoCanRenameRule: "$.rename",
			ChapterListRule:       "$.book.chapters[*]",
			ChapterNameRule:       "$.title",
			ChapterURLRule:        "$.url",
		}); err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&source).Error; err != nil {
			t.Fatal(err)
		}
		return source
	}

	sourceA := createSource("元数据甲源", "https://metadata-a.test")
	sourceB := createSource("元数据乙源", "https://metadata-b.test")

	createBody := fmt.Sprintf(`{"title":"搜索标题","author":"搜索作者","bookUrl":"https://metadata-a.test/book","sourceId":%d}`, sourceA.ID)
	createRequest := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(createBody))
	createRequest.Header.Set("Authorization", token)
	createRequest.Header.Set("Content-Type", "application/json")
	createWriter := httptest.NewRecorder()
	router.ServeHTTP(createWriter, createRequest)
	if createWriter.Code != http.StatusCreated {
		t.Fatalf("create remote book: code=%d body=%s", createWriter.Code, createWriter.Body.String())
	}
	var created models.Book
	if err := json.Unmarshal(createWriter.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	assertMetadataBook(t, created, "详情甲", "详情作者", "第一段\n　　第二段")

	titles["metadata-a.test"] = "刷新详情 作者：网页"
	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(created.ID), 10)+"/refresh", nil)
	refreshRequest.Header.Set("Authorization", token)
	refreshWriter := httptest.NewRecorder()
	router.ServeHTTP(refreshWriter, refreshRequest)
	if refreshWriter.Code != http.StatusOK {
		t.Fatalf("refresh remote book: code=%d body=%s", refreshWriter.Code, refreshWriter.Body.String())
	}
	var refreshed models.Book
	if err := server.db.First(&refreshed, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertMetadataBook(t, refreshed, "刷新详情", "详情作者", "第一段\n　　第二段")
	oldSourceChangeTime := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	if err := server.db.Model(&models.Book{}).Where("id = ?", created.ID).UpdateColumn("last_check_time", oldSourceChangeTime).Error; err != nil {
		t.Fatal(err)
	}

	changeBody := fmt.Sprintf(`{"sourceId":%d,"bookUrl":"https://metadata-b.test/book","title":"换源请求标题","author":"换源请求作者"}`, sourceB.ID)
	changeRequest := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(created.ID), 10)+"/change-source", strings.NewReader(changeBody))
	changeRequest.Header.Set("Authorization", token)
	changeRequest.Header.Set("Content-Type", "application/json")
	changeWriter := httptest.NewRecorder()
	router.ServeHTTP(changeWriter, changeRequest)
	if changeWriter.Code != http.StatusOK {
		t.Fatalf("change source: code=%d body=%s", changeWriter.Code, changeWriter.Body.String())
	}
	var changed models.Book
	if err := server.db.First(&changed, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertMetadataBook(t, changed, "详情乙", "详情作者", "第一段\n　　第二段")
	if changed.LastCheckTime <= oldSourceChangeTime {
		t.Fatalf("successful source change did not advance lastCheckTime: %+v", changed)
	}

	var beforeSessions int64
	if err := server.db.Model(&models.Book{}).Count(&beforeSessions).Error; err != nil {
		t.Fatal(err)
	}
	sessionBody := fmt.Sprintf(`{"title":"临时搜索标题","author":"临时搜索作者","bookUrl":"https://metadata-b.test/book","sourceId":%d}`, sourceB.ID)
	sessionRequest := httptest.NewRequest(http.MethodPost, "/api/reader/remote-sessions", strings.NewReader(sessionBody))
	sessionRequest.Header.Set("Authorization", token)
	sessionRequest.Header.Set("Content-Type", "application/json")
	sessionWriter := httptest.NewRecorder()
	router.ServeHTTP(sessionWriter, sessionRequest)
	if sessionWriter.Code != http.StatusCreated {
		t.Fatalf("create remote reader: code=%d body=%s", sessionWriter.Code, sessionWriter.Body.String())
	}
	var session struct {
		Book models.Book `json:"book"`
	}
	if err := json.Unmarshal(sessionWriter.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	assertMetadataBook(t, session.Book, "详情乙", "详情作者", "第一段\n　　第二段")
	var afterSessions int64
	if err := server.db.Model(&models.Book{}).Count(&afterSessions).Error; err != nil {
		t.Fatal(err)
	}
	if afterSessions != beforeSessions {
		t.Fatalf("temporary reader persisted a book row: before=%d after=%d", beforeSessions, afterSessions)
	}
}

func assertMetadataBook(t *testing.T, book models.Book, title, author, intro string) {
	t.Helper()
	if book.Title != title || book.Author != author || book.Intro != intro {
		t.Fatalf("metadata book = %+v, want title=%q author=%q intro=%q", book, title, author, intro)
	}
}
