package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUploadPublicResourceRejectsEntryAndAncestorSymlinks(t *testing.T) {
	router, server := setupTestServer(t)
	uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")

	entrySecret := filepath.Join(server.cfg.DataDir, "outside-entry-secret")
	if err := os.WriteFile(entrySecret, []byte("outside entry secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(uploadsRoot, "users", "1", "covers", "escape.png")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(entrySecret, entryPath); err != nil {
		t.Fatal(err)
	}

	outsideUsers := filepath.Join(server.cfg.DataDir, "outside-users")
	ancestorFile := filepath.Join(outsideUsers, "2", "covers", "ancestor.png")
	if err := os.MkdirAll(filepath.Dir(ancestorFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ancestorFile, []byte("outside ancestor secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outsideUsers, "2"), filepath.Join(uploadsRoot, "users", "2")); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "entry", path: "/uploads/users/1/covers/escape.png"},
		{name: "ancestor", path: "/uploads/users/2/covers/ancestor.png"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				response := uploadPublicResourceRequest(router, method, test.path, nil)
				if response.Code != http.StatusNotFound {
					t.Fatalf("%s %s = %d body=%q, want 404", method, test.path, response.Code, response.Body.String())
				}
				if strings.Contains(response.Body.String(), server.cfg.DataDir) || strings.Contains(response.Body.String(), "outside") {
					t.Fatalf("unsafe upload response exposed host data: %q", response.Body.String())
				}
			}
		})
	}

	for _, path := range []string{entryPath, filepath.Join(uploadsRoot, "users", "2")} {
		if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("read changed mounted symlink %s: info=%v err=%v", path, info, err)
		}
	}
}

func TestUploadPublicResourceRejectsRootSymlinkAndBackslashPath(t *testing.T) {
	t.Run("root symlink", func(t *testing.T) {
		router, server := setupTestServer(t)
		uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")
		outsideRoot := filepath.Join(server.cfg.DataDir, "outside-upload-root")
		outsideFile := filepath.Join(outsideRoot, "legacy", "root.png")
		if err := os.MkdirAll(filepath.Dir(outsideFile), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(outsideFile, []byte("outside root secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(uploadsRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideRoot, uploadsRoot); err != nil {
			t.Fatal(err)
		}

		response := uploadPublicResourceRequest(router, http.MethodGet, "/uploads/legacy/root.png", nil)
		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "outside root secret") {
			t.Fatalf("root symlink read = %d %q, want path-free 404", response.Code, response.Body.String())
		}
	})

	t.Run("backslash", func(t *testing.T) {
		router, server := setupTestServer(t)
		uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")
		name := `legacy\\escape.png`
		if err := os.WriteFile(filepath.Join(uploadsRoot, name), []byte("backslash file"), 0o600); err != nil {
			t.Fatal(err)
		}

		response := uploadPublicResourceRequest(router, http.MethodGet, "/uploads/"+url.PathEscape(name), nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("backslash path read = %d %q, want 404", response.Code, response.Body.String())
		}
	})
}

func TestUploadPublicResourceRejectsDirectoriesAndSpecialFiles(t *testing.T) {
	router, server := setupTestServer(t)
	uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")

	directory := filepath.Join(uploadsRoot, "users", "9", "covers", "directory.png")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(uploadsRoot, "users", "9", "covers", "pipe.png")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/uploads/users/9/covers/directory.png",
		"/uploads/users/9/covers/pipe.png",
	} {
		response := uploadPublicResourceRequest(router, http.MethodGet, path, nil)
		if response.Code != http.StatusNotFound || response.Body.Len() != 0 {
			t.Fatalf("GET %s = %d body=%q, want empty 404", path, response.Code, response.Body.String())
		}
	}

	for _, path := range []string{directory, fifo} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("read changed mounted object %s: %v", path, err)
		}
	}
}

func TestUploadPublicResourcePreservesRegularFileHTTPContract(t *testing.T) {
	router, server := setupTestServer(t)
	uploadsRoot := filepath.Join(server.cfg.DataDir, "uploads")
	modTime := time.Date(2026, time.August, 17, 3, 4, 5, 0, time.UTC)

	files := []struct {
		path string
		data string
	}{
		{path: "covers/legacy cover.png", data: "legacy-regular-bytes"},
		{path: "users/7/fonts/阅读 字体.ttf", data: "current-regular-bytes"},
	}
	for _, file := range files {
		fullPath := filepath.Join(uploadsRoot, filepath.FromSlash(file.path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(file.data), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(fullPath, modTime, modTime); err != nil {
			t.Fatal(err)
		}

		requestPath := "/uploads/" + escapeUploadResourcePath(file.path)
		get := uploadPublicResourceRequest(router, http.MethodGet, requestPath, nil)
		if get.Code != http.StatusOK || get.Body.String() != file.data {
			t.Fatalf("GET %s = %d %q", file.path, get.Code, get.Body.String())
		}
		if get.Header().Get("Content-Length") != stringInt(len(file.data)) || get.Header().Get("Last-Modified") == "" {
			t.Fatalf("GET %s headers length=%q modified=%q", file.path, get.Header().Get("Content-Length"), get.Header().Get("Last-Modified"))
		}

		head := uploadPublicResourceRequest(router, http.MethodHead, requestPath, nil)
		if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != stringInt(len(file.data)) {
			t.Fatalf("HEAD %s = %d length=%q body=%q", file.path, head.Code, head.Header().Get("Content-Length"), head.Body.String())
		}

		ranged := uploadPublicResourceRequest(router, http.MethodGet, requestPath, map[string]string{"Range": "bytes=1-4"})
		if ranged.Code != http.StatusPartialContent || ranged.Body.String() != file.data[1:5] || ranged.Header().Get("Content-Range") != "bytes 1-4/"+stringInt(len(file.data)) {
			t.Fatalf("Range %s = %d range=%q body=%q", file.path, ranged.Code, ranged.Header().Get("Content-Range"), ranged.Body.String())
		}

		notModified := uploadPublicResourceRequest(router, http.MethodGet, requestPath, map[string]string{"If-Modified-Since": get.Header().Get("Last-Modified")})
		if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
			t.Fatalf("conditional GET %s = %d body=%q", file.path, notModified.Code, notModified.Body.String())
		}

		invalidRange := uploadPublicResourceRequest(router, http.MethodGet, requestPath, map[string]string{"Range": "bytes=999-1000"})
		if invalidRange.Code != http.StatusRequestedRangeNotSatisfiable || invalidRange.Header().Get("Content-Range") != "bytes */"+stringInt(len(file.data)) {
			t.Fatalf("invalid Range %s = %d range=%q body=%q", file.path, invalidRange.Code, invalidRange.Header().Get("Content-Range"), invalidRange.Body.String())
		}
	}
}

func uploadPublicResourceRequest(router http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func escapeUploadResourcePath(path string) string {
	segments := strings.Split(path, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return strings.Join(segments, "/")
}

func stringInt(value int) string {
	return strconv.Itoa(value)
}
