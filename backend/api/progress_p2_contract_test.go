package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func putProgressContract(t *testing.T, router http.Handler, auth string, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/progress", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func progressContractBook(t *testing.T, server *Server, user models.User, title string, chapterTitles ...string) (models.Book, []models.Chapter) {
	t.Helper()
	book := models.Book{UserID: user.ID, Title: title, Author: "进度作者", URL: "https://progress.example/" + strconv.FormatInt(time.Now().UnixNano(), 10)}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := make([]models.Chapter, 0, len(chapterTitles))
	for index, title := range chapterTitles {
		chapter := models.Chapter{BookID: book.ID, Index: index, Title: title, URL: fmt.Sprintf("%s/%d", book.URL, index)}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
		chapters = append(chapters, chapter)
	}
	return book, chapters
}

func TestUpdateProgressCanonicalizesChapterIdentity(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	book, chapters := progressContractBook(t, server, user, "规范章节", "服务端第一章")

	response := putProgressContract(t, router, auth, fmt.Sprintf(
		`{"bookId":%d,"chapterIndex":0,"offset":18,"percent":0.3,"chapterPercent":0.4,"chapterTitle":"伪造标题"}`,
		book.ID,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("canonical progress save: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var saved models.ReadingProgress
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ChapterID != chapters[0].ID || saved.ChapterIndex != 0 || saved.ChapterTitle != chapters[0].Title {
		t.Fatalf("server must canonicalize chapter identity, got %+v want id=%d title=%q", saved, chapters[0].ID, chapters[0].Title)
	}
}

func TestUpdateProgressRejectsInvalidChapterIdentity(t *testing.T) {
	tests := []struct {
		name    string
		payload func(book models.Book, own []models.Chapter, foreign models.Chapter) string
	}{
		{
			name: "foreign chapter id",
			payload: func(book models.Book, _ []models.Chapter, foreign models.Chapter) string {
				return fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":1}`, book.ID, foreign.ID)
			},
		},
		{
			name: "chapter id and index mismatch",
			payload: func(book models.Book, own []models.Chapter, _ models.Chapter) string {
				return fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":1}`, book.ID, own[1].ID)
			},
		},
		{
			name: "negative chapter index",
			payload: func(book models.Book, _ []models.Chapter, _ models.Chapter) string {
				return fmt.Sprintf(`{"bookId":%d,"chapterIndex":-1,"offset":1}`, book.ID)
			},
		},
		{
			name: "negative offset",
			payload: func(book models.Book, own []models.Chapter, _ models.Chapter) string {
				return fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":-1}`, book.ID, own[0].ID)
			},
		},
		{
			name: "missing chapter index",
			payload: func(book models.Book, _ []models.Chapter, _ models.Chapter) string {
				return fmt.Sprintf(`{"bookId":%d,"chapterIndex":99,"offset":1}`, book.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			user := lifecycleUser(t, server, "testuser")
			book, own := progressContractBook(t, server, user, "非法章节", "第一章", "第二章")
			foreignBook, foreign := progressContractBook(t, server, user, "另一书", "外书章节")
			_ = foreignBook

			response := putProgressContract(t, router, auth, tt.payload(book, own, foreign[0]))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid progress identity: expected 400, got %d: %s", response.Code, response.Body.String())
			}
			var count int64
			if err := server.db.Model(&models.ReadingProgress{}).Where("user_id = ? AND book_id = ?", user.ID, book.ID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("invalid progress created %d durable rows", count)
			}
		})
	}
}

func TestConcurrentProgressWritesUseOneAtomicWinner(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	book, chapters := progressContractBook(t, server, user, "并发进度", "第一章", "第二章")
	existing := models.ReadingProgress{
		UserID: user.ID, BookID: book.ID, ChapterID: chapters[0].ID, ChapterIndex: 0,
		ChapterTitle: chapters[0].Title, UpdatedAt: time.Now().UTC(),
	}
	if err := server.db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	var firstReads atomic.Int32
	release := make(chan struct{})
	callbackName := "test:progress-cas-barrier"
	if err := server.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "reading_progresses" || firstReads.Load() >= 2 {
			return
		}
		if firstReads.Add(1) == 2 {
			close(release)
		}
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	defer server.db.Callback().Query().Remove(callbackName)

	client := server.hub.AddClient(user.ID, nil)
	defer server.hub.RemoveClient(client)
	base := existing.UpdatedAt.Format(time.RFC3339Nano)
	payloads := []string{
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":111,"chapterTitle":"第一章","baseUpdatedAt":%q,"clientId":"client-a"}`, book.ID, chapters[0].ID, base),
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":1,"offset":222,"chapterTitle":"第二章","baseUpdatedAt":%q,"clientId":"client-b"}`, book.ID, chapters[1].ID, base),
	}

	responses := make([]*httptest.ResponseRecorder, len(payloads))
	var wait sync.WaitGroup
	for index := range payloads {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			responses[index] = putProgressContract(t, router, auth, payloads[index])
		}(index)
	}
	wait.Wait()

	winners := 0
	conflicts := 0
	for _, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("concurrent save must return compatible 200, got %d: %s", response.Code, response.Body.String())
		}
		if response.Header().Get("X-OpenReader-Progress-Conflict") == "1" {
			conflicts++
		} else {
			winners++
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("same-base writes need one winner and one conflict, got winners=%d conflicts=%d", winners, conflicts)
	}

	broadcasts := 0
	for {
		select {
		case <-client.Send:
			broadcasts++
		default:
			if broadcasts != 1 {
				t.Fatalf("only the committed winner may broadcast, got %d events", broadcasts)
			}
			return
		}
	}
}

func TestConcurrentInitialProgressWritesUseOneAtomicWinner(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	book, chapters := progressContractBook(t, server, user, "首次并发", "第一章", "第二章")

	var firstReads atomic.Int32
	release := make(chan struct{})
	callbackName := "test:initial-progress-cas-barrier"
	if err := server.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "reading_progresses" || firstReads.Load() >= 2 {
			return
		}
		if firstReads.Add(1) == 2 {
			close(release)
		}
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	defer server.db.Callback().Query().Remove(callbackName)

	client := server.hub.AddClient(user.ID, nil)
	defer server.hub.RemoveClient(client)
	payloads := []string{
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":101,"clientId":"initial-a"}`, book.ID, chapters[0].ID),
		fmt.Sprintf(`{"bookId":%d,"chapterId":%d,"chapterIndex":1,"offset":202,"clientId":"initial-b"}`, book.ID, chapters[1].ID),
	}
	responses := make([]*httptest.ResponseRecorder, len(payloads))
	var wait sync.WaitGroup
	for index := range payloads {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			responses[index] = putProgressContract(t, router, auth, payloads[index])
		}(index)
	}
	wait.Wait()

	winners := 0
	conflicts := 0
	for _, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("concurrent first save must return compatible 200, got %d: %s", response.Code, response.Body.String())
		}
		if response.Header().Get("X-OpenReader-Progress-Conflict") == "1" {
			conflicts++
		} else {
			winners++
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("concurrent first writes need one winner and one conflict, got winners=%d conflicts=%d", winners, conflicts)
	}
	var count int64
	if err := server.db.Model(&models.ReadingProgress{}).Where("user_id = ? AND book_id = ?", user.ID, book.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent first writes created %d progress rows", count)
	}
	if len(client.Send) != 1 {
		t.Fatalf("concurrent first writes emitted %d events", len(client.Send))
	}
}

