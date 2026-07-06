package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/backup"
	"openreader/backend/services/epubreader"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func boolPointer(value bool) *bool {
	return &value
}

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
	backupSvc := backup.New(database, filepath.Join(cfg.DataDir, "webdav"))

	router := gin.New()
	RegisterRoutes(router, cfg, database, hub, sched, backupSvc)

	server := &Server{
		cfg:        cfg,
		db:         database,
		hub:        hub,
		scheduler:  sched,
		backupSvc:  backupSvc,
		epubReader: epubreader.New(cfg, database),
	}
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

func TestHealthIncludesBuildInfo(t *testing.T) {
	router, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["app"] != "openreader" || resp["buildDate"] == "" || resp["commit"] == "" {
		t.Fatalf("health missing build info: %+v", resp)
	}
}

func TestListTXTTocRules(t *testing.T) {
	router, _ := setupTestServer(t)
	auth := authHeader(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/txt-toc-rules", nil)
	req.Header.Set("Authorization", auth)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("txt toc rules: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var rules []engine.TXTTocRule
	if err := json.Unmarshal(w.Body.Bytes(), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatal("expected default txt toc rules")
	}
	if rules[0].Name == "" || rules[0].Rule == "" {
		t.Fatalf("unexpected first rule: %+v", rules[0])
	}
}

func TestRegisterAndLogin(t *testing.T) {
	router, server := setupTestServer(t)

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
	if registerResp.User.Role != "admin" {
		t.Fatalf("first registered user role = %q, want admin", registerResp.User.Role)
	}
	var storedFirst models.User
	if err := server.db.Where("username = ?", "alice").First(&storedFirst).Error; err != nil {
		t.Fatal(err)
	}
	if storedFirst.Role != "admin" {
		t.Fatalf("stored first user role = %q, want admin", storedFirst.Role)
	}
	adminReq := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	adminReq.Header.Set("Authorization", "Bearer "+registerResp.Token)
	adminW := httptest.NewRecorder()
	router.ServeHTTP(adminW, adminReq)
	if adminW.Code != http.StatusOK {
		t.Fatalf("first registered user should have admin access: %d %s", adminW.Code, adminW.Body.String())
	}

	secondBody := `{"username":"bob","password":"secret456"}`
	secondReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(secondBody))
	secondReq.Header.Set("Content-Type", "application/json")
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, secondReq)
	if secondW.Code != http.StatusOK {
		t.Fatalf("register second user: expected 200, got %d: %s", secondW.Code, secondW.Body.String())
	}
	var secondResp struct {
		User models.User `json:"user"`
	}
	if err := json.Unmarshal(secondW.Body.Bytes(), &secondResp); err != nil {
		t.Fatal(err)
	}
	if secondResp.User.Role != "user" {
		t.Fatalf("second registered user role = %q, want user", secondResp.User.Role)
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

func TestConcurrentRegistrationCreatesExactlyOneAdmin(t *testing.T) {
	router, server := setupTestServer(t)

	const registrations = 4
	var wait sync.WaitGroup
	wait.Add(registrations)
	statuses := make([]int, registrations)
	for index := 0; index < registrations; index++ {
		go func(index int) {
			defer wait.Done()
			body := fmt.Sprintf(`{"username":"parallel%d","password":"secret%d"}`, index, index)
			req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			statuses[index] = w.Code
		}(index)
	}
	wait.Wait()

	for index, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("parallel registration %d status = %d", index, status)
		}
	}
	var adminCount int64
	if err := server.db.Model(&models.User{}).Where("role = ?", "admin").Count(&adminCount).Error; err != nil {
		t.Fatal(err)
	}
	var userCount int64
	if err := server.db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		t.Fatal(err)
	}
	if adminCount != 1 || userCount != registrations {
		t.Fatalf("registered users=%d admins=%d, want %d/1", userCount, adminCount, registrations)
	}
}

func TestSearchPaginationUsesPageForSingleSourceAndCursorForMultipleSources(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var requestedMu sync.Mutex
	requested := make([]string, 0)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestedMu.Lock()
			requested = append(requested, request.URL.Path+"?"+request.URL.RawQuery)
			requestedMu.Unlock()
			sourceName := strings.TrimPrefix(request.URL.Path, "/")
			page := request.URL.Query().Get("page")
			if page == "" {
				page = "1"
			}
			body := fmt.Sprintf(
				`<article class="book"><a class="name" href="/book/%s/%s">%s 第%s页</a><span class="author">%s作者</span><span class="updated">刚刚更新</span></article>`,
				sourceName,
				page,
				sourceName,
				page,
				sourceName,
			)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	sources := make([]models.BookSource, 0, 3)
	for _, name := range []string{"source-a", "source-b", "source-c"} {
		source := models.BookSource{Name: name, Enabled: true, Charset: "utf-8"}
		if err := source.SetRules(models.BookSourceRule{
			SearchURL:          "https://search.example/" + name + "?q={keyword}&page={page}",
			BookListRule:       ".book",
			BookNameRule:       ".name",
			BookAuthorRule:     ".author",
			BookUpdateTimeRule: ".updated",
			BookURLRule:        ".name|attr:href",
		}); err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&source).Error; err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}

	singleBody := fmt.Sprintf(
		`{"keyword":"分页","sourceIds":[%d],"concurrentCount":1,"page":2,"lastIndex":-1,"searchSize":20}`,
		sources[0].ID,
	)
	singleReq := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(singleBody))
	singleReq.Header.Set("Content-Type", "application/json")
	singleReq.Header.Set("Authorization", token)
	singleW := httptest.NewRecorder()
	router.ServeHTTP(singleW, singleReq)
	if singleW.Code != http.StatusOK {
		t.Fatalf("single source search: expected 200, got %d: %s", singleW.Code, singleW.Body.String())
	}
	var singleResp searchResponse
	if err := json.Unmarshal(singleW.Body.Bytes(), &singleResp); err != nil {
		t.Fatal(err)
	}
	if singleResp.Page != 2 || !singleResp.HasMore || singleResp.LastIndex != -1 ||
		len(singleResp.List) != 1 || singleResp.List[0].Title != "source-a 第2页" ||
		singleResp.List[0].UpdateTime != "刚刚更新" {
		t.Fatalf("unexpected single source response: %+v", singleResp)
	}

	requestedMu.Lock()
	requested = requested[:0]
	requestedMu.Unlock()
	firstMultiBody := fmt.Sprintf(
		`{"keyword":"游标","sourceIds":[%d,%d,%d],"concurrentCount":1,"page":1,"lastIndex":-1,"searchSize":1}`,
		sources[1].ID,
		sources[0].ID,
		sources[2].ID,
	)
	firstMultiReq := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(firstMultiBody))
	firstMultiReq.Header.Set("Content-Type", "application/json")
	firstMultiReq.Header.Set("Authorization", token)
	firstMultiW := httptest.NewRecorder()
	router.ServeHTTP(firstMultiW, firstMultiReq)
	if firstMultiW.Code != http.StatusOK {
		t.Fatalf("first multi search: expected 200, got %d: %s", firstMultiW.Code, firstMultiW.Body.String())
	}
	var firstMultiResp searchResponse
	if err := json.Unmarshal(firstMultiW.Body.Bytes(), &firstMultiResp); err != nil {
		t.Fatal(err)
	}
	if firstMultiResp.LastIndex != 0 || !firstMultiResp.HasMore ||
		len(firstMultiResp.List) != 1 || firstMultiResp.List[0].SourceID != sources[1].ID {
		t.Fatalf("unexpected first multi response: %+v", firstMultiResp)
	}

	secondMultiBody := fmt.Sprintf(
		`{"keyword":"游标","sourceIds":[%d,%d,%d],"concurrentCount":1,"page":2,"lastIndex":0,"searchSize":1}`,
		sources[1].ID,
		sources[0].ID,
		sources[2].ID,
	)
	secondMultiReq := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(secondMultiBody))
	secondMultiReq.Header.Set("Content-Type", "application/json")
	secondMultiReq.Header.Set("Authorization", token)
	secondMultiW := httptest.NewRecorder()
	router.ServeHTTP(secondMultiW, secondMultiReq)
	if secondMultiW.Code != http.StatusOK {
		t.Fatalf("second multi search: expected 200, got %d: %s", secondMultiW.Code, secondMultiW.Body.String())
	}
	var secondMultiResp searchResponse
	if err := json.Unmarshal(secondMultiW.Body.Bytes(), &secondMultiResp); err != nil {
		t.Fatal(err)
	}
	if secondMultiResp.LastIndex != 1 || !secondMultiResp.HasMore ||
		len(secondMultiResp.List) != 1 || secondMultiResp.List[0].SourceID != sources[0].ID {
		t.Fatalf("unexpected second multi response: %+v", secondMultiResp)
	}

	requestedMu.Lock()
	gotRequests := append([]string(nil), requested...)
	requestedMu.Unlock()
	if len(gotRequests) != 2 ||
		!strings.HasPrefix(gotRequests[0], "/source-b?") ||
		!strings.HasPrefix(gotRequests[1], "/source-a?") {
		t.Fatalf("multi source cursor did not preserve requested source order: %v", gotRequests)
	}
}

func TestBookSourceCustomOrderIsStableAcrossListsAndConcurrentSearch(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	for _, item := range []struct {
		name  string
		order int
	}{
		{name: "late", order: 20},
		{name: "first", order: 5},
	} {
		source := models.BookSource{
			Name:        item.name,
			BaseURL:     "https://order.example/" + item.name,
			Charset:     "utf-8",
			CustomOrder: item.order,
			Enabled:     true,
		}
		if err := source.SetRules(models.BookSourceRule{
			SearchURL:    "https://order.example/" + item.name + "?q={keyword}",
			ExploreURL:   "https://order.example/" + item.name + "/explore",
			BookListRule: ".book",
			BookNameRule: ".name",
			BookURLRule:  ".name|attr:href",
		}); err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&source).Error; err != nil {
			t.Fatal(err)
		}
	}

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			name := strings.Trim(strings.TrimSuffix(request.URL.Path, "/explore"), "/")
			if name == "first" {
				time.Sleep(25 * time.Millisecond)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`<article class="book"><a class="name" href="/book/` + name + `">` + name + `</a></article>`,
				)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	listReq := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	listReq.Header.Set("Authorization", token)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	var listed []models.BookSource
	if listW.Code != http.StatusOK || json.Unmarshal(listW.Body.Bytes(), &listed) != nil ||
		len(listed) != 2 || listed[0].Name != "first" || listed[1].Name != "late" {
		t.Fatalf("source list custom order: %d %+v", listW.Code, listed)
	}

	exploreReq := httptest.NewRequest(http.MethodGet, "/api/explore/sources", nil)
	exploreReq.Header.Set("Authorization", token)
	exploreW := httptest.NewRecorder()
	router.ServeHTTP(exploreW, exploreReq)
	var explored []exploreSourceResponse
	if exploreW.Code != http.StatusOK || json.Unmarshal(exploreW.Body.Bytes(), &explored) != nil ||
		len(explored) != 2 || explored[0].Name != "first" || explored[1].Name != "late" {
		t.Fatalf("explore source custom order: %d %+v", exploreW.Code, explored)
	}

	searchReq := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"keyword":"排序","concurrentCount":2}`))
	searchReq.Header.Set("Authorization", token)
	searchReq.Header.Set("Content-Type", "application/json")
	searchW := httptest.NewRecorder()
	router.ServeHTTP(searchW, searchReq)
	var searched []engine.SearchResult
	if searchW.Code != http.StatusOK || json.Unmarshal(searchW.Body.Bytes(), &searched) != nil ||
		len(searched) != 2 || searched[0].SourceName != "first" || searched[1].SourceName != "late" {
		t.Fatalf("concurrent search custom order: %d %+v", searchW.Code, searched)
	}
}

func TestSearchExecutesImportedUpstreamPostOptions(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	source := models.BookSource{Name: "上游 POST 源", Enabled: true, Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    `https://post-search.example/search, {"method":"POST","body":{"keyword":"{keyword}","page":"{page}"},"headers":{"X-Page":"{page}"}}`,
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost ||
				request.Header.Get("Content-Type") != "application/json; charset=utf-8" ||
				request.Header.Get("X-Page") != "2" {
				t.Fatalf("unexpected POST search request: method=%s headers=%v", request.Method, request.Header)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != `{"keyword":"中文书","page":"2"}` {
				t.Fatalf("unexpected POST search body: %s", body)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`<article class="book"><a class="name" href="/book/2">POST 搜索结果</a></article>`,
				)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	body := fmt.Sprintf(
		`{"keyword":"中文书","sourceIds":[%d],"page":2,"lastIndex":-1,"searchSize":20}`,
		source.ID,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"POST 搜索结果"`) ||
		!strings.Contains(w.Body.String(), `"page":2`) {
		t.Fatalf("POST source search: expected parsed second page, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUserReaderSettingsRoundTrip(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	body := `{"value":{"fontSize":22,"pageMode":"mobile","miniInterface":true,"mode":"scroll"},"baseUpdatedAt":""}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings/reader", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save settings: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var saved struct {
		Key       string         `json:"key"`
		Value     map[string]any `json:"value"`
		UpdatedAt string         `json:"updatedAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Key != "reader" || saved.Value["pageMode"] != nil || saved.Value["miniInterface"] != nil || saved.Value["mode"] != "scroll" || saved.UpdatedAt == "" {
		t.Fatalf("unexpected saved settings: %+v", saved)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings/reader", nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("load settings: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var loaded struct {
		Value map[string]any `json:"value"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Value["fontSize"].(float64) != 22 {
		t.Fatalf("unexpected loaded settings: %+v", loaded.Value)
	}
}

func TestBackupIncludesUserData(t *testing.T) {
	_, server := setupTestServer(t)

	setting := models.UserSetting{UserID: 1, Key: "reader", Value: `{"fontSize":24,"pageMode":"mobile","miniInterface":true}`}
	if err := server.db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: 1, Name: "备份分组", SortOrder: 7}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	categoryExtra := models.Category{UserID: 1, Name: "备份分组二", SortOrder: 8}
	if err := server.db.Create(&categoryExtra).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: 1, CategoryID: &category.ID, Title: "备份书", URL: "https://book.example/backup"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookCategory{UserID: 1, BookID: book.ID, CategoryID: category.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookCategory{UserID: 1, BookID: book.ID, CategoryID: categoryExtra.ID}).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{UserID: 1, BookID: book.ID, ChapterIndex: 4, Offset: 99, ChapterTitle: "进度章"}
	if err := server.db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	bookmark := models.Bookmark{UserID: 1, BookID: book.ID, ChapterIndex: 2, Offset: 42, Title: "备份书签"}
	if err := server.db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}
	rule := models.ReplaceRule{UserID: 1, Name: "备份规则", Pattern: "foo", Replacement: "bar", Enabled: true}
	if err := server.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}

	path, err := server.backupSvc.RunNow()
	if err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	entries := make(map[string]string)
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(data)
	}
	for _, name := range []string{"userSettings.json", "categories.json", "bookshelf.json", "bookmarks.json", "readingProgress.json", "replaceRules.json"} {
		if entries[name] == "" {
			t.Fatalf("%s not found in backup", name)
		}
	}
	if !strings.Contains(entries["userSettings.json"], `"key": "reader"`) || !strings.Contains(entries["userSettings.json"], `fontSize`) {
		t.Fatalf("unexpected user settings backup: %s", entries["userSettings.json"])
	}
	if strings.Contains(entries["userSettings.json"], "pageMode") || strings.Contains(entries["userSettings.json"], "miniInterface") {
		t.Fatalf("user settings backup kept local page mode: %s", entries["userSettings.json"])
	}
	if !strings.Contains(entries["categories.json"], `"name": "备份分组"`) {
		t.Fatalf("unexpected categories backup: %s", entries["categories.json"])
	}
	if !strings.Contains(entries["bookshelf.json"], `"categoryName": "备份分组"`) || !strings.Contains(entries["bookshelf.json"], `"categoryNames":`) || !strings.Contains(entries["bookshelf.json"], `"备份分组二"`) {
		t.Fatalf("unexpected bookshelf backup: %s", entries["bookshelf.json"])
	}
	if !strings.Contains(entries["bookmarks.json"], `"bookTitle": "备份书"`) || !strings.Contains(entries["bookmarks.json"], `"title": "备份书签"`) {
		t.Fatalf("unexpected bookmarks backup: %s", entries["bookmarks.json"])
	}
	if !strings.Contains(entries["readingProgress.json"], `"bookTitle": "备份书"`) || !strings.Contains(entries["readingProgress.json"], `"chapterTitle": "进度章"`) {
		t.Fatalf("unexpected reading progress backup: %s", entries["readingProgress.json"])
	}
	if !strings.Contains(entries["replaceRules.json"], `"pattern": "foo"`) {
		t.Fatalf("unexpected replace rules backup: %s", entries["replaceRules.json"])
	}
}

