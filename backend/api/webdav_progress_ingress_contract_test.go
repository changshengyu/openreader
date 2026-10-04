package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestWebDAVProgressUploadUpdatesCallerShelfProgress(t *testing.T) {
	for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
		for _, directory := range []string{"bookProgress", "legado/bookProgress"} {
			for _, identity := range []string{"URL", "name-author"} {
				t.Run(prefix+"/"+directory+"/"+identity, func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					bearer := auth
					if identity == "URL" {
						auth = webDAVBasic("testuser", "test1234")
					}
					user := lifecycleUser(t, server, "testuser")
					book, chapters := progressContractBook(t, server, user, "WebDAV 同步书", "第一章", "第二章")
					previous := models.ReadingProgress{
						UserID: user.ID, BookID: book.ID, ChapterID: chapters[0].ID,
						ChapterIndex: 0, ChapterTitle: chapters[0].Title, Offset: 5,
						Mode: "scroll", UpdatedAt: time.Now().Add(-time.Minute),
					}
					if err := server.db.Create(&previous).Error; err != nil {
						t.Fatal(err)
					}
					mkdir := webDAVProtocolRequest(t, router, "MKCOL", prefix+"/"+directory, auth, "", nil)
					if mkdir.Code != http.StatusCreated {
						t.Fatalf("create progress directory = %d: %s", mkdir.Code, mkdir.Body.String())
					}
					payload := map[string]any{
						"name": book.Title, "author": book.Author,
						"durChapterIndex": 1, "durChapterPos": 37,
						"durChapterTime": time.Now().UnixMilli(), "durChapterTitle": "客户端标题",
					}
					if identity == "URL" {
						payload["bookUrl"] = book.URL
					}
					body, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					response := webDAVProtocolRequest(t, router, http.MethodPut, prefix+"/"+directory+"/progress.json", auth, string(body), nil)
					if response.Code != http.StatusCreated || response.Body.Len() != 0 {
						t.Fatalf("PUT progress = %d: %s", response.Code, response.Body.String())
					}
					stored, err := os.ReadFile(filepath.Join(server.webdavDir(), filepath.FromSlash(directory), "progress.json"))
					if err != nil || string(stored) != string(body) {
						t.Fatalf("uploaded progress bytes = %q: %v", stored, err)
					}
					progress, found, err := server.progressSvc.Get(user.ID, book.ID)
					if err != nil || !found || progress.ChapterIndex != 1 || progress.Offset != 37 || progress.ChapterID != chapters[1].ID || progress.ChapterTitle != chapters[1].Title {
						t.Fatalf("successful external upload did not update canonical caller progress: found=%v progress=%+v error=%v", found, progress, err)
					}
					if !progress.UpdatedAt.After(previous.UpdatedAt) {
						t.Fatalf("uploaded progress did not advance CAS version: %v", progress.UpdatedAt)
					}
					shelf := webDAVProtocolRequest(t, router, http.MethodGet, "/api/books", bearer, "", nil)
					if shelf.Code != http.StatusOK || !strings.Contains(shelf.Body.String(), `"chapterIndex":1`) || !strings.Contains(shelf.Body.String(), `"offset":37`) {
						t.Fatalf("shelf did not project committed uploaded progress: %d %s", shelf.Code, shelf.Body.String())
					}
				})
			}
		}
	}
}

