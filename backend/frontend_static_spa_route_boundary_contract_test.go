package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const (
	testFrontendIndex    = "<!doctype html><title>OpenReader contract index</title>"
	testFrontendManifest = `{"name":"OpenReader contract"}`
	testFrontendSVG      = `<svg xmlns="http://www.w3.org/2000/svg"></svg>`
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
	assetsDir := filepath.Join(root, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(root, "index.html"):           testFrontendIndex,
		filepath.Join(root, "manifest.webmanifest"): testFrontendManifest,
		filepath.Join(root, "openreader.svg"):       testFrontendSVG,
		filepath.Join(assetsDir, "app.js"):          "globalThis.openReaderContract = true",
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
