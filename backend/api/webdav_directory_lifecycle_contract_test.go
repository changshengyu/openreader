package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Exercise the handler after authorization, not an auth query short-circuit.
// A canceled request must not even initialize a previously missing user root.
func TestWebDAVMkcolCancelledAfterAuthorizationCreatesNoDirectories(t *testing.T) {
	for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
		for _, rootKind := range []string{"existing-admin", "fresh-admin", "fresh-member"} {
			for _, relative := range []string{"new", "new/parent/child"} {
				t.Run(prefix+"/"+rootKind+"/"+relative, func(t *testing.T) {
					originalRouter, server := setupTestServer(t)
					_ = authHeader(t, originalRouter)
					username := "testuser"
					root := server.webdavDir()
					if rootKind == "fresh-member" {
						_ = registerStorageTestUser(t, originalRouter, "mkcolmember")
						root = filepath.Join(root, "users", "mkcolmember")
						username = "mkcolmember"
					}
					user := lifecycleUser(t, server, username)
					if rootKind == "existing-admin" {
						if err := os.MkdirAll(root, 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(root, "sentinel"), []byte("keep"), 0o640); err != nil {
							t.Fatal(err)
						}
					} else if _, err := os.Lstat(root); !os.IsNotExist(err) {
						t.Fatalf("fresh-root precondition: %v", err)
					}
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					router := gin.New()
					router.Handle("MKCOL", prefix+"/*path", func(c *gin.Context) {
						c.Set("userID", user.ID)
						c.Set(storeUserContextKey, user)
						server.webdavMkcol(c)
					})
					request := httptest.NewRequest("MKCOL", prefix+"/"+relative, nil).WithContext(ctx)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					_, targetErr := os.Lstat(filepath.Join(root, "new"))
					_, rootErr := os.Lstat(root)
					if response.Code == 201 || response.Body.Len() != 0 || !os.IsNotExist(targetErr) || (rootKind != "existing-admin" && !os.IsNotExist(rootErr)) {
						t.Fatalf("cancelled MKCOL changed directories: status=%d body=%q target=%v root=%v", response.Code, response.Body.String(), targetErr, rootErr)
					}
					if rootKind == "existing-admin" {
						got, err := os.ReadFile(filepath.Join(root, "sentinel"))
						if err != nil || string(got) != "keep" {
							t.Fatalf("existing bytes changed: %q %v", got, err)
						}
					}
				})
			}
		}
	}
}

func TestLocalStoreMissingListDoesNotCreateChild(t *testing.T) {
	for _, private := range []bool{false, true} {
		for _, recursive := range []string{"0", "1"} {
			t.Run(map[bool]string{false: "admin", true: "member"}[private]+"/recursive-"+recursive, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				root := server.cfg.LocalStoreDir
				if private {
					auth = registerStorageTestUser(t, router, "listmember")
					root = filepath.Join(root, "users", "listmember")
				}
				request := httptest.NewRequest(http.MethodGet, "/api/local-store?path=missing/child&recursive="+recursive, nil)
				request.Header.Set("Authorization", auth)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				var body map[string]any
				decodeErr := json.Unmarshal(response.Body.Bytes(), &body)
				_, childErr := os.Lstat(filepath.Join(root, "missing"))
				if response.Code != 404 || decodeErr != nil || len(body) != 1 || body["error"] != "local store path not found" || !os.IsNotExist(childErr) {
					t.Fatalf("missing list created child/succeeded: status=%d body=%q decode=%v child=%v", response.Code, response.Body.String(), decodeErr, childErr)
				}
			})
		}
	}
}

