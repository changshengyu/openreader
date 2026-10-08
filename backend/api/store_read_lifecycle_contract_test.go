package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreReadCancelledAfterRootInitializationReturnsNoSuccessfulData(t *testing.T) {
	for _, item := range []struct{ action, method, url string }{
		{"dav-get", "GET", "/reader3/webdav/nested/original.txt"},
		{"dav-get", "GET", "/webdav/nested/original.txt"},
		{"dav-propfind", "PROPFIND", "/reader3/webdav/nested"},
		{"dav-propfind", "PROPFIND", "/webdav/nested"},
		{"dav-list", "GET", "/webdav/nested"},
		{"local-list", "GET", "/api/local-store?path=nested"},
		{"local-list", "GET", "/api/local-store?path=nested&recursive=1"},
		{"local-download", "GET", "/api/local-store/download?path=nested/original.txt"},
	} {
		t.Run(item.url, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			root := server.webdavDir()
			if strings.HasPrefix(item.action, "local-") {
				root = server.cfg.LocalStoreDir
			} else if strings.HasPrefix(item.url, "/reader3/") {
				auth = webDAVBasic("testuser", "test1234")
			}
			if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			final := filepath.Join(root, "nested", "original.txt")
			if err := os.WriteFile(final, []byte("original"), 0o640); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fired := false
			beforeStoreReadTestHook = func(action string) {
				if action == item.action && !fired {
					fired = true
					cancel()
				}
			}
			t.Cleanup(func() { beforeStoreReadTestHook = nil })
			request := httptest.NewRequest(item.method, item.url, nil).WithContext(ctx)
			request.Header.Set("Authorization", auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !fired || ctx.Err() == nil {
				t.Fatalf("authorized read boundary did not fire: fired=%v err=%v", fired, ctx.Err())
			}
			var body map[string]json.RawMessage
			_ = json.Unmarshal(response.Body.Bytes(), &body)
			_, hasItems := body["items"]
			if response.Code == 207 || hasItems || strings.Contains(response.Body.String(), "original") {
				t.Errorf("cancelled read still returned success data: status=%d body=%q", response.Code, response.Body.String())
			}
			data, err := os.ReadFile(final)
			if err != nil || string(data) != "original" {
				t.Fatalf("read changed fixture: bytes=%q err=%v", data, err)
			}
		})
	}
}

func TestStoreReadNormalRangeAndConditionalRemainCompatible(t *testing.T) {
	router, server := setupTestServer(t)
	admin := authHeader(t, router)
	member := registerStorageTestUser(t, router, "readmember")
	for _, user := range []struct{ auth, name string }{{admin, "testuser"}, {member, "readmember"}} {
		for _, root := range []string{server.webdavDir(), server.cfg.LocalStoreDir} {
			if user.name == "readmember" {
				root = filepath.Join(root, "users", user.name)
			}
			if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "nested", "original.txt"), []byte("0123456789"), 0o640); err != nil {
				t.Fatal(err)
			}
		}
		for _, url := range []string{"/reader3/webdav/nested/original.txt", "/webdav/nested/original.txt", "/api/local-store/download?path=nested/original.txt"} {
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request.Header.Set("Authorization", user.auth)
			request.Header.Set("Range", "bytes=2-4")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 206 || response.Body.String() != "234" || response.Header().Get("Content-Range") != "bytes 2-4/10" {
				t.Fatalf("normal Range regressed: url=%s status=%d body=%q headers=%v", url, response.Code, response.Body.String(), response.Header())
			}
			modified := response.Header().Get("Last-Modified")
			if modified == "" {
				t.Fatal("missing Last-Modified")
			}
			request = httptest.NewRequest(http.MethodGet, url, nil)
			request.Header.Set("Authorization", user.auth)
			request.Header.Set("If-Modified-Since", modified)
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 304 || response.Body.Len() != 0 {
				t.Fatalf("conditional read regressed: %d %q", response.Code, response.Body.String())
			}
			request = httptest.NewRequest(http.MethodGet, url, nil)
			request.Header.Set("Authorization", user.auth)
			request.Header.Set("Range", "bytes=20-30")
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 416 {
				t.Fatalf("invalid range regressed: %d %q", response.Code, response.Body.String())
			}
		}
	}
}