func TestProgressEndpointsAreUserScoped(t *testing.T) {
	router, server := setupTestServer(t)
	ownerAuth := authHeader(t, router)
	owner := lifecycleUser(t, server, "testuser")
	foreignAuth := registerLifecycleToken(t, router, "progressforeign")
	foreign := lifecycleUser(t, server, "progressforeign")
	book, chapters := progressContractBook(t, server, owner, "用户隔离进度", "第一章")

	response := putProgressContract(t, router, ownerAuth, fmt.Sprintf(
		`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":55}`,
		book.ID, chapters[0].ID,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("owner progress save: %d %s", response.Code, response.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/progress/"+strconv.FormatUint(uint64(book.ID), 10), nil)
	getReq.Header.Set("Authorization", foreignAuth)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getReq)
	if getResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign progress GET: expected 404, got %d: %s", getResponse.Code, getResponse.Body.String())
	}
	putResponse := putProgressContract(t, router, foreignAuth, fmt.Sprintf(
		`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":99}`,
		book.ID, chapters[0].ID,
	))
	if putResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign progress PUT: expected 404, got %d: %s", putResponse.Code, putResponse.Body.String())
	}
	var foreignCount int64
	if err := server.db.Model(&models.ReadingProgress{}).Where("user_id = ?", foreign.ID).Count(&foreignCount).Error; err != nil {
		t.Fatal(err)
	}
	if foreignCount != 0 {
		t.Fatalf("foreign caller created %d progress rows", foreignCount)
	}
}

