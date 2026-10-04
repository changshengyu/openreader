package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			if !ctx.fired || response.Code == 201 || response.Body.Len() != 0 || (action == "replace target" && response.Code != 403) {
				t.Fatalf("admitted MOVE boundary not protected: fired=%v status=%d body=%q", ctx.fired, response.Code, response.Body.String())
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
