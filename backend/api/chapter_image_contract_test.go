package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func chapterImagePNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExistingTextCacheHydratesChapterImagesAndServesCapability(t *testing.T) {
	var chapterRequests atomic.Int32
	var imageRequests atomic.Int32
	png := chapterImagePNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/chapter/1":
			chapterRequests.Add(1)
			_, _ = io.WriteString(w, `<main class="content">must not refetch</main>`)
		case "/assets/illustration":
			imageRequests.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			http.NotFound(w, request)
		}
	}))
	defer upstream.Close()

	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	source := createCacheStreamSource(t, server, upstream.URL)
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "章节图片契约书", URL: upstream.URL + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	originalImageURL := upstream.URL + "/assets/illustration?private=query"
	originalContent := `<p>开头</p><img src="` + originalImageURL + `" alt="插图" data-image-style="FULL"><p>结尾</p>`
	cachePath := filepath.Join("chapter-image-existing", "chapter-1.txt")
	fullCachePath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullCachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullCachePath, []byte(originalContent), 0o600); err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: upstream.URL + "/chapter/1", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	result, err := server.cacheBookChapters(context.Background(), book, nil, true, 1, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedCached != 1 || result.SuccessCount != 0 || result.FailedCount != 0 || result.CachedCount != 1 {
		t.Fatalf("existing cache counters changed while hydrating images: %+v", result)
	}
	if chapterRequests.Load() != 0 || imageRequests.Load() != 1 {
		t.Fatalf("requests chapter=%d image=%d, want 0/1", chapterRequests.Load(), imageRequests.Load())
	}

	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("chapter content status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Content               string            `json:"content"`
		CachedImages          map[string]string `json:"cachedImages"`
		CachedImagesExpiresAt string            `json:"cachedImagesExpiresAt"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Content != originalContent || strings.Contains(payload.Content, "/api/chapter-image/") {
		t.Fatalf("chapter content was rewritten and would change positions: %q", payload.Content)
	}
	capabilityURL := payload.CachedImages[originalImageURL]
	if !strings.HasPrefix(capabilityURL, "/api/chapter-image/") || payload.CachedImagesExpiresAt == "" {
		t.Fatalf("chapter image mapping=%v expires=%q", payload.CachedImages, payload.CachedImagesExpiresAt)
	}
	if strings.Contains(capabilityURL, "private=query") {
		t.Fatalf("capability leaked source query: %s", capabilityURL)
	}
	cacheBytes, err := os.ReadFile(fullCachePath)
	if err != nil || string(cacheBytes) != originalContent || strings.Contains(string(cacheBytes), "/api/chapter-image/") {
		t.Fatalf("persisted chapter text changed: %q err=%v", cacheBytes, err)
	}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		resourceRequest := httptest.NewRequest(method, capabilityURL, nil)
		resourceResponse := httptest.NewRecorder()
		router.ServeHTTP(resourceResponse, resourceRequest)
		if resourceResponse.Code != http.StatusOK || resourceResponse.Header().Get("Content-Type") != "image/png" ||
			resourceResponse.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s capability status=%d headers=%v body=%q", method, resourceResponse.Code, resourceResponse.Header(), resourceResponse.Body.String())
		}
		if method == http.MethodGet && !bytes.Equal(resourceResponse.Body.Bytes(), png) {
			t.Fatalf("GET capability bytes changed")
		}
		if method == http.MethodHead && resourceResponse.Body.Len() != 0 {
			t.Fatalf("HEAD capability returned a body")
		}
	}

	tampered := capabilityURL[:len(capabilityURL)-1] + "x"
	tamperResponse := httptest.NewRecorder()
	router.ServeHTTP(tamperResponse, httptest.NewRequest(http.MethodGet, tampered, nil))
	if tamperResponse.Code != http.StatusForbidden || strings.Contains(tamperResponse.Body.String(), server.cfg.CacheDir) {
		t.Fatalf("tampered capability status=%d body=%s", tamperResponse.Code, tamperResponse.Body.String())
	}

	if err := server.db.Model(&models.Book{}).Where("id = ?", book.ID).Update("user_id", user.ID+100).Error; err != nil {
		t.Fatal(err)
	}
	ownerResponse := httptest.NewRecorder()
	router.ServeHTTP(ownerResponse, httptest.NewRequest(http.MethodGet, capabilityURL, nil))
	if ownerResponse.Code != http.StatusNotFound {
		t.Fatalf("ownership-changed capability status=%d body=%s", ownerResponse.Code, ownerResponse.Body.String())
	}
	if err := server.db.Model(&models.Book{}).Where("id = ?", book.ID).Update("user_id", user.ID).Error; err != nil {
		t.Fatal(err)
	}

	statsRequest := httptest.NewRequest(http.MethodGet, "/api/cache/stats", nil)
	statsRequest.Header.Set("Authorization", auth)
	statsResponse := httptest.NewRecorder()
	router.ServeHTTP(statsResponse, statsRequest)
	var stats struct {
		Files int `json:"files"`
	}
	if statsResponse.Code != http.StatusOK || json.Unmarshal(statsResponse.Body.Bytes(), &stats) != nil || stats.Files < 3 {
		t.Fatalf("cache stats omitted image artifacts: status=%d body=%s", statsResponse.Code, statsResponse.Body.String())
	}

	clearRequest := httptest.NewRequest(http.MethodDelete, "/api/cache", nil)
	clearRequest.Header.Set("Authorization", auth)
	clearResponse := httptest.NewRecorder()
	router.ServeHTTP(clearResponse, clearRequest)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("clear cache status=%d body=%s", clearResponse.Code, clearResponse.Body.String())
	}
	removedResponse := httptest.NewRecorder()
	router.ServeHTTP(removedResponse, httptest.NewRequest(http.MethodGet, capabilityURL, nil))
	if removedResponse.Code != http.StatusNotFound {
		t.Fatalf("cleared capability status=%d body=%s", removedResponse.Code, removedResponse.Body.String())
	}
}

func TestEPUBExportEmbedsOnlyVerifiedCachedChapterImages(t *testing.T) {
	png := chapterImagePNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/image" {
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer upstream.Close()
	router, server := setupTestServer(t)
	authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	source := createCacheStreamSource(t, server, upstream.URL)
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "图片 EPUB", URL: upstream.URL + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	remoteURL := upstream.URL + "/image?credential=never-export"
	content := `正文<img src="` + remoteURL + `" alt="章图" data-image-style="FULL">`
	cachePath := filepath.Join("chapter-image-export", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: upstream.URL + "/chapter", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := server.cacheBookChapters(context.Background(), book, nil, true, 1, false, nil); err != nil {
		t.Fatal(err)
	}

	data, err := server.exportBookEPUB(book)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte)
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = body
	}
	imageName := ""
	for name, body := range files {
		if strings.HasPrefix(name, "OEBPS/Images/") {
			imageName = name
			if !bytes.Equal(body, png) {
				t.Fatalf("embedded image bytes changed")
			}
		}
	}
	if imageName == "" {
		t.Fatalf("EPUB omitted cached image: files=%v", mapKeys(files))
	}
	chapterXHTML := string(files["OEBPS/chapter-0001.xhtml"])
	opf := string(files["OEBPS/content.opf"])
	baseName := strings.TrimPrefix(imageName, "OEBPS/")
	if !strings.Contains(chapterXHTML, `src="`+baseName+`"`) || !strings.Contains(chapterXHTML, `alt="章图"`) ||
		!strings.Contains(opf, `media-type="image/png"`) || !strings.Contains(opf, baseName) {
		t.Fatalf("EPUB image references missing: image=%s xhtml=%s opf=%s", imageName, chapterXHTML, opf)
	}
	uncompressed := make([][]byte, 0, len(files))
	for _, body := range files {
		uncompressed = append(uncompressed, body)
	}
	archiveText := string(bytes.Join(uncompressed, nil))
	if strings.Contains(archiveText, "/api/chapter-image/") || strings.Contains(archiveText, "credential=never-export") || strings.Contains(archiveText, server.cfg.CacheDir) {
		t.Fatalf("EPUB leaked capability, source query, or host path")
	}
}

func TestDeletingRemoteBookRemovesOnlyItsChapterImageTree(t *testing.T) {
	png := chapterImagePNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer upstream.Close()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	source := createCacheStreamSource(t, server, upstream.URL)
	books := make([]models.Book, 2)
	chapters := make([]models.Chapter, 2)
	capabilities := make([]string, 2)
	imageURL := upstream.URL + "/shared"
	content := `<img src="` + imageURL + `">`
	for index := range books {
		books[index] = models.Book{UserID: user.ID, SourceID: source.ID, Title: "隔离书", URL: upstream.URL + "/book-" + strconv.Itoa(index)}
		if err := server.db.Create(&books[index]).Error; err != nil {
			t.Fatal(err)
		}
		chapters[index] = models.Chapter{BookID: books[index].ID, Index: 0, Title: "章节", URL: upstream.URL + "/chapter-" + strconv.Itoa(index)}
		if err := server.db.Create(&chapters[index]).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := server.chapterImages.CacheChapter(context.Background(), source, books[index], chapters[index], content); err != nil {
			t.Fatal(err)
		}
		mapping, _, err := server.chapterImages.CachedImages(books[index], chapters[index], content)
		if err != nil {
			t.Fatal(err)
		}
		capabilities[index] = mapping[imageURL]
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(books[0].ID), 10), nil)
	deleteRequest.Header.Set("Authorization", auth)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete book status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	for index, want := range []int{http.StatusNotFound, http.StatusOK} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, capabilities[index], nil))
		if response.Code != want {
			t.Fatalf("book %d capability status=%d, want %d", index, response.Code, want)
		}
	}
	stats, err := server.chapterImages.StatsUser(user.ID)
	if err != nil || stats.Files != 2 {
		t.Fatalf("delete book crossed image roots or left deleted files: stats=%+v err=%v", stats, err)
	}
}

func TestChangingSourceRemovesObsoleteChapterImageTree(t *testing.T) {
	png := chapterImagePNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/image":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "/new-book":
			_, _ = io.WriteString(w, `<html><body><h1 class="name">新书源书</h1><div class="chapter"><span class="title">新章</span><a href="/new-chapter">读</a></div></body></html>`)
		default:
			http.NotFound(w, request)
		}
	}))
	defer upstream.Close()
	restoreHTTPClient := engine.SetHTTPClient(upstream.Client())
	defer restoreHTTPClient()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	oldSource := createCacheStreamSource(t, server, upstream.URL)
	newSource := models.BookSource{Name: "新图片书源", BaseURL: upstream.URL, Charset: "utf-8", Enabled: true}
	if err := newSource.SetRules(models.BookSourceRule{
		BookInfoNameRule: ".name",
		ChapterListRule:  ".chapter",
		ChapterNameRule:  ".title|text",
		ChapterURLRule:   "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&newSource).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: oldSource.ID, Title: "旧图片书", URL: upstream.URL + "/old-book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧章", URL: upstream.URL + "/old-chapter"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	imageURL := upstream.URL + "/image"
	content := `<img src="` + imageURL + `">`
	if _, err := server.chapterImages.CacheChapter(context.Background(), oldSource, book, chapter, content); err != nil {
		t.Fatal(err)
	}

	body := `{"sourceId":` + strconv.FormatUint(uint64(newSource.ID), 10) + `,"bookUrl":` + strconv.Quote(upstream.URL+"/new-book") + `}`
	request := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/change-source", strings.NewReader(body))
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("change source status=%d body=%s", response.Code, response.Body.String())
	}
	stats, err := server.chapterImages.StatsUser(user.ID)
	if err != nil || stats.Files != 0 || stats.Bytes != 0 {
		t.Fatalf("source change retained obsolete image tree: stats=%+v err=%v", stats, err)
	}
}

func mapKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
