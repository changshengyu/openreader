package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Isolate the already-authorized handler boundary: cancellation before auth
// can merely deny a database lookup and never exercise the transfer service.
func TestWebDAVMoveCancelledAfterAuthorizationPreservesBothPaths(t *testing.T) {
	for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
		for _, directory := range []bool{false, true} {
			for _, overwrite := range []bool{false, true} {
				t.Run(prefix+map[bool]string{false: "/file", true: "/directory"}[directory]+map[bool]string{false: "/new", true: "/overwrite"}[overwrite], func(t *testing.T) {
					originalRouter, server := setupTestServer(t)
					_ = authHeader(t, originalRouter)
					user := lifecycleUser(t, server, "testuser")
					root := server.webdavDir()
					original := filepath.Join(root, "source")
					if directory {
						original = filepath.Join(original, "nested", "file.txt")
					}
					if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(original, []byte("original source"), 0o640); err != nil {
						t.Fatal(err)
					}
					final := filepath.Join(root, "target")
					if overwrite {
						if err := os.WriteFile(final, []byte("original target"), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					request := httptest.NewRequest("MOVE", prefix+"/source", nil).WithContext(ctx)
					request.Header.Set("Destination", "/webdav/target")
					if overwrite {
						request.Header.Set("Overwrite", "T")
					}
					router := gin.New()
					router.Handle("MOVE", prefix+"/*path", func(c *gin.Context) {
						c.Set(storeUserContextKey, user)
						server.webdavMove(c)
					})
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					got, sourceErr := os.ReadFile(original)
					if response.Code == 201 || response.Body.Len() != 0 || sourceErr != nil || string(got) != "original source" {
						t.Fatalf("cancelled authorized MOVE published: status=%d body=%q source=%q err=%v", response.Code, response.Body.String(), got, sourceErr)
					}
					if overwrite {
						got, err := os.ReadFile(final)
						if err != nil || string(got) != "original target" {
							t.Fatalf("cancelled MOVE lost old target: %q %v", got, err)
						}
					} else if _, err := os.Lstat(final); !os.IsNotExist(err) {
						t.Fatalf("cancelled MOVE created final: %v", err)
					}
				})
			}
		}
	}
}

func TestWebDAVMovePrivateDirectoryAndProgressFilesRemainOrdinary(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	memberAuth := registerStorageTestUser(t, router, "movemember")
	member := lifecycleUser(t, server, "movemember")
	book, _ := progressContractBook(t, server, member, "MOVE 不同步进度", "第一章")
	for _, path := range []string{"tree", "tree/nested", "bookProgress"} {
		r := webDAVProtocolRequest(t, router, "MKCOL", "/reader3/webdav/"+path, memberAuth, "", nil)
		if r.Code != 201 {
			t.Fatal(r.Code)
		}
	}
	encoded, err := json.Marshal(map[string]any{"bookUrl": book.URL, "durChapterIndex": 0, "durChapterPos": 50, "durChapterTime": time.Now().UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"tree/nested/file.txt": "nested data", "tree/progress.json": string(encoded)} {
		r := webDAVProtocolRequest(t, router, "PUT", "/webdav/"+name, memberAuth, data, nil)
		if r.Code != 201 {
			t.Fatal(r.Code)
		}
	}
	for _, target := range []struct{ source, dest string }{{"tree/progress.json", "bookProgress/moved.json"}, {"tree", "moved-tree"}} {
		r := webDAVProtocolRequest(t, router, "MOVE", "/reader3/webdav/"+target.source, webDAVBasic("movemember", "secret123"), "", map[string]string{"Destination": "/webdav/" + target.dest})
		if r.Code != 201 || r.Body.Len() != 0 {
			t.Fatalf("private MOVE: %d %s", r.Code, r.Body.String())
		}
		absent := webDAVProtocolRequest(t, router, "GET", "/webdav/"+target.source, memberAuth, "", nil)
		if absent.Code != 404 {
			t.Fatalf("MOVE source survived: %d", absent.Code)
		}
	}
	got := webDAVProtocolRequest(t, router, "GET", "/webdav/moved-tree/nested/file.txt", memberAuth, "", nil)
	if got.Code != 200 || got.Body.String() != "nested data" {
		t.Fatalf("recursive MOVE: %d %s", got.Code, got.Body.String())
	}
	foreign := webDAVProtocolRequest(t, router, "GET", "/webdav/moved-tree/nested/file.txt", adminAuth, "", nil)
	if foreign.Code != 404 {
		t.Fatalf("private MOVE escaped namespace: %d", foreign.Code)
	}
	if _, found, err := server.progressSvc.Get(member.ID, book.ID); err != nil || found {
		t.Fatalf("MOVE introduced PUT progress side effect: %v %v", found, err)
	}
}

func TestMovePostCommitPendingPreservesWebDAVAndLocalStoreSuccess(t *testing.T) {
	for _, endpoint := range []string{"/reader3/webdav", "/webdav", "/api/local-store/rename"} {
		t.Run(endpoint, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			root := server.webdavDir()
			if endpoint == "/api/local-store/rename" {
				root = server.cfg.LocalStoreDir
			}
			if err := os.MkdirAll(filepath.Join(root, "target"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "source"), []byte("published source"), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "target", "old"), []byte("old bytes"), 0o644); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(filepath.Join(root, "source"))
			if err != nil {
				t.Fatal(err)
			}
			unknown := ""
			ctx := &davCopyContext{Context: context.Background()}
			ctx.onWork = func() bool {
				final, err := os.Lstat(filepath.Join(root, "target"))
				if err != nil || !os.SameFile(before, final) {
					return false
				}
				entries, _ := os.ReadDir(root)
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".openreader-move-old-") {
						unknown = filepath.Join(root, entry.Name(), "unknown")
						if err := os.WriteFile(unknown, []byte("new entity"), 0o644); err != nil {
							t.Fatal(err)
						}
						return true
					}
				}
				return false
			}
			request := httptest.NewRequest("MOVE", endpoint+"/source", nil).WithContext(ctx)
			request.Header.Set("Destination", "/webdav/target")
			request.Header.Set("Overwrite", "T")
			if endpoint == "/api/local-store/rename" {
				request = httptest.NewRequest("PUT", endpoint, strings.NewReader(`{"path":"source","name":"target"}`)).WithContext(ctx)
				request.Header.Set("Content-Type", "application/json")
			}
			if endpoint == "/reader3/webdav" {
				auth = webDAVBasic("testuser", "test1234")
			}
			request.Header.Set("Authorization", auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !ctx.fired.Load() || response.Header().Get("X-OpenReader-WebDAV-Cleanup") != "pending" {
				t.Fatalf("postcommit diagnostic missing: fired=%v %d %s", ctx.fired.Load(), response.Code, response.Body.String())
			}
			if endpoint == "/api/local-store/rename" {
				var result map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != 200 || len(result) != 1 || result["path"] != "target" {
					t.Fatalf("LocalStore committed rename lost success: %d %s", response.Code, response.Body.String())
				}
			} else if response.Code != 201 || response.Body.Len() != 0 {
				t.Fatalf("WebDAV committed move lost success: %d %s", response.Code, response.Body.String())
			}
			for path, want := range map[string]string{filepath.Join(root, "target"): "published source", unknown: "new entity", filepath.Join(filepath.Dir(unknown), "old"): "old bytes"} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("postcommit bytes lost: %q %v", got, err)
				}
			}
		})
	}
}