func TestWebDAVProgressIngressPrivateRootAndScopedNotification(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	memberAuth := registerStorageTestUser(t, router, "progressmember")
	admin := lifecycleUser(t, server, "testuser")
	member := lifecycleUser(t, server, "progressmember")
	book, chapters := progressContractBook(t, server, member, "私有同步书", "第一章", "第二章")
	foreign, _ := progressContractBook(t, server, admin, "私有同步书", "第一章", "第二章")
	host := httptest.NewServer(router)
	defer host.Close()
	ownerWS, _, err := dialSyncWebSocket(host.URL, bearerToken(memberAuth), "")
	if err != nil {
		t.Fatal(err)
	}
	defer ownerWS.Close()
	otherWS, _, err := dialSyncWebSocket(host.URL, bearerToken(adminAuth), "")
	if err != nil {
		t.Fatal(err)
	}
	defer otherWS.Close()
	ownerRead := readWebSocket(ownerWS)
	otherRead := readWebSocket(otherWS)
	if response := webDAVProtocolRequest(t, router, "MKCOL", "/reader3/webdav/legado/bookProgress", memberAuth, "", nil); response.Code != 201 {
		t.Fatal(response.Code)
	}
	body, _ := json.Marshal(map[string]any{"bookUrl": book.URL, "durChapterIndex": 1, "durChapterPos": 240, "durChapterTime": time.Now().UnixMilli()})
	put := webDAVProtocolRequest(t, router, "PUT", "/webdav/legado/bookProgress/member.json", webDAVBasic("progressmember", "secret123"), string(body), nil)
	if put.Code != 201 || put.Header().Get("X-OpenReader-Progress-Sync") != "" {
		t.Fatalf("PUT = %d: %v", put.Code, put.Header())
	}
	select {
	case message := <-ownerRead:
		var event struct {
			Type    string            `json:"type"`
			Payload progressBroadcast `json:"payload"`
		}
		if message.err != nil || json.Unmarshal(message.payload, &event) != nil || event.Type != "progress_update" || event.Payload.BookID != book.ID || event.Payload.ChapterID != chapters[1].ID || event.Payload.Offset != 240 || event.Payload.Book.Progress == nil {
			t.Fatalf("owner notification: %s %v", message.payload, message.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("owner progress notification missing")
	}
	duplicateRead := readWebSocket(ownerWS)
	put = webDAVProtocolRequest(t, router, "PUT", "/reader3/webdav/legado/bookProgress/member.json", memberAuth, string(body), nil)
	if put.Code != 201 {
		t.Fatal(put.Code)
	}
	select {
	case message := <-duplicateRead:
		t.Fatalf("duplicate broadcast: %s %v", message.payload, message.err)
	case message := <-otherRead:
		t.Fatalf("cross-user broadcast: %s %v", message.payload, message.err)
	case <-time.After(websocketContractReadWait):
	}
	stored, err := os.ReadFile(filepath.Join(server.webdavDir(), "users", "progressmember", "legado", "bookProgress", "member.json"))
	if err != nil || string(stored) != string(body) {
		t.Fatalf("private bytes: %q %v", stored, err)
	}
	if _, found, err := server.progressSvc.Get(admin.ID, foreign.ID); err != nil || found {
		t.Fatalf("foreign progress modified %v %v", found, err)
	}
}

func TestWebDAVProgressIngressSQLFailureReportsFileOnlySuccess(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	book, _ := progressContractBook(t, server, user, "SQL 失败书", "第一章")
	if err := server.db.Callback().Create().Before("gorm:create").Register("progress-ingress-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "reading_progresses" {
			tx.AddError(errors.New("injected progress write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	webDAVProtocolRequest(t, router, "MKCOL", "/webdav/bookProgress", auth, "", nil)
	body, _ := json.Marshal(map[string]any{"bookUrl": book.URL, "durChapterIndex": 0, "durChapterPos": 100, "durChapterTime": time.Now().UnixMilli()})
	response := webDAVProtocolRequest(t, router, "PUT", "/webdav/bookProgress/failure.json", auth, string(body), nil)
	if response.Code != 201 || response.Body.Len() != 0 || response.Header().Get("X-OpenReader-Progress-Sync") != "failed" {
		t.Fatalf("missing file-only diagnostic: %d %s %v", response.Code, response.Body.String(), response.Header())
	}
	get := webDAVProtocolRequest(t, router, "GET", "/reader3/webdav/bookProgress/failure.json", auth, "", nil)
	if get.Code != 200 || get.Body.String() != string(body) {
		t.Fatalf("file compensation lost: %d %s", get.Code, get.Body.String())
	}
	if _, found, err := server.progressSvc.Get(user.ID, book.ID); err != nil || found {
		t.Fatalf("SQL failure persisted progress: %v %v", found, err)
	}
}