func TestProgressSaveMirrorsExistingWebDAVDirectory(t *testing.T) {
	tests := []struct {
		name       string
		regular    bool
		legacyPath bool
	}{
		{name: "administrator top-level directory"},
		{name: "regular user legacy directory", regular: true, legacyPath: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			user := lifecycleUser(t, server, "testuser")
			root := server.webdavDir()
			if tt.regular {
				auth = registerLifecycleToken(t, router, "progressmember")
				user = lifecycleUser(t, server, "progressmember")
				root = filepath.Join(root, "users", engine.SafeFilename(user.Username))
			}
			progressDir := filepath.Join(root, "bookProgress")
			if tt.legacyPath {
				progressDir = filepath.Join(root, "legado", "bookProgress")
			}
			if err := os.MkdirAll(progressDir, 0o755); err != nil {
				t.Fatal(err)
			}
			book, chapters := progressContractBook(t, server, user, "进度镜像", "镜像章节")

			response := putProgressContract(t, router, auth, fmt.Sprintf(
				`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":321,"chapterTitle":"镜像章节"}`,
				book.ID, chapters[0].ID,
			))
			if response.Code != http.StatusOK {
				t.Fatalf("save mirrored progress: expected 200, got %d: %s", response.Code, response.Body.String())
			}

			mirrorPath := filepath.Join(progressDir, engine.SafeBookFolderName(book.Title, book.Author)+".json")
			data, err := os.ReadFile(mirrorPath)
			if err != nil {
				t.Fatalf("read progress mirror: %v", err)
			}
			var mirror struct {
				Name            string `json:"name"`
				Author          string `json:"author"`
				BookURL         string `json:"bookUrl"`
				DurChapterIndex int    `json:"durChapterIndex"`
				DurChapterPos   int    `json:"durChapterPos"`
				DurChapterTitle string `json:"durChapterTitle"`
			}
			if err := json.Unmarshal(data, &mirror); err != nil {
				t.Fatalf("decode progress mirror: %v", err)
			}
			if mirror.Name != book.Title || mirror.Author != book.Author || mirror.BookURL != book.URL ||
				mirror.DurChapterIndex != 0 || mirror.DurChapterPos != 321 || mirror.DurChapterTitle != chapters[0].Title {
				t.Fatalf("unexpected progress mirror: %+v", mirror)
			}
		})
	}
}

func TestProgressSaveDoesNotCreateOrCrossWebDAVRoots(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth := registerLifecycleToken(t, router, "disabledprogress")
	user := lifecycleUser(t, server, "disabledprogress")
	if err := server.db.Model(&models.User{}).Where("id = ?", user.ID).Update("can_access_webdav", false).Error; err != nil {
		t.Fatal(err)
	}
	privateRoot := filepath.Join(server.webdavDir(), "users", engine.SafeFilename(user.Username))
	progressDir := filepath.Join(privateRoot, "bookProgress")
	if err := os.MkdirAll(progressDir, 0o755); err != nil {
		t.Fatal(err)
	}
	book, chapters := progressContractBook(t, server, user, "禁止镜像", "第一章")
	response := putProgressContract(t, router, auth, fmt.Sprintf(
		`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":9}`,
		book.ID, chapters[0].ID,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("progress must save even without WebDAV permission: %d %s", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(progressDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("disabled user wrote WebDAV progress files: %+v", entries)
	}

	var saved models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&saved).Error; err != nil {
		t.Fatalf("database progress must remain authoritative: %v", err)
	}
}

func TestProgressSaveDoesNotCreateMissingWebDAVFeatureDirectory(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	book, chapters := progressContractBook(t, server, user, "无镜像目录", "第一章")
	response := putProgressContract(t, router, auth, fmt.Sprintf(
		`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":12}`,
		book.ID, chapters[0].ID,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("save without WebDAV feature directory: %d %s", response.Code, response.Body.String())
	}
	for _, path := range []string{
		filepath.Join(server.webdavDir(), "bookProgress"),
		filepath.Join(server.webdavDir(), "legado", "bookProgress"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("progress save created missing WebDAV feature directory %q: %v", path, err)
		}
	}
}

func TestProgressMirrorRejectsSymlinkWithoutRollingBackDatabase(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	outside := t.TempDir()
	if err := os.MkdirAll(server.webdavDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(server.webdavDir(), "bookProgress")); err != nil {
		t.Fatal(err)
	}
	book, chapters := progressContractBook(t, server, user, "符号链接镜像", "第一章")
	response := putProgressContract(t, router, auth, fmt.Sprintf(
		`{"bookId":%d,"chapterId":%d,"chapterIndex":0,"offset":64}`,
		book.ID, chapters[0].ID,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("symlink mirror failure must not roll back progress: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-OpenReader-Progress-WebDAV") != "failed" {
		t.Fatalf("symlink mirror must expose a path-free failure marker, got %q", response.Header().Get("X-OpenReader-Progress-WebDAV"))
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("progress mirror followed symlink outside its root: %+v", entries)
	}
	var saved models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&saved).Error; err != nil {
		t.Fatalf("database progress was rolled back after mirror failure: %v", err)
	}
	if saved.Offset != 64 {
		t.Fatalf("database progress changed after mirror failure: %+v", saved)
	}
}