func TestAdminUsersIncludesGlobalSourceCount(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&user).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookSource{Name: "源一", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookSource{Name: "源二", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin users: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var users []struct {
		Username    string `json:"username"`
		SourceCount int64  `json:"sourceCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "testuser" || users[0].SourceCount != 2 {
		t.Fatalf("unexpected admin users response: %+v", users)
	}
}

func TestAdminUserManagementActions(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var admin models.User
	if err := server.db.Where("username = ?", "testuser").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&admin).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(`{"username":"managed","password":"secret123","canEditSources":false,"canAccessStore":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create managed user: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var managed models.User
	if err := json.Unmarshal(w.Body.Bytes(), &managed); err != nil {
		t.Fatal(err)
	}
	if managed.Username != "managed" || managed.CanEditSources {
		t.Fatalf("unexpected managed user response: %+v", managed)
	}

	resetReq := httptest.NewRequest(http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(managed.ID), 10)+"/password", strings.NewReader(`{"password":"changed123"}`))
	resetReq.Header.Set("Content-Type", "application/json")
	resetReq.Header.Set("Authorization", token)
	resetW := httptest.NewRecorder()
	router.ServeHTTP(resetW, resetReq)
	if resetW.Code != http.StatusOK {
		t.Fatalf("reset password: expected 200, got %d: %s", resetW.Code, resetW.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"managed","password":"changed123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	router.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("login with reset password: expected 200, got %d: %s", loginW.Code, loginW.Body.String())
	}

	category := models.Category{UserID: managed.ID, Name: "待删分组"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: managed.ID, CategoryID: &category.ID, Title: "待删书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReadingProgress{UserID: managed.ID, BookID: book.ID, ChapterIndex: 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Bookmark{UserID: managed.ID, BookID: book.ID, Title: "书签"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReplaceRule{UserID: managed.ID, Name: "规则", Pattern: "foo", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	rss := models.RSSSource{UserID: managed.ID, Title: "RSS", URL: "https://example.com/rss", Enabled: true}
	if err := server.db.Create(&rss).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.RSSArticle{UserID: managed.ID, SourceID: rss.ID, Title: "文章"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.UserSetting{UserID: managed.ID, Key: "reader", Value: `{}`}).Error; err != nil {
		t.Fatal(err)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-delete", strings.NewReader(fmt.Sprintf(`{"ids":[%d]}`, managed.ID)))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteReq.Header.Set("Authorization", token)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("delete users: expected 200, got %d: %s", deleteW.Code, deleteW.Body.String())
	}

	for name, query := range map[string]func() int64{
		"users": func() int64 {
			var count int64
			_ = server.db.Model(&models.User{}).Where("id = ?", managed.ID).Count(&count).Error
			return count
		},
		"books": func() int64 {
			var count int64
			_ = server.db.Model(&models.Book{}).Where("user_id = ?", managed.ID).Count(&count).Error
			return count
		},
		"chapters": func() int64 {
			var count int64
			_ = server.db.Model(&models.Chapter{}).Where("book_id = ?", book.ID).Count(&count).Error
			return count
		},
		"progress": func() int64 {
			var count int64
			_ = server.db.Model(&models.ReadingProgress{}).Where("user_id = ?", managed.ID).Count(&count).Error
			return count
		},
		"bookmarks": func() int64 {
			var count int64
			_ = server.db.Model(&models.Bookmark{}).Where("user_id = ?", managed.ID).Count(&count).Error
			return count
		},
		"replace rules": func() int64 {
			var count int64
			_ = server.db.Model(&models.ReplaceRule{}).Where("user_id = ?", managed.ID).Count(&count).Error
			return count
		},
		"rss": func() int64 {
			var count int64
			_ = server.db.Model(&models.RSSSource{}).Where("user_id = ?", managed.ID).Count(&count).Error
			return count
		},
		"user settings": func() int64 {
			var count int64
			_ = server.db.Model(&models.UserSetting{}).Where("user_id = ?", managed.ID).Count(&count).Error
			return count
		},
	} {
		if count := query(); count != 0 {
			t.Fatalf("%s were not deleted, count=%d", name, count)
		}
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
	book := models.Book{UserID: user.ID, SourceID: 1, Title: "旧书名", Author: "旧作者", CanUpdate: true}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"title":"新书名","author":"新作者","coverUrl":"https://example.com/cover.jpg","customCoverUrl":"/uploads/covers/custom.jpg","intro":"新简介","canUpdate":false}`
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
	if updated.CoverURL != "https://example.com/cover.jpg" || updated.CustomCoverURL != "/uploads/covers/custom.jpg" {
		t.Fatalf("unexpected cover fields after update: %+v", updated)
	}
	if updated.CanUpdate {
		t.Fatalf("expected canUpdate to be false after update: %+v", updated)
	}
}

func TestUpdateBookPartialPayloadPreservesFields(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "分组", Show: true}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:         user.ID,
		SourceID:       1,
		CategoryID:     &category.ID,
		Title:          "原书名",
		Author:         "原作者",
		CoverURL:       "https://example.com/source-cover.jpg",
		CustomCoverURL: "/uploads/covers/custom.jpg",
		Intro:          "原简介",
		CanUpdate:      true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10), strings.NewReader(`{"canUpdate":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("partial update book: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Book
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != book.Title || updated.Author != book.Author || updated.Intro != book.Intro {
		t.Fatalf("partial update should preserve text fields: %+v", updated)
	}
	if updated.CoverURL != book.CoverURL || updated.CustomCoverURL != book.CustomCoverURL {
		t.Fatalf("partial update should preserve cover fields: %+v", updated)
	}
	if updated.CategoryID == nil || *updated.CategoryID != category.ID {
		t.Fatalf("partial update should preserve category: %+v", updated.CategoryID)
	}
	if updated.CanUpdate {
		t.Fatalf("expected canUpdate false after partial update: %+v", updated)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10), strings.NewReader(`{"categoryId":null}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("clear category: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	updated = models.Book{}
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.CategoryID != nil {
		t.Fatalf("expected category to be cleared when categoryId is null: %+v", updated.CategoryID)
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
	progress := models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterIndex: 3, Percent: 0.42, ChapterPercent: 0.73, ChapterTitle: "第三章"}
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
			BookID         uint    `json:"bookId"`
			ChapterIndex   int     `json:"chapterIndex"`
			Percent        float64 `json:"percent"`
			ChapterPercent float64 `json:"chapterPercent"`
			ChapterTitle   string  `json:"chapterTitle"`
		} `json:"progress"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &books); err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Progress == nil || books[0].Progress.BookID != book.ID || books[0].Progress.ChapterIndex != 3 {
		t.Fatalf("expected embedded progress, got %+v", books)
	}
	if books[0].Progress.ChapterPercent != 0.73 || books[0].Progress.ChapterTitle != "第三章" {
		t.Fatalf("expected chapter progress embedded, got %+v", books[0].Progress)
	}
}

func TestGetBookIncludesShelfProgress(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "详情进度"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{
		UserID:         user.ID,
		BookID:         book.ID,
		ChapterIndex:   5,
		Offset:         1234,
		Percent:        0.12,
		ChapterPercent: 0.34,
		ChapterTitle:   "第六章",
	}
	if err := server.db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10), nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get book: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var item struct {
		ID           uint      `json:"id"`
		ShelfOrderAt time.Time `json:"shelfOrderAt"`
		Progress     *struct {
			BookID         uint    `json:"bookId"`
			ChapterIndex   int     `json:"chapterIndex"`
			Offset         int     `json:"offset"`
			ChapterPercent float64 `json:"chapterPercent"`
			ChapterTitle   string  `json:"chapterTitle"`
		} `json:"progress"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID != book.ID || item.Progress == nil || item.Progress.BookID != book.ID {
		t.Fatalf("expected detail shelf item with progress, got %+v", item)
	}
	if item.Progress.ChapterIndex != 5 || item.Progress.Offset != 1234 || item.Progress.ChapterPercent != 0.34 || item.Progress.ChapterTitle != "第六章" {
		t.Fatalf("unexpected detail progress: %+v", item.Progress)
	}
	if item.ShelfOrderAt.IsZero() || !item.ShelfOrderAt.Equal(progress.UpdatedAt) {
		t.Fatalf("expected shelfOrderAt to follow progress update time, got shelf=%s progress=%s", item.ShelfOrderAt, progress.UpdatedAt)
	}
}

func TestBookShelfListItemIncludesProgressOrder(t *testing.T) {
	_, server := setupTestServer(t)

	user := models.User{Username: "payload-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "广播书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&book).Updates(map[string]any{
		"created_at": time.Now().Add(-3 * time.Hour),
		"updated_at": time.Now().Add(-3 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{
		UserID:         user.ID,
		BookID:         book.ID,
		ChapterIndex:   9,
		Offset:         2048,
		Percent:        0.31,
		ChapterPercent: 0.55,
		ChapterTitle:   "第十章",
	}
	if err := server.db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}

	item := server.bookShelfListItem(user.ID, book)
	if item.Progress == nil || item.Progress.BookID != book.ID || item.Progress.ChapterIndex != 9 {
		t.Fatalf("expected embedded progress in shelf payload, got %+v", item.Progress)
	}
	if item.ShelfOrderAt.IsZero() || !item.ShelfOrderAt.Equal(item.Progress.UpdatedAt) {
		t.Fatalf("expected shelf order to follow progress update time, got shelf=%s progress=%s", item.ShelfOrderAt, item.Progress.UpdatedAt)
	}
}

func TestUpdateProgressPersistsChapterPosition(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "章节内进度"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 8, Title: "第九章"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":8,"offset":2048,"percent":0.021,"chapterPercent":0.638,"chapterTitle":"第九章","mode":"scroll"}`, book.ID, chapter.ID)
	req := httptest.NewRequest(http.MethodPut, "/api/progress", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update progress: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var saved models.ReadingProgress
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Offset != 2048 || saved.ChapterPercent != 0.638 || saved.ChapterTitle != "第九章" {
		t.Fatalf("unexpected saved progress: %+v", saved)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/progress/"+strconv.FormatUint(uint64(book.ID), 10), nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get progress: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var loaded models.ReadingProgress
	if err := json.Unmarshal(w.Body.Bytes(), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Offset != 2048 || loaded.ChapterPercent != 0.638 || loaded.ChapterTitle != "第九章" {
		t.Fatalf("unexpected loaded progress: %+v", loaded)
	}
}

func TestUpdateProgressRejectsStaleClientBase(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "多端进度"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	existing := models.ReadingProgress{
		UserID:         user.ID,
		BookID:         book.ID,
		ChapterIndex:   12,
		Offset:         4096,
		Percent:        0.4,
		ChapterPercent: 0.62,
		ChapterTitle:   "第十三章",
		UpdatedAt:      time.Now().UTC(),
	}
	if err := server.db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	staleBase := existing.UpdatedAt.Add(-time.Minute).Format(time.RFC3339Nano)
	body := fmt.Sprintf(`{"bookId":%d,"chapterIndex":3,"offset":128,"percent":0.1,"chapterPercent":0.2,"chapterTitle":"第四章","baseUpdatedAt":%q}`, book.ID, staleBase)
	req := httptest.NewRequest(http.MethodPut, "/api/progress", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update progress: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-OpenReader-Progress-Conflict") != "1" {
		t.Fatalf("expected stale progress conflict header, got %q", w.Header().Get("X-OpenReader-Progress-Conflict"))
	}

	var returned models.ReadingProgress
	if err := json.Unmarshal(w.Body.Bytes(), &returned); err != nil {
		t.Fatal(err)
	}
	if returned.ChapterIndex != existing.ChapterIndex || returned.Offset != existing.Offset || returned.ChapterTitle != existing.ChapterTitle {
		t.Fatalf("expected existing progress to be returned, got %+v", returned)
	}

	var saved models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.ChapterIndex != existing.ChapterIndex || saved.Offset != existing.Offset || saved.ChapterTitle != existing.ChapterTitle {
		t.Fatalf("stale update overwrote progress: %+v", saved)
	}
}

func TestUpdateProgressRejectsOlderClientWithoutBase(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "无基线旧进度"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	existing := models.ReadingProgress{
		UserID:         user.ID,
		BookID:         book.ID,
		ChapterIndex:   20,
		Offset:         8000,
		Percent:        0.6,
		ChapterPercent: 0.44,
		ChapterTitle:   "第二十一章",
		UpdatedAt:      time.Now().UTC(),
	}
	if err := server.db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	clientUpdatedAt := existing.UpdatedAt.Add(-2 * time.Minute).Format(time.RFC3339Nano)
	body := fmt.Sprintf(`{"bookId":%d,"chapterIndex":2,"offset":12,"percent":0.02,"chapterPercent":0.03,"chapterTitle":"第三章","clientUpdatedAt":%q}`, book.ID, clientUpdatedAt)
	req := httptest.NewRequest(http.MethodPut, "/api/progress", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update progress: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-OpenReader-Progress-Conflict") != "1" {
		t.Fatalf("expected stale progress conflict header, got %q", w.Header().Get("X-OpenReader-Progress-Conflict"))
	}

	var saved models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.ChapterIndex != existing.ChapterIndex || saved.Offset != existing.Offset || saved.ChapterTitle != existing.ChapterTitle {
		t.Fatalf("stale no-base update overwrote progress: %+v", saved)
	}
}

func TestListBooksOrdersByRecentProgressThenShelfTime(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	oldBook := models.Book{UserID: user.ID, Title: "旧书"}
	newBook := models.Book{UserID: user.ID, Title: "新导入"}
	readBook := models.Book{UserID: user.ID, Title: "最近读"}
	if err := server.db.Create(&oldBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&newBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&readBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&oldBook).Updates(map[string]any{
		"created_at": time.Now().Add(-4 * time.Hour),
		"updated_at": time.Now().Add(-4 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&newBook).Updates(map[string]any{
		"created_at": time.Now().Add(-2 * time.Hour),
		"updated_at": time.Now().Add(-2 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&readBook).Updates(map[string]any{
		"created_at": time.Now().Add(-6 * time.Hour),
		"updated_at": time.Now().Add(-6 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{UserID: user.ID, BookID: readBook.ID, ChapterIndex: 1, Percent: 0.2}
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
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &books); err != nil {
		t.Fatal(err)
	}
	if len(books) != 3 {
		t.Fatalf("expected 3 books, got %+v", books)
	}
	want := []uint{readBook.ID, newBook.ID, oldBook.ID}
	for index, id := range want {
		if books[index].ID != id {
			t.Fatalf("unexpected order at %d: got %+v want %+v", index, books, want)
		}
	}
}

func TestListBooksIncludesRemoteCachedChapterCount(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	remoteBook := models.Book{UserID: user.ID, SourceID: 1, Title: "远程缓存书"}
	localBook := models.Book{UserID: user.ID, SourceID: 0, Title: "本地书"}
	if err := server.db.Create(&remoteBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&localBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: remoteBook.ID, Index: 0, Title: "远程已缓存", CachePath: "remote-cache/chapter-1.txt"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: remoteBook.ID, Index: 1, Title: "远程未缓存"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: localBook.ID, Index: 0, Title: "本地章节", CachePath: "local-cache/chapter-1.txt"}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list books: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var items []bookListItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	countByTitle := map[string]int64{}
	for _, item := range items {
		countByTitle[item.Title] = item.CachedChapterCount
	}
	if countByTitle["远程缓存书"] != 1 {
		t.Fatalf("expected remote cached count 1, got %+v", countByTitle)
	}
	if countByTitle["本地书"] != 0 {
		t.Fatalf("expected local book server cached count 0, got %+v", countByTitle)
	}
}

func TestListBooksOrdersNewImportBeforeStaleProgress(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	staleReadBook := models.Book{UserID: user.ID, Title: "旧进度"}
	newBook := models.Book{UserID: user.ID, Title: "新导入"}
	if err := server.db.Create(&staleReadBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&newBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&staleReadBook).Updates(map[string]any{
		"created_at": time.Now().Add(-8 * time.Hour),
		"updated_at": time.Now().Add(-8 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&newBook).Updates(map[string]any{
		"created_at": time.Now().Add(-1 * time.Hour),
		"updated_at": time.Now().Add(-1 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	progress := models.ReadingProgress{
		UserID:       user.ID,
		BookID:       staleReadBook.ID,
		ChapterIndex: 1,
		Percent:      0.2,
		UpdatedAt:    time.Now().Add(-6 * time.Hour),
	}
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
		ID           uint      `json:"id"`
		ShelfOrderAt time.Time `json:"shelfOrderAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &books); err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 || books[0].ID != newBook.ID || books[1].ID != staleReadBook.ID {
		t.Fatalf("expected new import before stale progress, got %+v", books)
	}
	if books[0].ShelfOrderAt.IsZero() || books[1].ShelfOrderAt.IsZero() {
		t.Fatalf("expected shelfOrderAt on listed books, got %+v", books)
	}
	if !books[0].ShelfOrderAt.After(books[1].ShelfOrderAt) {
		t.Fatalf("expected shelfOrderAt to match list order, got %+v", books)
	}
}

func TestBookMutationsReturnShelfListItems(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "单书分组"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books", strings.NewReader(`{"title":"单书响应"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID           uint      `json:"id"`
		Title        string    `json:"title"`
		ShelfOrderAt time.Time `json:"shelfOrderAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Title != "单书响应" || created.ShelfOrderAt.IsZero() {
		t.Fatalf("expected create response shelf item, got %+v", created)
	}

	body := `{"categoryId":` + strconv.FormatUint(uint64(category.ID), 10) + `}`
	req = httptest.NewRequest(http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(created.ID), 10)+"/category", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update category: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated struct {
		ID           uint      `json:"id"`
		CategoryID   *uint     `json:"categoryId"`
		ShelfOrderAt time.Time `json:"shelfOrderAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.CategoryID == nil || *updated.CategoryID != category.ID || updated.ShelfOrderAt.IsZero() {
		t.Fatalf("expected category response shelf item, got %+v", updated)
	}
}

func TestBookMultiCategoryMembership(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	categoryA := models.Category{UserID: user.ID, Name: "多分组A"}
	categoryB := models.Category{UserID: user.ID, Name: "多分组B"}
	if err := server.db.Create(&categoryA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&categoryB).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "多分组书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"categoryIds":[%d,%d]}`, categoryA.ID, categoryB.ID)
	req := httptest.NewRequest(http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/category", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set multi category: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated struct {
		ID          uint   `json:"id"`
		CategoryID  *uint  `json:"categoryId"`
		CategoryIDs []uint `json:"categoryIds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.CategoryID == nil || *updated.CategoryID != categoryA.ID || !sameUintSet(updated.CategoryIDs, []uint{categoryA.ID, categoryB.ID}) {
		t.Fatalf("expected both categories in response, got %+v", updated)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books?categoryId="+strconv.FormatUint(uint64(categoryB.ID), 10), nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("filter category B: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var filtered []struct {
		ID          uint   `json:"id"`
		CategoryIDs []uint `json:"categoryIds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].ID != book.ID || !sameUintSet(filtered[0].CategoryIDs, []uint{categoryA.ID, categoryB.ID}) {
		t.Fatalf("expected book to appear under second category, got %+v", filtered)
	}

	body = fmt.Sprintf(`{"action":"category-remove","bookIds":[%d],"categoryId":%d}`, book.ID, categoryA.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch remove category: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var batchResp struct {
		Books []struct {
			ID          uint   `json:"id"`
			CategoryID  *uint  `json:"categoryId"`
			CategoryIDs []uint `json:"categoryIds"`
		} `json:"books"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &batchResp); err != nil {
		t.Fatal(err)
	}
	if len(batchResp.Books) != 1 || batchResp.Books[0].CategoryID == nil || *batchResp.Books[0].CategoryID != categoryB.ID || !sameUintSet(batchResp.Books[0].CategoryIDs, []uint{categoryB.ID}) {
		t.Fatalf("expected only category B after remove, got %+v", batchResp)
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
	var categoryResp struct {
		Affected int `json:"affected"`
		Books    []struct {
			ID           uint      `json:"id"`
			CategoryID   *uint     `json:"categoryId"`
			ShelfOrderAt time.Time `json:"shelfOrderAt"`
		} `json:"books"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &categoryResp); err != nil {
		t.Fatal(err)
	}
	if categoryResp.Affected != 2 || len(categoryResp.Books) != 2 {
		t.Fatalf("expected category response with 2 updated books, got %+v", categoryResp)
	}
	for _, item := range categoryResp.Books {
		if item.CategoryID == nil || *item.CategoryID != category.ID || item.ShelfOrderAt.IsZero() {
			t.Fatalf("expected updated shelf item with category and shelf order, got %+v", item)
		}
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
	var deleteResp struct {
		Affected   int    `json:"affected"`
		DeletedIDs []uint `json:"deletedIds"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &deleteResp); err != nil {
		t.Fatal(err)
	}
	if deleteResp.Affected != 2 || len(deleteResp.DeletedIDs) != 2 {
		t.Fatalf("expected delete response with 2 deleted ids, got %+v", deleteResp)
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

func TestUpdateCategoryVisibility(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "可隐藏分组", Show: true, SortOrder: 10}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/categories/"+strconv.FormatUint(uint64(category.ID), 10), strings.NewReader(`{"show":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("hide category: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated models.Category
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Show {
		t.Fatalf("expected hidden category, got %+v", updated)
	}
	if updated.Name != category.Name {
		t.Fatalf("visibility update should not rename category: %+v", updated)
	}
}

func TestDeleteCategoryRejectsNonEmptyCategory(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "非空分组", SortOrder: 10}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, CategoryID: &category.ID, Title: "分组内的书", Author: "作者"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/categories/"+strconv.FormatUint(uint64(category.ID), 10), nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete non-empty category: expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var storedCategory models.Category
	if err := server.db.First(&storedCategory, category.ID).Error; err != nil {
		t.Fatalf("category should still exist: %v", err)
	}
	var storedBook models.Book
	if err := server.db.First(&storedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedBook.CategoryID == nil || *storedBook.CategoryID != category.ID {
		t.Fatalf("book category should be preserved, got %+v", storedBook.CategoryID)
	}
}

func TestSourceManagement(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	// create source
	body := `{"name":"测试书源","baseUrl":"https://example.com","bookUrlPattern":"/book/\\d+$","bookSourceType":1,"bookSourceComment":"音频测试源","charset":"utf-8","concurrentRate":"3/1000","header":"{\"X-Source\":\"yes\"}","loginUrl":"https://example.com/login","loginCheckJs":"check()","lastUpdateTime":1750000000000,"weight":6,"respondTime":4321,"enabledExplore":false}`
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
	if sources[0].ConcurrentRate != "3/1000" {
		t.Fatalf("source concurrent rate was not persisted: %+v", sources[0])
	}
	if sources[0].Header != `{"X-Source":"yes"}` ||
		sources[0].LoginURL != "https://example.com/login" || sources[0].LoginCheckJS != "check()" ||
		sources[0].LastUpdateTime != 1750000000000 || sources[0].Weight != 6 || sources[0].RespondTime != 4321 {
		t.Fatalf("source upstream metadata was not persisted: %+v", sources[0])
	}
	if sources[0].IsExploreEnabled() {
		t.Fatalf("source enabledExplore=false was not persisted: %+v", sources[0])
	}
	if sources[0].BookURLPattern != `/book/\d+$` || sources[0].SourceType != 1 || sources[0].Comment != "音频测试源" {
		t.Fatalf("source detail metadata was not persisted: %+v", sources[0])
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
		Name:           "待编辑",
		BaseURL:        "https://example.com",
		SearchURL:      "https://example.com/search",
		Charset:        "gbk",
		ConcurrentRate: "1000",
		Header:         `{"X-Source":"old"}`,
		LoginURL:       "https://example.com/login",
		LoginCheckJS:   "check()",
		LastUpdateTime: 1750000000000,
		Weight:         6,
		RespondTime:    4321,
		Group:          "旧分组",
		Rules:          `{"searchUrl":"x"}`,
		Enabled:        true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"name":"待编辑","baseUrl":"","searchUrl":"","charset":"","concurrentRate":"","header":"","loginUrl":"","loginCheckJs":"","lastUpdateTime":0,"weight":0,"respondTime":0,"group":"","rules":"","enabled":false}`
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
	if updated.BaseURL != "" || updated.SearchURL != "" || updated.ConcurrentRate != "" || updated.Header != "" ||
		updated.LoginURL != "" || updated.LoginCheckJS != "" || updated.LastUpdateTime != 0 ||
		updated.Weight != 0 || updated.RespondTime != 0 || updated.Group != "" ||
		updated.Rules != "" || updated.Charset != "utf-8" || updated.Enabled {
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

func TestSourceEditingPermissionBlocksMutations(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	if err := server.db.Model(&models.User{}).Where("username = ?", "testuser").Update("can_edit_sources", false).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(`{"name":"禁止新增"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("create source without permission: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("list sources without edit permission: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestDecodeBookSourcesEnabledDefaults(t *testing.T) {
	sources, err := decodeBookSources([]byte(`[
		{"name":"默认启用"},
		{"name":"显式停用","enabled":false,"enabledExplore":false}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if !sources[0].Enabled || !sources[0].IsExploreEnabled() {
		t.Fatalf("expected missing enable flags to default true: %+v", sources[0])
	}
	if sources[0].RespondTime != 180000 {
		t.Fatalf("expected missing respondTime to use upstream default: %+v", sources[0])
	}
	if sources[1].Enabled || sources[1].IsExploreEnabled() {
		t.Fatalf("expected explicit false flags to be preserved: %+v", sources[1])
	}
}

func TestDecodeBookSourcesAcceptsUpstreamReaderFields(t *testing.T) {
	sources, err := decodeBookSources([]byte(`[
		{
			"bookSourceName":"上游源",
			"bookSourceUrl":"https://reader.example",
			"bookSourceGroup":"分组A",
			"searchUrl":"https://reader.example/search, {\"method\":\"POST\",\"body\":\"key={{key}}&page={{page}}\",\"headers\":{\"X-Page\":\"{{page}}\"}}",
			"exploreUrl":"https://reader.example/top/{page}",
			"headerMap":{"User-Agent":"OpenReader Test","Referer":"https://reader.example"},
			"concurrentRate":"2/1000",
			"loginUrl":"https://reader.example/login",
			"loginCheckJs":"return source.isLogin()",
			"customOrder":37,
			"lastUpdateTime":1710000000000,
			"weight":12,
			"respondTime":3456,
			"bookUrlPattern":"/detail/\\d+$",
			"bookSourceType":1,
			"bookSourceComment":"上游注释",
			"enabledExplore":false,
			"enabled":false,
			"ruleSearch":{
				"bookList":".book",
				"name":".name",
				"author":".author",
				"coverUrl":"img@src",
				"intro":".intro",
				"kind":".kind",
				"wordCount":".words",
				"lastChapter":".last",
				"updateTime":".updated",
				"bookUrl":"a@href"
			},
			"ruleExplore":{
				"bookList":".explore-book",
				"name":".explore-name",
				"author":".explore-author",
				"coverUrl":"img@data-src",
				"intro":".explore-intro",
				"kind":".explore-kind",
				"wordCount":".explore-words",
				"lastChapter":".explore-last",
				"updateTime":".explore-updated",
				"bookUrl":"a@data-url"
			},
			"ruleBookInfo":{
				"init":"@js:book.init = true",
				"name":"h1@text",
				"author":".detail-author@text",
				"coverUrl":"img.cover@data-src",
				"intro":".detail-intro@text",
				"kind":".detail-kind@text",
				"lastChapter":".detail-last@text",
				"updateTime":".detail-update@text",
				"wordCount":".detail-words@text",
				"tocUrl":".catalog@href",
				"canReName":".can-rename"
			},
			"ruleToc":{
				"preUpdateJs":"@js:book.preUpdate = true",
				"chapterList":"-.chapter",
				"chapterName":".title",
				"chapterUrl":"a@href",
				"isVolume":".volume",
				"isVip":".vip",
				"updateTime":".chapter-updated"
			},
			"ruleContent":{
				"content":"#content",
				"webJs":"@js:result",
				"sourceRegex":"source-(.*)",
				"replaceRegex":"replace##with",
				"imageStyle":"FULL"
			}
		}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	source := sources[0]
	if source.Name != "上游源" || source.BaseURL != "https://reader.example" || source.Group != "分组A" ||
		source.BookURLPattern != `/detail/\d+$` || source.SourceType != 1 || source.Comment != "上游注释" ||
		source.Charset != "auto" ||
		source.ConcurrentRate != "2/1000" || !strings.Contains(source.Header, `"Referer":"https://reader.example"`) ||
		source.LoginURL != "https://reader.example/login" ||
		source.LoginCheckJS != "return source.isLogin()" || source.CustomOrder != 37 ||
		source.LastUpdateTime != 1710000000000 || source.Weight != 12 || source.RespondTime != 3456 ||
		source.Enabled || source.IsExploreEnabled() {
		t.Fatalf("unexpected upstream source mapping: %+v", source)
	}
	rule, err := source.ParsedRules()
	if err != nil {
		t.Fatal(err)
	}
	if rule.SearchURL != `https://reader.example/search, {"method":"POST","body":"key={keyword}&page={page}","headers":{"X-Page":"{page}"}}` ||
		rule.ExploreURL != "https://reader.example/top/{page}" ||
		rule.BookListRule != ".book" ||
		rule.BookNameRule != ".name" ||
		rule.BookURLRule != "a|attr:href" ||
		rule.BookKindRule != ".kind" ||
		rule.BookWordCountRule != ".words" ||
		rule.BookUpdateTimeRule != ".updated" ||
		rule.ExploreBookListRule != ".explore-book" ||
		rule.ExploreBookNameRule != ".explore-name" ||
		rule.ExploreBookAuthorRule != ".explore-author" ||
		rule.ExploreBookCoverRule != "img|attr:data-src" ||
		rule.ExploreBookIntroRule != ".explore-intro" ||
		rule.ExploreBookKindRule != ".explore-kind" ||
		rule.ExploreBookWordCountRule != ".explore-words" ||
		rule.ExploreLatestChapterRule != ".explore-last" ||
		rule.ExploreBookUpdateTimeRule != ".explore-updated" ||
		rule.ExploreBookURLRule != "a|attr:data-url" ||
		rule.BookInfoInitRule != "@js:book.init = true" ||
		rule.BookInfoNameRule != "h1|text" ||
		rule.BookInfoAuthorRule != ".detail-author|text" ||
		rule.BookInfoCoverRule != "img.cover|attr:data-src" ||
		rule.BookInfoIntroRule != ".detail-intro|text" ||
		rule.BookInfoKindRule != ".detail-kind|text" ||
		rule.BookInfoLatestChapterRule != ".detail-last|text" ||
		rule.BookInfoUpdateTimeRule != ".detail-update|text" ||
		rule.BookInfoWordCountRule != ".detail-words|text" ||
		rule.BookInfoCanRenameRule != ".can-rename" ||
		rule.TOCURLRule != ".catalog|attr:href" ||
		rule.ChapterPreUpdateJSRule != "@js:book.preUpdate = true" ||
		rule.ChapterListRule != "-.chapter" ||
		rule.ChapterURLRule != "a|attr:href" ||
		rule.ChapterIsVolumeRule != ".volume" ||
		rule.ChapterIsVIPRule != ".vip" ||
		rule.ChapterUpdateTimeRule != ".chapter-updated" ||
		rule.ContentRule != "#content" ||
		rule.ContentWebJSRule != "@js:result" ||
		rule.ContentSourceRegex != "source-(.*)" ||
		rule.ContentReplaceRegex != "replace##with" ||
		rule.ContentImageStyle != "FULL" ||
		rule.Headers["User-Agent"] != "OpenReader Test" ||
		rule.Headers["Referer"] != "https://reader.example" {
		t.Fatalf("unexpected converted rules: %+v", rule)
	}
	if exported := exportUpstreamURLTemplate(rule.SearchURL); exported !=
		`https://reader.example/search, {"method":"POST","body":"key={{key}}&page={{page}}","headers":{"X-Page":"{{page}}"}}` {
		t.Fatalf("POST URL options were not preserved for upstream export: %s", exported)
	}
}

func TestBookSourceParsedRulesMergesRawStaticHeader(t *testing.T) {
	source := models.BookSource{
		Header: `{"X-Base":"raw","X-Override":"raw"}`,
		Rules:  `{"headers":{"x-override":"rule","X-Rule":"yes"}}`,
	}
	rule, err := source.ParsedRules()
	if err != nil {
		t.Fatal(err)
	}
	if rule.Headers["X-Base"] != "raw" || rule.Headers["x-override"] != "rule" || rule.Headers["X-Rule"] != "yes" {
		t.Fatalf("merged source headers = %+v", rule.Headers)
	}
	for name := range rule.Headers {
		if name == "X-Override" {
			t.Fatalf("case-insensitive overridden raw header remained: %+v", rule.Headers)
		}
	}

	source.Header = "@js:return {'X-Dynamic':'yes'}"
	rule, err = source.ParsedRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rule.Headers) != 2 || rule.Headers["X-Base"] != "" {
		t.Fatalf("dynamic raw header should be preserved but not executed: %+v", rule.Headers)
	}
}

func TestBookSourceCompatibilityRuleNormalization(t *testing.T) {
	for _, test := range []struct {
		name     string
		upstream string
		internal string
		exported string
	}{
		{name: "attribute", upstream: "a.book@href", internal: "a.book|attr:href", exported: "a.book@href"},
		{name: "data attribute", upstream: "img@data-src", internal: "img|attr:data-src", exported: "img@data-src"},
		{name: "explicit text", upstream: ".name@text", internal: ".name|text", exported: ".name@text"},
		{name: "explicit html", upstream: "#content@html", internal: "#content|html", exported: "#content@html"},
		{name: "plain selector", upstream: ".book", internal: ".book", exported: ".book"},
		{name: "xpath remains untouched", upstream: "//a/@href", internal: "//a/@href", exported: "//a/@href"},
	} {
		t.Run(test.name, func(t *testing.T) {
			internal := normalizeUpstreamSelectorRule(test.upstream)
			if internal != test.internal {
				t.Fatalf("normalize %q = %q, want %q", test.upstream, internal, test.internal)
			}
			if exported := exportUpstreamSelectorRule(internal); exported != test.exported {
				t.Fatalf("export %q = %q, want %q", internal, exported, test.exported)
			}
		})
	}

	if got := normalizeUpstreamURLTemplate("https://example/search?q={{key}}&page={{page}}"); got != "https://example/search?q={keyword}&page={page}" {
		t.Fatalf("normalize upstream URL = %q", got)
	}
	if got := exportUpstreamURLTemplate("https://example/search?q={keyword}&page={page}"); got != "https://example/search?q={{key}}&page={{page}}" {
		t.Fatalf("export upstream URL = %q", got)
	}
}

func TestImportSourcesAcceptsUpstreamReaderFields(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bookSources.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write([]byte(`[
		{
			"bookSourceName":"上传上游源",
			"bookSourceUrl":"https://upload-reader.example",
			"bookSourceGroup":"上传分组",
			"searchUrl":"https://upload-reader.example/search?q={{key}}",
			"exploreUrl":"https://upload-reader.example/explore/{{page}}",
			"headerMap":{"X-Source-Token":"upload-secret","Referer":"https://upload-reader.example/"},
			"loginUrl":"https://upload-reader.example/login",
			"loginCheckJs":"checkLogin()",
			"lastUpdateTime":1720000000000,
			"weight":8,
			"respondTime":9876,
			"ruleSearch":{"bookList":".item","name":".name","bookUrl":"a@href"},
			"ruleExplore":{"bookList":".explore-item","name":".explore-name","bookUrl":"a@data-url"},
			"ruleBookInfo":{"name":".detail-name","author":".detail-author","coverUrl":"img@data-src","intro":".detail-intro","tocUrl":".catalog@href","canReName":".allow-rename"},
			"ruleToc":{"chapterList":".chapter","chapterName":".chapter-name","chapterUrl":"a@href","nextTocUrl":".toc-next@href"},
			"ruleContent":{"content":".content","nextContentUrl":".content-next@href"}
		}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sources/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("import upstream sources: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"imported":1`) {
		t.Fatalf("unexpected import response: %s", w.Body.String())
	}

	var source models.BookSource
	if err := server.db.Where("name = ?", "上传上游源").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.BaseURL != "https://upload-reader.example" || source.Group != "上传分组" ||
		!strings.Contains(source.Header, `"X-Source-Token":"upload-secret"`) ||
		source.LoginURL != "https://upload-reader.example/login" || source.LoginCheckJS != "checkLogin()" ||
		source.LastUpdateTime != 1720000000000 || source.Weight != 8 || source.RespondTime != 9876 {
		t.Fatalf("unexpected imported source: %+v", source)
	}
	rule, err := source.ParsedRules()
	if err != nil {
		t.Fatal(err)
	}
	if rule.BookListRule != ".item" || rule.BookNameRule != ".name" || rule.BookURLRule != "a|attr:href" ||
		rule.ExploreURL != "https://upload-reader.example/explore/{page}" ||
		rule.ExploreBookListRule != ".explore-item" ||
		rule.ExploreBookNameRule != ".explore-name" ||
		rule.ExploreBookURLRule != "a|attr:data-url" ||
		rule.BookInfoNameRule != ".detail-name" ||
		rule.BookInfoAuthorRule != ".detail-author" ||
		rule.BookInfoCoverRule != "img|attr:data-src" ||
		rule.BookInfoIntroRule != ".detail-intro" ||
		rule.BookInfoCanRenameRule != ".allow-rename" ||
		rule.TOCURLRule != ".catalog|attr:href" ||
		rule.ChapterListRule != ".chapter" ||
		rule.ChapterNameRule != ".chapter-name" ||
		rule.ChapterURLRule != "a|attr:href" ||
		rule.NextTOCURLRule != ".toc-next|attr:href" ||
		rule.ContentRule != ".content" ||
		rule.NextContentURLRule != ".content-next|attr:href" ||
		rule.Headers["X-Source-Token"] != "upload-secret" ||
		rule.Headers["Referer"] != "https://upload-reader.example/" {
		t.Fatalf("unexpected imported rules: %+v", rule)
	}
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("X-Source-Token") != "upload-secret" ||
				request.Header.Get("Referer") != "https://upload-reader.example/" {
				t.Fatalf("imported source headers were not applied: %v", request.Header)
			}
			body := ""
			switch request.URL.Path {
			case "/search":
				if request.URL.Query().Get("q") != "请求头" {
					t.Fatalf("upstream {{key}} placeholder was not normalized: %s", request.URL.String())
				}
				body = `<article class="item"><span class="name">请求头测试书</span><a href="/book/1">详情</a></article>`
			case "/explore/2":
				body = `<article class="explore-item"><span class="explore-name">独立探索规则书籍</span><a data-url="/book/2">详情</a></article>`
			case "/book/2":
				body = `
					<span class="allow-rename">1</span>
					<h1 class="detail-name">详情页完整书名</h1>
					<span class="detail-author">详情页作者</span>
					<img data-src="/detail-cover.jpg">
					<div class="detail-intro">详情页简介</div>
					<a class="catalog" href="/catalog/2">目录</a>
				`
			case "/catalog/2":
				body = `
					<article class="chapter"><span class="chapter-name">详情页解析目录</span><a href="/chapter/2">阅读</a></article>
					<a class="toc-next" href="/catalog/3">下一页</a>
				`
			case "/catalog/3":
				body = `<article class="chapter"><span class="chapter-name">分页目录第二章</span><a href="/chapter/3">阅读</a></article>`
			case "/chapter/2":
				if request.URL.Query().Get("page") == "2" {
					body = `<main class="content">正文第二页</main>`
				} else {
					body = `<main class="content">正文第一页</main><a class="content-next" href="/chapter/2?page=2">下一页</a>`
				}
			default:
				t.Fatalf("unexpected imported source request: %s", request.URL.String())
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

	testReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/sources/%d/test", source.ID), strings.NewReader(`{"keyword":"请求头"}`))
	testReq.Header.Set("Content-Type", "application/json")
	testReq.Header.Set("Authorization", token)
	testW := httptest.NewRecorder()
	router.ServeHTTP(testW, testReq)
	if testW.Code != http.StatusOK || !strings.Contains(testW.Body.String(), `"请求头测试书"`) {
		t.Fatalf("test imported source with headers: expected parsed result, got %d: %s", testW.Code, testW.Body.String())
	}

	exploreReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/explore/%d?page=2", source.ID), nil)
	exploreReq.Header.Set("Authorization", token)
	exploreW := httptest.NewRecorder()
	router.ServeHTTP(exploreW, exploreReq)
	if exploreW.Code != http.StatusOK ||
		!strings.Contains(exploreW.Body.String(), `"独立探索规则书籍"`) ||
		!strings.Contains(exploreW.Body.String(), `"https://upload-reader.example/book/2"`) {
		t.Fatalf("explore imported source with independent rules: expected parsed result, got %d: %s", exploreW.Code, exploreW.Body.String())
	}

	tocReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/sources/%d/test-chapter", source.ID), strings.NewReader(`{"bookUrl":"https://upload-reader.example/book/2"}`))
	tocReq.Header.Set("Content-Type", "application/json")
	tocReq.Header.Set("Authorization", token)
	tocW := httptest.NewRecorder()
	router.ServeHTTP(tocW, tocReq)
	if tocW.Code != http.StatusOK ||
		!strings.Contains(tocW.Body.String(), `"详情页解析目录"`) ||
		!strings.Contains(tocW.Body.String(), `"分页目录第二章"`) ||
		!strings.Contains(tocW.Body.String(), `"https://upload-reader.example/chapter/2"`) {
		t.Fatalf("resolve imported ruleBookInfo.tocUrl: expected parsed catalog, got %d: %s", tocW.Code, tocW.Body.String())
	}

	contentReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/sources/%d/test-content", source.ID), strings.NewReader(`{"chapterUrl":"https://upload-reader.example/chapter/2"}`))
	contentReq.Header.Set("Content-Type", "application/json")
	contentReq.Header.Set("Authorization", token)
	contentW := httptest.NewRecorder()
	router.ServeHTTP(contentW, contentReq)
	if contentW.Code != http.StatusOK ||
		!strings.Contains(contentW.Body.String(), `正文第一页\n正文第二页`) {
		t.Fatalf("load imported paginated content: expected joined content, got %d: %s", contentW.Code, contentW.Body.String())
	}

	addReq := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(fmt.Sprintf(
		`{"title":"搜索结果书名","author":"搜索作者","bookUrl":"https://upload-reader.example/book/2","sourceId":%d}`,
		source.ID,
	)))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.Header.Set("Authorization", token)
	addW := httptest.NewRecorder()
	router.ServeHTTP(addW, addReq)
	if addW.Code != http.StatusCreated {
		t.Fatalf("create imported upstream remote book: expected 201, got %d: %s", addW.Code, addW.Body.String())
	}
	var added models.Book
	if err := json.Unmarshal(addW.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	if added.Title != "详情页完整书名" ||
		added.Author != "详情页作者" ||
		added.CoverURL != "https://upload-reader.example/detail-cover.jpg" ||
		added.Intro != "详情页简介" ||
		added.ChapterCount != 2 {
		t.Fatalf("imported ruleBookInfo should enrich remote book: %+v", added)
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
			SourceID uint   `json:"sourceId"`
			Name     string `json:"name"`
			Group    string `json:"group"`
			Enabled  bool   `json:"enabled"`
			OK       bool   `json:"ok"`
			Message  string `json:"message"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].OK || !strings.Contains(resp.Results[0].Message, "no search URL") {
		t.Fatalf("unexpected batch result: %+v", resp.Results)
	}
	if resp.Results[0].SourceID == 0 || resp.Results[0].Name != "无搜索地址" || !resp.Results[0].Enabled {
		t.Fatalf("batch result missing source metadata: %+v", resp.Results[0])
	}
}

func TestBatchTestSourcesRespectsTimeout(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	requestCanceled := make(chan struct{}, 1)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			select {
			case <-time.After(1200 * time.Millisecond):
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`<html><body><div class="book">慢源</div></body></html>`)),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			case <-req.Context().Done():
				requestCanceled <- struct{}{}
				return nil, req.Context().Err()
			}
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "慢源", BaseURL: "https://slow.example", Enabled: true, Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "https://slow.example/search?q={keyword}",
		BookListRule: ".book",
		BookNameRule: ".book|text",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/sources/batch-test", strings.NewReader(`{"keyword":"测试","timeout":1000,"concurrent":3}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch test: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Results []struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].OK || resp.Results[0].Message != "search timeout" {
		t.Fatalf("expected timeout result, got %+v", resp.Results)
	}
	if elapsed := time.Since(started); elapsed >= 1150*time.Millisecond {
		t.Fatalf("expected request context to stop slow source early, took %s", elapsed)
	}
	select {
	case <-requestCanceled:
	default:
		t.Fatal("expected slow source HTTP request to receive context cancellation")
	}
}

func TestBatchTestSourcesDoesNotTruncateLargeSourceList(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	sources := make([]models.BookSource, 0, 85)
	for i := 0; i < 85; i++ {
		sources = append(sources, models.BookSource{Name: fmt.Sprintf("源%02d", i), Enabled: true})
	}
	if err := server.db.Create(&sources).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sources/batch-test", strings.NewReader(`{"keyword":"测试","concurrent":5}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch test sources: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Results []struct {
			SourceID uint `json:"sourceId"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != len(sources) {
		t.Fatalf("expected all sources to be checked, got %d of %d", len(resp.Results), len(sources))
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

	body = `{"action":"group","sourceIds":[` + strconv.FormatUint(uint64(sourceA.ID), 10) + `,` + strconv.FormatUint(uint64(sourceB.ID), 10) + `],"group":"优先分组"}`
	reqGroup := httptest.NewRequest(http.MethodPost, "/api/sources/batch", strings.NewReader(body))
	reqGroup.Header.Set("Content-Type", "application/json")
	reqGroup.Header.Set("Authorization", token)
	wGroup := httptest.NewRecorder()
	router.ServeHTTP(wGroup, reqGroup)
	if wGroup.Code != http.StatusOK {
		t.Fatalf("batch group sources: expected 200, got %d: %s", wGroup.Code, wGroup.Body.String())
	}

	var grouped int64
	if err := server.db.Model(&models.BookSource{}).Where("\"group\" = ?", "优先分组").Count(&grouped).Error; err != nil {
		t.Fatal(err)
	}
	if grouped != 2 {
		t.Fatalf("expected 2 grouped sources, got %d", grouped)
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

func TestSourceUsagePreventsDeletingUsedSources(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	usedSource := models.BookSource{Name: "使用中源", BaseURL: "https://used.example", Enabled: true}
	freeSource := models.BookSource{Name: "空闲源", BaseURL: "https://free.example", Enabled: true}
	if err := server.db.Create(&usedSource).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&freeSource).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: usedSource.ID, Title: "使用该源的书", URL: "https://used.example/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	reqList := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	reqList.Header.Set("Authorization", token)
	wList := httptest.NewRecorder()
	router.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("list sources: expected 200, got %d: %s", wList.Code, wList.Body.String())
	}
	var listed []models.BookSource
	if err := json.Unmarshal(wList.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	usageByName := map[string]int{}
	for _, source := range listed {
		usageByName[source.Name] = source.UsedBookCount
	}
	if usageByName["使用中源"] != 1 || usageByName["空闲源"] != 0 {
		t.Fatalf("unexpected used book counts: %+v", listed)
	}

	reqDelete := httptest.NewRequest(http.MethodDelete, "/api/sources/"+strconv.FormatUint(uint64(usedSource.ID), 10), nil)
	reqDelete.Header.Set("Authorization", token)
	wDelete := httptest.NewRecorder()
	router.ServeHTTP(wDelete, reqDelete)
	if wDelete.Code != http.StatusConflict || !strings.Contains(wDelete.Body.String(), `"usedBookCount":1`) {
		t.Fatalf("delete used source should be blocked, got %d: %s", wDelete.Code, wDelete.Body.String())
	}

	body := fmt.Sprintf(`{"action":"delete","sourceIds":[%d,%d]}`, usedSource.ID, freeSource.ID)
	reqBatch := httptest.NewRequest(http.MethodPost, "/api/sources/batch", strings.NewReader(body))
	reqBatch.Header.Set("Content-Type", "application/json")
	reqBatch.Header.Set("Authorization", token)
	wBatch := httptest.NewRecorder()
	router.ServeHTTP(wBatch, reqBatch)
	if wBatch.Code != http.StatusOK || !strings.Contains(wBatch.Body.String(), `"affected":1`) || !strings.Contains(wBatch.Body.String(), `"skippedUsed":1`) {
		t.Fatalf("batch delete should skip used source, got %d: %s", wBatch.Code, wBatch.Body.String())
	}

	var remaining []models.BookSource
	if err := server.db.Order("name asc").Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Name != "使用中源" {
		t.Fatalf("expected only used source to remain, got %+v", remaining)
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

func TestSaveAndRestoreDefaultSources(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	if err := server.db.Create(&models.BookSource{Name: "默认源一", BaseURL: "https://one.example", Enabled: true, Group: "默认"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookSource{Name: "默认源二", BaseURL: "https://two.example", Enabled: false}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sources/default/save", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"count":2`) {
		t.Fatalf("save default sources: expected count, got %d: %s", w.Code, w.Body.String())
	}

	reqClear := httptest.NewRequest(http.MethodDelete, "/api/sources", nil)
	reqClear.Header.Set("Authorization", token)
	wClear := httptest.NewRecorder()
	router.ServeHTTP(wClear, reqClear)
	if wClear.Code != http.StatusOK {
		t.Fatalf("clear sources before restore: expected 200, got %d: %s", wClear.Code, wClear.Body.String())
	}
	if err := server.db.Create(&models.BookSource{Name: "临时源", BaseURL: "https://temp.example", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	reqStatus := httptest.NewRequest(http.MethodGet, "/api/sources/default", nil)
	reqStatus.Header.Set("Authorization", token)
	wStatus := httptest.NewRecorder()
	router.ServeHTTP(wStatus, reqStatus)
	if wStatus.Code != http.StatusOK || !strings.Contains(wStatus.Body.String(), `"configured":true`) {
		t.Fatalf("default source status: expected configured, got %d: %s", wStatus.Code, wStatus.Body.String())
	}

	reqRestore := httptest.NewRequest(http.MethodPost, "/api/sources/default/restore", nil)
	reqRestore.Header.Set("Authorization", token)
	wRestore := httptest.NewRecorder()
	router.ServeHTTP(wRestore, reqRestore)
	if wRestore.Code != http.StatusOK || !strings.Contains(wRestore.Body.String(), `"imported":2`) {
		t.Fatalf("restore default sources: expected restored sources, got %d: %s", wRestore.Code, wRestore.Body.String())
	}

	var sources []models.BookSource
	if err := server.db.Order("name asc").Find(&sources).Error; err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].Name != "默认源一" || sources[0].Group != "默认" || sources[1].Name != "默认源二" || sources[1].Enabled {
		t.Fatalf("unexpected restored sources: %+v", sources)
	}
}

func TestRestoreDefaultSourcesRequiresSnapshot(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/sources/default/restore", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("restore default sources: expected 404 without snapshot, got %d: %s", w.Code, w.Body.String())
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
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"count":1`) || !strings.Contains(w.Body.String(), "预览源") || !strings.Contains(w.Body.String(), `"sources"`) {
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

func TestExportSourcesSupportsSelectedIDs(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	sources := []models.BookSource{
		{
			Name:           "导出源一",
			BaseURL:        "https://one.example",
			SearchURL:      "https://one.example/legacy-search?q={keyword}",
			Charset:        "gbk",
			ConcurrentRate: "2/1000",
			Header:         "@js:return source.loginHeader()",
			LoginURL:       "https://one.example/login",
			LoginCheckJS:   "checkLogin()",
			CustomOrder:    37,
			LastUpdateTime: 1730000000000,
			Weight:         15,
			RespondTime:    2468,
			BookURLPattern: `/detail/\d+$`,
			SourceType:     1,
			Comment:        "导出注释",
			Enabled:        true,
			EnabledExplore: boolPointer(false),
			Group:          "导出分组",
		},
		{Name: "导出源二", BaseURL: "https://two.example", Charset: "utf-8", Enabled: true},
		{Name: "导出源三", BaseURL: "https://three.example", Charset: "utf-8", Enabled: false},
	}
	if err := sources[0].SetRules(models.BookSourceRule{
		SearchURL:                 "https://one.example/search?q={keyword}",
		ExploreURL:                "https://one.example/explore/{page}",
		BookListRule:              ".book",
		BookNameRule:              ".name",
		BookAuthorRule:            ".author",
		BookCoverRule:             "img|attr:src",
		BookIntroRule:             ".intro",
		BookKindRule:              ".kind",
		BookWordCountRule:         ".words",
		LatestChapterRule:         ".latest",
		BookUpdateTimeRule:        ".updated",
		BookURLRule:               "a|attr:href",
		ExploreBookListRule:       ".explore-card",
		ExploreBookNameRule:       ".explore-title",
		ExploreBookAuthorRule:     ".explore-author",
		ExploreBookCoverRule:      "img|attr:data-src",
		ExploreBookIntroRule:      ".explore-intro",
		ExploreBookKindRule:       ".explore-kind",
		ExploreBookWordCountRule:  ".explore-words",
		ExploreLatestChapterRule:  ".explore-latest",
		ExploreBookUpdateTimeRule: ".explore-updated",
		ExploreBookURLRule:        "a|attr:data-url",
		ExplorePaginationRule:     ".explore-next|attr:href",
		BookInfoInitRule:          "@js:book.init = true",
		BookInfoNameRule:          ".detail-name",
		BookInfoAuthorRule:        ".detail-author",
		BookInfoCoverRule:         "img.detail-cover|attr:data-src",
		BookInfoIntroRule:         ".detail-intro",
		BookInfoKindRule:          ".detail-kind",
		BookInfoLatestChapterRule: ".detail-latest",
		BookInfoUpdateTimeRule:    ".detail-update",
		BookInfoWordCountRule:     ".detail-words",
		BookInfoCanRenameRule:     ".can-rename",
		TOCURLRule:                ".catalog|attr:href",
		ChapterPreUpdateJSRule:    "@js:book.preUpdate = true",
		ChapterListRule:           "-.chapter",
		ChapterNameRule:           ".title",
		ChapterURLRule:            "a|attr:href",
		ChapterIsVolumeRule:       ".volume",
		ChapterIsVIPRule:          ".vip",
		ChapterUpdateTimeRule:     ".chapter-updated",
		NextTOCURLRule:            ".toc-next|attr:href",
		ContentRule:               "#content",
		NextContentURLRule:        ".content-next|attr:href",
		ContentWebJSRule:          "@js:result",
		ContentSourceRegex:        "source-(.*)",
		ContentReplaceRegex:       "replace##with",
		ContentImageStyle:         "FULL",
		PaginationRule:            ".next|attr:href",
		Headers: map[string]string{
			"Referer": "https://one.example/",
		},
		TextReplaceRules: []models.TextReplaceRule{
			{Pattern: "广告", Replacement: ""},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&sources).Error; err != nil {
		t.Fatal(err)
	}

	query := fmt.Sprintf("/api/sources/export?sourceIds=%d,%d", sources[2].ID, sources[0].ID)
	req := httptest.NewRequest(http.MethodGet, query, nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("export selected sources: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	for _, internalField := range []string{`"id":`, `"createdAt":`, `"updatedAt":`, `"usedBookCount":`} {
		if strings.Contains(w.Body.String(), internalField) {
			t.Fatalf("export should not expose internal database field %s: %s", internalField, w.Body.String())
		}
	}

	var exported []exportedBookSource
	if err := json.Unmarshal(w.Body.Bytes(), &exported); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	if len(exported) != 2 {
		t.Fatalf("expected 2 selected sources, got %+v", exported)
	}
	if exported[0].BookSourceName != "导出源三" || exported[1].BookSourceName != "导出源一" {
		t.Fatalf("expected selected sources ordered by customOrder then id, got %+v", exported)
	}
	for _, source := range exported {
		if source.BookSourceName == "导出源二" {
			t.Fatalf("unselected source should not be exported: %+v", exported)
		}
	}
	var first exportedBookSource
	for _, source := range exported {
		if source.BookSourceName == "导出源一" {
			first = source
			break
		}
	}
	if first.BookSourceURL != "https://one.example" ||
		first.BookSourceGroup != "导出分组" ||
		first.SearchURL != "https://one.example/search?q={{key}}" ||
		first.ExploreURL != "https://one.example/explore/{{page}}" ||
		first.Charset != "gbk" ||
		first.ConcurrentRate != "2/1000" ||
		first.Header != "@js:return source.loginHeader()" ||
		first.LoginURL != "https://one.example/login" ||
		first.LoginCheckJS != "checkLogin()" ||
		first.CustomOrder != 37 ||
		first.LastUpdateTime != 1730000000000 ||
		first.Weight != 15 ||
		first.RespondTime != 2468 ||
		first.BookURLPattern != `/detail/\d+$` ||
		first.BookSourceType != 1 ||
		first.BookSourceComment != "导出注释" ||
		first.EnabledExplore ||
		first.RuleSearch.BookList != ".book" ||
		first.RuleSearch.Name != ".name" ||
		first.RuleSearch.BookURL != "a@href" ||
		first.RuleSearch.CoverURL != "img@src" ||
		first.RuleSearch.Kind != ".kind" ||
		first.RuleSearch.WordCount != ".words" ||
		first.RuleSearch.UpdateTime != ".updated" ||
		first.RuleExplore.BookList != ".explore-card" ||
		first.RuleExplore.Name != ".explore-title" ||
		first.RuleExplore.CoverURL != "img@data-src" ||
		first.RuleExplore.BookURL != "a@data-url" ||
		first.RuleExplore.Kind != ".explore-kind" ||
		first.RuleExplore.WordCount != ".explore-words" ||
		first.RuleExplore.UpdateTime != ".explore-updated" ||
		first.RuleBookInfo.Init != "@js:book.init = true" ||
		first.RuleBookInfo.Name != ".detail-name" ||
		first.RuleBookInfo.Author != ".detail-author" ||
		first.RuleBookInfo.CoverURL != "img.detail-cover@data-src" ||
		first.RuleBookInfo.Intro != ".detail-intro" ||
		first.RuleBookInfo.Kind != ".detail-kind" ||
		first.RuleBookInfo.LastChapter != ".detail-latest" ||
		first.RuleBookInfo.UpdateTime != ".detail-update" ||
		first.RuleBookInfo.WordCount != ".detail-words" ||
		first.RuleBookInfo.TOCURL != ".catalog@href" ||
		first.RuleBookInfo.CanRename != ".can-rename" ||
		first.RuleTOC.PreUpdateJS != "@js:book.preUpdate = true" ||
		first.RuleTOC.ChapterList != "-.chapter" ||
		first.RuleTOC.ChapterURL != "a@href" ||
		first.RuleTOC.IsVolume != ".volume" ||
		first.RuleTOC.IsVIP != ".vip" ||
		first.RuleTOC.UpdateTime != ".chapter-updated" ||
		first.RuleTOC.NextTOCURL != ".toc-next@href" ||
		first.RuleContent.Content != "#content" ||
		first.RuleContent.NextContentURL != ".content-next@href" ||
		first.RuleContent.WebJS != "@js:result" ||
		first.RuleContent.SourceRegex != "source-(.*)" ||
		first.RuleContent.ReplaceRegex != "replace##with" ||
		first.RuleContent.ImageStyle != "FULL" ||
		!strings.Contains(first.Rules, `"paginationRule":".next|attr:href"`) ||
		!strings.Contains(first.Rules, `"textReplaceRules"`) {
		t.Fatalf("expected upstream-compatible fields plus lossless extensions, got %+v", first)
	}

	roundTripped, err := decodeBookSources(w.Body.Bytes())
	if err != nil {
		t.Fatalf("re-import exported sources: %v", err)
	}
	if len(roundTripped) != 2 {
		t.Fatalf("expected two round-tripped sources, got %+v", roundTripped)
	}
	var reimported models.BookSource
	for _, source := range roundTripped {
		if source.Name == sources[0].Name {
			reimported = source
			break
		}
	}
	reimportedRule, err := reimported.ParsedRules()
	if err != nil {
		t.Fatal(err)
	}
	if reimported.Name != sources[0].Name ||
		reimported.BaseURL != sources[0].BaseURL ||
		reimported.Group != sources[0].Group ||
		reimported.Charset != sources[0].Charset ||
		reimported.Header != sources[0].Header ||
		reimported.LoginURL != sources[0].LoginURL ||
		reimported.LoginCheckJS != sources[0].LoginCheckJS ||
		reimported.CustomOrder != sources[0].CustomOrder ||
		reimported.LastUpdateTime != sources[0].LastUpdateTime ||
		reimported.Weight != sources[0].Weight ||
		reimported.RespondTime != sources[0].RespondTime ||
		reimported.BookURLPattern != sources[0].BookURLPattern ||
		reimported.SourceType != sources[0].SourceType ||
		reimported.Comment != sources[0].Comment ||
		reimportedRule.PaginationRule != ".next|attr:href" ||
		reimportedRule.ExploreBookListRule != ".explore-card" ||
		reimportedRule.ExploreBookURLRule != "a|attr:data-url" ||
		reimportedRule.BookKindRule != ".kind" ||
		reimportedRule.BookWordCountRule != ".words" ||
		reimportedRule.BookUpdateTimeRule != ".updated" ||
		reimportedRule.ExploreBookKindRule != ".explore-kind" ||
		reimportedRule.ExploreBookWordCountRule != ".explore-words" ||
		reimportedRule.ExploreBookUpdateTimeRule != ".explore-updated" ||
		reimportedRule.ExplorePaginationRule != ".explore-next|attr:href" ||
		reimportedRule.BookInfoInitRule != "@js:book.init = true" ||
		reimportedRule.BookInfoNameRule != ".detail-name" ||
		reimportedRule.BookInfoCoverRule != "img.detail-cover|attr:data-src" ||
		reimportedRule.BookInfoLatestChapterRule != ".detail-latest" ||
		reimportedRule.BookInfoCanRenameRule != ".can-rename" ||
		reimportedRule.ChapterPreUpdateJSRule != "@js:book.preUpdate = true" ||
		reimportedRule.ChapterListRule != "-.chapter" ||
		reimportedRule.ChapterIsVolumeRule != ".volume" ||
		reimportedRule.ChapterIsVIPRule != ".vip" ||
		reimportedRule.ChapterUpdateTimeRule != ".chapter-updated" ||
		reimportedRule.NextTOCURLRule != ".toc-next|attr:href" ||
		reimportedRule.NextContentURLRule != ".content-next|attr:href" ||
		reimportedRule.ContentWebJSRule != "@js:result" ||
		reimportedRule.ContentSourceRegex != "source-(.*)" ||
		reimportedRule.ContentReplaceRegex != "replace##with" ||
		reimportedRule.ContentImageStyle != "FULL" ||
		len(reimportedRule.TextReplaceRules) != 1 ||
		reimportedRule.Headers["Referer"] != "https://one.example/" {
		t.Fatalf("export should round-trip without losing OpenReader rules: source=%+v rule=%+v", reimported, reimportedRule)
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
				Body:       io.NopCloser(strings.NewReader(`[{"name":"同名源","baseUrl":"https://new.example","charset":"gbk","header":"@js:return dynamicHeaders()","loginUrl":"https://new.example/login","loginCheckJs":"check()","lastUpdateTime":1740000000000,"weight":9,"respondTime":1357,"enabled":false}]`)),
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
	if updated.BaseURL != "https://new.example" || updated.Charset != "gbk" ||
		updated.Header != "@js:return dynamicHeaders()" ||
		updated.LoginURL != "https://new.example/login" || updated.LoginCheckJS != "check()" ||
		updated.LastUpdateTime != 1740000000000 || updated.Weight != 9 || updated.RespondTime != 1357 ||
		updated.Enabled {
		t.Fatalf("source was not updated correctly: %+v", updated)
	}
}

func TestSourceCandidatesAndChangeSourceUseCandidateURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	upstream := "https://source.test"
	var searchMu sync.Mutex
	searchQueries := make([]string, 0)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.Path {
			case "/search":
				searchMu.Lock()
				searchQueries = append(searchQueries, req.URL.Query().Get("q"))
				searchMu.Unlock()
				body = `<html><body>
					<div class="book">
						<a class="link" href="/book-new"><span class="title">候选书</span></a>
						<span class="author">新作者</span>
						<span class="latest">第一百章 新来源</span>
						<span class="kind">玄幻</span>
						<span class="word-count">12345</span>
						<p class="intro">新书源简介</p>
					</div>
				</body></html>`
			case "/book-new":
				body = `<html><body>
					<span class="allow-rename">1</span>
					<h1 class="detail-name">换源详情书名</h1>
					<span class="detail-author">换源详情作者</span>
					<img class="detail-cover" src="/switch-cover.jpg">
					<p class="detail-intro">换源详情简介</p>
					<span class="detail-kind">仙侠</span>
					<span class="detail-word-count">45678</span>
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
		Name:       "候选源",
		Group:      "优先",
		BaseURL:    upstream,
		SourceType: 1,
		Charset:    "utf-8",
		Enabled:    true,
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:             upstream + "/search?q={keyword}",
		BookListRule:          ".book",
		BookNameRule:          ".title|text",
		BookAuthorRule:        ".author|text",
		BookIntroRule:         ".intro|text",
		BookKindRule:          ".kind|text",
		BookWordCountRule:     ".word-count|text",
		LatestChapterRule:     ".latest|text",
		BookURLRule:           ".link|attr:href",
		BookInfoCanRenameRule: ".allow-rename",
		BookInfoNameRule:      ".detail-name",
		BookInfoAuthorRule:    ".detail-author",
		BookInfoCoverRule:     ".detail-cover|attr:src",
		BookInfoIntroRule:     ".detail-intro",
		BookInfoKindRule:      ".detail-kind",
		BookInfoWordCountRule: ".detail-word-count",
		ChapterListRule:       ".chapter",
		ChapterNameRule:       "a|text",
		ChapterURLRule:        "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	otherSource := models.BookSource{
		Name:    "其他源",
		Group:   "其他",
		BaseURL: upstream,
		Charset: "utf-8",
		Enabled: true,
	}
	if err := otherSource.SetRules(models.BookSourceRule{
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
	if err := server.db.Create(&otherSource).Error; err != nil {
		t.Fatal(err)
	}

	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "候选书", URL: upstream + "/old"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/source-candidates?group=%E4%BC%98%E5%85%88&limit=1&offset=0", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("source candidates: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var candidates []struct {
		SourceID           uint   `json:"sourceId"`
		Title              string `json:"title"`
		BookURL            string `json:"bookUrl"`
		LatestChapterTitle string `json:"latestChapterTitle"`
		Kind               string `json:"kind"`
		WordCount          string `json:"wordCount"`
		Current            bool   `json:"current"`
		Type               int    `json:"type"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	var target struct {
		SourceID           uint   `json:"sourceId"`
		Title              string `json:"title"`
		BookURL            string `json:"bookUrl"`
		LatestChapterTitle string `json:"latestChapterTitle"`
		Kind               string `json:"kind"`
		WordCount          string `json:"wordCount"`
		Current            bool   `json:"current"`
		Type               int    `json:"type"`
	}
	for _, candidate := range candidates {
		if !candidate.Current {
			target = candidate
			break
		}
	}
	if target.BookURL != upstream+"/book-new" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
	if target.SourceID != source.ID {
		t.Fatalf("source candidates should honor group filter, got source %d", target.SourceID)
	}
	if target.LatestChapterTitle != "第一百章 新来源" {
		t.Fatalf("source candidates should expose latest chapter, got %+v", target)
	}
	if target.Type != 1 {
		t.Fatalf("source candidates should expose source type, got %+v", target)
	}
	if target.Kind != "玄幻" || target.WordCount != "1.2万字" {
		t.Fatalf("source candidates should expose list metadata, got %+v", target)
	}

	pagedReq := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/source-candidates?group=%E4%BC%98%E5%85%88&limit=1&offset=0&paged=1", nil)
	pagedReq.Header.Set("Authorization", token)
	pagedW := httptest.NewRecorder()
	router.ServeHTTP(pagedW, pagedReq)
	if pagedW.Code != http.StatusOK {
		t.Fatalf("paged source candidates: expected 200, got %d: %s", pagedW.Code, pagedW.Body.String())
	}
	var pagedCandidates struct {
		List []struct {
			SourceID           uint   `json:"sourceId"`
			BookURL            string `json:"bookUrl"`
			LatestChapterTitle string `json:"latestChapterTitle"`
			Current            bool   `json:"current"`
		} `json:"list"`
		NextOffset int  `json:"nextOffset"`
		HasMore    bool `json:"hasMore"`
		Total      int  `json:"total"`
		Searched   int  `json:"searched"`
		Matched    int  `json:"matched"`
	}
	if err := json.Unmarshal(pagedW.Body.Bytes(), &pagedCandidates); err != nil {
		t.Fatal(err)
	}
	if pagedCandidates.Total != 1 || pagedCandidates.NextOffset != 1 || pagedCandidates.HasMore {
		t.Fatalf("unexpected paged metadata: %+v", pagedCandidates)
	}
	if pagedCandidates.Searched != 1 || pagedCandidates.Matched != 1 {
		t.Fatalf("unexpected paged search stats: %+v", pagedCandidates)
	}
	if len(pagedCandidates.List) == 0 {
		t.Fatalf("expected paged candidates, got %+v", pagedCandidates)
	}

	queryReq := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/source-candidates?group=%E4%BC%98%E5%85%88&q=%E5%88%AB%E5%90%8D&limit=1", nil)
	queryReq.Header.Set("Authorization", token)
	queryW := httptest.NewRecorder()
	router.ServeHTTP(queryW, queryReq)
	if queryW.Code != http.StatusOK {
		t.Fatalf("queried source candidates: expected 200, got %d: %s", queryW.Code, queryW.Body.String())
	}
	searchMu.Lock()
	foundQuery := false
	for _, query := range searchQueries {
		if query == "别名" {
			foundQuery = true
			break
		}
	}
	searchMu.Unlock()
	if !foundQuery {
		t.Fatalf("expected source candidate search to use custom query, got %#v", searchQueries)
	}

	body := `{"sourceId":` + strconv.FormatUint(uint64(target.SourceID), 10) + `,"bookUrl":` + strconv.Quote(target.BookURL) + `,"title":"候选书","author":"新作者"}`
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
	if updated.URL != upstream+"/book-new" ||
		updated.Title != "换源详情书名" ||
		updated.Author != "换源详情作者" ||
		updated.CoverURL != upstream+"/switch-cover.jpg" ||
		updated.Intro != "换源详情简介" ||
		updated.Kind != "仙侠" ||
		updated.WordCount != "4.6万字" ||
		updated.Type != 1 ||
		updated.ChapterCount != 2 ||
		updated.LastChapter != "第二章" {
		t.Fatalf("book was not switched to candidate URL: %+v", updated)
	}
}

func TestCreateRemoteBookAcceptsMultipleCategories(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	upstream := "https://remote-book.test"
	detailTitle := "详情远程书"
	detailIntro := "详情简介"
	detailKind := "科幻"
	detailWordCount := "23456"
	chapterTitle := "第一卷"
	chapterVolume := "yes"
	chapterVIP := "0"
	chapterUpdated := "昨日"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`<html><body>
					<section class="recommend">
						<h1 class="detail-name">推荐书</h1>
						<span class="detail-author">推荐作者</span>
						<img class="detail-cover" data-src="/wrong-cover.jpg">
						<p class="detail-intro">推荐简介</p>
						<span class="detail-kind">推荐分类</span>
						<span class="detail-word-count">123</span>
					</section>
					<section class="detail-main">
						<span class="allow-rename">1</span>
						<h1 class="detail-name">%s</h1>
						<span class="detail-author">详情作者</span>
						<img class="detail-cover" data-src="/cover-detail.jpg">
						<p class="detail-intro">%s</p>
						<span class="detail-kind">%s</span>
						<span class="detail-word-count">%s</span>
						<li class="chapter">
							<span class="chapter-title">%s</span>
							<span class="chapter-volume">%s</span>
							<span class="chapter-vip">%s</span>
							<span class="chapter-updated">%s</span>
						</li>
					</section>
				</body></html>`, detailTitle, detailIntro, detailKind, detailWordCount, chapterTitle, chapterVolume, chapterVIP, chapterUpdated))),
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
	categoryA := models.Category{UserID: user.ID, Name: "远程分组 A"}
	categoryB := models.Category{UserID: user.ID, Name: "远程分组 B"}
	if err := server.db.Create(&categoryA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&categoryB).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "远程源", BaseURL: upstream, SourceType: 1, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoInitRule:      ".detail-main",
		BookInfoCanRenameRule: ".allow-rename",
		BookInfoNameRule:      ".detail-name",
		BookInfoAuthorRule:    ".detail-author",
		BookInfoCoverRule:     ".detail-cover|attr:data-src",
		BookInfoIntroRule:     ".detail-intro",
		BookInfoKindRule:      ".detail-kind",
		BookInfoWordCountRule: ".detail-word-count",
		ChapterListRule:       ".chapter",
		ChapterNameRule:       ".chapter-title",
		ChapterURLRule:        "a|attr:href",
		ChapterIsVolumeRule:   ".chapter-volume",
		ChapterIsVIPRule:      ".chapter-vip",
		ChapterUpdateTimeRule: ".chapter-updated",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"title":"远程书","bookUrl":"` + upstream + `/book","sourceId":` + strconv.FormatUint(uint64(source.ID), 10) + `,"categoryIds":[` + strconv.FormatUint(uint64(categoryA.ID), 10) + `,` + strconv.FormatUint(uint64(categoryB.ID), 10) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create remote book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var book struct {
		models.Book
		CategoryIDs []uint `json:"categoryIds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &book); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(book.CategoryIDs, []uint{categoryA.ID, categoryB.ID}) {
		t.Fatalf("expected both categories on remote book, got %+v", book.CategoryIDs)
	}
	if book.CategoryID == nil || *book.CategoryID != categoryA.ID {
		t.Fatalf("expected first category as compatibility category, got %+v", book.Book)
	}
	if !book.CanUpdate {
		t.Fatalf("expected remote book to enable update checks by default, got %+v", book)
	}
	if book.Type != 1 {
		t.Fatalf("expected remote book to preserve source type: %+v", book.Book)
	}
	if book.Title != "详情远程书" ||
		book.Author != "详情作者" ||
		book.CoverURL != upstream+"/cover-detail.jpg" ||
		book.Intro != "详情简介" ||
		book.Kind != "科幻" ||
		book.WordCount != "2.3万字" {
		t.Fatalf("expected detail page metadata to override search payload: %+v", book.Book)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || !chapters[0].IsVolume || chapters[0].Title != "第一卷" || chapters[0].URL != "第一卷0" || chapters[0].Tag != "昨日" {
		t.Fatalf("expected remote chapter flags to be persisted: %+v", chapters)
	}

	detailTitle = "刷新后的详情书名"
	detailIntro = "刷新后的详情简介"
	detailKind = "悬疑"
	detailWordCount = "连载中"
	chapterTitle = "收费章"
	chapterVolume = "false"
	chapterVIP = "yes"
	chapterUpdated = "今日"
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh", nil)
	refreshReq.Header.Set("Authorization", token)
	refreshW := httptest.NewRecorder()
	router.ServeHTTP(refreshW, refreshReq)
	if refreshW.Code != http.StatusOK {
		t.Fatalf("refresh remote book: expected 200, got %d: %s", refreshW.Code, refreshW.Body.String())
	}
	var refreshed models.Book
	if err := server.db.First(&refreshed, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.Title != detailTitle ||
		refreshed.Intro != detailIntro ||
		refreshed.Kind != detailKind ||
		refreshed.WordCount != detailWordCount {
		t.Fatalf("refresh should update detail page metadata: %+v", refreshed)
	}
	chapters = nil
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 ||
		chapters[0].IsVolume ||
		chapters[0].Title != "🔒收费章" ||
		chapters[0].URL != upstream+"/book" ||
		chapters[0].Tag != "今日" {
		t.Fatalf("refresh should update remote chapter flags: %+v", chapters)
	}
}

func TestRemoteBookInfoPreservesNameWithoutCanRename(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	upstream := "https://no-rename.test"
	detailTitle := "详情标题"
	detailAuthor := "详情作者"
	detailIntro := "详情简介"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			responseBody := fmt.Sprintf(`<html><body>
				<h1 class="detail-name">%s</h1>
				<span class="detail-author">%s</span>
				<p class="detail-intro">%s</p>
				<div class="chapter"><span class="chapter-title">第一章</span><a href="/c1">阅读</a></div>
			</body></html>`, detailTitle, detailAuthor, detailIntro)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "不可改名源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoNameRule:   ".detail-name",
		BookInfoAuthorRule: ".detail-author",
		BookInfoIntroRule:  ".detail-intro",
		ChapterListRule:    ".chapter",
		ChapterNameRule:    ".chapter-title",
		ChapterURLRule:     "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"title":"搜索标题","author":"搜索作者","bookUrl":"` + upstream + `/book","sourceId":` + strconv.FormatUint(uint64(source.ID), 10) + `}`
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
	if book.Title != "搜索标题" || book.Author != "搜索作者" || book.Intro != "详情简介" {
		t.Fatalf("book info without canReName should preserve name/author only: %+v", book)
	}

	detailTitle = "刷新详情标题"
	detailAuthor = "刷新详情作者"
	detailIntro = "刷新详情简介"
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh", nil)
	refreshReq.Header.Set("Authorization", token)
	refreshW := httptest.NewRecorder()
	router.ServeHTTP(refreshW, refreshReq)
	if refreshW.Code != http.StatusOK {
		t.Fatalf("refresh remote book: expected 200, got %d: %s", refreshW.Code, refreshW.Body.String())
	}
	var refreshed models.Book
	if err := server.db.First(&refreshed, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.Title != "搜索标题" ||
		refreshed.Author != "搜索作者" ||
		refreshed.Intro != "刷新详情简介" {
		t.Fatalf("refresh without canReName should preserve name/author only: %+v", refreshed)
	}
}

func TestRemoteBookKeepsAndExecutesSourceRequestOptions(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	requests := make([]string, 0, 3)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Method != http.MethodPost || request.Header.Get("X-Source") != "static" {
				t.Fatalf("unexpected source request: %s %s headers=%v", request.Method, request.URL, request.Header)
			}
			requestKey := request.URL.Path + "?" + string(body)
			requests = append(requests, requestKey)

			responseBody := ""
			switch requestKey {
			case "/book?id=11":
				responseBody = `
					<span class="allow-rename">1</span>
					<h1 class="detail-title">请求选项书籍</h1>
					<a class="catalog" href='/catalog, {"method":"POST","body":"book=11"}'>目录</a>
				`
			case "/catalog?book=11":
				responseBody = `
					<div class="chapter">
						<span class="chapter-title">第一章</span>
						<a href='/content, {"method":"POST","body":"chapter=1","headers":{"X-Chapter":"one"}}'>阅读</a>
					</div>
				`
			case "/content?chapter=1":
				if request.Header.Get("X-Chapter") != "one" {
					t.Fatalf("chapter request header option missing: %v", request.Header)
				}
				responseBody = `<main class="content">API 闭环正文 广告</main>`
			default:
				t.Fatalf("unexpected source request: %s", requestKey)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{
		Name:    "请求选项 API 源",
		BaseURL: "https://source-options.test",
		Charset: "utf-8",
		Enabled: true,
	}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoNameRule:      ".detail-title",
		BookInfoCanRenameRule: ".allow-rename",
		TOCURLRule:            ".catalog|attr:href",
		ChapterListRule:       ".chapter",
		ChapterNameRule:       ".chapter-title",
		ChapterURLRule:        "a|attr:href",
		ContentRule:           ".content",
		ContentReplaceRegex:   "##\\s*广告##",
		Headers:               map[string]string{"X-Source": "static"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	bookURL := `/book, {"method":"POST","body":"id=11"}`
	body := `{"title":"搜索结果标题","bookUrl":` + strconv.Quote(bookURL) +
		`,"sourceId":` + strconv.FormatUint(uint64(source.ID), 10) + `}`
	addReq := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(body))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.Header.Set("Authorization", token)
	addW := httptest.NewRecorder()
	router.ServeHTTP(addW, addReq)
	if addW.Code != http.StatusCreated {
		t.Fatalf("create remote book with request options: expected 201, got %d: %s", addW.Code, addW.Body.String())
	}

	var added models.Book
	if err := json.Unmarshal(addW.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	if added.Title != "请求选项书籍" || added.URL != bookURL || added.ChapterCount != 1 {
		t.Fatalf("remote book request options were not preserved: %+v", added)
	}
	var chapter models.Chapter
	if err := server.db.Where("book_id = ?", added.ID).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	expectedChapterURL := `https://source-options.test/content, {"method":"POST","body":"chapter=1","headers":{"X-Chapter":"one"}}`
	if chapter.URL != expectedChapterURL {
		t.Fatalf("chapter request options were not persisted: %+v", chapter)
	}

	contentReq := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/api/books/%d/chapters/0/content", added.ID),
		nil,
	)
	contentReq.Header.Set("Authorization", token)
	contentW := httptest.NewRecorder()
	router.ServeHTTP(contentW, contentReq)
	if contentW.Code != http.StatusOK ||
		!strings.Contains(contentW.Body.String(), "API 闭环正文") ||
		strings.Contains(contentW.Body.String(), "广告") {
		t.Fatalf("load remote content with request options: expected content, got %d: %s", contentW.Code, contentW.Body.String())
	}
	if strings.Join(requests, ",") != "/book?id=11,/catalog?book=11,/content?chapter=1" {
		t.Fatalf("unexpected API source request sequence: %+v", requests)
	}
}

func TestSchedulerSkipsBooksWithCanUpdateDisabled(t *testing.T) {
	router, server := setupTestServer(t)
	authHeader(t, router)

	var calls int
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<li class="chapter"><a href="/c1">第一章</a></li>
					<li class="chapter"><a href="/c2">第二章</a></li>
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
	source := models.BookSource{Name: "追更源", BaseURL: "https://updates.example", Charset: "utf-8", Enabled: true}
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
	book := models.Book{
		UserID:       user.ID,
		SourceID:     source.ID,
		Title:        "关闭追更",
		URL:          "https://updates.example/book",
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&book).Update("can_update", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "/c1"}).Error; err != nil {
		t.Fatal(err)
	}

	if got := server.scheduler.CheckNow(); got != 0 {
		t.Fatalf("expected no new chapters for disabled book, got %d", got)
	}
	if calls != 0 {
		t.Fatalf("expected disabled book to skip remote request, got %d calls", calls)
	}
	var count int64
	if err := server.db.Model(&models.Chapter{}).Where("book_id = ?", book.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected no chapters added for disabled book, got %d", count)
	}
}

func TestCheckUpdatesScopesToCurrentUserAndReturnsShelfItems(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var calls int
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<li class="chapter"><a href="/c1">第一章</a></li>
					<li class="chapter"><a href="/c2">第二章</a></li>
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
	otherUser := models.User{Username: "other-user", PasswordHash: "hash", CanEditSources: true, CanAccessStore: true}
	if err := server.db.Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "手动追更源", BaseURL: "https://manual-update.example", Charset: "utf-8", Enabled: true}
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
	book := models.Book{
		UserID:       user.ID,
		SourceID:     source.ID,
		Title:        "当前用户书",
		URL:          "https://manual-update.example/current",
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	otherBook := models.Book{
		UserID:       otherUser.ID,
		SourceID:     source.ID,
		Title:        "其它用户书",
		URL:          "https://manual-update.example/other",
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&otherBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "/c1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: otherBook.ID, Index: 0, Title: "第一章", URL: "/c1"}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/check-updates", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("check updates: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		NewChapters int `json:"newChapters"`
		Books       []struct {
			ID           uint      `json:"id"`
			ChapterCount int       `json:"chapterCount"`
			LastChapter  string    `json:"lastChapter"`
			ShelfOrderAt time.Time `json:"shelfOrderAt"`
		} `json:"books"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.NewChapters != 1 || len(resp.Books) != 1 || resp.Books[0].ID != book.ID {
		t.Fatalf("expected one updated shelf item for current user, got %+v", resp)
	}
	if resp.Books[0].ChapterCount != 2 || resp.Books[0].LastChapter != "第二章" || resp.Books[0].ShelfOrderAt.IsZero() {
		t.Fatalf("expected updated chapter metadata in shelf item, got %+v", resp.Books[0])
	}
	if calls != 1 {
		t.Fatalf("expected only current user's book to be checked, got %d calls", calls)
	}

	var updatedOther models.Book
	if err := server.db.First(&updatedOther, otherBook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedOther.ChapterCount != 1 || updatedOther.LastChapter != "第一章" {
		t.Fatalf("expected other user's book to stay unchanged, got %+v", updatedOther)
	}
}

func TestCreateRemoteBookReusesExistingURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	categoryA := models.Category{UserID: user.ID, Name: "新分组 A"}
	categoryB := models.Category{UserID: user.ID, Name: "新分组 B"}
	if err := server.db.Create(&categoryA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&categoryB).Error; err != nil {
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

	body := `{"title":"已有书","bookUrl":"https://book.example/existing","sourceId":` + strconv.FormatUint(uint64(source.ID), 10) + `,"categoryIds":[` + strconv.FormatUint(uint64(categoryA.ID), 10) + `,` + strconv.FormatUint(uint64(categoryB.ID), 10) + `]}`
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
	var updated struct {
		models.Book
		CategoryIDs []uint `json:"categoryIds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.CategoryIDs, []uint{categoryA.ID, categoryB.ID}) {
		t.Fatalf("expected existing book categories updated, got %+v", updated.CategoryIDs)
	}
	if updated.CategoryID == nil || *updated.CategoryID != categoryA.ID {
		t.Fatalf("expected first category as compatibility category, got %+v", updated.Book)
	}
	var categoryRows int64
	if err := server.db.Model(&models.BookCategory{}).Where("user_id = ? AND book_id = ?", user.ID, book.ID).Count(&categoryRows).Error; err != nil {
		t.Fatal(err)
	}
	if categoryRows != 2 {
		t.Fatalf("expected two persisted category memberships, got %d", categoryRows)
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

func TestBatchCreateAndDeleteBookmarks(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "批量书签书"}
	otherBook := models.Book{UserID: user.ID, Title: "其它书籍"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&otherBook).Error; err != nil {
		t.Fatal(err)
	}

	body := `[
		{"chapterIndex":1,"offset":12,"percent":0.25,"title":"第一条","excerpt":"摘录一"},
		{"chapterIndex":2,"offset":24,"percent":0.5,"note":"笔记二"}
	]`
	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/bookmarks/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("batch create bookmarks: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created []models.Bookmark
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || created[0].Title != "第一条" || created[1].Title != "书签" {
		t.Fatalf("unexpected created bookmarks: %+v", created)
	}
	for _, bookmark := range created {
		if bookmark.UserID != user.ID || bookmark.BookID != book.ID || bookmark.ID == 0 {
			t.Fatalf("batch bookmark was not scoped correctly: %+v", bookmark)
		}
	}

	otherBookmark := models.Bookmark{UserID: user.ID, BookID: otherBook.ID, Title: "不能被跨书删除"}
	if err := server.db.Create(&otherBookmark).Error; err != nil {
		t.Fatal(err)
	}
	deleteBody := fmt.Sprintf(`{"ids":[%d,%d,%d,%d]}`, created[0].ID, otherBookmark.ID, created[0].ID, 999999)
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/bookmarks/batch-delete", strings.NewReader(deleteBody))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteReq.Header.Set("Authorization", token)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("batch delete bookmarks: expected 200, got %d: %s", deleteW.Code, deleteW.Body.String())
	}
	var deleted struct {
		DeletedIDs []uint `json:"deletedIds"`
	}
	if err := json.Unmarshal(deleteW.Body.Bytes(), &deleted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(deleted.DeletedIDs, []uint{created[0].ID}) {
		t.Fatalf("unexpected deleted bookmark ids: %+v", deleted.DeletedIDs)
	}

	var remaining []models.Bookmark
	if err := server.db.Order("id asc").Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 || remaining[0].ID != created[1].ID || remaining[1].ID != otherBookmark.ID {
		t.Fatalf("batch delete crossed scope or removed wrong rows: %+v", remaining)
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
	if err := os.WriteFile(fullPath, []byte("第一段内容\n这里有一个特殊关键词用于搜索\n第二个特殊关键词也应命中\n第三个特 殊 关 键 词也应命中\n换行拆开的隐 藏\n关 键 词\n夫君御驾亲征了！！！\n太元圣女隔着一段正文才出现下-2\n结尾"), 0o644); err != nil {
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
		ChapterIndex             int    `json:"chapterIndex"`
		ChapterTitle             string `json:"chapterTitle"`
		Excerpt                  string `json:"excerpt"`
		Query                    string `json:"query"`
		ResultCountWithinChapter int    `json:"resultCountWithinChapter"`
		LineIndex                int    `json:"lineIndex"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 || matches[0].ChapterIndex != 0 || !strings.Contains(matches[0].Excerpt, "特殊关键词") {
		t.Fatalf("unexpected matches: %+v", matches)
	}
	if matches[0].Query != "特殊关键词" || matches[0].ResultCountWithinChapter != 0 || matches[1].ResultCountWithinChapter != 1 || matches[2].ResultCountWithinChapter != 2 {
		t.Fatalf("unexpected match metadata: %+v", matches)
	}
	if !strings.Contains(matches[2].Excerpt, "特 殊 关 键 词") {
		t.Fatalf("expected normalized mixed match, got %+v", matches[2])
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
	if len(matches) != 1 || matches[0].LineIndex != 6 || !strings.Contains(matches[0].Excerpt, "夫君御驾亲征了") {
		t.Fatalf("unexpected punctuation-normalized matches: %+v", matches)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("太元圣女 下-2"), nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search split terms: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	matches = nil
	if err := json.Unmarshal(w.Body.Bytes(), &matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].LineIndex != 7 || !strings.Contains(matches[0].Excerpt, "太元圣女") {
		t.Fatalf("unexpected split-term matches: %+v", matches)
	}
}

func TestChapterContentRecoversMovedLocalBookCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	book := models.Book{
		UserID:      user.ID,
		SourceID:    0,
		Title:       "迁移本地书",
		LibraryPath: filepath.Join("data", "testuser", "moved-book"),
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	contentPath := filepath.Join("aa", "chapter.txt")
	currentPath := filepath.Join(server.cfg.LibraryDir, book.LibraryPath, "content", contentPath)
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentPath, []byte("迁移后的本地正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldContainerPath := filepath.Join(string(os.PathSeparator), "old-openreader", "library", book.LibraryPath, "content", contentPath)
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: oldContainerPath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chapter content: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Content != "迁移后的本地正文" {
		t.Fatalf("unexpected content %q", resp.Content)
	}

	var updated models.Chapter
	if err := server.db.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	expectedCachePath := filepath.Join("content", contentPath)
	if updated.CachePath != expectedCachePath {
		t.Fatalf("expected cache path self-healed to portable path %q, got %q", expectedCachePath, updated.CachePath)
	}
}

func TestChapterContentRebuildsMissingLocalBookCacheFromSourceFile(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	libraryPath := filepath.Join("data", "testuser", "source-book")
	originalFile := filepath.Join(libraryPath, "源书.txt")
	sourcePath := filepath.Join(server.cfg.LibraryDir, originalFile)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("第一章 起\n第一章正文。\n第二章 承\n第二章正文。"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := models.Book{
		UserID:       user.ID,
		SourceID:     0,
		Title:        "源文件本地书",
		URL:          "local://book_source",
		LibraryPath:  libraryPath,
		OriginalFile: originalFile,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 1, Title: "第二章 承", URL: "local://book_source/chapter_1", CachePath: filepath.Join("content", "missing.txt")}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/1/content", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chapter content: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Content != "第二章正文。" {
		t.Fatalf("unexpected rebuilt content %q", resp.Content)
	}

	var updated models.Chapter
	if err := server.db.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CachePath == chapter.CachePath || !strings.HasPrefix(updated.CachePath, "content") {
		t.Fatalf("expected cache path rebuilt under content, got %q", updated.CachePath)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LibraryDir, book.LibraryPath, updated.CachePath)); err != nil {
		t.Fatalf("expected rebuilt cache file, stat err=%v", err)
	}
}

func TestSearchBookContentPaged(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "分页搜索"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"第一章目标", "第二章目标", "第三章目标"} {
		cachePath := filepath.Join("paged-search", fmt.Sprintf("chapter-%d.txt", i))
		fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		chapter := models.Chapter{BookID: book.ID, Index: i, Title: fmt.Sprintf("第%d章", i+1), CachePath: cachePath}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=1", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("paged search: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var first struct {
		List      []map[string]any `json:"list"`
		LastIndex int              `json:"lastIndex"`
		HasMore   bool             `json:"hasMore"`
		Total     int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.List) != 1 || first.LastIndex != 0 || !first.HasMore || first.Total != 3 {
		t.Fatalf("unexpected first page: %+v", first)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=0&chapterLimit=2", nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("paged search second page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var second struct {
		List      []map[string]any `json:"list"`
		LastIndex int              `json:"lastIndex"`
		HasMore   bool             `json:"hasMore"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.List) != 2 || second.LastIndex != 2 || second.HasMore {
		t.Fatalf("unexpected second page: %+v", second)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?keyword="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=3&size=1", nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("paged search keyword/size aliases: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var aliasResult struct {
		List      []map[string]any `json:"list"`
		LastIndex int              `json:"lastIndex"`
		HasMore   bool             `json:"hasMore"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &aliasResult); err != nil {
		t.Fatal(err)
	}
	if len(aliasResult.List) != 1 || aliasResult.LastIndex != 0 || !aliasResult.HasMore {
		t.Fatalf("unexpected keyword/size alias result: %+v", aliasResult)
	}
}

func TestSearchLocalBookContentKeepsRequestedPageSize(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "本地长篇搜索", SourceID: 0}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		cachePath := filepath.Join("local-paged-search", fmt.Sprintf("chapter-%d.txt", i))
		fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(fmt.Sprintf("第%d章目标", i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		chapter := models.Chapter{BookID: book.ID, Index: i, Title: fmt.Sprintf("第%d章", i+1), CachePath: cachePath}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=2&localFull=1", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("local paged search: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		List      []map[string]any `json:"list"`
		LastIndex int              `json:"lastIndex"`
		HasMore   bool             `json:"hasMore"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.List) != 2 || result.LastIndex != 1 || !result.HasMore {
		t.Fatalf("local search ignored requested page size: %+v", result)
	}
}

func TestLegacySearchBookContentByURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "上游正文搜索", URL: "https://book.example/legacy-search"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"第一章目标", "第二章目标", "第三章目标"} {
		cachePath := filepath.Join("legacy-content-search", fmt.Sprintf("chapter-%d.txt", i))
		fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		chapter := models.Chapter{BookID: book.ID, Index: i, Title: fmt.Sprintf("第%d章", i+1), CachePath: cachePath}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}

	body := `{"url":"https://book.example/legacy-search","keyword":"目标","lastIndex":-1,"size":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/reader3/searchBookContent", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy search post: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var first struct {
		IsSuccess bool `json:"isSuccess"`
		Data      struct {
			List      []map[string]any `json:"list"`
			LastIndex int              `json:"lastIndex"`
			HasMore   bool             `json:"hasMore"`
			Total     int              `json:"total"`
		} `json:"data"`
		ErrorMsg string `json:"errorMsg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if !first.IsSuccess || len(first.Data.List) != 1 || first.Data.LastIndex != 0 || !first.Data.HasMore || first.Data.Total != 3 {
		t.Fatalf("unexpected legacy first result: %+v body=%s", first, w.Body.String())
	}
	if first.Data.List[0]["resultText"] == "" || !strings.Contains(fmt.Sprint(first.Data.List[0]["resultText"]), "目标") {
		t.Fatalf("legacy result missing upstream resultText field: %+v", first.Data.List[0])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/reader3/searchBookContent?bookUrl="+url.QueryEscape("https://book.example/legacy-search")+"&keyword="+url.QueryEscape("目标")+"&lastIndex=0&size=2", nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy search get: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var second struct {
		IsSuccess bool `json:"isSuccess"`
		Data      struct {
			List      []map[string]any `json:"list"`
			LastIndex int              `json:"lastIndex"`
			HasMore   bool             `json:"hasMore"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if !second.IsSuccess || len(second.Data.List) != 2 || second.Data.LastIndex != 2 || second.Data.HasMore {
		t.Fatalf("unexpected legacy second result: %+v body=%s", second, w.Body.String())
	}
}

func TestSearchBookContentScansAheadUntilMatch(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "跨页正文搜索"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"第一章无", "第二章无", "第三章目标", "第四章无"} {
		cachePath := filepath.Join("scan-ahead-search", fmt.Sprintf("chapter-%d.txt", i))
		fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		chapter := models.Chapter{BookID: book.ID, Index: i, Title: fmt.Sprintf("第%d章", i+1), CachePath: cachePath}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=1&scanUntilMatch=1&scanLimit=4", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("scan-ahead search: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		List      []map[string]any `json:"list"`
		LastIndex int              `json:"lastIndex"`
		HasMore   bool             `json:"hasMore"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.List) != 1 || result.LastIndex != 2 || !result.HasMore {
		t.Fatalf("unexpected scan-ahead result: %+v", result)
	}
}

func TestSearchBookContentPerChapterLimit(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "单章多命中"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	lines := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		lines = append(lines, fmt.Sprintf("第%d段目标词", i+1))
	}
	cachePath := filepath.Join("per-chapter-search", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标词")+"&paged=1&lastIndex=-1&chapterLimit=1&perChapterLimit=12&matchLimit=20", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("paged search: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		List []map[string]any `json:"list"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.List) != 12 {
		t.Fatalf("expected all 12 matches from one chapter, got %d: %+v", len(result.List), result.List)
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

	source := models.BookSource{Name: "缓存源", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "缓存书", SourceID: source.ID}
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
	var result struct {
		Book bookListItem `json:"book"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Book.CachedChapterCount != 1 {
		t.Fatalf("expected cached chapter count in response book, got %+v", result.Book)
	}
}

func TestCacheBookContentDefaultsToFiftyChapters(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "缓存源", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "缓存书", SourceID: source.ID}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		cachePath := filepath.Join("cache-limit", fmt.Sprintf("chapter-%d.txt", i))
		fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("已缓存正文"), 0o644); err != nil {
			t.Fatal(err)
		}
		chapter := models.Chapter{BookID: book.ID, Index: i, Title: fmt.Sprintf("第%d章", i+1), CachePath: cachePath}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache", strings.NewReader(`{"all":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cache book: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"requested":50`) || !strings.Contains(w.Body.String(), `"cached":50`) {
		t.Fatalf("expected default cache window of 50 chapters, got %s", w.Body.String())
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
	source := models.BookSource{Name: "缓存统计源", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{Title: "缓存统计", UserID: 1, SourceID: source.ID}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	localCachePath := filepath.Join("stats", "local-chapter.txt")
	localFullPath := filepath.Join(server.cfg.CacheDir, localCachePath)
	if err := os.WriteFile(localFullPath, []byte("本地正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	localBook := models.Book{Title: "本地书", UserID: 1}
	if err := server.db.Create(&localBook).Error; err != nil {
		t.Fatal(err)
	}
	localChapter := models.Chapter{BookID: localBook.ID, Index: 0, Title: "本地章", CachePath: localCachePath}
	if err := server.db.Create(&localChapter).Error; err != nil {
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
	if _, err := os.Stat(localFullPath); err != nil {
		t.Fatalf("expected local book content to remain, stat err=%v", err)
	}
	var updated models.Chapter
	if err := server.db.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CachePath != "" {
		t.Fatalf("expected chapter cache path reset, got %q", updated.CachePath)
	}
	var updatedLocal models.Chapter
	if err := server.db.First(&updatedLocal, localChapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedLocal.CachePath == "" {
		t.Fatal("expected local book cache path to remain")
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

func TestReplaceRuleScopeAndPlainTextMode(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/replace-rules", strings.NewReader(`{"name":"当前书文本规则","pattern":"广告[0-9]+","replacement":"净化","scope":"目标书;local://target","isRegex":false,"isEnabled":true}`))
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
	if rule.Scope != "目标书;local://target" || rule.IsRegex == nil || *rule.IsRegex {
		t.Fatalf("unexpected replace rule fields: %+v", rule)
	}

	cachePath := filepath.Join("replace", "plain.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("广告[0-9]+\n广告123\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "目标书", URL: "local://target"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("chapter content: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	body := w2.Body.String()
	if !strings.Contains(body, "净化") || !strings.Contains(body, "广告123") {
		t.Fatalf("expected plain text scoped replacement only, got: %s", body)
	}

	other := models.Book{UserID: user.ID, Title: "其他书", URL: "local://target"}
	if err := server.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherChapter := models.Chapter{BookID: other.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&otherChapter).Error; err != nil {
		t.Fatal(err)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(other.ID), 10)+"/chapters/0/content", nil)
	req3.Header.Set("Authorization", token)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("other chapter content: expected 200, got %d: %s", w3.Code, w3.Body.String())
	}
	if strings.Contains(w3.Body.String(), "净化") {
		t.Fatalf("scoped replace rule should not affect other book: %s", w3.Body.String())
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

	source := models.BookSource{Name: "批量缓存源", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	bookA := models.Book{UserID: user.ID, Title: "缓存 A", SourceID: source.ID}
	bookB := models.Book{UserID: user.ID, Title: "缓存 B", SourceID: source.ID}
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

func TestBatchBooksCacheLimitsToTenChaptersPerBook(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "批量缓存限制源", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "缓存限制", SourceID: source.ID}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		cachePath := filepath.Join("batch-cache-limit", fmt.Sprintf("chapter-%d.txt", i))
		fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("已缓存正文"), 0o644); err != nil {
			t.Fatal(err)
		}
		chapter := models.Chapter{BookID: book.ID, Index: i, Title: fmt.Sprintf("第%d章", i+1), CachePath: cachePath}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}

	body := `{"action":"cache","bookIds":[` + strconv.FormatUint(uint64(book.ID), 10) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch cache: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"cached":10`) || !strings.Contains(w.Body.String(), `"requested":10`) {
		t.Fatalf("expected batch cache to stop at 10 chapters, got %s", w.Body.String())
	}
}

func TestBatchBooksClearCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	source := models.BookSource{Name: "批量清缓存源", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "清缓存", SourceID: source.ID}
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

func TestExportBooksAsTXT(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("export", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("第一章正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "导出TXT书", Author: "作者"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"bookIds":[` + strconv.FormatUint(uint64(book.ID), 10) + `],"format":"txt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("export txt: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected text/plain export, got %q", contentType)
	}
	text := w.Body.String()
	if !strings.Contains(text, "导出TXT书") || !strings.Contains(text, "第一章") || !strings.Contains(text, "第一章正文") {
		t.Fatalf("unexpected txt export: %q", text)
	}
}

func TestExportMultipleBooksAsTXTZip(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	var bookIDs []string
	for i := 0; i < 2; i++ {
		cachePath := filepath.Join("export-zip", fmt.Sprintf("chapter-%d.txt", i))
		fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(fmt.Sprintf("正文%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		book := models.Book{UserID: user.ID, Title: fmt.Sprintf("导出Zip书%d", i)}
		if err := server.db.Create(&book).Error; err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}).Error; err != nil {
			t.Fatal(err)
		}
		bookIDs = append(bookIDs, strconv.FormatUint(uint64(book.ID), 10))
	}

	body := `{"bookIds":[` + strings.Join(bookIDs, ",") + `],"format":"txt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("export txt zip: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("expected 2 txt files, got %d", len(reader.File))
	}
	var contents string
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		contents += string(data)
	}
	if !strings.Contains(contents, "正文0") || !strings.Contains(contents, "正文1") {
		t.Fatalf("unexpected zip contents: %q", contents)
	}
}

func TestExportBookAsEPUB(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("export-epub", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("EPUB正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "导出EPUB书", Author: "作者"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"bookIds":[` + strconv.FormatUint(uint64(book.ID), 10) + `],"format":"epub"}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("export epub: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "application/epub+zip") {
		t.Fatalf("expected epub content type, got %q", contentType)
	}
	reader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = string(data)
	}
	if files["mimetype"] != "application/epub+zip" {
		t.Fatalf("missing epub mimetype: %q", files["mimetype"])
	}
	if !strings.Contains(files["OEBPS/content.opf"], "导出EPUB书") || !strings.Contains(files["OEBPS/nav.xhtml"], "第一章") || !strings.Contains(files["OEBPS/chapter-0001.xhtml"], "EPUB正文") {
		t.Fatalf("unexpected epub files: %+v", files)
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

	reqRecursive := httptest.NewRequest(http.MethodGet, "/api/local-store?recursive=1", nil)
	reqRecursive.Header.Set("Authorization", token)
	wRecursive := httptest.NewRecorder()
	router.ServeHTTP(wRecursive, reqRecursive)
	if wRecursive.Code != http.StatusOK {
		t.Fatalf("recursive local store: expected 200, got %d: %s", wRecursive.Code, wRecursive.Body.String())
	}
	var recursiveListing struct {
		Recursive bool `json:"recursive"`
		Items     []struct {
			Path       string `json:"path"`
			Importable bool   `json:"importable"`
		} `json:"items"`
	}
	if err := json.Unmarshal(wRecursive.Body.Bytes(), &recursiveListing); err != nil {
		t.Fatal(err)
	}
	if !recursiveListing.Recursive {
		t.Fatalf("expected recursive listing flag, got %+v", recursiveListing)
	}
	foundNestedBook := false
	for _, item := range recursiveListing.Items {
		if item.Path == filepath.Join("nested", "book.txt") && item.Importable {
			foundNestedBook = true
		}
	}
	if !foundNestedBook {
		t.Fatalf("recursive listing did not include nested book: %+v", recursiveListing.Items)
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

func TestLocalStoreDownloadFile(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "download.txt"), []byte("下载内容"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local-store/download?path=download.txt", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("download local store file: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "下载内容" {
		t.Fatalf("unexpected downloaded content: %s", w.Body.String())
	}
	if disposition := w.Header().Get("Content-Disposition"); !strings.Contains(disposition, "download.txt") {
		t.Fatalf("expected attachment filename, got %q", disposition)
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
	categoryB := models.Category{UserID: user.ID, Name: "书仓分组B"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&categoryB).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "store.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	previewReq := httptest.NewRequest(http.MethodPost, "/api/local-store/import-preview", strings.NewReader(`{"paths":["store.txt"]}`))
	previewReq.Header.Set("Content-Type", "application/json")
	previewReq.Header.Set("Authorization", token)
	previewW := httptest.NewRecorder()
	router.ServeHTTP(previewW, previewReq)
	if previewW.Code != http.StatusOK || !strings.Contains(previewW.Body.String(), `"chapterCount":1`) {
		t.Fatalf("local store preview: expected parsed book, got %d: %s", previewW.Code, previewW.Body.String())
	}
	var previewBookCount int64
	if err := server.db.Model(&models.Book{}).Where("title = ?", "store").Count(&previewBookCount).Error; err != nil {
		t.Fatal(err)
	}
	if previewBookCount != 0 {
		t.Fatalf("local store preview must not create books, got %d", previewBookCount)
	}

	body := fmt.Sprintf(`{"items":[{"path":"store.txt","title":"书仓自定义书名","author":"书仓作者"}],"categoryIds":[%d,%d]}`, category.ID, categoryB.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/local-store/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("local store import: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Imported []struct {
			Path string `json:"path"`
			Book *struct {
				ID           uint      `json:"id"`
				CategoryID   *uint     `json:"categoryId"`
				CategoryIDs  []uint    `json:"categoryIds"`
				ShelfOrderAt time.Time `json:"shelfOrderAt"`
			} `json:"book"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Imported) != 1 || payload.Imported[0].Book == nil || payload.Imported[0].Book.ShelfOrderAt.IsZero() {
		t.Fatalf("expected imported shelf item response, got %+v", payload.Imported)
	}
	if payload.Imported[0].Book.CategoryID == nil || *payload.Imported[0].Book.CategoryID != category.ID {
		t.Fatalf("expected imported shelf item category %d, got %+v", category.ID, payload.Imported[0].Book.CategoryID)
	}
	if !sameUintSet(payload.Imported[0].Book.CategoryIDs, []uint{category.ID, categoryB.ID}) {
		t.Fatalf("expected imported shelf item in both categories, got %+v", payload.Imported[0].Book.CategoryIDs)
	}

	var book models.Book
	if err := server.db.Where("title = ?", "书仓自定义书名").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.Author != "书仓作者" {
		t.Fatalf("expected imported author override, got %q", book.Author)
	}
	if book.CategoryID == nil || *book.CategoryID != category.ID {
		t.Fatalf("expected imported book category %d, got %+v", category.ID, book.CategoryID)
	}
	if ids := server.bookCategoryIDs(user.ID, book); !sameUintSet(ids, []uint{category.ID, categoryB.ID}) {
		t.Fatalf("expected imported book in both categories, got %+v", ids)
	}
}

func TestDirectImportReturnsShelfListItem(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "直接导入分组"}
	categoryB := models.Category{UserID: user.ID, Name: "直接导入分组B"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&categoryB).Error; err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "direct.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("第一章 开始\n正文")); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("categoryIds", strconv.FormatUint(uint64(category.ID), 10)); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("categoryIds", strconv.FormatUint(uint64(categoryB.ID), 10)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/imports/books", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("direct import: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var item struct {
		ID           uint      `json:"id"`
		Title        string    `json:"title"`
		CategoryID   *uint     `json:"categoryId"`
		CategoryIDs  []uint    `json:"categoryIds"`
		ShelfOrderAt time.Time `json:"shelfOrderAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID == 0 || item.Title != "direct" {
		t.Fatalf("expected imported shelf item, got %+v", item)
	}
	if item.CategoryID == nil || *item.CategoryID != category.ID {
		t.Fatalf("expected category %d in shelf item, got %+v", category.ID, item.CategoryID)
	}
	if !sameUintSet(item.CategoryIDs, []uint{category.ID, categoryB.ID}) {
		t.Fatalf("expected both direct import categories, got %+v", item.CategoryIDs)
	}
	if item.ShelfOrderAt.IsZero() {
		t.Fatalf("expected shelfOrderAt in direct import response, got %+v", item)
	}
}

func TestDirectImportPreviewDoesNotCreateBook(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var before int64
	if err := server.db.Model(&models.Book{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "preview.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("第一章 开始\n正文\n第二章 继续\n正文")); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("title", "预览书名"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/imports/books/preview", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("direct import preview: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var preview struct {
		Title        string `json:"title"`
		ChapterCount int    `json:"chapterCount"`
		Chapters     []struct {
			Title string `json:"title"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Title != "预览书名" || preview.ChapterCount < 1 || len(preview.Chapters) != preview.ChapterCount {
		t.Fatalf("unexpected direct preview: %+v", preview)
	}
	var after int64
	if err := server.db.Model(&models.Book{}).Count(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("preview must not create books: before=%d after=%d", before, after)
	}
}

func TestDirectImportReusesStagedUploadForReparseAndImport(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	request := func(path string, fields map[string]string, withFile bool) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if withFile {
			part, err := writer.CreateFormFile("file", "staged.txt")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte("第一章 开始\n正文\n第二章 继续\n正文")); err != nil {
				t.Fatal(err)
			}
		}
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	first := request("/api/imports/books/preview", nil, true)
	if first.Code != http.StatusOK {
		t.Fatalf("first staged preview: expected 200, got %d: %s", first.Code, first.Body.String())
	}
	var preview struct {
		ImportToken  string `json:"importToken"`
		ChapterCount int    `json:"chapterCount"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !validLocalImportToken(preview.ImportToken) || preview.ChapterCount < 1 {
		t.Fatalf("unexpected staged preview: %+v", preview)
	}
	dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(1), preview.ImportToken)
	if _, err := os.Stat(dataPath); err != nil {
		t.Fatalf("staged book data missing: %v", err)
	}
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("staged metadata missing: %v", err)
	}

	second := request("/api/imports/books/preview", map[string]string{
		"importToken": preview.ImportToken,
		"tocRule":     `^第.+章.*$`,
	}, false)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"chapterCount":2`) {
		t.Fatalf("token reparse: expected 2 chapters, got %d: %s", second.Code, second.Body.String())
	}

	imported := request("/api/imports/books", map[string]string{
		"importToken": preview.ImportToken,
		"title":       "复用上传测试",
		"tocRule":     `^第.+章.*$`,
	}, false)
	if imported.Code != http.StatusCreated {
		t.Fatalf("token import: expected 201, got %d: %s", imported.Code, imported.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.Book{}).Where("title = ?", "复用上传测试").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected imported book, got %d", count)
	}
	if _, err := os.Stat(dataPath); !os.IsNotExist(err) {
		t.Fatalf("staged data should be removed after import, got %v", err)
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("staged metadata should be removed after import, got %v", err)
	}
}

func TestDirectEPUBImportAndRefreshUseTocRule(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	epubData := testEPUBArchive(t)

	request := func(path string, rule string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "rules.epub")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(epubData); err != nil {
			t.Fatal(err)
		}
		if err := writer.WriteField("tocRule", rule); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	previewW := request("/api/imports/books/preview", "toc")
	if previewW.Code != http.StatusOK {
		t.Fatalf("epub preview: expected 200, got %d: %s", previewW.Code, previewW.Body.String())
	}
	var preview struct {
		Chapters []struct {
			Title string `json:"title"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(previewW.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Chapters) != 2 || preview.Chapters[0].Title != "目录二" || preview.Chapters[1].Title != "目录一" {
		t.Fatalf("preview ignored epub toc rule: %+v", preview.Chapters)
	}

	importW := request("/api/imports/books", "spin")
	if importW.Code != http.StatusCreated {
		t.Fatalf("epub import: expected 201, got %d: %s", importW.Code, importW.Body.String())
	}
	var imported bookListItem
	if err := json.Unmarshal(importW.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	var book models.Book
	if err := server.db.First(&book, imported.ID).Error; err != nil {
		t.Fatal(err)
	}
	if book.TOCRule != "spin" {
		t.Fatalf("epub import toc rule = %q, want spin", book.TOCRule)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].Title != "正文一" || chapters[1].Title != "正文二" {
		t.Fatalf("import ignored spine rule: %+v", chapters)
	}
	if chapters[0].ResourcePath != "OPS/one.xhtml" || chapters[1].ResourcePath != "OPS/two.xhtml" {
		t.Fatalf("epub resource paths were not imported: %+v", chapters)
	}

	contentReq := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content",
		nil,
	)
	contentReq.Header.Set("Authorization", token)
	contentW := httptest.NewRecorder()
	router.ServeHTTP(contentW, contentReq)
	if contentW.Code != http.StatusOK {
		t.Fatalf("epub chapter content: expected 200, got %d: %s", contentW.Code, contentW.Body.String())
	}
	var contentResponse struct {
		Format            string         `json:"format"`
		ResourceURL       string         `json:"resourceUrl"`
		ResourceExpiresAt string         `json:"resourceExpiresAt"`
		Content           string         `json:"content"`
		Chapter           models.Chapter `json:"chapter"`
	}
	if err := json.Unmarshal(contentW.Body.Bytes(), &contentResponse); err != nil {
		t.Fatal(err)
	}
	if contentResponse.Format != "epub" || contentResponse.ResourceURL == "" || contentResponse.ResourceExpiresAt == "" {
		t.Fatalf("missing epub resource metadata: %+v", contentResponse)
	}
	if !strings.Contains(contentResponse.Content, "内容一") || contentResponse.Chapter.ResourcePath != "OPS/one.xhtml" {
		t.Fatalf("epub response lost text/resource path: %+v", contentResponse)
	}
	if strings.Contains(contentResponse.ResourceURL, strings.TrimPrefix(token, "Bearer ")) {
		t.Fatal("resource URL leaked the login JWT")
	}

	resourceReq := httptest.NewRequest(http.MethodGet, contentResponse.ResourceURL, nil)
	resourceW := httptest.NewRecorder()
	router.ServeHTTP(resourceW, resourceReq)
	if resourceW.Code != http.StatusOK {
		t.Fatalf("epub XHTML resource: expected 200, got %d: %s", resourceW.Code, resourceW.Body.String())
	}
	if !strings.Contains(resourceW.Body.String(), "openreader-epub-bridge") ||
		!strings.Contains(resourceW.Body.String(), "内容一") ||
		strings.Contains(resourceW.Body.String(), "epub-authored-script") {
		t.Fatalf("unexpected served XHTML: %s", resourceW.Body.String())
	}
	if resourceW.Header().Get("X-Content-Type-Options") != "nosniff" ||
		resourceW.Header().Get("Referrer-Policy") != "no-referrer" ||
		resourceW.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("missing EPUB security headers: %v", resourceW.Header())
	}

	resourcePrefix := strings.TrimSuffix(contentResponse.ResourceURL, "OPS/one.xhtml")
	cssReq := httptest.NewRequest(http.MethodGet, resourcePrefix+"OPS/styles/book.css", nil)
	cssW := httptest.NewRecorder()
	router.ServeHTTP(cssW, cssReq)
	if cssW.Code != http.StatusOK || !strings.Contains(cssW.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("epub CSS resource: got %d %q: %s", cssW.Code, cssW.Header().Get("Content-Type"), cssW.Body.String())
	}
	imageReq := httptest.NewRequest(http.MethodGet, resourcePrefix+"OPS/images/cover.svg", nil)
	imageW := httptest.NewRecorder()
	router.ServeHTTP(imageW, imageReq)
	if imageW.Code != http.StatusOK || !strings.Contains(imageW.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("epub image resource: got %d %q: %s", imageW.Code, imageW.Header().Get("Content-Type"), imageW.Body.String())
	}

	extractionRoot := filepath.Join(server.cfg.LibraryDir, book.LibraryPath, ".epub-resources")
	if err := os.RemoveAll(extractionRoot); err != nil {
		t.Fatal(err)
	}
	rebuildReq := httptest.NewRequest(http.MethodGet, contentResponse.ResourceURL, nil)
	rebuildW := httptest.NewRecorder()
	router.ServeHTTP(rebuildW, rebuildReq)
	if rebuildW.Code != http.StatusOK || !strings.Contains(rebuildW.Body.String(), "内容一") {
		t.Fatalf("epub resource rebuild: got %d: %s", rebuildW.Code, rebuildW.Body.String())
	}

	parts := strings.Split(strings.TrimPrefix(contentResponse.ResourceURL, "/api/epub-resource/"), "/")
	if len(parts) < 2 {
		t.Fatalf("unexpected resource URL: %q", contentResponse.ResourceURL)
	}
	tamperedCapability := parts[0]
	if strings.HasSuffix(tamperedCapability, "a") {
		tamperedCapability = strings.TrimSuffix(tamperedCapability, "a") + "b"
	} else {
		tamperedCapability += "a"
	}
	tamperedReq := httptest.NewRequest(
		http.MethodGet,
		"/api/epub-resource/"+tamperedCapability+"/OPS/one.xhtml",
		nil,
	)
	tamperedW := httptest.NewRecorder()
	router.ServeHTTP(tamperedW, tamperedReq)
	if tamperedW.Code == http.StatusOK {
		t.Fatal("tampered EPUB capability unexpectedly succeeded")
	}

	unsupportedReq := httptest.NewRequest(
		http.MethodGet,
		resourcePrefix+"OPS/scripts/evil.js",
		nil,
	)
	unsupportedW := httptest.NewRecorder()
	router.ServeHTTP(unsupportedW, unsupportedReq)
	if unsupportedW.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("unsupported EPUB script: expected 415, got %d: %s", unsupportedW.Code, unsupportedW.Body.String())
	}
	missingReq := httptest.NewRequest(
		http.MethodGet,
		resourcePrefix+"OPS/images/missing.png",
		nil,
	)
	missingW := httptest.NewRecorder()
	router.ServeHTTP(missingW, missingReq)
	if missingW.Code != http.StatusNotFound {
		t.Fatalf("missing EPUB resource: expected 404, got %d: %s", missingW.Code, missingW.Body.String())
	}

	if err := server.db.Model(&models.Chapter{}).
		Where("id = ?", chapters[0].ID).
		Update("resource_path", "").Error; err != nil {
		t.Fatal(err)
	}
	recoverReq := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content",
		nil,
	)
	recoverReq.Header.Set("Authorization", token)
	recoverW := httptest.NewRecorder()
	router.ServeHTTP(recoverW, recoverReq)
	if recoverW.Code != http.StatusOK {
		t.Fatalf("legacy EPUB path recovery: expected 200, got %d: %s", recoverW.Code, recoverW.Body.String())
	}
	var recovered models.Chapter
	if err := server.db.First(&recovered, chapters[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.ResourcePath != "OPS/one.xhtml" {
		t.Fatalf("legacy EPUB resource path was not recovered: %+v", recovered)
	}

	sourcePath := filepath.Join(server.cfg.LibraryDir, book.OriginalFile)
	if err := os.WriteFile(sourcePath, testEPUBArchiveWithBody(t, "更新后的内容一。"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleReq := httptest.NewRequest(http.MethodGet, contentResponse.ResourceURL, nil)
	staleW := httptest.NewRecorder()
	router.ServeHTTP(staleW, staleReq)
	if staleW.Code != http.StatusForbidden {
		t.Fatalf("stale EPUB capability: expected 403, got %d: %s", staleW.Code, staleW.Body.String())
	}

	if err := server.db.Model(&models.Book{}).
		Where("id = ?", book.ID).
		Update("user_id", book.UserID+100).Error; err != nil {
		t.Fatal(err)
	}
	ownershipReq := httptest.NewRequest(http.MethodGet, recoverResourceURL(t, recoverW.Body.Bytes()), nil)
	ownershipW := httptest.NewRecorder()
	router.ServeHTTP(ownershipW, ownershipReq)
	if ownershipW.Code != http.StatusNotFound {
		t.Fatalf("ownership-changed EPUB capability: expected 404, got %d: %s", ownershipW.Code, ownershipW.Body.String())
	}
	if err := server.db.Model(&models.Book{}).
		Where("id = ?", book.ID).
		Update("user_id", book.UserID).Error; err != nil {
		t.Fatal(err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", strings.NewReader(`{"tocRule":"toc"}`))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshReq.Header.Set("Authorization", token)
	refreshW := httptest.NewRecorder()
	router.ServeHTTP(refreshW, refreshReq)
	if refreshW.Code != http.StatusOK {
		t.Fatalf("refresh epub: expected 200, got %d: %s", refreshW.Code, refreshW.Body.String())
	}
	chapters = nil
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].Title != "目录二" || chapters[1].Title != "目录一" {
		t.Fatalf("refresh ignored toc rule: %+v", chapters)
	}
	if err := server.db.First(&book, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if book.TOCRule != "toc" {
		t.Fatalf("refreshed epub toc rule = %q, want toc", book.TOCRule)
	}
}

func testEPUBArchive(t *testing.T) []byte {
	return testEPUBArchiveWithBody(t, "内容一。")
}

func recoverResourceURL(t *testing.T, response []byte) string {
	t.Helper()
	var payload struct {
		ResourceURL string `json:"resourceUrl"`
	}
	if err := json.Unmarshal(response, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ResourceURL == "" {
		t.Fatalf("response has no EPUB resource URL: %s", response)
	}
	return payload.ResourceURL
}

func testEPUBArchiveWithBody(t *testing.T, firstChapterBody string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	write := func(name string, content string) {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	write("META-INF/container.xml", `<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`)
	write("OPS/content.opf", `<package>
  <metadata><title>规则书</title></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="one" href="one.xhtml" media-type="application/xhtml+xml"/>
    <item id="two" href="two.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="one"/><itemref idref="two"/></spine>
</package>`)
	write("OPS/nav.xhtml", `<html><body><nav epub:type="toc"><a href="two.xhtml">目录二</a><a href="one.xhtml">目录一</a></nav></body></html>`)
	write("OPS/one.xhtml", `<html><head>
  <link rel="stylesheet" href="styles/book.css"/>
  <script id="epub-authored-script">window.evil = true</script>
</head><body>
  <h1 id="start">正文一</h1>
  <p>`+firstChapterBody+`</p>
  <img src="images/cover.svg" alt="封面"/>
  <a href="#start">页内链接</a>
  <a href="two.xhtml">下一章</a>
</body></html>`)
	write("OPS/two.xhtml", `<html><body><h1>正文二</h1><p>内容二。</p><a href="one.xhtml">上一章</a></body></html>`)
	write("OPS/styles/book.css", `body { color: rgb(12, 34, 56); }`)
	write("OPS/images/cover.svg", `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20"/></svg>`)
	write("OPS/scripts/evil.js", `window.epubAuthoredScript = true`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestRefreshLocalBookReparsesArchivedSource(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "refresh.txt"), []byte("第一章 开始\n旧正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/local-store/import", strings.NewReader(`{"paths":["refresh.txt"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("local store import: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var book models.Book
	if err := server.db.Where("title = ?", "refresh").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(server.cfg.LibraryDir, book.OriginalFile)
	next := "第一章、开始\n新正文"
	if err := os.WriteFile(sourcePath, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh local book: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var refreshed models.Book
	if err := server.db.First(&refreshed, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.ChapterCount != 1 || refreshed.LastChapter != "第一章、开始" {
		t.Fatalf("unexpected refreshed book: %+v", refreshed)
	}
	var chapter models.Chapter
	if err := server.db.Where("book_id = ? AND `index` = ?", book.ID, 0).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chapter content: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "新正文") || chapter.CachePath == "" {
		t.Fatalf("expected refreshed chapter content and cache, chapter=%+v body=%s", chapter, w.Body.String())
	}

	next = "== 第一节 ==\n第一节正文\n== 第二节 ==\n第二节正文"
	if err := os.WriteFile(sourcePath, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", strings.NewReader(`{"tocRule":"^== .+ ==$"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh local book with toc rule: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := server.db.First(&refreshed, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.TOCRule != "^== .+ ==$" || refreshed.ChapterCount != 2 || refreshed.LastChapter != "== 第二节 ==" {
		t.Fatalf("unexpected refreshed book with toc rule: %+v", refreshed)
	}
}

func TestLocalStoreImportDirectoryRecursively(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	nestedDir := filepath.Join(server.cfg.LocalStoreDir, "nested", "deeper")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "nested", "alpha.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "beta.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "ignore.bin"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"paths":["nested"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/local-store/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("local store directory import: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload struct {
		Imported []struct {
			Path  string       `json:"path"`
			Book  *models.Book `json:"book"`
			Error string       `json:"error"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Imported) != 2 {
		t.Fatalf("expected 2 imported files, got %+v", payload.Imported)
	}
	wantPaths := []string{"nested/alpha.txt", "nested/deeper/beta.txt"}
	for i, want := range wantPaths {
		if payload.Imported[i].Path != want {
			t.Fatalf("expected stable local store import order %v, got %+v", wantPaths, payload.Imported)
		}
	}
}

func TestLocalStoreImportRootRecursively(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	nestedDir := filepath.Join(server.cfg.LocalStoreDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "root.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "child.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/local-store/import", strings.NewReader(`{"paths":[""]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("local store root import: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload struct {
		Imported []struct {
			Path string       `json:"path"`
			Book *models.Book `json:"book"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Imported) != 2 {
		t.Fatalf("expected root import to include nested files, got %+v", payload.Imported)
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
	if !strings.Contains(w2.Body.String(), "<getcontentlength>12</getcontentlength>") || !strings.Contains(w2.Body.String(), "<lastmodified>") {
		t.Fatalf("webdav list should include size and modified time, got %s", w2.Body.String())
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

func TestTriggerBackupReturnsWebDAVFileName(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/backup/trigger", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("trigger backup: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Name == "" || payload.Name != payload.Path || !strings.HasPrefix(payload.Name, "backup_") || !strings.HasSuffix(payload.Name, ".zip") {
		t.Fatalf("unexpected backup response: %+v", payload)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.DataDir, "webdav", payload.Name)); err != nil {
		t.Fatalf("backup file was not created in webdav dir: %v", err)
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
	if _, err := file.Write([]byte(`[{"name":"恢复书","author":"恢复作者","bookUrl":"https://book.example/1","coverUrl":"https://book.example/cover.jpg","customCoverUrl":"/uploads/covers/custom-restore.jpg","intro":"简介"}]`)); err != nil {
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
	if book.Author != "恢复作者" || book.URL != "https://book.example/1" || book.CustomCoverURL != "/uploads/covers/custom-restore.jpg" {
		t.Fatalf("unexpected restored book: %+v", book)
	}
	var progress models.ReadingProgress
	if err := server.db.Where("book_id = ?", book.ID).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterIndex != 2 || progress.Offset != 88 {
		t.Fatalf("unexpected restored progress: %+v", progress)
	}

	var updateBuffer bytes.Buffer
	updateZip := zip.NewWriter(&updateBuffer)
	updateFile, err := updateZip.Create("myBookShelf.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updateFile.Write([]byte(`[{"name":"恢复书","author":"二次恢复作者","bookUrl":"https://book.example/1","intro":"二次简介"}]`)); err != nil {
		t.Fatal(err)
	}
	if err := updateZip.Close(); err != nil {
		t.Fatal(err)
	}
	var updateBody bytes.Buffer
	updateWriter := multipart.NewWriter(&updateBody)
	updatePart, err := updateWriter.CreateFormFile("file", "backup-update.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updatePart.Write(updateBuffer.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := updateWriter.Close(); err != nil {
		t.Fatal(err)
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/api/backup/restore-legado", &updateBody)
	updateReq.Header.Set("Content-Type", updateWriter.FormDataContentType())
	updateReq.Header.Set("Authorization", token)
	updateW := httptest.NewRecorder()
	router.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("restore existing backup: expected 200, got %d: %s", updateW.Code, updateW.Body.String())
	}
	if !strings.Contains(updateW.Body.String(), `"books":1`) {
		t.Fatalf("expected existing restored book to count as updated, got %s", updateW.Body.String())
	}
	if err := server.db.First(&book, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if book.Author != "二次恢复作者" || book.Intro != "二次简介" {
		t.Fatalf("expected existing book metadata to update, got %+v", book)
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
	settingFile, err := zipWriter.Create("userSettings.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settingFile.Write([]byte(`[
		{"userId":99,"key":"search","value":"{\"searchType\":\"group\",\"group\":\"默认分组\",\"concurrent\":32}"},
		{"userId":99,"key":"reader","value":"{\"fontSize\":24,\"pageMode\":\"mobile\",\"miniInterface\":true}"}
	]`)); err != nil {
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
	if !strings.Contains(w.Body.String(), `"settings":2`) {
		t.Fatalf("expected two restored settings, got %s", w.Body.String())
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
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	var setting models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", user.ID, "search").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(setting.Value, `"concurrent":32`) {
		t.Fatalf("unexpected restored setting: %+v", setting)
	}
	setting = models.UserSetting{}
	if err := server.db.Where("user_id = ? AND key = ?", user.ID, "reader").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(setting.Value, "pageMode") || strings.Contains(setting.Value, "miniInterface") {
		t.Fatalf("restored reader setting kept local page mode: %+v", setting)
	}
}

func TestRestoreOpenReaderBackupImportsUserData(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	progressFile, err := zipWriter.Create("readingProgress.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := progressFile.Write([]byte(`[{"bookTitle":"OpenReader备份书","bookUrl":"https://book.example/openreader","chapterIndex":5,"offset":128,"chapterPercent":0.66,"chapterTitle":"第五章"}]`)); err != nil {
		t.Fatal(err)
	}
	bookmarkFile, err := zipWriter.Create("bookmarks.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookmarkFile.Write([]byte(`[{"bookTitle":"OpenReader备份书","bookUrl":"https://book.example/openreader","chapterIndex":1,"offset":42,"percent":0.4,"title":"书签标题","excerpt":"摘录"}]`)); err != nil {
		t.Fatal(err)
	}
	bookFile, err := zipWriter.Create("bookshelf.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookFile.Write([]byte(`[{"title":"OpenReader备份书","author":"作者","url":"https://book.example/openreader","coverUrl":"https://book.example/openreader-cover.jpg","customCoverUrl":"/uploads/covers/openreader-custom.jpg","lastChapter":"最新章","chapterCount":12,"canUpdate":true,"categoryName":"OpenReader分组","categoryNames":["OpenReader分组","OpenReader分组二"]}]`)); err != nil {
		t.Fatal(err)
	}
	categoryFile, err := zipWriter.Create("categories.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := categoryFile.Write([]byte(`[{"name":"OpenReader分组","color":"#336699","sortOrder":3},{"name":"OpenReader分组二","color":"#663399","sortOrder":4}]`)); err != nil {
		t.Fatal(err)
	}
	ruleFile, err := zipWriter.Create("replaceRules.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ruleFile.Write([]byte(`[{"name":"规则","pattern":"foo","replacement":"bar","enabled":true}]`)); err != nil {
		t.Fatal(err)
	}
	rssFile, err := zipWriter.Create("rssSources.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rssFile.Write([]byte(`[{"sourceName":"OpenReader RSS","sourceUrl":"https://rss.example/openreader.xml","sourceIcon":"https://rss.example/icon.png","sourceGroup":"资讯","sourceComment":"恢复注释","customOrder":7,"concurrentRate":"3/1000","headerMap":{"X-Restore":"yes"},"loginUrl":"https://rss.example/login","loginCheckJs":"check()","articleStyle":2,"sortUrl":"综合::https://rss.example/openreader-sort.xml","ruleArticles":"article","ruleTitle":"title","rulePubDate":"date","ruleImage":"img","ruleLink":"a@href","ruleContent":"content","style":"body{}","enableJs":false,"loadWithBaseUrl":false,"enabled":false}]`)); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "openreader-backup.zip")
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
		t.Fatalf("restore openreader backup: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	for _, expected := range []string{`"books":1`, `"categories":2`, `"bookmarks":1`, `"progress":1`, `"replaceRules":1`, `"rssSources":1`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("expected %s in restore result, got %s", expected, w.Body.String())
		}
	}

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	var book models.Book
	if err := server.db.Where("user_id = ? AND title = ?", user.ID, "OpenReader备份书").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.URL != "https://book.example/openreader" || book.ChapterCount != 12 || book.CategoryID == nil || book.CustomCoverURL != "/uploads/covers/openreader-custom.jpg" {
		t.Fatalf("unexpected restored openreader book: %+v", book)
	}
	var category models.Category
	if err := server.db.Where("user_id = ? AND name = ?", user.ID, "OpenReader分组").First(&category).Error; err != nil {
		t.Fatal(err)
	}
	var categoryExtra models.Category
	if err := server.db.Where("user_id = ? AND name = ?", user.ID, "OpenReader分组二").First(&categoryExtra).Error; err != nil {
		t.Fatal(err)
	}
	if book.CategoryID == nil || *book.CategoryID != category.ID {
		t.Fatalf("expected restored book category %d, got %+v", category.ID, book.CategoryID)
	}
	var restoredCategoryRows []models.BookCategory
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).Find(&restoredCategoryRows).Error; err != nil {
		t.Fatal(err)
	}
	restoredCategoryIDs := make([]uint, 0, len(restoredCategoryRows))
	for _, row := range restoredCategoryRows {
		restoredCategoryIDs = append(restoredCategoryIDs, row.CategoryID)
	}
	if !sameUintSet(restoredCategoryIDs, []uint{category.ID, categoryExtra.ID}) {
		t.Fatalf("expected restored book categories, got %+v", restoredCategoryRows)
	}
	var progress models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterIndex != 5 || progress.Offset != 128 || progress.ChapterTitle != "第五章" {
		t.Fatalf("unexpected restored progress: %+v", progress)
	}
	var bookmark models.Bookmark
	if err := server.db.Where("user_id = ? AND book_id = ? AND title = ?", user.ID, book.ID, "书签标题").First(&bookmark).Error; err != nil {
		t.Fatal(err)
	}
	if bookmark.Offset != 42 || bookmark.ChapterIndex != 1 {
		t.Fatalf("unexpected restored bookmark: %+v", bookmark)
	}
	var rule models.ReplaceRule
	if err := server.db.Where("user_id = ? AND pattern = ?", user.ID, "foo").First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if rule.Replacement != "bar" || !rule.Enabled {
		t.Fatalf("unexpected restored replace rule: %+v", rule)
	}
	var rssSource models.RSSSource
	if err := server.db.Where("user_id = ? AND url = ?", user.ID, "https://rss.example/openreader.xml").First(&rssSource).Error; err != nil {
		t.Fatal(err)
	}
	if rssSource.Title != "OpenReader RSS" || rssSource.Icon != "https://rss.example/icon.png" || rssSource.Group != "资讯" || rssSource.Comment != "恢复注释" || rssSource.CustomOrder != 7 || rssSource.Enabled {
		t.Fatalf("unexpected restored rss source: %+v", rssSource)
	}
	if rssSource.ConcurrentRate != "3/1000" || !strings.Contains(rssSource.Header, `"X-Restore":"yes"`) || rssSource.LoginURL != "https://rss.example/login" || rssSource.LoginCheckJS != "check()" {
		t.Fatalf("unexpected restored RSS transport fields: %+v", rssSource)
	}
	if rssSource.SingleURL || rssSource.ArticleStyle != 2 || rssSource.SortURL != "综合::https://rss.example/openreader-sort.xml" || rssSource.RuleArticles != "article" || rssSource.RuleTitle != "title" || rssSource.RulePubDate != "date" || rssSource.RuleImage != "img" || rssSource.RuleLink != "a@href" || rssSource.RuleContent != "content" || rssSource.Style != "body{}" || rssSource.EnableJS || rssSource.LoadWithBaseURL {
		t.Fatalf("unexpected restored advanced rss source fields: %+v", rssSource)
	}
}

func TestBatchUpsertAndDeleteReplaceRules(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	isRegex := true
	existing := models.ReplaceRule{
		UserID: user.ID, Name: "已有规则", Pattern: "旧匹配", Replacement: "旧替换",
		Scope: "*", IsRegex: &isRegex, Enabled: true,
	}
	otherUserRule := models.ReplaceRule{
		UserID: user.ID + 100, Name: "其它用户规则", Pattern: "不能删除",
		Scope: "*", IsRegex: &isRegex, Enabled: true,
	}
	if err := server.db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&otherUserRule).Error; err != nil {
		t.Fatal(err)
	}

	body := `[
		{"name":"已有规则","pattern":"新匹配","replacement":"新替换","scope":"目标书","isRegex":false,"enabled":false},
		{"name":"新增规则","pattern":"广告","replacement":"","scope":"*","isRegex":true,"isEnabled":true},
		{"name":"","pattern":"无名称"},
		{"name":"无匹配","pattern":""}
	]`
	req := httptest.NewRequest(http.MethodPost, "/api/replace-rules/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("batch upsert replace rules: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var upserted struct {
		Rules   []models.ReplaceRule `json:"rules"`
		Created int                  `json:"created"`
		Updated int                  `json:"updated"`
		Skipped int                  `json:"skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &upserted); err != nil {
		t.Fatal(err)
	}
	if upserted.Created != 1 || upserted.Updated != 1 || upserted.Skipped != 2 || len(upserted.Rules) != 2 {
		t.Fatalf("unexpected batch upsert summary: %+v", upserted)
	}
	if upserted.Rules[0].ID != existing.ID || upserted.Rules[0].Pattern != "新匹配" || upserted.Rules[0].Enabled {
		t.Fatalf("expected existing rule to be updated in place, got %+v", upserted.Rules[0])
	}

	var ownRules []models.ReplaceRule
	if err := server.db.Where("user_id = ?", user.ID).Order("id asc").Find(&ownRules).Error; err != nil {
		t.Fatal(err)
	}
	if len(ownRules) != 2 || ownRules[0].ID != existing.ID || ownRules[1].Name != "新增规则" {
		t.Fatalf("expected one updated and one created rule, got %+v", ownRules)
	}

	deleteBody := fmt.Sprintf(`{"ids":[%d,%d,%d,%d]}`, ownRules[0].ID, ownRules[1].ID, otherUserRule.ID, ownRules[0].ID)
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/replace-rules/batch-delete", strings.NewReader(deleteBody))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteReq.Header.Set("Authorization", token)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("batch delete replace rules: expected 200, got %d: %s", deleteW.Code, deleteW.Body.String())
	}
	var deleted struct {
		DeletedIDs []uint `json:"deletedIds"`
	}
	if err := json.Unmarshal(deleteW.Body.Bytes(), &deleted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(deleted.DeletedIDs, []uint{ownRules[0].ID, ownRules[1].ID}) {
		t.Fatalf("unexpected deleted replace rule ids: %+v", deleted.DeletedIDs)
	}

	var remaining []models.ReplaceRule
	if err := server.db.Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != otherUserRule.ID {
		t.Fatalf("batch delete crossed user scope: %+v", remaining)
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

	req := httptest.NewRequest(http.MethodPost, "/api/replace-rules/test", strings.NewReader(`{"pattern":"广告[0-9]+","replacement":"","isRegex":true,"text":"广告123\n正文"}`))
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

	req2 := httptest.NewRequest(http.MethodPost, "/api/replace-rules/test", strings.NewReader(`{"pattern":"广告[0-9]+","replacement":"净化","isRegex":false,"text":"广告[0-9]+\n广告123"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("test plain replace rule: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), `净化\n广告123`) {
		t.Fatalf("unexpected plain replace rule test result: %s", w2.Body.String())
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
							<enclosure url="/images/a.jpg" type="image/jpeg"></enclosure>
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
	if !strings.Contains(w3.Body.String(), "https://rss.example/images/a.jpg") {
		t.Fatalf("list rss articles: expected article image, got %d: %s", w3.Code, w3.Body.String())
	}

	var count int64
	if err := server.db.Model(&models.RSSArticle{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one rss article, got %d", count)
	}
	var article models.RSSArticle
	if err := server.db.Where("link = ?", "https://rss.example/a").First(&article).Error; err != nil {
		t.Fatal(err)
	}
	if article.Image != "https://rss.example/images/a.jpg" {
		t.Fatalf("expected rss article image to persist, got %+v", article)
	}
}

func TestFetchRSSArticlesSupportsRDFRootItems(t *testing.T) {
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
					<rdf:RDF
						xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
						xmlns:dc="http://purl.org/dc/elements/1.1/"
						xmlns:media="http://search.yahoo.com/mrss/">
						<channel rdf:about="https://rss.example/rdf.xml">
							<title>RDF 订阅</title>
						</channel>
						<item rdf:about="https://rss.example/posts/rdf">
							<Title>RDF 文章</Title>
							<Link>/posts/rdf</Link>
							<dc:creator>RDF 作者</dc:creator>
							<Description>RDF 摘要</Description>
							<PubDate>Sun, 29 Jun 2026 10:00:00 +0800</PubDate>
							<media:thumbnail url="/covers/rdf.jpg"></media:thumbnail>
						</item>
					</rdf:RDF>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	articles, err := fetchRSSArticles(models.RSSSource{
		Title: "RDF RSS",
		URL:   "https://rss.example/rdf.xml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 {
		t.Fatalf("rdf articles = %d, want 1: %+v", len(articles), articles)
	}
	article := articles[0]
	if article.Title != "RDF 文章" ||
		article.Link != "https://rss.example/posts/rdf" ||
		article.GUID != "https://rss.example/posts/rdf" ||
		article.Author != "RDF 作者" ||
		article.Image != "https://rss.example/covers/rdf.jpg" ||
		article.PubDate != "Sun, 29 Jun 2026 10:00:00 +0800" {
		t.Fatalf("unexpected rdf article: %+v", article)
	}
}

func TestFetchRSSArticlesHonorsSingleURLBeforeSortURL(t *testing.T) {
	requestedPaths := make([]string, 0, 2)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedPaths = append(requestedPaths, req.URL.Path)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<rss><channel><item>
					<title>地址验证</title>
					<link>/article</link>
				</item></channel></rss>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.RSSSource{
		URL:       "https://rss.example/feed.xml",
		SortURL:   "分类::https://rss.example/category.xml",
		SingleURL: true,
	}
	if _, err := fetchRSSArticles(source); err != nil {
		t.Fatal(err)
	}
	source.SingleURL = false
	if _, err := fetchRSSArticles(source); err != nil {
		t.Fatal(err)
	}
	if strings.Join(requestedPaths, ",") != "/feed.xml,/category.xml" {
		t.Fatalf("singleUrl request paths = %v", requestedPaths)
	}

	singleOptions := rssSourceSortOptions(models.RSSSource{
		URL:       "https://rss.example/feed.xml",
		SortURL:   "分类 A::/a&&分类 B::/b",
		SingleURL: true,
	})
	if len(singleOptions) != 1 || singleOptions[0].URL != "https://rss.example/feed.xml" || singleOptions[0].Name != "" {
		t.Fatalf("singleUrl sort options = %+v", singleOptions)
	}
}

func TestRSSRefreshUsesGUIDWithinSortWithoutDuplicatingArticles(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
					<rss version="2.0"><channel><item>
						<title>无链接文章</title>
						<guid isPermaLink="false">article-guid-1</guid>
						<description>只有标准 GUID</description>
						<pubDate>Sun, 29 Jun 2026 11:00:00 +0800</pubDate>
					</item></channel></rss>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	createReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"GUID RSS","url":"https://rss.example/guid.xml","enabled":true}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", token)
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create rss source: expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var source models.RSSSource
	if err := json.Unmarshal(createW.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	legacyArticles := []models.RSSArticle{
		{
			UserID:   source.UserID,
			SourceID: source.ID,
			Sort:     "分类 A",
			Title:    "无链接文章",
			PubDate:  "Sun, 29 Jun 2026 11:00:00 +0800",
			IsRead:   true,
		},
		{
			UserID:   source.UserID,
			SourceID: source.ID,
			Sort:     "分类 A",
			Title:    "无链接文章",
			PubDate:  "Sun, 29 Jun 2026 11:00:00 +0800",
			Favorite: true,
		},
	}
	if err := server.db.Create(&legacyArticles).Error; err != nil {
		t.Fatal(err)
	}

	refresh := func(sortName string) *httptest.ResponseRecorder {
		t.Helper()
		path := "/api/rss/sources/" + strconv.FormatUint(uint64(source.ID), 10) + "/refresh?sortName=" + url.QueryEscape(sortName)
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	firstW := refresh("分类 A")
	if firstW.Code != http.StatusOK || !strings.Contains(firstW.Body.String(), `"imported":0`) {
		t.Fatalf("first refresh: got %d: %s", firstW.Code, firstW.Body.String())
	}
	var sameSort []models.RSSArticle
	if err := server.db.Where("source_id = ? AND sort = ?", source.ID, "分类 A").Find(&sameSort).Error; err != nil {
		t.Fatal(err)
	}
	if len(sameSort) != 1 ||
		sameSort[0].Link != "" ||
		sameSort[0].GUID != "article-guid-1" ||
		!sameSort[0].IsRead ||
		!sameSort[0].Favorite {
		t.Fatalf("legacy duplicates were not merged into the guid article: %+v", sameSort)
	}

	secondW := refresh("分类 A")
	if secondW.Code != http.StatusOK || !strings.Contains(secondW.Body.String(), `"imported":0`) {
		t.Fatalf("second refresh should update existing article: got %d: %s", secondW.Code, secondW.Body.String())
	}
	sameSort = nil
	if err := server.db.Where("source_id = ? AND sort = ?", source.ID, "分类 A").Find(&sameSort).Error; err != nil {
		t.Fatal(err)
	}
	if len(sameSort) != 1 || !sameSort[0].IsRead || !sameSort[0].Favorite {
		t.Fatalf("same-sort refresh duplicated article or lost state: %+v", sameSort)
	}

	thirdW := refresh("分类 B")
	if thirdW.Code != http.StatusOK || !strings.Contains(thirdW.Body.String(), `"imported":1`) {
		t.Fatalf("cross-sort refresh should keep a separate article: got %d: %s", thirdW.Code, thirdW.Body.String())
	}
	var total int64
	if err := server.db.Model(&models.RSSArticle{}).Where("source_id = ?", source.ID).Count(&total).Error; err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("cross-sort article count = %d, want 2", total)
	}
}

func TestRSSSourceRefreshUsesEmbeddedArticleImagesAsFallback(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
					<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
						<channel>
							<item>
								<title>摘要首图</title>
								<link>https://rss.example/posts/summary</link>
								<pubDate>应被后置 time 覆盖</pubDate>
								<time>刚刚</time>
								<description><![CDATA[<p>摘要</p><img src="../covers/summary.jpg"><img src="/covers/later.jpg">]]></description>
								<content:encoded><![CDATA[<img src="/covers/content-ignored.jpg">]]></content:encoded>
							</item>
							<item>
								<title>正文首图</title>
								<link>https://rss.example/posts/content</link>
								<time>应被后置 pubDate 覆盖</time>
								<pubDate>Sun, 29 Jun 2026 09:00:00 +0800</pubDate>
								<description>无图摘要</description>
								<content:encoded><![CDATA[<p>正文</p><img src="/covers/content.jpg">]]></content:encoded>
							</item>
						</channel>
					</rss>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"内嵌图片 RSS","url":"https://rss.example/feeds/news.xml","enabled":true}`))
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
	if err := server.db.Create(&models.RSSArticle{
		UserID:   source.UserID,
		SourceID: source.ID,
		Title:    "旧摘要首图",
		Link:     "https://rss.example/posts/summary",
		PubDate:  "旧时间",
	}).Error; err != nil {
		t.Fatal(err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh", nil)
	refreshReq.Header.Set("Authorization", token)
	refreshW := httptest.NewRecorder()
	router.ServeHTTP(refreshW, refreshReq)
	if refreshW.Code != http.StatusOK || !strings.Contains(refreshW.Body.String(), `"imported":1`) {
		t.Fatalf("refresh rss source: expected one new and one updated article, got %d: %s", refreshW.Code, refreshW.Body.String())
	}

	var articles []models.RSSArticle
	if err := server.db.Where("source_id = ?", source.ID).Order("title asc").Find(&articles).Error; err != nil {
		t.Fatal(err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected two rss articles, got %+v", articles)
	}
	images := make(map[string]string, len(articles))
	for _, article := range articles {
		images[article.Title] = article.Image
	}
	if images["摘要首图"] != "https://rss.example/covers/summary.jpg" {
		t.Fatalf("description image fallback = %q", images["摘要首图"])
	}
	if images["正文首图"] != "https://rss.example/covers/content.jpg" {
		t.Fatalf("content image fallback = %q", images["正文首图"])
	}
	pubDates := make(map[string]string, len(articles))
	for _, article := range articles {
		pubDates[article.Title] = article.PubDate
	}
	if pubDates["摘要首图"] != "刚刚" {
		t.Fatalf("time fallback was not preserved: %q", pubDates["摘要首图"])
	}
	if pubDates["正文首图"] != "Sun, 29 Jun 2026 09:00:00 +0800" {
		t.Fatalf("pubDate was not preserved: %q", pubDates["正文首图"])
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/rss/articles?sourceId="+strconv.FormatUint(uint64(source.ID), 10), nil)
	listReq.Header.Set("Authorization", token)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK || !strings.Contains(listW.Body.String(), `"pubDate":"刚刚"`) {
		t.Fatalf("rss list did not expose the original date: %d %s", listW.Code, listW.Body.String())
	}
}

func TestDecodeRSSDocumentPreservesUpstreamImageEventOrder(t *testing.T) {
	parsed, err := decodeRSSDocument(`<?xml version="1.0" encoding="UTF-8"?>
		<rss version="2.0" xmlns:media="http://search.yahoo.com/mrss/" xmlns:content="http://purl.org/rss/1.0/modules/content/">
			<channel>
				<item>
					<title>后置 enclosure</title>
					<description><![CDATA[<img src="/description-a.jpg">]]></description>
					<media:thumbnail url="/thumbnail-a.jpg"></media:thumbnail>
					<enclosure url="/enclosure-a.jpg" type="image/jpeg"></enclosure>
				</item>
				<item>
					<title>后置 thumbnail</title>
					<enclosure url="/enclosure-b.jpg" type="image/jpeg"></enclosure>
					<description><![CDATA[<img src="/description-b.jpg">]]></description>
					<media:thumbnail url="/thumbnail-b.jpg"></media:thumbnail>
					<content:encoded><![CDATA[<img src="/content-b.jpg">]]></content:encoded>
				</item>
				<item>
					<title>摘要兜底</title>
					<description><![CDATA[<img src="/description-c.jpg">]]></description>
					<content:encoded><![CDATA[<img src="/content-c.jpg">]]></content:encoded>
				</item>
				<item>
					<title>正文兜底</title>
					<description>没有图片</description>
					<content:encoded><![CDATA[<img src="/content-d.jpg">]]></content:encoded>
				</item>
				<item>
					<title>media content 扩展</title>
					<description><![CDATA[<img src="/description-e.jpg">]]></description>
					<media:content url="/media-e.jpg" type="image/jpeg"></media:content>
				</item>
				<item>
					<title>无类型 enclosure</title>
					<description><![CDATA[<img src="/description-f.jpg">]]></description>
					<enclosure url="/untyped-f.jpg"></enclosure>
				</item>
			</channel>
		</rss>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Items) != 6 {
		t.Fatalf("decoded item count = %d", len(parsed.Items))
	}
	want := []string{"/enclosure-a.jpg", "/thumbnail-b.jpg", "/description-c.jpg", "/content-d.jpg", "/description-e.jpg", "/description-f.jpg"}
	for index, item := range parsed.Items {
		if item.Image != want[index] || !item.imageSelected {
			t.Fatalf("item %q image = %q (selected=%v), want %q", item.Title, item.Image, item.imageSelected, want[index])
		}
	}
	if image := resolveRSSItemImage("https://rss.example/feed.xml", parsed.Items[4]); image != "https://rss.example/media-e.jpg" {
		t.Fatalf("media:content extension image = %q", image)
	}
}

func TestDecodeRSSDocumentPreservesUpstreamDateEventOrder(t *testing.T) {
	parsed, err := decodeRSSDocument(`<rss><channel>
		<item><title>后置 time</title><pubDate>正式日期</pubDate><time>  刚刚  </time></item>
		<item><title>后置 pubDate</title><time>昨天</time><pubDate>  Sun, 29 Jun 2026 09:00:00 +0800  </pubDate></item>
		<item><title>后置空 time</title><pubDate>旧日期</pubDate><time></time></item>
	</channel></rss>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Items) != 3 {
		t.Fatalf("decoded item count = %d", len(parsed.Items))
	}
	want := []string{"  刚刚  ", "Sun, 29 Jun 2026 09:00:00 +0800", ""}
	for index, item := range parsed.Items {
		if item.Date != want[index] {
			t.Fatalf("item %q date = %q, want %q", item.Title, item.Date, want[index])
		}
	}
}

func TestAtomSourceRefreshResolvesRelativeArticleImages(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
					<feed xmlns="http://www.w3.org/2005/Atom" xmlns:media="http://search.yahoo.com/mrss/">
						<entry>
							<title>Atom 文章</title>
							<id>tag:rss.example,2026:atom</id>
							<link href="/posts/atom"></link>
							<summary>Atom 摘要</summary>
							<media:thumbnail url="../covers/atom.jpg"></media:thumbnail>
							<updated>2026-06-24T08:00:00Z</updated>
						</entry>
					</feed>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"Atom 源","url":"https://rss.example/feeds/atom.xml","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create atom source: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var source models.RSSSource
	if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh", nil)
	refreshReq.Header.Set("Authorization", token)
	refreshW := httptest.NewRecorder()
	router.ServeHTTP(refreshW, refreshReq)
	if refreshW.Code != http.StatusOK {
		t.Fatalf("refresh atom source: expected 200, got %d: %s", refreshW.Code, refreshW.Body.String())
	}

	var article models.RSSArticle
	if err := server.db.Where("source_id = ?", source.ID).First(&article).Error; err != nil {
		t.Fatal(err)
	}
	if article.Link != "https://rss.example/posts/atom" || article.Image != "https://rss.example/covers/atom.jpg" {
		t.Fatalf("atom relative URLs were not resolved: %+v", article)
	}
	if article.PubDate != "2026-06-24T08:00:00Z" {
		t.Fatalf("atom date was not preserved: %q", article.PubDate)
	}
	if article.GUID != "tag:rss.example,2026:atom" {
		t.Fatalf("atom id was not preserved: %q", article.GUID)
	}
}

func TestRSSRuleSourceRefreshesListAndLoadsContentLazily(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var listRequests int
	var contentRequests int
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("X-Feed-Token") != "secret" {
				t.Fatalf("RSS request missing custom header: %v", req.Header)
			}
			var body string
			switch req.URL.Path {
			case "/news":
				listRequests++
				body = `<main>
					<article class="entry">
						<a class="title" href="/post/1">规则文章</a>
						<time datetime="2026-06-20T10:00:00Z"></time>
						<div class="summary"><b>规则摘要</b><script>alert(1)</script></div>
						<img data-src="/images/1.jpg">
					</article>
				</main>`
			case "/post/1":
				contentRequests++
				body = `<div class="content"><p onclick="bad()">规则正文</p><img src="/content.jpg"><script>bad()</script></div>`
			default:
				t.Fatalf("unexpected RSS request URL: %s", req.URL.String())
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

	payload := `{
		"title":"规则 RSS",
		"url":"https://rss.example/feed",
		"singleUrl":false,
		"sortUrl":"全部::/all&&新闻::/news",
		"headerMap":{"X-Feed-Token":"secret"},
		"ruleArticles":"//article[@class='entry']",
		"ruleTitle":".title|text",
		"rulePubDate":"time@datetime",
		"ruleDescription":".summary|html",
		"ruleImage":"img@data-src",
		"ruleLink":".title@href",
		"ruleContent":".content|html",
		"enabled":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rule RSS source: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var source models.RSSSource
	if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh?sortUrl=%2Fnews", nil)
	refreshReq.Header.Set("Authorization", token)
	refreshW := httptest.NewRecorder()
	router.ServeHTTP(refreshW, refreshReq)
	if refreshW.Code != http.StatusOK || !strings.Contains(refreshW.Body.String(), `"imported":1`) ||
		!strings.Contains(refreshW.Body.String(), `"sortUrl":"https://rss.example/news"`) {
		t.Fatalf("refresh rule RSS source: got %d: %s", refreshW.Code, refreshW.Body.String())
	}
	if listRequests != 1 || contentRequests != 0 {
		t.Fatalf("refresh requests list=%d content=%d, want 1/0", listRequests, contentRequests)
	}
	var article models.RSSArticle
	if err := server.db.Where("source_id = ?", source.ID).First(&article).Error; err != nil {
		t.Fatal(err)
	}
	if article.Link != "https://rss.example/post/1" || article.Image != "https://rss.example/images/1.jpg" {
		t.Fatalf("rule URLs were not resolved: %+v", article)
	}
	if article.Sort != "新闻" {
		t.Fatalf("rule article sort = %q, want 新闻", article.Sort)
	}
	if article.PubDate != "2026-06-20T10:00:00Z" {
		t.Fatalf("rule article date was not preserved: %q", article.PubDate)
	}
	if strings.Contains(article.Summary, "<script") || !strings.Contains(article.Summary, "规则摘要") {
		t.Fatalf("rule summary was not sanitized: %s", article.Summary)
	}
	if article.Content != "" {
		t.Fatalf("article content was fetched eagerly: %q", article.Content)
	}

	contentURL := "/api/rss/articles/" + strconv.FormatUint(uint64(article.ID), 10) + "/content"
	for requestIndex := 0; requestIndex < 2; requestIndex++ {
		contentReq := httptest.NewRequest(http.MethodGet, contentURL, nil)
		contentReq.Header.Set("Authorization", token)
		contentW := httptest.NewRecorder()
		router.ServeHTTP(contentW, contentReq)
		if contentW.Code != http.StatusOK {
			t.Fatalf("load rule RSS content: got %d: %s", contentW.Code, contentW.Body.String())
		}
		body := contentW.Body.String()
		if !strings.Contains(body, "规则正文") || !strings.Contains(body, "https://rss.example/content.jpg") {
			t.Fatalf("unexpected rule RSS content: %s", body)
		}
		if strings.Contains(body, "onclick") || strings.Contains(body, "<script") {
			t.Fatalf("unsafe rule RSS content: %s", body)
		}
	}
	if contentRequests != 1 {
		t.Fatalf("content requests = %d, want cached after first request", contentRequests)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/rss/articles?sourceId="+strconv.FormatUint(uint64(source.ID), 10)+"&sort="+url.QueryEscape("新闻"), nil)
	listReq.Header.Set("Authorization", token)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK || !strings.Contains(listW.Body.String(), "规则文章") {
		t.Fatalf("filtered RSS sort did not return article: %d %s", listW.Code, listW.Body.String())
	}
}

func TestRSSRuleSourceExecutesPageRequestsAndArticleOptions(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	listBodies := make([]string, 0, 2)
	var contentRequests int
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Header.Get("X-Feed") != "static" {
				t.Fatalf("RSS static header missing: %v", request.Header)
			}
			responseBody := ""
			responseRequest := request
			switch request.URL.Path {
			case "/news":
				if request.Method != http.MethodPost {
					t.Fatalf("RSS page method = %s, want POST", request.Method)
				}
				listBodies = append(listBodies, string(body))
				if request.Header.Get("X-Page") != strings.TrimPrefix(string(body), "page=") {
					t.Fatalf("RSS page option header mismatch: body=%s headers=%v", body, request.Header)
				}
				switch string(body) {
				case "page=1":
					responseBody = `
						<article class="entry">
							<a class="title" data-url='/post/1, {"method":"POST","body":"id=1","headers":{"X-Article":"one"}}'>第一页文章</a>
						</article>
					`
				case "page=2":
					responseBody = `
						<article class="entry">
							<a class="title" data-url='/post/1, {"method":"POST","body":"id=1","headers":{"X-Article":"one"}}'>第一页文章重复</a>
						</article>
						<article class="entry">
							<a class="title" data-url="/post/2">第二页文章</a>
						</article>
					`
				default:
					t.Fatalf("unexpected RSS page body: %s", body)
				}
			case "/post/1":
				contentRequests++
				if request.Method != http.MethodPost || string(body) != "id=1" ||
					request.Header.Get("X-Article") != "one" || request.Header.Get("X-Page") != "" {
					t.Fatalf("RSS article options missing or leaked: %s body=%s headers=%v", request.Method, body, request.Header)
				}
				responseBody = `<div class="content">分页源正文<img src="./image.jpg"></div>`
				redirectedURL, err := url.Parse("https://cdn.rss.example/articles/1")
				if err != nil {
					t.Fatal(err)
				}
				responseRequest = request.Clone(request.Context())
				responseRequest.URL = redirectedURL
			default:
				t.Fatalf("unexpected RSS request: %s", request.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
				Request:    responseRequest,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.RSSSource{
		UserID:          1,
		Title:           "分页规则 RSS",
		URL:             `https://rss.example/news, {"method":"POST","body":"page=<1,2>","headers":{"X-Page":"<1,2>"}}`,
		Header:          `{"X-Feed":"static"}`,
		RuleArticles:    ".entry",
		RuleNextPage:    "PAGE",
		RuleTitle:       ".title",
		RuleLink:        ".title@data-url",
		RuleContent:     ".content|html",
		LoadWithBaseURL: true,
		Enabled:         true,
	}
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source.UserID = user.ID
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10)+"/refresh", nil)
	refreshReq.Header.Set("Authorization", token)
	refreshW := httptest.NewRecorder()
	router.ServeHTTP(refreshW, refreshReq)
	if refreshW.Code != http.StatusOK ||
		!strings.Contains(refreshW.Body.String(), `"pages":2`) ||
		!strings.Contains(refreshW.Body.String(), `"total":2`) {
		t.Fatalf("refresh paginated RSS source: got %d: %s", refreshW.Code, refreshW.Body.String())
	}
	if strings.Join(listBodies, ",") != "page=1,page=2" {
		t.Fatalf("RSS PAGE requests were not bounded by repeated request descriptor: %+v", listBodies)
	}

	var articles []models.RSSArticle
	if err := server.db.Where("source_id = ?", source.ID).Order("title asc").Find(&articles).Error; err != nil {
		t.Fatal(err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected two deduplicated RSS articles, got %+v", articles)
	}
	var first models.RSSArticle
	for _, article := range articles {
		if article.Title == "第一页文章" {
			first = article
			break
		}
	}
	expectedLink := `https://rss.example/post/1, {"method":"POST","body":"id=1","headers":{"X-Article":"one"}}`
	if first.ID == 0 || first.Link != expectedLink {
		t.Fatalf("RSS article request options were not persisted: %+v", first)
	}

	contentReq := httptest.NewRequest(http.MethodGet, "/api/rss/articles/"+strconv.FormatUint(uint64(first.ID), 10)+"/content", nil)
	contentReq.Header.Set("Authorization", token)
	contentW := httptest.NewRecorder()
	router.ServeHTTP(contentW, contentReq)
	if contentW.Code != http.StatusOK ||
		!strings.Contains(contentW.Body.String(), "分页源正文") ||
		!strings.Contains(contentW.Body.String(), "https://cdn.rss.example/articles/image.jpg") {
		t.Fatalf("load RSS POST article content: got %d: %s", contentW.Code, contentW.Body.String())
	}
	if contentRequests != 1 {
		t.Fatalf("RSS article content requests = %d, want 1", contentRequests)
	}
}

func TestFetchRSSRuleArticlesFollowsNextLinksWithoutLoops(t *testing.T) {
	requested := make([]string, 0, 2)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = append(requested, request.URL.Path)
			body := ""
			switch request.URL.Path {
			case "/list/1":
				body = `<article><a href="/post/1">第一篇</a></article><a class="next" href="/list/2">下一页</a>`
			case "/list/2":
				body = `<article><a href="/post/2">第二篇</a></article><a class="next" href="/list/1">循环</a>`
			default:
				t.Fatalf("unexpected RSS next-page request: %s", request.URL)
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

	source := models.RSSSource{
		URL:          "https://rss.example/list/1",
		RuleArticles: "article",
		RuleNextPage: ".next@href",
		RuleTitle:    "a",
		RuleLink:     "a@href",
	}
	articles, pages, err := fetchRSSArticlesContext(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(articles) != 2 ||
		strings.Join(requested, ",") != "/list/1,/list/2" {
		t.Fatalf("unexpected RSS next-page result: pages=%d articles=%+v requested=%v", pages, articles, requested)
	}
}

func TestFetchRSSRuleArticlesUsesSourceURLForRelativeArticleLinks(t *testing.T) {
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() != "https://cdn.rss.example/categories/tech/page.html" {
				t.Fatalf("unexpected RSS category request: %s", request.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`<article><a href="../post/1">文章</a><img src="../cover.jpg"></article>`,
				)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.RSSSource{
		URL:          "https://rss.example/feeds/main.xml",
		RuleArticles: "article",
		RuleTitle:    "a",
		RuleImage:    "img@src",
		RuleLink:     "a@href",
	}
	articles, pages, err := fetchRSSArticlesContext(
		context.Background(),
		source,
		"https://cdn.rss.example/categories/tech/page.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 || len(articles) != 1 {
		t.Fatalf("pages=%d articles=%+v", pages, articles)
	}
	if articles[0].Link != "https://rss.example/post/1" {
		t.Fatalf("article link = %q", articles[0].Link)
	}
	if articles[0].Image != "https://cdn.rss.example/categories/cover.jpg" {
		t.Fatalf("article image = %q", articles[0].Image)
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
	if !source.SingleURL {
		t.Fatalf("new RSS source should keep upstream editor default singleUrl=true: %+v", source)
	}
}

func TestRSSSourcePreservesUpstreamFieldsAndOrder(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	first := `{"sourceName":"后导入","sourceUrl":"https://rss.example/late.xml","sourceIcon":"https://rss.example/late.png","sourceGroup":"新闻","sourceComment":"源注释","customOrder":20,"concurrentRate":"2/1000","headerMap":{"Referer":"https://rss.example/"},"loginUrl":"https://rss.example/login","loginCheckJs":"return true","singleUrl":false,"articleStyle":1,"sortUrl":"分类::https://rss.example/cat.xml","ruleArticles":"//item","ruleNextPage":"next@href","ruleTitle":"title","rulePubDate":"pubDate","ruleDescription":"description|html","ruleImage":"image","ruleLink":"link","ruleContent":"content","style":"article{}","enableJs":false,"loadWithBaseUrl":false,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(first))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create upstream rss source: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var source models.RSSSource
	if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	if source.Title != "后导入" || source.URL != "https://rss.example/late.xml" || source.Icon != "https://rss.example/late.png" || source.Group != "新闻" || source.Comment != "源注释" || source.CustomOrder != 20 {
		t.Fatalf("upstream rss fields were not preserved: %+v", source)
	}
	if source.ConcurrentRate != "2/1000" || !strings.Contains(source.Header, `"Referer":"https://rss.example/"`) || source.LoginURL != "https://rss.example/login" || source.LoginCheckJS != "return true" {
		t.Fatalf("upstream RSS transport fields were not preserved: %+v", source)
	}
	if source.SingleURL || source.ArticleStyle != 1 || source.SortURL != "分类::https://rss.example/cat.xml" || source.RuleArticles != "//item" || source.RuleNextPage != "next@href" || source.RuleTitle != "title" || source.RulePubDate != "pubDate" || source.RuleDescription != "description|html" || source.RuleImage != "image" || source.RuleLink != "link" || source.RuleContent != "content" || source.Style != "article{}" || source.EnableJS || source.LoadWithBaseURL {
		t.Fatalf("upstream advanced rss fields were not preserved: %+v", source)
	}

	second := `{"title":"先显示","url":"https://rss.example/early.xml","icon":"https://rss.example/early.png","group":"技术","customOrder":1,"enabled":true}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(second))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("create current rss source: expected 201, got %d: %s", w2.Code, w2.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/rss/sources", nil)
	req3.Header.Set("Authorization", token)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("list rss sources: expected 200, got %d: %s", w3.Code, w3.Body.String())
	}
	var sources []models.RSSSource
	if err := json.Unmarshal(w3.Body.Bytes(), &sources); err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].Title != "先显示" || sources[1].Title != "后导入" {
		t.Fatalf("expected sources ordered by customOrder, got %+v", sources)
	}
}

func TestBackupExportsRSSSources(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "rss-backup", PasswordHash: "hash", LastActiveAt: time.Now()}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{
		UserID:          user.ID,
		Title:           "备份 RSS",
		URL:             "https://rss.example/backup.xml",
		Icon:            "https://rss.example/backup.png",
		Group:           "资讯",
		Comment:         "备份注释",
		CustomOrder:     4,
		ConcurrentRate:  "2/1000",
		Header:          `{"X-Backup":"yes"}`,
		LoginURL:        "https://rss.example/login",
		LoginCheckJS:    "check()",
		SingleURL:       false,
		ArticleStyle:    1,
		SortURL:         "分类::https://rss.example/backup-cat.xml",
		RuleImage:       "cover",
		RuleContent:     "content",
		Style:           "body{}",
		EnableJS:        false,
		LoadWithBaseURL: false,
		Enabled:         true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	backupDir := t.TempDir()
	backupSvc := backup.New(server.db, backupDir)
	backupPath, err := backupSvc.RunNow()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	var found bool
	for _, file := range reader.File {
		if file.Name != "rssSources.json" {
			continue
		}
		found = true
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{`"sourceName": "备份 RSS"`, `"sourceUrl": "https://rss.example/backup.xml"`, `"sourceIcon": "https://rss.example/backup.png"`, `"sourceGroup": "资讯"`, `"sourceComment": "备份注释"`, `"comment": "备份注释"`, `"concurrentRate": "2/1000"`, `"header": "{\"X-Backup\":\"yes\"}"`, `"loginUrl": "https://rss.example/login"`, `"loginCheckJs": "check()"`, `"singleUrl": false`, `"sortUrl": "分类::https://rss.example/backup-cat.xml"`, `"ruleImage": "cover"`, `"ruleContent": "content"`, `"style": "body{}"`, `"loadWithBaseUrl": false`} {
			if !strings.Contains(string(data), expected) {
				t.Fatalf("expected %s in rssSources.json, got %s", expected, string(data))
			}
		}
	}
	if !found {
		t.Fatalf("expected rssSources.json in backup")
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

func TestUploadCoverRejectsUnsupportedImageFormat(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", "cover"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "cover.webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("webp-data")); err != nil {
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
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unsupported file type") {
		t.Fatalf("upload cover webp: expected unsupported file type, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUploadFontAssetStoresPublicFontFile(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", "font"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "reader.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("ttf-data")); err != nil {
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
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"/uploads/fonts/`) {
		t.Fatalf("upload font asset: expected public font URL, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(resp.URL, "/uploads/fonts/")
	if _, err := os.Stat(filepath.Join(server.cfg.DataDir, "uploads", "fonts", name)); err != nil {
		t.Fatalf("uploaded font file missing: %v", err)
	}
}

func TestDeleteUploadAssetRemovesOnlyUploads(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	uploadsDir := filepath.Join(server.cfg.DataDir, "uploads", "fonts")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fontPath := filepath.Join(uploadsDir, "reader.ttf")
	if err := os.WriteFile(fontPath, []byte("ttf-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/uploads", strings.NewReader(`{"url":"/uploads/fonts/reader.ttf"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete upload asset: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(fontPath); !os.IsNotExist(err) {
		t.Fatalf("expected uploaded font to be removed, stat err=%v", err)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/uploads", strings.NewReader(`{"url":"/uploads/../openreader.db"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete upload traversal: expected 400, got %d: %s", w.Code, w.Body.String())
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

func TestRSSArticlesSupportPagination(t *testing.T) {
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
	for i := 0; i < 3; i++ {
		article := models.RSSArticle{
			UserID:      user.ID,
			SourceID:    source.ID,
			Title:       fmt.Sprintf("分页文章%d", i+1),
			Link:        fmt.Sprintf("https://rss.example/%d", i+1),
			PublishedAt: time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC),
		}
		if err := server.db.Create(&article).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/rss/articles?page=1&limit=2", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"hasMore":true`) || !strings.Contains(w.Body.String(), "分页文章3") {
		t.Fatalf("rss page 1 should include newest rows and hasMore=true, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "分页文章1") {
		t.Fatalf("rss page 1 should respect limit, got %s", w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/rss/articles?page=2&limit=2", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), `"hasMore":false`) || !strings.Contains(w2.Body.String(), "分页文章1") {
		t.Fatalf("rss page 2 should include remaining row, got %d: %s", w2.Code, w2.Body.String())
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
					<div class="book"><a class="link" href="/book"><span class="title">探索书</span></a><span class="author">作者</span><span class="updated">今日更新</span></div>
				</body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "探索源", BaseURL: "https://explore.example", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		ExploreURL:                "https://explore.example/top",
		BookListRule:              ".search-only",
		BookNameRule:              ".search-title|text",
		BookURLRule:               ".search-link|attr:href",
		ExploreBookListRule:       ".book",
		ExploreBookNameRule:       ".title|text",
		ExploreBookAuthorRule:     ".author|text",
		ExploreBookUpdateTimeRule: ".updated|text",
		ExploreBookURLRule:        ".link|attr:href",
		ExplorePaginationRule:     ".explore-next|attr:href",
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
	if w2.Code != http.StatusOK ||
		!strings.Contains(w2.Body.String(), "探索书") ||
		!strings.Contains(w2.Body.String(), "今日更新") ||
		!strings.Contains(w2.Body.String(), "https://explore.example/book") {
		t.Fatalf("explore books: expected result, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestEnabledExploreDisablesOnlyDiscovery(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<div class="book"><a class="link" href="/book"><span class="title">仍可搜索</span></a></div>
				</body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{
		Name:           "关闭发现源",
		BaseURL:        "https://explore-disabled.example",
		Charset:        "utf-8",
		Enabled:        true,
		EnabledExplore: boolPointer(false),
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "https://explore-disabled.example/search?key={keyword}",
		ExploreURL:   "https://explore-disabled.example/top",
		BookListRule: ".book",
		BookNameRule: ".title|text",
		BookURLRule:  ".link|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Select("*").Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/explore/sources", nil)
	listReq.Header.Set("Authorization", token)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK || strings.Contains(listW.Body.String(), "关闭发现源") {
		t.Fatalf("disabled explore source leaked into discovery: %d %s", listW.Code, listW.Body.String())
	}

	exploreReq := httptest.NewRequest(http.MethodGet, "/api/explore/"+strconv.FormatUint(uint64(source.ID), 10), nil)
	exploreReq.Header.Set("Authorization", token)
	exploreW := httptest.NewRecorder()
	router.ServeHTTP(exploreW, exploreReq)
	if exploreW.Code != http.StatusNotFound {
		t.Fatalf("disabled explore source remained executable: %d %s", exploreW.Code, exploreW.Body.String())
	}

	searchReq := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"keyword":"测试","sourceIds":[`+strconv.FormatUint(uint64(source.ID), 10)+`]}`))
	searchReq.Header.Set("Authorization", token)
	searchReq.Header.Set("Content-Type", "application/json")
	searchW := httptest.NewRecorder()
	router.ServeHTTP(searchW, searchReq)
	if searchW.Code != http.StatusOK || !strings.Contains(searchW.Body.String(), "仍可搜索") {
		t.Fatalf("enabled source stopped searching when only discovery was disabled: %d %s", searchW.Code, searchW.Body.String())
	}
}

func TestExploreSourcesExposeExploreGroups(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	source := models.BookSource{Name: "分组探索源", BaseURL: "https://explore.example", Charset: "utf-8", Enabled: true, Group: "玄幻"}
	if err := source.SetRules(models.BookSourceRule{
		ExploreURL: "热门::https://explore.example/top/{page}\n完本::https://explore.example/done/{page}\n\n新书::https://explore.example/new/{page}",
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
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `"exploreGroups"`) || !strings.Contains(body, `"热门"`) || !strings.Contains(body, `"新书"`) {
		t.Fatalf("explore sources: expected parsed groups, got %d: %s", w.Code, body)
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

func TestExploreBooksUsesSelectedExploreURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var requested string

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requested = req.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<div class="book"><a class="link" href="/book-category"><span class="title">分类书</span></a></div>
				</body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "分类探索源", BaseURL: "https://explore.example", Charset: "utf-8", Enabled: true}
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

	selected := url.QueryEscape("https://explore.example/category/{page}")
	req := httptest.NewRequest(http.MethodGet, "/api/explore/"+strconv.FormatUint(uint64(source.ID), 10)+"?page=3&url="+selected, nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "分类书") {
		t.Fatalf("explore selected url: expected result, got %d: %s", w.Code, w.Body.String())
	}
	if requested != "https://explore.example/category/3" {
		t.Fatalf("expected selected explore URL, got %q", requested)
	}
}

func TestImportFromWebDAVImportsBook(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	categoryA := models.Category{UserID: user.ID, Name: "WebDAV分组A"}
	categoryB := models.Category{UserID: user.ID, Name: "WebDAV分组B"}
	if err := server.db.Create(&categoryA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&categoryB).Error; err != nil {
		t.Fatal(err)
	}

	webdavDir := filepath.Join(server.cfg.DataDir, "webdav", "books")
	if err := os.MkdirAll(webdavDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webdavDir, "webdav-book.txt"), []byte("第一章 开始\n正文内容"), 0o644); err != nil {
		t.Fatal(err)
	}

	previewReq := httptest.NewRequest(http.MethodPost, "/api/webdav/import-preview", strings.NewReader(`{"paths":["books/webdav-book.txt"]}`))
	previewReq.Header.Set("Content-Type", "application/json")
	previewReq.Header.Set("Authorization", token)
	previewW := httptest.NewRecorder()
	router.ServeHTTP(previewW, previewReq)
	if previewW.Code != http.StatusOK || !strings.Contains(previewW.Body.String(), `"chapterCount":1`) {
		t.Fatalf("webdav import preview: expected parsed book, got %d: %s", previewW.Code, previewW.Body.String())
	}

	body := fmt.Sprintf(`{"items":[{"path":"books/webdav-book.txt","title":"WebDAV自定义书名"}],"categoryIds":[%d,%d]}`, categoryA.ID, categoryB.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/webdav/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("import webdav: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Imported []struct {
			Path string `json:"path"`
			Book *struct {
				ID           uint      `json:"id"`
				CategoryIDs  []uint    `json:"categoryIds"`
				ShelfOrderAt time.Time `json:"shelfOrderAt"`
			} `json:"book"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Imported) != 1 || payload.Imported[0].Book == nil || payload.Imported[0].Book.ShelfOrderAt.IsZero() {
		t.Fatalf("expected imported shelf item in response, got %+v", payload.Imported)
	}
	if !sameUintSet(payload.Imported[0].Book.CategoryIDs, []uint{categoryA.ID, categoryB.ID}) {
		t.Fatalf("expected webdav import in both categories, got %+v", payload.Imported[0].Book.CategoryIDs)
	}

	var book models.Book
	if err := server.db.Where("title = ?", "WebDAV自定义书名").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.ChapterCount == 0 {
		t.Fatalf("expected imported chapters, got %+v", book)
	}
	if ids := server.bookCategoryIDs(user.ID, book); !sameUintSet(ids, []uint{categoryA.ID, categoryB.ID}) {
		t.Fatalf("expected webdav book in both categories, got %+v", ids)
	}
}

func TestImportFromWebDAVImportsDirectoryRecursively(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	webdavDir := filepath.Join(server.cfg.DataDir, "webdav", "books", "nested")
	if err := os.MkdirAll(webdavDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.DataDir, "webdav", "books", "root.txt"), []byte("第一章 开始\n正文内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webdavDir, "child.txt"), []byte("第一章 开始\n正文内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webdavDir, "ignore.bin"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webdav/import", strings.NewReader(`{"paths":["books"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("import webdav directory: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload struct {
		Imported []struct {
			Path string `json:"path"`
			Book *struct {
				ID           uint      `json:"id"`
				ShelfOrderAt time.Time `json:"shelfOrderAt"`
			} `json:"book"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Imported) != 2 {
		t.Fatalf("expected directory import to include nested files, got %+v", payload.Imported)
	}
	wantPaths := []string{"books/nested/child.txt", "books/root.txt"}
	for i, want := range wantPaths {
		if payload.Imported[i].Path != want {
			t.Fatalf("expected stable webdav import order %v, got %+v", wantPaths, payload.Imported)
		}
	}
	for _, item := range payload.Imported {
		if item.Book == nil || item.Book.ID == 0 || item.Book.ShelfOrderAt.IsZero() {
			t.Fatalf("expected directory import shelf items, got %+v", payload.Imported)
		}
	}
}

func sameUintSet(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[uint]int, len(a))
	for _, value := range a {
		counts[value]++
	}
	for _, value := range b {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}