func TestLocalStoreCancelledDirectoryCreatesNeitherRootNorParents(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh-root", true: "existing-root"}[existing], func(t *testing.T) {
			originalRouter, server := setupTestServer(t)
			_ = authHeader(t, originalRouter)
			user := lifecycleUser(t, server, "testuser")
			root := server.cfg.LocalStoreDir
			if existing {
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatalf("fresh-root precondition: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			router := gin.New()
			router.POST("/api/local-store/directory", func(c *gin.Context) {
				c.Set("userID", user.ID)
				server.createLocalStoreDirectory(c)
			})
			request := httptest.NewRequest(http.MethodPost, "/api/local-store/directory", strings.NewReader(`{"path":"new/parent","name":"child"}`)).WithContext(ctx)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			_, targetErr := os.Lstat(filepath.Join(root, "new"))
			_, rootErr := os.Lstat(root)
			if response.Code == 201 || !os.IsNotExist(targetErr) || (!existing && !os.IsNotExist(rootErr)) {
				t.Fatalf("cancelled directory created paths: status=%d body=%q target=%v root=%v", response.Code, response.Body.String(), targetErr, rootErr)
			}
		})
	}
}

func TestDirectoryLifecycleFreshRootsAndExistingCollectionsRemainCompatible(t *testing.T) {
	router, server := setupTestServer(t)
	admin := authHeader(t, router)
	member := registerStorageTestUser(t, router, "directorymember")
	for _, auth := range []string{admin, member} {
		for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
			for attempt := 0; attempt < 2; attempt++ {
				response := webDAVProtocolRequest(t, router, "MKCOL", prefix+"/nested/parent/child", auth, "", nil)
				if response.Code != 201 || response.Body.Len() != 0 {
					t.Fatalf("recursive/idempotent MKCOL: %d %s", response.Code, response.Body.String())
				}
			}
		}
		request := httptest.NewRequest(http.MethodGet, "/api/local-store", nil)
		request.Header.Set("Authorization", auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("fresh LocalStore root: %d %s", response.Code, response.Body.String())
		}
	}
	for _, root := range []string{server.webdavDir(), filepath.Join(server.webdavDir(), "users", "directorymember")} {
		if info, err := os.Lstat(filepath.Join(root, "nested", "parent", "child")); err != nil || !info.IsDir() {
			t.Fatalf("recursive private/root path missing: %v %v", info, err)
		}
	}
}

func TestWebDAVMkcolWorkingRootReplacementReturnsEmpty403(t *testing.T) {
	for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
		t.Run(prefix, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			if prefix == "/reader3/webdav" {
				auth = webDAVBasic("testuser", "test1234")
			}
			root, outside := server.webdavDir(), t.TempDir()
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			ctx := &davCopyContext{Context: context.Background()}
			ctx.onWork = func() bool {
				entries, _ := os.ReadDir(root)
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".openreader-directory-") {
						if err := os.Rename(root, root+"-held"); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(outside, root); err != nil {
							t.Fatal(err)
						}
						return true
					}
				}
				return false
			}
			request := httptest.NewRequest("MKCOL", prefix+"/new/child", nil).WithContext(ctx)
			request.Header.Set("Authorization", auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !ctx.fired.Load() || response.Code != 403 || response.Body.Len() != 0 {
				t.Fatalf("MKCOL working boundary: fired=%v status=%d body=%q", ctx.fired.Load(), response.Code, response.Body.String())
			}
			for _, checked := range []string{outside, root + "-held"} {
				entries, err := os.ReadDir(checked)
				if err != nil || len(entries) != 0 {
					t.Fatalf("replacement/original acquired new directories: %v %v", entries, err)
				}
			}
		})
	}
}

func TestLocalStoreUploadCancelledDuringParentCreationWritesNoFiles(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth := registerStorageTestUser(t, router, "directoryupload")
	root := filepath.Join(server.cfg.LocalStoreDir, "users", "directoryupload")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("path", "incoming/parent"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "book.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("must not publish")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &davCopyContext{Context: base}
	ctx.onWork = func() bool {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".openreader-directory-") {
				cancel()
				return true
			}
		}
		return false
	}
	request := httptest.NewRequest(http.MethodPost, "/api/local-store/upload", &body).WithContext(ctx)
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	entries, err := os.ReadDir(root)
	if !ctx.fired.Load() || response.Code == 201 || err != nil || len(entries) != 0 || strings.Contains(response.Body.String(), server.cfg.LocalStoreDir) {
		t.Fatalf("upload parent cancellation: fired=%v status=%d body=%q entries=%v err=%v", ctx.fired.Load(), response.Code, response.Body.String(), entries, err)
	}
}
