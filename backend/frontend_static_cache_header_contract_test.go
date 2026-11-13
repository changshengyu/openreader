package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// PWA service worker contract W15 (docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md):
// the SPA shell and every non-hashed public file revalidate on each use, Vite's
// hashed build output is immutable, and the service worker script itself is never
// served from a stale HTTP cache.
const (
	frontendRevalidateCacheControl = "no-cache"
	frontendImmutableCacheControl  = "public, max-age=31536000, immutable"
	testFrontendServiceWorker      = "self.addEventListener('fetch', () => {})"
	testFrontendHashedEntry        = "export const contract = 'hashed entry'"
	testFrontendHashedStyle        = ".reader-shell{}"
)

func writeFrontendCacheHeaderFixture(t *testing.T) string {
	t.Helper()
	root := writeFrontendBoundaryFixture(t)
	for path, body := range map[string]string{
		filepath.Join(root, "sw.js"):                         testFrontendServiceWorker,
		filepath.Join(root, "assets", "index-B7xQk2aZ.js"):   testFrontendHashedEntry,
		filepath.Join(root, "assets", "Reader-yPmDiT1v.css"): testFrontendHashedStyle,
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func assertFrontendCacheControl(t *testing.T, router http.Handler, method, target, want string) {
	t.Helper()
	response := performFrontendBoundaryRequest(router, method, target)
	if response.Code != http.StatusOK {
		t.Fatalf("%s %s = %d %q, want 200", method, target, response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != want {
		t.Fatalf("%s %s Cache-Control = %q, want %q", method, target, got, want)
	}
}

func TestFrontendShellDocumentRevalidatesOnEveryNavigation(t *testing.T) {
	router := newFrontendBoundaryRouter(t, writeFrontendCacheHeaderFixture(t))
	for _, target := range []string{
		"/",
		"/login",
		"/books/42",
		"/books/42/read?chapter=3",
		"/reader/remote/session-token",
		"/settings?panel=reader",
		"/index.html",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			assertFrontendCacheControl(t, router, method, target, frontendRevalidateCacheControl)
		}
	}

	headers := http.Header{"If-Modified-Since": []string{time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)}}
	revalidated := performFrontendBoundaryRequestWithHeaders(router, http.MethodGet, "/books/42/read", headers)
	if revalidated.Code != http.StatusNotModified {
		t.Fatalf("conditional shell GET = %d, want 304", revalidated.Code)
	}
	if got := revalidated.Header().Get("Cache-Control"); got != frontendRevalidateCacheControl {
		t.Fatalf("304 shell Cache-Control = %q, want %q", got, frontendRevalidateCacheControl)
	}
}

func TestFrontendHashedBuildAssetsAreImmutable(t *testing.T) {
	router := newFrontendBoundaryRouter(t, writeFrontendCacheHeaderFixture(t))
	for _, target := range []string{
		"/assets/index-B7xQk2aZ.js",
		"/assets/Reader-yPmDiT1v.css",
		"/assets/app.js",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			assertFrontendCacheControl(t, router, method, target, frontendImmutableCacheControl)
		}
	}
}

func TestFrontendServiceWorkerAndPublicFilesRevalidate(t *testing.T) {
	router := newFrontendBoundaryRouter(t, writeFrontendCacheHeaderFixture(t))

	worker := performFrontendBoundaryRequest(router, http.MethodGet, "/sw.js")
	if worker.Code != http.StatusOK || worker.Body.String() != testFrontendServiceWorker {
		t.Fatalf("GET /sw.js = %d %q, want the worker script", worker.Code, worker.Body.String())
	}
	if contentType := worker.Header().Get("Content-Type"); !strings.Contains(strings.ToLower(contentType), "javascript") {
		t.Fatalf("GET /sw.js Content-Type = %q, want a JavaScript MIME type", contentType)
	}
	if got := worker.Header().Get("Cache-Control"); got != frontendRevalidateCacheControl {
		t.Fatalf("GET /sw.js Cache-Control = %q, want %q", got, frontendRevalidateCacheControl)
	}

	for _, target := range []string{
		"/sw.js",
		"/manifest.webmanifest",
		"/openreader.svg",
		"/themes/content_0.png",
		"/bg/" + url.PathEscape("山水画.jpg"),
		"/nested/reader/texture.png",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			assertFrontendCacheControl(t, router, method, target, frontendRevalidateCacheControl)
		}
	}
}

// Control: the cache directives belong to served frontend files only. Route errors,
// API responses and missing build files keep their existing headers.
func TestFrontendCacheHeadersStayOffRouteErrorsAndAPI(t *testing.T) {
	router := newFrontendBoundaryRouter(t, writeFrontendCacheHeaderFixture(t))
	for _, target := range []string{
		"/no-such-page",
		"/assets/does-not-exist.js",
		"/api/does-not-exist",
		"/books/42/read/extra",
	} {
		response := performFrontendBoundaryRequest(router, http.MethodGet, target)
		assertRouteError(t, response, http.StatusNotFound, "NOT_FOUND", "route not found")
		if got := response.Header().Get("Cache-Control"); got != "" {
			t.Fatalf("GET %s route error Cache-Control = %q, want none", target, got)
		}
	}

	health := performFrontendBoundaryRequest(router, http.MethodGet, "/api/health")
	if health.Code != http.StatusOK {
		t.Fatalf("GET /api/health = %d, want 200", health.Code)
	}
	if got := health.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("GET /api/health Cache-Control = %q, want none", got)
	}
}