func TestWebDAVMoveAdmittedBoundaryRejectsTargetChangeOrCancellation(t *testing.T) {
	for _, action := range []string{"replace target", "cancel"} {
		t.Run(action, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			for _, name := range []string{"source", "target"} {
				r := webDAVProtocolRequest(t, router, "PUT", "/webdav/"+name, auth, "original "+name, nil)
				if r.Code != 201 {
					t.Fatal(r.Code)
				}
			}
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &davCopyContext{Context: base}
			ctx.onWork = func() bool {
				entries, _ := os.ReadDir(server.webdavDir())
				found := false
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".openreader-move-source-") {
						found = true
					}
				}
				if !found {
					return false
				}
				if action == "cancel" {
					cancel()
				} else {
					target := filepath.Join(server.webdavDir(), "target")
					if err := os.Rename(target, target+"-held"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(target, []byte("later final"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return true
			}
			request := httptest.NewRequest("MOVE", "/reader3/webdav/source", nil).WithContext(ctx)
			request.Header.Set("Authorization", auth)
			request.Header.Set("Destination", "/webdav/target")
			request.Header.Set("Overwrite", "T")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !ctx.fired.Load() || response.Code == 201 || response.Body.Len() != 0 || (action == "replace target" && response.Code != 403) {
				t.Fatalf("admitted MOVE boundary not protected: fired=%v status=%d body=%q", ctx.fired.Load(), response.Code, response.Body.String())
			}
			wantTarget := "original target"
			if action == "replace target" {
				wantTarget = "later final"
			}
			for name, expected := range map[string]string{"source": "original source", "target": wantTarget} {
				got, err := os.ReadFile(filepath.Join(server.webdavDir(), name))
				if err != nil || string(got) != expected {
					t.Fatalf("MOVE lost current %s: %q %v", name, got, err)
				}
			}
		})
	}
}
