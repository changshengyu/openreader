package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/backup"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func setupTestServer(t *testing.T) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := config.Config{
		DataDir:       t.TempDir(),
		CacheDir:      t.TempDir(),
		LibraryDir:    t.TempDir(),
		DatabasePath:  t.TempDir() + "/test.db",
		JWTSecret:     "test-secret",
		LocalStoreDir: t.TempDir() + "/localStore",
	}

	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	hub := readersync.NewHub()
	sched := scheduler.New(database, 1)
	backupSvc := backup.New(database, t.TempDir())

	router := gin.New()
	RegisterRoutes(router, cfg, database, hub, sched, backupSvc)

	server := &Server{cfg: cfg, db: database, hub: hub, scheduler: sched, backupSvc: backupSvc}
	return router, server
}

func authHeader(t *testing.T, router *gin.Engine) string {
	t.Helper()
	body := `{"username":"testuser","password":"test1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return "Bearer " + resp.Token
}

func TestRegisterAndLogin(t *testing.T) {
	router, _ := setupTestServer(t)

	// register
	body := `{"username":"alice","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("register: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var registerResp struct {
		Token string      `json:"token"`
		User  models.User `json:"user"`
	}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	if registerResp.Token == "" {
		t.Fatal("register: no token in response")
	}

	// login
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestBookCRUD(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	// create book
	body := `{"title":"测试书籍","author":"作者名"}`
	req := httptest.NewRequest(http.MethodPost, "/api/books", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var book models.Book
	json.Unmarshal(w.Body.Bytes(), &book)
	if book.Title != "测试书籍" {
		t.Fatalf("wrong title: %q", book.Title)
	}

	// list books
	req2 := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("list books: expected 200, got %d", w2.Code)
	}

	var books []models.Book
	json.Unmarshal(w2.Body.Bytes(), &books)
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}

	// get book
	req3 := httptest.NewRequest(http.MethodGet, "/api/books/"+strings.TrimPrefix(w.Body.String(), `{"id":`), nil)
	req3.Header.Set("Authorization", token)
	_ = req3
}

func TestUpdateBook(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "旧书名", Author: "旧作者"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"title":"新书名","author":"新作者","coverUrl":"https://example.com/cover.jpg","intro":"新简介"}`
	req := httptest.NewRequest(http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update book: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Book
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "新书名" || updated.Author != "新作者" || updated.Intro != "新简介" {
		t.Fatalf("unexpected updated book: %+v", updated)
	}
}

func TestListBooksIncludesProgress(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "有进度"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterIndex: 3, Percent: 0.42}
	if err := server.db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list books: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var books []struct {
		ID       uint `json:"id"`
		Progress *struct {
			BookID       uint    `json:"bookId"`
			ChapterIndex int     `json:"chapterIndex"`
			Percent      float64 `json:"percent"`
		} `json:"progress"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &books); err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Progress == nil || books[0].Progress.BookID != book.ID || books[0].Progress.ChapterIndex != 3 {
		t.Fatalf("expected embedded progress, got %+v", books)
	}
}

func TestBatchBooksCategoryAndDelete(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "批量分组"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	bookA := models.Book{UserID: user.ID, Title: "A"}
	bookB := models.Book{UserID: user.ID, Title: "B"}
	if err := server.db.Create(&bookA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&bookB).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"action":"category","bookIds":[` + strconv.FormatUint(uint64(bookA.ID), 10) + `,` + strconv.FormatUint(uint64(bookB.ID), 10) + `],"categoryId":` + strconv.FormatUint(uint64(category.ID), 10) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch category: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.Book{}).Where("category_id = ?", category.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 categorized books, got %d", count)
	}

	body = `{"action":"delete","bookIds":[` + strconv.FormatUint(uint64(bookA.ID), 10) + `,` + strconv.FormatUint(uint64(bookB.ID), 10) + `]}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("batch delete: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if err := server.db.Model(&models.Book{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 books after batch delete, got %d", count)
	}
}

