package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/config"
)

const (
	testFrontendIndex    = "<!doctype html><title>OpenReader contract index</title>"
	testFrontendManifest = `{"name":"OpenReader contract"}`
	testFrontendSVG      = `<svg xmlns="http://www.w3.org/2000/svg"></svg>`
	testFrontendPNG      = "\x89PNG\r\n\x1a\nOpenReader theme contract"
	testFrontendJPEG     = "\xff\xd8\xffOpenReader background contract\xff\xd9"
)

func newFrontendBoundaryRouter(t *testing.T, publicDir string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/api/private", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/ws/sync", func(c *gin.Context) {
		c.Status(http.StatusSwitchingProtocols)
	})
	router.GET("/uploads/*resourcePath", func(c *gin.Context) {
		c.Status(http.StatusUnauthorized)
	})
	router.GET("/webdav/*path", func(c *gin.Context) {
		c.Status(http.StatusUnauthorized)
	})
	router.OPTIONS("/webdav/*path", func(c *gin.Context) {
		c.Header("DAV", "1,2")
		c.Status(http.StatusOK)
	})
	serveFrontend(router, publicDir)
	return router
}

func writeFrontendBoundaryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{
		"assets",
		"themes",
		"bg",
		filepath.Join("nested", "reader"),
		filepath.Join("api"),
		filepath.Join("books", "42"),
	} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		filepath.Join(root, "index.html"):                      testFrontendIndex,
		filepath.Join(root, "manifest.webmanifest"):            testFrontendManifest,
		filepath.Join(root, "openreader.svg"):                  testFrontendSVG,
		filepath.Join(root, "assets", "app.js"):                "globalThis.openReaderContract = true",
		filepath.Join(root, "themes", "content_0.png"):         testFrontendPNG,
		filepath.Join(root, "bg", "山水画.jpg"):                   testFrontendJPEG,
		filepath.Join(root, "nested", "reader", "texture.png"): testFrontendPNG,
		filepath.Join(root, "api", "does-not-exist"):           "must not shadow API namespace",
		filepath.Join(root, "books", "42", "read"):             "must not shadow history route",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func performFrontendBoundaryRequest(router http.Handler, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func performFrontendBoundaryRequestWithHeaders(router http.Handler, method, target string, headers http.Header) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.Header = headers.Clone()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertRouteError(t *testing.T, response *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, status, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	want := `{"error":{"code":"` + code + `","message":"` + message + `"}}`
	if strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("body = %q, want %q", response.Body.String(), want)
	}
}

