package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"openreader/backend/services/webdavfs"
)

type davCopyContext struct {
	context.Context
	onWork func() bool
	mu     sync.Mutex
	fired  atomic.Bool
}

func (c *davCopyContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fired.Load() && c.onWork() {
		c.fired.Store(true)
		return nil
	}
	return c.Context.Err()
}

func TestWebDAVCopyWorkingTargetChangeReturnsEmpty403(t *testing.T) {
	for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
		t.Run(prefix, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			if prefix == "/reader3/webdav" {
				auth = webDAVBasic("testuser", "test1234")
			}
			for _, name := range []string{"source.txt", "target.txt"} {
				response := webDAVProtocolRequest(t, router, "PUT", prefix+"/"+name, auth, name, nil)
				if response.Code != 201 {
					t.Fatal(response.Code)
				}
			}
			ctx := &davCopyContext{Context: context.Background()}
			ctx.onWork = func() bool {
				entries, _ := os.ReadDir(server.webdavDir())
				found := false
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".openreader-copy-stage-") || strings.HasPrefix(entry.Name(), ".webdav-copy-") {
						found = true
					}
				}
				if !found {
					return false
				}
				target := filepath.Join(server.webdavDir(), "target.txt")
				if err := os.Rename(target, target+"-held"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("new final"), 0o644); err != nil {
					t.Fatal(err)
				}
				return true
			}
			req := httptest.NewRequest("COPY", prefix+"/source.txt", nil).WithContext(ctx)
			req.Header.Set("Authorization", auth)
			req.Header.Set("Destination", "/webdav/target.txt")
			req.Header.Set("Overwrite", "T")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if !ctx.fired.Load() || response.Code != 403 || response.Body.Len() != 0 {
				t.Fatalf("COPY changed target: fired=%v %d %s", ctx.fired.Load(), response.Code, response.Body.String())
			}
			data, err := os.ReadFile(filepath.Join(server.webdavDir(), "target.txt"))
			if err != nil || string(data) != "new final" {
				t.Fatalf("new final overwritten: %q %v", data, err)
			}
		})
	}
}

func TestWebDAVTransferPostCommitCleanupDiagnostic(t *testing.T) {
	for _, pending := range []bool{false, true} {
		router := gin.New()
		router.Handle("COPY", "/transfer", func(c *gin.Context) {
			var err error
			if pending {
				err = webdavfs.ErrCopyCleanupPending
			}
			writeWebDAVTransferResult(c, err)
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("COPY", "/transfer", nil))
		want := ""
		if pending {
			want = "pending"
		}
		if response.Code != 201 || response.Body.Len() != 0 || response.Header().Get("X-OpenReader-WebDAV-Cleanup") != want {
			t.Fatalf("postcommit response: %d %s %v", response.Code, response.Body.String(), response.Header())
		}
	}
}

func TestWebDAVCopyPrivateDirectoryAndProgressFilesRemainOrdinary(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	memberAuth := registerStorageTestUser(t, router, "copymember")
	member := lifecycleUser(t, server, "copymember")
	book, _ := progressContractBook(t, server, member, "COPY 不同步进度", "第一章")
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
	body := string(encoded)
	for name, data := range map[string]string{"tree/nested/file.txt": "nested data", "tree/progress.json": body} {
		r := webDAVProtocolRequest(t, router, "PUT", "/webdav/"+name, memberAuth, data, nil)
		if r.Code != 201 {
			t.Fatal(r.Code)
		}
	}
	for _, target := range []struct{ source, dest string }{{"tree", "copied-tree"}, {"tree/progress.json", "bookProgress/copied.json"}} {
		r := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/"+target.source, webDAVBasic("copymember", "secret123"), "", map[string]string{"Destination": "/webdav/" + target.dest})
		if r.Code != 201 || r.Body.Len() != 0 {
			t.Fatalf("private COPY: %d %s", r.Code, r.Body.String())
		}
	}
	got := webDAVProtocolRequest(t, router, "GET", "/webdav/copied-tree/nested/file.txt", memberAuth, "", nil)
	if got.Code != 200 || got.Body.String() != "nested data" {
		t.Fatalf("recursive copy %d %s", got.Code, got.Body.String())
	}
	foreign := webDAVProtocolRequest(t, router, "GET", "/webdav/copied-tree/nested/file.txt", adminAuth, "", nil)
	if foreign.Code != 404 {
		t.Fatalf("private copy escaped user root: %d", foreign.Code)
	}
	if _, found, err := server.progressSvc.Get(member.ID, book.ID); err != nil || found {
		t.Fatalf("COPY introduced PUT progress side effect %v %v", found, err)
	}
}