func TestCategoriesAndFilter(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	// create category
	catBody := `{"name":"科幻","color":"#336699"}`
	req := httptest.NewRequest(http.MethodPost, "/api/categories", strings.NewReader(catBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create category: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// create book with category
	var cat models.Category
	json.Unmarshal(w.Body.Bytes(), &cat)
	bookBody := `{"title":"三体","author":"刘慈欣","categoryId":` + strings.TrimPrefix(w.Body.String(), `{"id":`) + ``
	req2 := httptest.NewRequest(http.MethodPost, "/api/books", strings.NewReader(bookBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	_ = cat
	_ = w2
}

func TestReorderCategories(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	first := models.Category{UserID: user.ID, Name: "第一", SortOrder: 10}
	second := models.Category{UserID: user.ID, Name: "第二", SortOrder: 20}
	if err := server.db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"ids":[` + strconv.FormatUint(uint64(second.ID), 10) + `,` + strconv.FormatUint(uint64(first.ID), 10) + `]}`
	req := httptest.NewRequest(http.MethodPut, "/api/categories/reorder", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder categories: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var categories []models.Category
	if err := json.Unmarshal(w.Body.Bytes(), &categories); err != nil {
		t.Fatal(err)
	}
	if len(categories) != 2 || categories[0].ID != second.ID || categories[1].ID != first.ID {
		t.Fatalf("unexpected category order: %+v", categories)
	}
}

func TestSourceManagement(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	// create source
	body := `{"name":"测试书源","baseUrl":"https://example.com","charset":"utf-8"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create source: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// list sources
	req2 := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("list sources: expected 200, got %d", w2.Code)
	}

	var sources []models.BookSource
	json.Unmarshal(w2.Body.Bytes(), &sources)
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	// delete source
	req3 := httptest.NewRequest(http.MethodDelete, "/api/sources/1", nil)
	req3.Header.Set("Authorization", token)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNoContent {
		t.Fatalf("delete source: expected 204, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestUpdateSourceCanClearOptionalFields(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	source := models.BookSource{
		Name:      "待编辑",
		BaseURL:   "https://example.com",
		SearchURL: "https://example.com/search",
		Charset:   "gbk",
		Group:     "旧分组",
		Rules:     `{"searchUrl":"x"}`,
		Enabled:   true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"name":"待编辑","baseUrl":"","searchUrl":"","charset":"","group":"","rules":"","enabled":false}`
	req := httptest.NewRequest(http.MethodPut, "/api/sources/"+strconv.FormatUint(uint64(source.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update source: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.BookSource
	if err := server.db.First(&updated, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.BaseURL != "" || updated.SearchURL != "" || updated.Group != "" || updated.Rules != "" || updated.Charset != "utf-8" || updated.Enabled {
		t.Fatalf("source optional fields were not cleared: %+v", updated)
	}
}

func TestCreateSourceRespectsEnabledFlag(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(`{"name":"停用源","enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create source: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var source models.BookSource
	if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	if source.Enabled {
		t.Fatalf("expected source to remain disabled: %+v", source)
	}
}

func TestDecodeBookSourcesEnabledDefaults(t *testing.T) {
	sources, err := decodeBookSources([]byte(`[
		{"name":"默认启用"},
		{"name":"显式停用","enabled":false}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if !sources[0].Enabled {
		t.Fatalf("expected missing enabled to default true: %+v", sources[0])
	}
	if sources[1].Enabled {
		t.Fatalf("expected explicit false to be preserved: %+v", sources[1])
	}
}

func TestBatchTestSourcesReturnsPerSourceResults(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	body := `{"name":"无搜索地址","charset":"utf-8","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create source: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/sources/batch-test", strings.NewReader(`{"keyword":"测试"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("batch test: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp struct {
		Results []struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].OK || !strings.Contains(resp.Results[0].Message, "no search URL") {
		t.Fatalf("unexpected batch result: %+v", resp.Results)
	}
}

func TestBatchSourcesEnableDisableAndDelete(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	sourceA := models.BookSource{Name: "A", Enabled: true}
	sourceB := models.BookSource{Name: "B", Enabled: true}
	if err := server.db.Create(&sourceA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&sourceB).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"action":"disable","sourceIds":[` + strconv.FormatUint(uint64(sourceA.ID), 10) + `,` + strconv.FormatUint(uint64(sourceB.ID), 10) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch disable sources: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var disabled int64
	if err := server.db.Model(&models.BookSource{}).Where("enabled = ?", false).Count(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if disabled != 2 {
		t.Fatalf("expected 2 disabled sources, got %d", disabled)
	}

	body = `{"action":"delete","sourceIds":[` + strconv.FormatUint(uint64(sourceA.ID), 10) + `]}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/sources/batch", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("batch delete sources: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.BookSource{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 source after delete, got %d", count)
	}
}

func TestClearSources(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	if err := server.db.Create(&models.BookSource{Name: "A", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookSource{Name: "B", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/sources", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"affected":2`) {
		t.Fatalf("clear sources: expected affected count, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.BookSource{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected all sources cleared, got %d", count)
	}
}

func TestImportRemoteSourceUsesRawJSON(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"name":"远程源","baseUrl":"https://remote.example","charset":"utf-8","enabled":true}]`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	req := httptest.NewRequest(http.MethodPost, "/api/sources/remote", strings.NewReader(`{"url":"https://remote.example/sources.json"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("remote source import: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"imported":1`) {
		t.Fatalf("unexpected remote import response: %s", w.Body.String())
	}

	var source models.BookSource
	if err := server.db.Where("name = ?", "远程源").First(&source).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPreviewRemoteSourceDoesNotImport(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"name":"预览源","baseUrl":"https://preview.example"}]`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	req := httptest.NewRequest(http.MethodPost, "/api/sources/remote-preview", strings.NewReader(`{"url":"https://remote.example/sources.json"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"count":1`) || !strings.Contains(w.Body.String(), "预览源") {
		t.Fatalf("remote source preview: expected preview, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.BookSource{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("preview should not import sources, got %d", count)
	}
}

func TestRemoteSourceImportUpdatesExistingByName(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	existing := models.BookSource{Name: "同名源", BaseURL: "https://old.example", Charset: "utf-8", Enabled: true}
	if err := server.db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"name":"同名源","baseUrl":"https://new.example","charset":"gbk","enabled":false}]`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	req := httptest.NewRequest(http.MethodPost, "/api/sources/remote", strings.NewReader(`{"url":"https://remote.example/sources.json"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"updated":1`) {
		t.Fatalf("remote source import should update existing source, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.BookSource{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected no duplicate source, got %d", count)
	}
	var updated models.BookSource
	if err := server.db.First(&updated, existing.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.BaseURL != "https://new.example" || updated.Charset != "gbk" || updated.Enabled {
		t.Fatalf("source was not updated correctly: %+v", updated)
	}
}

func TestSourceCandidatesAndChangeSourceUseCandidateURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	upstream := "https://source.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.Path {
			case "/search":
				body = `<html><body>
					<div class="book">
						<a class="link" href="/book-new"><span class="title">候选书</span></a>
						<span class="author">新作者</span>
						<p class="intro">新书源简介</p>
					</div>
				</body></html>`
			case "/book-new":
				body = `<html><body>
					<ul>
						<li class="chapter"><a href="/c1">第一章</a></li>
						<li class="chapter"><a href="/c2">第二章</a></li>
					</ul>
				</body></html>`
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	source := models.BookSource{
		Name:    "候选源",
		BaseURL: upstream,
		Charset: "utf-8",
		Enabled: true,
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:       upstream + "/search?q={keyword}",
		BookListRule:    ".book",
		BookNameRule:    ".title|text",
		BookAuthorRule:  ".author|text",
		BookIntroRule:   ".intro|text",
		BookURLRule:     ".link|attr:href",
		ChapterListRule: ".chapter",
		ChapterNameRule: "a|text",
		ChapterURLRule:  "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "候选书", URL: upstream + "/old"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/source-candidates", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("source candidates: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var candidates []struct {
		SourceID uint   `json:"sourceId"`
		Title    string `json:"title"`
		BookURL  string `json:"bookUrl"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].BookURL != upstream+"/book-new" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}

	body := `{"sourceId":` + strconv.FormatUint(uint64(candidates[0].SourceID), 10) + `,"bookUrl":` + strconv.Quote(candidates[0].BookURL) + `,"title":"候选书","author":"新作者"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/change-source", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("change source: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var updated models.Book
	if err := server.db.First(&updated, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.URL != upstream+"/book-new" || updated.ChapterCount != 2 || updated.LastChapter != "第二章" {
		t.Fatalf("book was not switched to candidate URL: %+v", updated)
	}
}

func TestCreateRemoteBookAcceptsCategory(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	upstream := "https://remote-book.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<li class="chapter"><a href="/c1">第一章</a></li>
				</body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "远程分组"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "远程源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		ChapterListRule: ".chapter",
		ChapterNameRule: "a|text",
		ChapterURLRule:  "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"title":"远程书","bookUrl":"` + upstream + `/book","sourceId":` + strconv.FormatUint(uint64(source.ID), 10) + `,"categoryId":` + strconv.FormatUint(uint64(category.ID), 10) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create remote book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var book models.Book
	if err := json.Unmarshal(w.Body.Bytes(), &book); err != nil {
		t.Fatal(err)
	}
	if book.CategoryID == nil || *book.CategoryID != category.ID {
		t.Fatalf("expected category on remote book, got %+v", book)
	}
}

func TestCreateRemoteBookReusesExistingURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "新分组"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "已有源", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "已有书", URL: "https://book.example/existing"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"title":"已有书","bookUrl":"https://book.example/existing","sourceId":` + strconv.FormatUint(uint64(source.ID), 10) + `,"categoryId":` + strconv.FormatUint(uint64(category.ID), 10) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reuse remote book: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.Book{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected no duplicate books, got %d", count)
	}
	var updated models.Book
	if err := server.db.First(&updated, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CategoryID == nil || *updated.CategoryID != category.ID {
		t.Fatalf("expected existing book category updated, got %+v", updated)
	}
}

func TestUnauthorizedAccess(t *testing.T) {
	router, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDeleteBookCascadesReaderState(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	book := models.Book{UserID: user.ID, Title: "待删除"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章"}
	bookmark := models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterIndex: 0, Title: "书签"}
	progress := models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterIndex: 0, Percent: 0.5}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10), nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete book: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	for _, model := range []any{&models.Book{}, &models.Chapter{}, &models.Bookmark{}, &models.ReadingProgress{}} {
		if err := server.db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("expected %T count 0, got %d", model, count)
		}
	}
}

func TestUpdateBookmark(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "书签书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	bookmark := models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterIndex: 0, Title: "旧标题"}
	if err := server.db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"title":"新标题","excerpt":"新摘录","note":"新笔记"}`
	req := httptest.NewRequest(http.MethodPut, "/api/bookmarks/"+strconv.FormatUint(uint64(bookmark.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update bookmark: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Bookmark
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "新标题" || updated.Excerpt != "新摘录" || updated.Note != "新笔记" {
		t.Fatalf("unexpected bookmark: %+v", updated)
	}
}

func TestSearchBookContentUsesCachedChapter(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	cachePath := filepath.Join("cached", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("第一段内容\n这里有一个特殊关键词用于搜索\n第二个特殊关键词也应命中\n换行拆开的隐 藏\n关 键 词\n夫君御驾亲征了！！！\n结尾"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := models.Book{UserID: user.ID, Title: "可搜索"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("特殊关键词"), nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search content: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var matches []struct {
		ChapterIndex int    `json:"chapterIndex"`
		ChapterTitle string `json:"chapterTitle"`
		Excerpt      string `json:"excerpt"`
		LineIndex    int    `json:"lineIndex"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].ChapterIndex != 0 || !strings.Contains(matches[0].Excerpt, "特殊关键词") {
		t.Fatalf("unexpected matches: %+v", matches)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("隐藏关键词"), nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search normalized content: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	matches = nil
	if err := json.Unmarshal(w.Body.Bytes(), &matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || !strings.Contains(matches[0].Excerpt, "隐 藏") {
		t.Fatalf("unexpected normalized matches: %+v", matches)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("夫君御驾亲征了!"), nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search punctuation-normalized content: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	matches = nil
	if err := json.Unmarshal(w.Body.Bytes(), &matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].LineIndex != 5 || !strings.Contains(matches[0].Excerpt, "夫君御驾亲征了") {
		t.Fatalf("unexpected punctuation-normalized matches: %+v", matches)
	}
}

func TestCacheBookContentUsesCachedChapter(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	cachePath := filepath.Join("cached", "chapter-cache.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("已缓存正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := models.Book{UserID: user.ID, Title: "缓存书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"chapterIndex":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cache chapter: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"cached":1`) {
		t.Fatalf("expected cached count 1, got %s", w.Body.String())
	}
}

func TestCacheStatsAndClearCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	if err := os.MkdirAll(filepath.Join(server.cfg.CacheDir, "stats"), 0o755); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("stats", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.WriteFile(fullPath, []byte("缓存正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{Title: "缓存统计", UserID: 1}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cache/stats", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cachedChapters":1`) || !strings.Contains(w.Body.String(), `"files":1`) {
		t.Fatalf("cache stats: expected cached counts, got %d: %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodDelete, "/api/cache", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("clear cache: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removed, stat err=%v", err)
	}
	var updated models.Chapter
	if err := server.db.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CachePath != "" {
		t.Fatalf("expected chapter cache path reset, got %q", updated.CachePath)
	}
}

func TestReplaceRuleCRUDAndChapterContentAppliesRules(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/replace-rules", strings.NewReader(`{"name":"去广告","pattern":"广告[0-9]+","replacement":"","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create replace rule: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var rule models.ReplaceRule
	if err := json.Unmarshal(w.Body.Bytes(), &rule); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/replace-rules", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), "去广告") {
		t.Fatalf("list replace rules: expected rule, got %d: %s", w2.Code, w2.Body.String())
	}

	cachePath := filepath.Join("replace", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("广告123\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "替换书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
	req3.Header.Set("Authorization", token)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("chapter content: expected 200, got %d: %s", w3.Code, w3.Body.String())
	}
	if strings.Contains(w3.Body.String(), "广告123") || !strings.Contains(w3.Body.String(), "正文") {
		t.Fatalf("replace rule was not applied to content: %s", w3.Body.String())
	}

	req4 := httptest.NewRequest(http.MethodDelete, "/api/replace-rules/"+strconv.FormatUint(uint64(rule.ID), 10), nil)
	req4.Header.Set("Authorization", token)
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusNoContent {
		t.Fatalf("delete replace rule: expected 204, got %d: %s", w4.Code, w4.Body.String())
	}
}

func TestBatchBooksCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(server.cfg.CacheDir, "batch-cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	bookA := models.Book{UserID: user.ID, Title: "缓存 A"}
	bookB := models.Book{UserID: user.ID, Title: "缓存 B"}
	if err := server.db.Create(&bookA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&bookB).Error; err != nil {
		t.Fatal(err)
	}
	cacheA := filepath.Join("batch-cache", "a.txt")
	cacheB := filepath.Join("batch-cache", "b.txt")
	if err := os.WriteFile(filepath.Join(server.cfg.CacheDir, cacheA), []byte("A 正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.CacheDir, cacheB), []byte("B 正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: bookA.ID, Index: 0, Title: "第一章", CachePath: cacheA}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: bookB.ID, Index: 0, Title: "第一章", CachePath: cacheB}).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"action":"cache","bookIds":[` + strconv.FormatUint(uint64(bookA.ID), 10) + `,` + strconv.FormatUint(uint64(bookB.ID), 10) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch cache: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"cached":2`) || !strings.Contains(w.Body.String(), `"requested":2`) {
		t.Fatalf("unexpected batch cache response: %s", w.Body.String())
	}
}

func TestBatchBooksClearCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	book := models.Book{UserID: user.ID, Title: "清缓存"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("clear-cache", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"action":"clear-cache","bookIds":[` + strconv.FormatUint(uint64(book.ID), 10) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch clear cache: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"cleared":1`) {
		t.Fatalf("unexpected clear cache response: %s", w.Body.String())
	}

	var updated models.Chapter
	if err := server.db.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CachePath != "" {
		t.Fatalf("expected cache path cleared, got %q", updated.CachePath)
	}
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removed, stat error: %v", err)
	}
}

func TestExportBooks(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "导出书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterIndex: 0, Title: "书签"}).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"bookIds":[` + strconv.FormatUint(uint64(book.ID), 10) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("export books: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if disposition := w.Header().Get("Content-Disposition"); !strings.Contains(disposition, "openreader-books.json") {
		t.Fatalf("missing export attachment header: %q", disposition)
	}

	var exported struct {
		Count int `json:"count"`
		Books []struct {
			Book      models.Book       `json:"book"`
			Chapters  []models.Chapter  `json:"chapters"`
			Bookmarks []models.Bookmark `json:"bookmarks"`
		} `json:"books"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Count != 1 || len(exported.Books) != 1 || exported.Books[0].Book.Title != "导出书" || len(exported.Books[0].Chapters) != 1 || len(exported.Books[0].Bookmarks) != 1 {
		t.Fatalf("unexpected export payload: %+v", exported)
	}
}

func TestLocalStoreBrowseAndDelete(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	nestedDir := filepath.Join(server.cfg.LocalStoreDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "book.txt"), []byte("正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local-store?path=nested", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list local store: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var listing struct {
		Path  string `json:"path"`
		Items []struct {
			Name       string `json:"name"`
			Path       string `json:"path"`
			Importable bool   `json:"importable"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Path != "nested" || len(listing.Items) != 1 || listing.Items[0].Path != filepath.Join("nested", "book.txt") || !listing.Items[0].Importable {
		t.Fatalf("unexpected listing: %+v", listing)
	}

	req2 := httptest.NewRequest(http.MethodDelete, "/api/local-store?path="+url.QueryEscape(filepath.Join("nested", "book.txt")), nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("delete local store: expected 204, got %d: %s", w2.Code, w2.Body.String())
	}
	if _, err := os.Stat(filepath.Join(nestedDir, "book.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
}

func TestLocalStoreRejectsEscapedPath(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/local-store?path=../outside", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for escaped path, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalStoreCreateDirectoryAndRename(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/local-store/directory", strings.NewReader(`{"path":"","name":"新目录"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create local directory: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, "新目录")); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodPut, "/api/local-store/rename", strings.NewReader(`{"path":"新目录","name":"重命名目录"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("rename local item: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, "重命名目录")); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStoreImportAcceptsCategory(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "书仓分组"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "store.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"paths":["store.txt"],"categoryId":` + strconv.FormatUint(uint64(category.ID), 10) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/local-store/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("local store import: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var book models.Book
	if err := server.db.Where("title = ?", "store").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.CategoryID == nil || *book.CategoryID != category.ID {
		t.Fatalf("expected imported book category %d, got %+v", category.ID, book.CategoryID)
	}
}

func TestWebDAVPutListGetAndDelete(t *testing.T) {
	router, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPut, "/webdav/backups/sample.txt", strings.NewReader("hello webdav"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("webdav put: expected 201, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/webdav/backups", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusMultiStatus || !strings.Contains(w2.Body.String(), "sample.txt") {
		t.Fatalf("webdav list: expected multistatus with file, got %d: %s", w2.Code, w2.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet, "/webdav/backups/sample.txt", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK || strings.TrimSpace(w3.Body.String()) != "hello webdav" {
		t.Fatalf("webdav get: expected file, got %d: %s", w3.Code, w3.Body.String())
	}

	req4 := httptest.NewRequest(http.MethodDelete, "/webdav/backups/sample.txt", nil)
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusNoContent {
		t.Fatalf("webdav delete: expected 204, got %d", w4.Code)
	}
}

func TestWebDAVMkcolAndMove(t *testing.T) {
	router, _ := setupTestServer(t)

	req := httptest.NewRequest("MKCOL", "/webdav/books", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("webdav mkcol: expected 201, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodPut, "/webdav/books/a.txt", strings.NewReader("hello"))
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("webdav put: expected 201, got %d", w2.Code)
	}

	req3 := httptest.NewRequest("MOVE", "/webdav/books/a.txt", nil)
	req3.Header.Set("Destination", "/webdav/books/b.txt")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusCreated {
		t.Fatalf("webdav move: expected 201, got %d", w3.Code)
	}

	req4 := httptest.NewRequest(http.MethodGet, "/webdav/books/b.txt", nil)
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK || strings.TrimSpace(w4.Body.String()) != "hello" {
		t.Fatalf("webdav moved file get: expected file, got %d: %s", w4.Code, w4.Body.String())
	}
}

func TestWebDAVRejectsEscapedPath(t *testing.T) {
	router, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/webdav/../outside.txt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for escaped path, got %d", w.Code)
	}
}

func TestRestoreLegadoBackupImportsBookshelf(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	file, err := zipWriter.Create("myBookShelf.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(`[{"name":"恢复书","author":"恢复作者","bookUrl":"https://book.example/1","coverUrl":"https://book.example/cover.jpg","intro":"简介"}]`)); err != nil {
		t.Fatal(err)
	}
	progressFile, err := zipWriter.Create("bookProgress/progress.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := progressFile.Write([]byte(`{"bookUrl":"https://book.example/1","durChapter":2,"durChapterPos":88}`)); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "backup.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(zipBuffer.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/backup/restore-legado", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("restore backup: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"books":1`) {
		t.Fatalf("expected one restored book, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"progress":1`) {
		t.Fatalf("expected one restored progress, got %s", w.Body.String())
	}

	var book models.Book
	if err := server.db.Where("title = ?", "恢复书").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.Author != "恢复作者" || book.URL != "https://book.example/1" {
		t.Fatalf("unexpected restored book: %+v", book)
	}
	var progress models.ReadingProgress
	if err := server.db.Where("book_id = ?", book.ID).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterIndex != 2 || progress.Offset != 88 {
		t.Fatalf("unexpected restored progress: %+v", progress)
	}
}

func TestRestoreWebDAVBackupImportsBookshelf(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	existingSource := models.BookSource{Name: "备份源", BaseURL: "https://old-source.example", Charset: "utf-8", Enabled: true}
	if err := server.db.Create(&existingSource).Error; err != nil {
		t.Fatal(err)
	}

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	sourceFile, err := zipWriter.Create("bookSource.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceFile.Write([]byte(`[{"name":"备份源","baseUrl":"https://new-source.example","charset":"gbk","enabled":false}]`)); err != nil {
		t.Fatal(err)
	}
	file, err := zipWriter.Create("myBookShelf.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(`[{"name":"WebDAV恢复书","author":"恢复作者","bookUrl":"https://book.example/webdav","durChapter":3,"durChapterPos":120}]`)); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(server.cfg.DataDir, "webdav", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "backup.zip"), zipBuffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/backup/restore-webdav", strings.NewReader(`{"path":"backups/backup.zip"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("restore webdav backup: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"books":1`) {
		t.Fatalf("expected one restored book, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"sources":1`) {
		t.Fatalf("expected one restored source update, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"progress":1`) {
		t.Fatalf("expected one restored progress, got %s", w.Body.String())
	}

	var book models.Book
	if err := server.db.Where("title = ?", "WebDAV恢复书").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	var progress models.ReadingProgress
	if err := server.db.Where("book_id = ?", book.ID).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterIndex != 3 || progress.Offset != 120 {
		t.Fatalf("unexpected restored progress: %+v", progress)
	}
	var source models.BookSource
	if err := server.db.First(&source, existingSource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if source.BaseURL != "https://new-source.example" || source.Charset != "gbk" || source.Enabled {
		t.Fatalf("unexpected restored source update: %+v", source)
	}
}

func TestCreateReplaceRuleRespectsEnabledFlag(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/replace-rules", strings.NewReader(`{"name":"停用规则","pattern":"广告","replacement":"","enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create replace rule: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rule models.ReplaceRule
	if err := json.Unmarshal(w.Body.Bytes(), &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Enabled {
		t.Fatalf("expected replace rule to remain disabled: %+v", rule)
	}
}

func TestReplaceRuleTestEndpoint(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/replace-rules/test", strings.NewReader(`{"pattern":"广告[0-9]+","replacement":"","text":"广告123\n正文"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("test replace rule: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"changed":true`) || !strings.Contains(w.Body.String(), `\n正文`) {
		t.Fatalf("unexpected replace rule test result: %s", w.Body.String())
	}
}

func TestRSSSourceRefreshImportsArticles(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
					<rss version="2.0"><channel>
						<item>
							<title>RSS 文章</title>
							<link>https://rss.example/a</link>
							<description>文章摘要</description>
							<author>作者</author>
							<pubDate>Mon, 02 Jan 2006 15:04:05 +0000</pubDate>
						</item>
					</channel></rss>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"测试 RSS","url":"https://rss.example/feed.xml","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rss source: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var source models.RSSSource
	if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), `"imported":1`) {
		t.Fatalf("refresh rss source: expected import, got %d: %s", w2.Code, w2.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/rss/articles?sourceId="+strconv.FormatUint(uint64(source.ID), 10), nil)
	req3.Header.Set("Authorization", token)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK || !strings.Contains(w3.Body.String(), "RSS 文章") || !strings.Contains(w3.Body.String(), "文章摘要") {
		t.Fatalf("list rss articles: expected article, got %d: %s", w3.Code, w3.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.RSSArticle{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one rss article, got %d", count)
	}
}

func TestCreateRSSSourceRespectsEnabledFlag(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"停用 RSS","url":"https://rss.example/feed.xml","enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rss source: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var source models.RSSSource
	if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	if source.Enabled {
		t.Fatalf("expected rss source to remain disabled: %+v", source)
	}
}

func TestUploadAssetStoresPublicFile(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", "cover"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "cover.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("png-data")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"/uploads/covers/`) {
		t.Fatalf("upload asset: expected public URL, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(resp.URL, "/uploads/covers/")
	if _, err := os.Stat(filepath.Join(server.cfg.DataDir, "uploads", "covers", name)); err != nil {
		t.Fatalf("uploaded file missing: %v", err)
	}
}

func TestRSSArticleStateCanBeUpdatedAndFiltered(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{UserID: user.ID, Title: "RSS", URL: "https://rss.example/feed.xml", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	article := models.RSSArticle{UserID: user.ID, SourceID: source.ID, Title: "未读文章", Link: "https://rss.example/a"}
	if err := server.db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"isRead":true,"favorite":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/rss/articles/"+strconv.FormatUint(uint64(article.ID), 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"isRead":true`) || !strings.Contains(w.Body.String(), `"favorite":true`) {
		t.Fatalf("update rss article: expected updated state, got %d: %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/rss/articles?unread=true", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || strings.Contains(w2.Body.String(), "未读文章") {
		t.Fatalf("unread filter should hide read article, got %d: %s", w2.Code, w2.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/rss/articles?favorite=true", nil)
	req3.Header.Set("Authorization", token)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK || !strings.Contains(w3.Body.String(), "未读文章") {
		t.Fatalf("favorite filter should include article, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestExploreBooksUsesExploreURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var requested []string

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requested = append(requested, req.URL.String())
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<div class="book"><a class="link" href="/book"><span class="title">探索书</span></a><span class="author">作者</span></div>
				</body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "探索源", BaseURL: "https://explore.example", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		ExploreURL:     "https://explore.example/top",
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

	req := httptest.NewRequest(http.MethodGet, "/api/explore/sources", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "探索源") {
		t.Fatalf("explore sources: expected source, got %d: %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/explore/"+strconv.FormatUint(uint64(source.ID), 10), nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), "探索书") || !strings.Contains(w2.Body.String(), "https://explore.example/book") {
		t.Fatalf("explore books: expected result, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestExploreBooksSupportsPagePlaceholder(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var requested string

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requested = req.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<div class="book"><a class="link" href="/book-2"><span class="title">第二页书</span></a></div>
				</body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "分页探索源", BaseURL: "https://explore.example", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		ExploreURL:   "https://explore.example/top/{page}",
		BookListRule: ".book",
		BookNameRule: ".title|text",
		BookURLRule:  ".link|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/explore/"+strconv.FormatUint(uint64(source.ID), 10)+"?page=2", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "第二页书") || !strings.Contains(w.Body.String(), `"hasMore":true`) {
		t.Fatalf("explore page: expected page response, got %d: %s", w.Code, w.Body.String())
	}
	if requested != "https://explore.example/top/2" {
		t.Fatalf("expected page placeholder URL, got %q", requested)
	}
}

func TestImportFromWebDAVImportsBook(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	webdavDir := filepath.Join(server.cfg.DataDir, "webdav", "books")
	if err := os.MkdirAll(webdavDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webdavDir, "webdav-book.txt"), []byte("第一章 开始\n正文内容"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webdav/import", strings.NewReader(`{"paths":["books/webdav-book.txt"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("import webdav: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"book"`) {
		t.Fatalf("expected imported book in response, got %s", w.Body.String())
	}

	var book models.Book
	if err := server.db.Where("title = ?", "webdav-book").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.ChapterCount == 0 {
		t.Fatalf("expected imported chapters, got %+v", book)
	}
}