func TestFrontendHistoryRoutesOnlyFallbackForGetAndHead(t *testing.T) {
	router := newFrontendBoundaryRouter(t, writeFrontendBoundaryFixture(t))
	routes := []string{
		"/",
		"/login",
		"/search?keyword=reader",
		"/discover",
		"/local-store",
		"/sources?panel=remote",
		"/source-debug",
		"/bookSourceDebug",
		"/bookSourceDebug/",
		"/settings?panel=reader",
		"/books/42",
		"/books/42/read?chapter=3",
		"/reader/remote/session-token",
	}

	for _, target := range routes {
		t.Run("GET "+target, func(t *testing.T) {
			response := performFrontendBoundaryRequest(router, http.MethodGet, target)
			if response.Code != http.StatusOK || response.Body.String() != testFrontendIndex {
				t.Fatalf("GET %s = %d %q, want index", target, response.Code, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
				t.Fatalf("GET %s Content-Type = %q, want text/html", target, contentType)
			}
		})

		t.Run("HEAD "+target, func(t *testing.T) {
			response := performFrontendBoundaryRequest(router, http.MethodHead, target)
			if response.Code != http.StatusOK {
				t.Fatalf("HEAD %s = %d, want 200", target, response.Code)
			}
			if response.Body.Len() != 0 {
				t.Fatalf("HEAD %s returned %d body bytes", target, response.Body.Len())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
				t.Fatalf("HEAD %s Content-Type = %q, want text/html", target, contentType)
			}
		})
	}

	for _, target := range []string{
		"/no-such-page",
		"/login/extra",
		"/books",
		"/books/42/read/extra",
		"/reader/remote",
		"/reader/remote/session-token/extra",
		"/api/does-not-exist",
		"/ws/does-not-exist",
	} {
		t.Run("unknown "+target, func(t *testing.T) {
			assertRouteError(t, performFrontendBoundaryRequest(router, http.MethodGet, target), http.StatusNotFound, "NOT_FOUND", "route not found")
		})
	}

	assertRouteError(t, performFrontendBoundaryRequest(router, http.MethodPost, "/books/42/read"), http.StatusNotFound, "NOT_FOUND", "route not found")
}

func TestFrontendRootFilesAndAssetsKeepFileSemantics(t *testing.T) {
	root := writeFrontendBoundaryFixture(t)
	router := newFrontendBoundaryRouter(t, root)

	for _, test := range []struct {
		path        string
		body        string
		contentType string
	}{
		{path: "/manifest.webmanifest", body: testFrontendManifest, contentType: "application/manifest+json"},
		{path: "/openreader.svg", body: testFrontendSVG, contentType: "image/svg+xml"},
		{path: "/assets/app.js", body: "globalThis.openReaderContract = true", contentType: "javascript"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := performFrontendBoundaryRequest(router, http.MethodGet, test.path)
			if response.Code != http.StatusOK || response.Body.String() != test.body {
				t.Fatalf("GET %s = %d %q, want file bytes", test.path, response.Code, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.Contains(strings.ToLower(contentType), test.contentType) {
				t.Fatalf("GET %s Content-Type = %q, want %q", test.path, contentType, test.contentType)
			}

			head := performFrontendBoundaryRequest(router, http.MethodHead, test.path)
			if head.Code != http.StatusOK || head.Body.Len() != 0 {
				t.Fatalf("HEAD %s = %d with %d body bytes", test.path, head.Code, head.Body.Len())
			}
		})
	}

	outside := filepath.Join(t.TempDir(), "outside.js")
	if err := os.WriteFile(outside, []byte("outside secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{
		filepath.Join(root, "linked.js"),
		filepath.Join(root, "assets", "linked.js"),
	} {
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlink fixture unavailable: %v", err)
		}
	}

	for _, target := range []string{
		"/manifest.webmanifest/more",
		"/directory",
		"/linked.js",
		"/assets/does-not-exist.js",
		"/assets/linked.js",
		"/assets/../index.html",
		"/manifest.webmanifest%5Cextra",
	} {
		t.Run("reject "+target, func(t *testing.T) {
			response := performFrontendBoundaryRequest(router, http.MethodGet, target)
			assertRouteError(t, response, http.StatusNotFound, "NOT_FOUND", "route not found")
			if strings.Contains(response.Body.String(), "outside secret") || strings.Contains(response.Body.String(), testFrontendIndex) {
				t.Fatalf("GET %s exposed file or SPA bytes: %q", target, response.Body.String())
			}
		})
	}
}

func TestFrontendPublicSubtreeFilesKeepStaticHTTPAndRoutePrecedence(t *testing.T) {
	root := writeFrontendBoundaryFixture(t)
	router := newFrontendBoundaryRouter(t, root)

	for _, test := range []struct {
		path        string
		body        string
		contentType string
	}{
		{path: "/themes/content_0.png?scheme=parchment", body: testFrontendPNG, contentType: "image/png"},
		{path: "/bg/山水画.jpg", body: testFrontendJPEG, contentType: "image/jpeg"},
		{path: "/bg/%E5%B1%B1%E6%B0%B4%E7%94%BB.jpg", body: testFrontendJPEG, contentType: "image/jpeg"},
		{path: "/nested/reader/texture.png", body: testFrontendPNG, contentType: "image/png"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := performFrontendBoundaryRequest(router, http.MethodGet, test.path)
			if response.Code != http.StatusOK || response.Body.String() != test.body {
				t.Fatalf("GET %s = %d %q, want static file bytes", test.path, response.Code, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(strings.ToLower(contentType), test.contentType) {
				t.Fatalf("GET %s Content-Type = %q, want %q", test.path, contentType, test.contentType)
			}

			head := performFrontendBoundaryRequest(router, http.MethodHead, test.path)
			if head.Code != http.StatusOK || head.Body.Len() != 0 {
				t.Fatalf("HEAD %s = %d with %d body bytes", test.path, head.Code, head.Body.Len())
			}
		})
	}

	ranged := performFrontendBoundaryRequestWithHeaders(router, http.MethodGet, "/themes/content_0.png", http.Header{
		"Range": []string{"bytes=0-7"},
	})
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != testFrontendPNG[:8] {
		t.Fatalf("range response = %d %q, want first 8 PNG bytes", ranged.Code, ranged.Body.String())
	}

	initial := performFrontendBoundaryRequest(router, http.MethodGet, "/themes/content_0.png")
	lastModified := initial.Header().Get("Last-Modified")
	if lastModified == "" {
		t.Fatal("static response omitted Last-Modified")
	}
	conditional := performFrontendBoundaryRequestWithHeaders(router, http.MethodGet, "/themes/content_0.png", http.Header{
		"If-Modified-Since": []string{lastModified},
	})
	if conditional.Code != http.StatusNotModified || conditional.Body.Len() != 0 {
		t.Fatalf("conditional response = %d with %d body bytes, want 304", conditional.Code, conditional.Body.Len())
	}

	if response := performFrontendBoundaryRequest(router, http.MethodGet, "/api/health"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("public collision shadowed API route: %d %q", response.Code, response.Body.String())
	}
	assertRouteError(t, performFrontendBoundaryRequest(router, http.MethodGet, "/api/does-not-exist"), http.StatusNotFound, "NOT_FOUND", "route not found")
	if response := performFrontendBoundaryRequest(router, http.MethodGet, "/uploads/example"); response.Code != http.StatusUnauthorized {
		t.Fatalf("public fallback shadowed uploads namespace: %d", response.Code)
	}
	if response := performFrontendBoundaryRequest(router, http.MethodGet, "/books/42/read"); response.Code != http.StatusOK || response.Body.String() != testFrontendIndex {
		t.Fatalf("public collision shadowed history route: %d %q", response.Code, response.Body.String())
	}
	assertRouteError(t, performFrontendBoundaryRequest(router, http.MethodPost, "/themes/content_0.png"), http.StatusNotFound, "NOT_FOUND", "route not found")
}

func TestFrontendPublicSubtreeRejectsUnsafeObjects(t *testing.T) {
	root := writeFrontendBoundaryFixture(t)
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "outside.png")
	if err := os.WriteFile(outsideFile, []byte("outside secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideTree := filepath.Join(outsideRoot, "tree")
	if err := os.Mkdir(outsideTree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideTree, "secret.png"), []byte("ancestor secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "themes", "linked.png")); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	if err := os.Symlink(outsideTree, filepath.Join(root, "linked-tree")); err != nil {
		t.Skipf("ancestor symlink fixture unavailable: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "themes", "stream.png"), 0o600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}

	router := newFrontendBoundaryRouter(t, root)
	for _, target := range []string{
		"/themes",
		"/themes/missing.png",
		"/themes/linked.png",
		"/themes/stream.png",
		"/linked-tree/secret.png",
		"/themes/../index.html",
		"/themes/content_0.png%5Cextra",
	} {
		t.Run(target, func(t *testing.T) {
			response := performFrontendBoundaryRequest(router, http.MethodGet, target)
			assertRouteError(t, response, http.StatusNotFound, "NOT_FOUND", "route not found")
			if strings.Contains(response.Body.String(), root) || strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("GET %s leaked path or outside bytes: %q", target, response.Body.String())
			}
		})
	}
}

func TestFrontendOpenedFileKeepsVerifiedIdentityAfterPathReplacement(t *testing.T) {
	root := writeFrontendBoundaryFixture(t)
	path := filepath.Join(root, "manifest.webmanifest")
	file, expected, err := openFrontendFile(root, "manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(expected, current) {
		t.Fatal("replacement unexpectedly retained the verified file identity")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != testFrontendManifest {
		t.Fatalf("opened handle read %q, want original bytes", data)
	}
}

func TestFrontendBoundaryReturnsMethodNotAllowedForRegisteredServerRoutes(t *testing.T) {
	router := newFrontendBoundaryRouter(t, writeFrontendBoundaryFixture(t))
	for _, target := range []string{
		"/api/health",
		"/api/private",
		"/ws/sync",
		"/assets/app.js",
		"/webdav/example.txt",
	} {
		t.Run(target, func(t *testing.T) {
			response := performFrontendBoundaryRequest(router, http.MethodPatch, target)
			assertRouteError(t, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			if allow := response.Header().Get("Allow"); allow == "" {
				t.Fatalf("PATCH %s omitted Allow", target)
			}
		})
	}

	options := performFrontendBoundaryRequest(router, http.MethodOptions, "/webdav/example.txt")
	if options.Code != http.StatusOK || options.Header().Get("DAV") != "1,2" {
		t.Fatalf("WebDAV OPTIONS = %d DAV=%q", options.Code, options.Header().Get("DAV"))
	}
}

func TestRouteErrorsDoNotDependOnFrontendBuildPresence(t *testing.T) {
	router := newFrontendBoundaryRouter(t, filepath.Join(t.TempDir(), "missing-public"))
	assertRouteError(t, performFrontendBoundaryRequest(router, http.MethodGet, "/api/does-not-exist"), http.StatusNotFound, "NOT_FOUND", "route not found")

	method := performFrontendBoundaryRequest(router, http.MethodPatch, "/api/health")
	assertRouteError(t, method, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	if allow := method.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Fatalf("PATCH /api/health Allow = %q, want GET", allow)
	}
}

func TestFrontendMethodHandlingPreservesCORSOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors(config.Config{CORSOrigin: "https://reader.example"}))
	router.GET("/api/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.OPTIONS("/webdav/*path", func(c *gin.Context) {
		c.Header("DAV", "1,2")
		c.Status(http.StatusOK)
	})
	serveFrontend(router, writeFrontendBoundaryFixture(t))

	ordinary := performFrontendBoundaryRequest(router, http.MethodOptions, "/api/health")
	if ordinary.Code != http.StatusNoContent {
		t.Fatalf("ordinary OPTIONS = %d, want 204", ordinary.Code)
	}
	dav := performFrontendBoundaryRequest(router, http.MethodOptions, "/webdav/example.txt")
	if dav.Code != http.StatusOK || dav.Header().Get("DAV") != "1,2" {
		t.Fatalf("WebDAV OPTIONS = %d DAV=%q", dav.Code, dav.Header().Get("DAV"))
	}
}
