package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openreader/backend/models"
)

func TestWebDAVProgressUploadUpdatesCallerShelfProgress(t *testing.T) {
	for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
		for _, directory := range []string{"bookProgress", "legado/bookProgress"} {
			for _, identity := range []string{"URL", "name-author"} {
				t.Run(prefix+"/"+directory+"/"+identity, func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
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
					shelf := webDAVProtocolRequest(t, router, http.MethodGet, "/api/books", auth, "", nil)
					if shelf.Code != http.StatusOK || !strings.Contains(shelf.Body.String(), `"chapterIndex":1`) || !strings.Contains(shelf.Body.String(), `"offset":37`) {
						t.Fatalf("shelf did not project committed uploaded progress: %d %s", shelf.Code, shelf.Body.String())
					}
				})
			}
		}
	}
}
