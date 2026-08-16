package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/models"
)

const (
	testWebDAVImportBodyLimit  = 1 << 20
	testWebDAVRestoreBodyLimit = 16 << 10
)

func webDAVTestRoot(server *Server) string {
	return filepath.Join(server.cfg.DataDir, "webdav")
}

func webDAVTestUser(t *testing.T, server *Server) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func writeWebDAVTestFile(t *testing.T, server *Server, relative string, data []byte) string {
	t.Helper()
	path := filepath.Join(webDAVTestRoot(server), filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func countFilesBelow(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && !entry.IsDir() {
			count++
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return count
}

func assertWebDAVRejectedWithoutImportSideEffects(t *testing.T, server *Server, events <-chan []byte) {
	t.Helper()
	var books int64
	if err := server.db.Model(&models.Book{}).Count(&books).Error; err != nil {
		t.Fatal(err)
	}
	if books != 0 {
		t.Errorf("rejected WebDAV request created %d books", books)
	}
	stageRoot := filepath.Join(server.cfg.CacheDir, "import-previews")
	if files := countFilesBelow(t, stageRoot); files != 0 {
		t.Errorf("rejected WebDAV request left %d staged files", files)
	}
	if emitted := drainBookWriteEvents(events); len(emitted) != 0 {
		t.Errorf("rejected WebDAV request emitted events: %v", emitted)
	}
}

func TestWebDAVImportJSONUsesBoundedSingleInputAndRawCardinality(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		body       func(t *testing.T) []byte
		chunked    bool
		wantStatus int
	}{
		{
			name:     "preview declared overflow",
			endpoint: "/api/webdav/import-preview",
			body: func(t *testing.T) []byte {
				return []byte(`{"paths":["book.txt"],"padding":"` + strings.Repeat("x", testWebDAVImportBodyLimit) + `"}`)
			},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:     "import chunked overflow",
			endpoint: "/api/webdav/import",
			body: func(t *testing.T) []byte {
				return []byte(`{"paths":["book.txt"],"padding":"` + strings.Repeat("x", testWebDAVImportBodyLimit) + `"}`)
			},
			chunked:    true,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:     "preview second JSON",
			endpoint: "/api/webdav/import-preview",
			body: func(t *testing.T) []byte {
				return []byte(`{"paths":["book.txt"]}{"paths":["second.txt"]}`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "import trailing garbage",
			endpoint: "/api/webdav/import",
			body: func(t *testing.T) []byte {
				return []byte(`{"paths":["book.txt"]} trailing`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "preview dual path and item inputs",
			endpoint: "/api/webdav/import-preview",
			body: func(t *testing.T) []byte {
				return []byte(`{"paths":["book.txt"],"items":[{"path":"book.txt"}]}`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "preview 201 raw paths",
			endpoint: "/api/webdav/import-preview",
			body: func(t *testing.T) []byte {
				paths := make([]string, 0, 201)
				for index := 0; index < 201; index++ {
					paths = append(paths, fmt.Sprintf("missing-%03d.txt", index))
				}
				body, err := json.Marshal(map[string]any{"paths": paths})
				if err != nil {
					t.Fatal(err)
				}
				return body
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "import 201 raw items",
			endpoint: "/api/webdav/import",
			body: func(t *testing.T) []byte {
				items := make([]map[string]string, 0, 201)
				for index := 0; index < 201; index++ {
					items = append(items, map[string]string{"path": fmt.Sprintf("missing-%03d.txt", index)})
				}
				body, err := json.Marshal(map[string]any{"items": items})
				if err != nil {
					t.Fatal(err)
				}
				return body
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "import 201 raw category IDs",
			endpoint: "/api/webdav/import",
			body: func(t *testing.T) []byte {
				body, err := json.Marshal(map[string]any{
					"paths":       []string{"missing.txt"},
					"categoryIds": make([]uint, 201),
				})
				if err != nil {
					t.Fatal(err)
				}
				return body
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "preview null body",
			endpoint: "/api/webdav/import-preview",
			body: func(t *testing.T) []byte {
				return []byte("null")
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			user := webDAVTestUser(t, server)
			events := server.hub.AddClient(user.ID, nil).Send
			writeWebDAVTestFile(t, server, "book.txt", []byte("第一章 开始\n正文"))

			response, _ := performLocalStoreRequest(
				router,
				http.MethodPost,
				test.endpoint,
				auth,
				"application/json",
				test.body(t),
				test.chunked,
			)
			if response.Code != test.wantStatus {
				t.Errorf("expected %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
			assertWebDAVRejectedWithoutImportSideEffects(t, server, events)
		})
	}
}

func TestWebDAVImportJSONAcceptsExactOneMiBBody(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	body := exactPaddedJSONObject(
		t,
		testWebDAVImportBodyLimit,
		`{"paths":["missing.txt"],"padding":"`,
		`"}`,
	)
	response, _ := performLocalStoreRequest(
		router,
		http.MethodPost,
		"/api/webdav/import-preview",
		auth,
		"application/json",
		body,
		false,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("exact 1 MiB body: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if root := webDAVTestRoot(server); countFilesBelow(t, root) != 0 {
		t.Fatalf("missing exact-limit path must not create the WebDAV root or files")
	}
}

func TestWebDAVImportExpansionRejectsItemTwoHundredOneBeforeSideEffects(t *testing.T) {
	for _, endpoint := range []string{"/api/webdav/import-preview", "/api/webdav/import"} {
		t.Run(endpoint, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			user := webDAVTestUser(t, server)
			events := server.hub.AddClient(user.ID, nil).Send
			for index := 0; index < 201; index++ {
				writeWebDAVTestFile(t, server, fmt.Sprintf("bulk/book-%03d.epub", index), []byte("not-an-epub"))
			}

			response, _ := performLocalStoreRequest(
				router,
				http.MethodPost,
				endpoint,
				auth,
				"application/json",
				[]byte(`{"paths":["bulk"]}`),
				false,
			)
			if response.Code != http.StatusBadRequest {
				t.Errorf("201-file expansion: expected 400, got %d with %d response bytes", response.Code, response.Body.Len())
			}
			assertWebDAVRejectedWithoutImportSideEffects(t, server, events)
		})
	}
}

func TestWebDAVImportExpansionAcceptsExactlyTwoHundredInStableOrder(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	for index := 199; index >= 0; index-- {
		writeWebDAVTestFile(t, server, fmt.Sprintf("bulk/book-%03d.epub", index), []byte("not-an-epub"))
	}
	response, _ := performLocalStoreRequest(
		router,
		http.MethodPost,
		"/api/webdav/import-preview",
		auth,
		"application/json",
		[]byte(`{"paths":["bulk"]}`),
		false,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("200-file expansion: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []struct {
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 200 {
		t.Fatalf("exact expansion returned %d items, want 200", len(payload.Items))
	}
	for index, item := range payload.Items {
		want := fmt.Sprintf("bulk/book-%03d.epub", index)
		if item.Path != want {
			t.Fatalf("item %d path = %q, want %q", index, item.Path, want)
		}
	}
}

func TestWebDAVDirectoryImportSkipsNestedSymlinkButKeepsSafeFile(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	root := webDAVTestRoot(server)
	writeWebDAVTestFile(t, server, "selected/safe.txt", []byte("第一章 安全\n正文"))
	outside := t.TempDir()
	bait := []byte("第一章 根外诱饵\n不得读取")
	if err := os.WriteFile(filepath.Join(outside, "bait.txt"), bait, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "bait.txt"), filepath.Join(root, "selected", "escape.txt")); err != nil {
		t.Fatal(err)
	}

	response, _ := performLocalStoreRequest(
		router,
		http.MethodPost,
		"/api/webdav/import-preview",
		auth,
		"application/json",
		[]byte(`{"paths":["selected"]}`),
		false,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("safe directory preview: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload stagedStoragePreview
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Path != "selected/safe.txt" || payload.Items[0].Book == nil {
		t.Errorf("unsafe nested entries must be skipped while the safe file remains: %+v", payload.Items)
	}
	stageRoot := filepath.Join(server.cfg.CacheDir, "import-previews")
	_ = filepath.WalkDir(stageRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".book") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("read staged import %s: %v", path, readErr)
			return nil
		}
		if bytes.Equal(data, bait) {
			t.Errorf("nested symlink bytes were staged from outside the WebDAV root")
		}
		return nil
	})
}

func TestWebDAVDirectoryImportPlannerSkipsNestedSpecialFile(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{
		Username:       "planner-admin",
		PasswordHash:   "not-used",
		Role:           "admin",
		CanAccessStore: true,
	}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	writeWebDAVTestFile(t, server, "selected/safe.txt", []byte("safe"))
	fifoPath := filepath.Join(webDAVTestRoot(server), "selected", "blocked.txt")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set(storeUserContextKey, user)

	files, ok := server.webDAVImportFiles(context, "selected")
	if !ok {
		t.Fatalf("directory planning failed: %d %s", context.Writer.Status(), context.Writer.Header().Get("Content-Type"))
	}
	if len(files) != 1 || files[0].relativePath != "selected/safe.txt" {
		t.Fatalf("special file must be skipped while safe neighbor remains: %+v", files)
	}
}

func TestWebDAVImportRejectsExplicitSpecialFileWithoutOpeningIt(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	root := webDAVTestRoot(server)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	fifoPath := filepath.Join(root, "blocked.bin")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	response, _ := performLocalStoreRequest(
		router,
		http.MethodPost,
		"/api/webdav/import-preview",
		auth,
		"application/json",
		[]byte(`{"paths":["blocked.bin"]}`),
		false,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("explicit special file: expected 400, got %d: %s", response.Code, response.Body.String())
	}
	info, err := os.Lstat(fifoPath)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("special source changed: mode=%v err=%v", info, err)
	}
}

func TestWebDAVTokenOnlyRetryAndImportDoNotTouchMountedRoot(t *testing.T) {
	for _, mode := range []string{"reparse", "import"} {
		t.Run(mode, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			writeWebDAVTestFile(t, server, "snapshot.txt", []byte("第一章 快照\n正文"))
			preview := previewStorageBook(t, router, auth, "/api/webdav/import-preview", "snapshot.txt")
			token := preview.Items[0].ImportToken

			root := webDAVTestRoot(server)
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.Symlink(outside, root); err != nil {
				t.Fatal(err)
			}

			if mode == "reparse" {
				result := reparseStagedStorageBook(t, router, auth, "/api/webdav/import-preview", "snapshot.txt", token, `^第.+章.*$`)
				if len(result.Items) != 1 || result.Items[0].ImportToken != token {
					t.Fatalf("token-only reparse lost immutable stage: %+v", result.Items)
				}
			} else {
				book := importStagedStorageBook(t, router, auth, "/api/webdav/import", "snapshot.txt", token, "WebDAV token-only import")
				if book.ID == 0 {
					t.Fatal("token-only import did not create a book")
				}
			}
			info, err := os.Lstat(root)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("token-only request touched mounted WebDAV root: mode=%v err=%v", info, err)
			}
		})
	}
}

func TestWebDAVHandlersAuthorizeBeforeReadingOversizedBodies(t *testing.T) {
	for _, endpoint := range []string{
		"/api/webdav/import-preview",
		"/api/webdav/import",
		"/api/backup/restore-webdav",
	} {
		t.Run(endpoint+" unauthenticated", func(t *testing.T) {
			router, server := setupTestServer(t)
			body := []byte(strings.Repeat("x", testWebDAVImportBodyLimit+1))
			response, _ := performLocalStoreRequest(router, http.MethodPost, endpoint, "", "application/json", body, true)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 before body admission, got %d: %s", response.Code, response.Body.String())
			}
			if _, err := os.Stat(webDAVTestRoot(server)); !os.IsNotExist(err) {
				t.Fatalf("unauthenticated request touched WebDAV root: %v", err)
			}
		})

		t.Run(endpoint+" forbidden", func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			user := webDAVTestUser(t, server)
			denied := false
			if err := server.db.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{
				"can_access_store":  false,
				"can_access_webdav": &denied,
			}).Error; err != nil {
				t.Fatal(err)
			}
			body := []byte(strings.Repeat("x", testWebDAVImportBodyLimit+1))
			response, _ := performLocalStoreRequest(router, http.MethodPost, endpoint, auth, "application/json", body, true)
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected 403 before body admission, got %d: %s", response.Code, response.Body.String())
			}
			if _, err := os.Stat(webDAVTestRoot(server)); !os.IsNotExist(err) {
				t.Fatalf("forbidden request touched WebDAV root: %v", err)
			}
		})
	}
}

func assertWebDAVRestoreRejectedWithoutSideEffects(t *testing.T, server *Server, events <-chan []byte, sourcePath string, source []byte) {
	t.Helper()
	var books int64
	if err := server.db.Model(&models.Book{}).Count(&books).Error; err != nil {
		t.Fatal(err)
	}
	if books != 0 {
		t.Errorf("rejected WebDAV restore created %d books", books)
	}
	if staged := countFilesBelow(t, filepath.Join(server.cfg.CacheDir, "backup-uploads")); staged != 0 {
		t.Errorf("rejected WebDAV restore left %d backup snapshots", staged)
	}
	if emitted := drainBookWriteEvents(events); len(emitted) != 0 {
		t.Errorf("rejected WebDAV restore emitted events: %v", emitted)
	}
	if current, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(current, source) {
		t.Errorf("rejected WebDAV restore changed mounted source: equal=%v err=%v", bytes.Equal(current, source), err)
	}
}

func TestWebDAVRestoreJSONUsesBoundedSingleDocumentBeforeRestore(t *testing.T) {
	tests := []struct {
		name       string
		body       func() []byte
		chunked    bool
		wantStatus int
	}{
		{
			name: "declared overflow",
			body: func() []byte {
				return []byte(`{"path":"backup.zip","padding":"` + strings.Repeat("x", testWebDAVRestoreBodyLimit) + `"}`)
			},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "chunked overflow",
			body: func() []byte {
				return []byte(`{"path":"backup.zip","padding":"` + strings.Repeat("x", testWebDAVRestoreBodyLimit) + `"}`)
			},
			chunked:    true,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "second JSON",
			body: func() []byte {
				return []byte(`{"path":"backup.zip"}{"path":"second.zip"}`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "trailing garbage",
			body: func() []byte {
				return []byte(`{"path":"backup.zip"} trailing`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "null body",
			body: func() []byte {
				return []byte("null")
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			user := webDAVTestUser(t, server)
			events := server.hub.AddClient(user.ID, nil).Send
			archive := makeBackupRestoreZIP(t, map[string]string{
				"myBookShelf.json": `[{"name":"wire boundary book","author":"contract","bookUrl":"https://webdav.example/wire"}]`,
			})
			sourcePath := writeWebDAVTestFile(t, server, "backup.zip", archive)

			response, _ := performLocalStoreRequest(
				router,
				http.MethodPost,
				"/api/backup/restore-webdav",
				auth,
				"application/json",
				test.body(),
				test.chunked,
			)
			if response.Code != test.wantStatus {
				t.Errorf("expected %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
			assertWebDAVRestoreRejectedWithoutSideEffects(t, server, events, sourcePath, archive)
		})
	}
}

func TestWebDAVRestoreJSONAcceptsExactSixteenKiBBody(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	archive := makeBackupRestoreZIP(t, map[string]string{"myBookShelf.json": `[]`})
	writeWebDAVTestFile(t, server, "backup.zip", archive)
	body := exactPaddedJSONObject(
		t,
		testWebDAVRestoreBodyLimit,
		`{"path":"backup.zip","padding":"`,
		`"}`,
	)
	response, _ := performLocalStoreRequest(
		router,
		http.MethodPost,
		"/api/backup/restore-webdav",
		auth,
		"application/json",
		body,
		false,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("exact 16 KiB restore body: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if staged := countFilesBelow(t, filepath.Join(server.cfg.CacheDir, "backup-uploads")); staged != 0 {
		t.Fatalf("successful restore left %d backup snapshots", staged)
	}
}

func TestWebDAVRestoreUsesCallerPrivateOpenedSnapshotAndCleansIt(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := webDAVTestUser(t, server)
	original := makeBackupRestoreZIP(t, map[string]string{
		"myBookShelf.json": `[{"name":"snapshot original","author":"contract","bookUrl":"https://webdav.example/original"}]`,
	})
	replacement := makeBackupRestoreZIP(t, map[string]string{
		"myBookShelf.json": `[{"name":"snapshot replacement","author":"contract","bookUrl":"https://webdav.example/replacement"}]`,
	})
	sourcePath := writeWebDAVTestFile(t, server, "snapshot.zip", original)

	started := make(chan struct{})
	release := make(chan struct{})
	var callbackOnce sync.Once
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	if err := server.db.Callback().Create().Before("gorm:create").Register("test:webdav-opened-snapshot", func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "Book" {
			return
		}
		callbackOnce.Do(func() {
			close(started)
			<-release
		})
	}); err != nil {
		t.Fatal(err)
	}

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		writer, _ := performLocalStoreRequest(
			router,
			http.MethodPost,
			"/api/backup/restore-webdav",
			auth,
			"application/json",
			[]byte(`{"path":"snapshot.zip"}`),
			false,
		)
		response <- writer
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		unblock()
		select {
		case writer := <-response:
			t.Fatalf("restore never reached durable work: %d %s", writer.Code, writer.Body.String())
		case <-time.After(5 * time.Second):
			t.Fatal("restore did not reach durable work or return")
		}
	}

	snapshotRoot := filepath.Join(server.cfg.CacheDir, "backup-uploads", fmt.Sprintf("%d", user.ID))
	entries, readErr := os.ReadDir(snapshotRoot)
	snapshotOK := readErr == nil && len(entries) == 1
	if snapshotOK {
		data, err := os.ReadFile(filepath.Join(snapshotRoot, entries[0].Name()))
		snapshotOK = err == nil && bytes.Equal(data, original)
	}

	replacementPath := filepath.Join(filepath.Dir(sourcePath), ".snapshot-replacement.zip")
	if err := os.WriteFile(replacementPath, replacement, 0o644); err != nil {
		unblock()
		t.Fatal(err)
	}
	if err := os.Rename(replacementPath, sourcePath); err != nil {
		unblock()
		t.Fatal(err)
	}
	unblock()

	var writer *httptest.ResponseRecorder
	select {
	case writer = <-response:
	case <-time.After(5 * time.Second):
		t.Fatal("restore did not complete after durable work was released")
	}
	if !snapshotOK {
		t.Errorf("restore did not retain one caller-private snapshot of the opened source during durable work")
	}
	if writer.Code != http.StatusOK {
		t.Errorf("snapshot restore: expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	var originalBook models.Book
	if err := server.db.Where("user_id = ? AND title = ?", user.ID, "snapshot original").First(&originalBook).Error; err != nil {
		t.Errorf("original opened snapshot was not restored: %v", err)
	}
	var replacementBooks int64
	if err := server.db.Model(&models.Book{}).Where("user_id = ? AND title = ?", user.ID, "snapshot replacement").Count(&replacementBooks).Error; err != nil {
		t.Fatal(err)
	}
	if replacementBooks != 0 {
		t.Errorf("replacement mounted source changed in-flight restore")
	}
	if current, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(current, replacement) {
		t.Errorf("restore changed replacement mounted source: equal=%v err=%v", bytes.Equal(current, replacement), err)
	}
	if files := countFilesBelow(t, snapshotRoot); files != 0 {
		t.Errorf("restore left %d caller-private backup snapshots", files)
	}
}
