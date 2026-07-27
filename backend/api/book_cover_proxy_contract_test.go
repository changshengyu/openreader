package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestBookCoverResponseProjectionPreservesRawURL(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	source := models.BookSource{Name: "封面投影书源", BaseURL: "https://books.example", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	rawURL := "https://cover.example/cover.png?credential=keep-server-side"
	book := models.Book{
		UserID:   user.ID,
		SourceID: source.ID,
		Title:    "封面投影书",
		URL:      "https://books.example/book/1",
		CoverURL: rawURL,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list books status=%d body=%s", response.Code, response.Body.String())
	}
	var items []struct {
		CoverURL         string `json:"coverUrl"`
		CoverResourceURL string `json:"coverResourceUrl"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CoverURL != rawURL ||
		!strings.HasPrefix(items[0].CoverResourceURL, "/api/cover/") ||
		strings.Contains(items[0].CoverResourceURL, "credential") {
		t.Fatalf("unexpected book cover projection: %+v", items)
	}

	var persisted models.Book
	if err := server.db.First(&persisted, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.CoverURL != rawURL {
		t.Fatalf("projection polluted persisted cover: %q", persisted.CoverURL)
	}

	results := []engine.SearchResult{{Title: "搜索书", SourceID: source.ID, CoverURL: rawURL}}
	projected := server.projectSearchResultCovers(user.ID, results)
	if projected[0].CoverURL != rawURL ||
		projected[0].CoverResourceURL == nil ||
		!strings.HasPrefix(*projected[0].CoverResourceURL, "/api/cover/") {
		t.Fatalf("search projection=%+v", projected[0])
	}
	if results[0].CoverResourceURL != nil {
		t.Fatalf("projection mutated parser result: %+v", results[0])
	}

	unsafe := server.projectSearchResultCovers(user.ID, []engine.SearchResult{{
		Title:    "不安全封面",
		CoverURL: "http://127.0.0.1/private.png",
	}})
	if unsafe[0].CoverResourceURL == nil || *unsafe[0].CoverResourceURL != "" {
		t.Fatalf("unsafe remote cover must be marked handled without a browser fallback: %+v", unsafe[0])
	}
}

func TestBookCoverCapabilityHandlerRejectsInvalidTokenWithoutAuth(t *testing.T) {
	router, _ := setupTestServer(t)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/cover/not-a-capability", nil))
		if response.Code != http.StatusForbidden ||
			strings.Contains(response.Body.String(), "not-a-capability") {
			t.Fatalf("%s invalid capability status=%d body=%q", method, response.Code, response.Body.String())
		}
		if method == http.MethodHead && response.Body.Len() != 0 {
			t.Fatalf("HEAD error returned body=%q", response.Body.String())
		}
	}
}

func TestBookCoverCapabilityHandlerReturnsNotFoundAfterOwnerDeletion(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	resourceURL, err := server.coverImages.Project(user.ID, 0, "https://cover.example/deleted-owner.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.db.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, resourceURL, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s deleted-owner status=%d body=%q", method, response.Code, response.Body.String())
		}
		if method == http.MethodHead && response.Body.Len() != 0 {
			t.Fatalf("HEAD unavailable cover returned body=%q", response.Body.String())
		}
	}
}

func TestBookCoverCapabilityHandlerServesVerifiedCacheWithoutAuth(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	rawURL := "https://cover.example/cached.png?credential=hidden"
	resourceURL, err := server.coverImages.Project(user.ID, 0, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(normalized.String()))
	cachePath := filepath.Join(
		server.cfg.CacheDir,
		"cover-images",
		"user-"+strconv.FormatUint(uint64(user.ID), 10),
		hex.EncodeToString(sum[:])+".img",
	)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	png := chapterImagePNG(t)
	if err := os.WriteFile(cachePath, png, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, resourceURL, nil))
		if response.Code != http.StatusOK ||
			response.Header().Get("Content-Type") != "image/png" ||
			response.Header().Get("Content-Length") != strconv.Itoa(len(png)) ||
			response.Header().Get("Cache-Control") != "private, max-age=86400" ||
			response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s status=%d headers=%v body=%q", method, response.Code, response.Header(), response.Body.String())
		}
		if method == http.MethodGet && !bytes.Equal(response.Body.Bytes(), png) {
			t.Fatal("GET cover bytes changed")
		}
		if method == http.MethodHead && response.Body.Len() != 0 {
			t.Fatalf("HEAD cover returned body=%q", response.Body.String())
		}
	}
}
