package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	readerdb "openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/booksources"
)

// Contract: docs/compat/request-cancellation-remaining-handlers-p2-contract.md sections 3 and 6.

var errRemainingCancellationInjected = errors.New("injected remaining handler failure")

type remainingCancellationLog struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (l *remainingCancellationLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.Write(data)
}

func (l *remainingCancellationLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.String()
}

// remainingCancellationFixture is a fresh server whose GORM statements go to
// the production logger writing into a buffer. Its first account, testuser,
// is the administrator.
type remainingCancellationFixture struct {
	router *gin.Engine
	server *Server
	auth   string
	admin  models.User
	log    *remainingCancellationLog
	armed  atomic.Bool
	fired  atomic.Bool
	hooks  atomic.Int32
}

func newRemainingCancellationFixture(t *testing.T) *remainingCancellationFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	output := &remainingCancellationLog{}
	// Handlers copy the root Config on WithContext, so every later statement uses this logger.
	server.db.Logger = readerdb.NewLogger(log.New(output, "", 0))
	return &remainingCancellationFixture{
		router: router,
		server: server,
		auth:   auth,
		admin:  lifecycleUser(t, server, "testuser"),
		log:    output,
	}
}

type remainingCancellationHook struct {
	register func(database *gorm.DB, name string, fn func(*gorm.DB)) error
	remove   func(database *gorm.DB, name string) error
}

var (
	remainingBeforeQuery = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Query().Before("gorm:query").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Query().Remove(name) },
	}
	remainingBeforeCreate = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Create().Before("gorm:create").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Create().Remove(name) },
	}
	remainingBeforeUpdate = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Update().Before("gorm:update").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Update().Remove(name) },
	}
	remainingBeforeDelete = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Delete().Before("gorm:delete").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Delete().Remove(name) },
	}
	remainingBeforeDeleteBegins = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Delete().Before("gorm:begin_transaction").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Delete().Remove(name) },
	}
	remainingAfterDeleteCommit = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Delete().After("gorm:commit_or_rollback_transaction").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Delete().Remove(name) },
	}
	// Injected failures run after the statement's SQL was built and executed,
	// so the production logger prints them against the calling file.
	remainingBeforeCreateCommit = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Create().Before("gorm:commit_or_rollback_transaction").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Create().Remove(name) },
	}
	remainingBeforeDeleteCommit = remainingCancellationHook{
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Delete().Before("gorm:commit_or_rollback_transaction").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Delete().Remove(name) },
	}
)

// install runs action once, at the first statement of the armed request that matches.
func (f *remainingCancellationFixture) install(t *testing.T, hook remainingCancellationHook, match func(*gorm.DB) bool, action func(*gorm.DB)) {
	t.Helper()
	name := fmt.Sprintf("test:remaining-cancellation-%d", f.hooks.Add(1))
	if err := hook.register(f.server.db, name, func(tx *gorm.DB) {
		if f.armed.Load() && match(tx) && f.fired.CompareAndSwap(false, true) {
			action(tx)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hook.remove(f.server.db, name) })
}

// fireOnce reports whether a non-GORM seam reached its point for the first time in the armed request.
func (f *remainingCancellationFixture) fireOnce() bool {
	return f.armed.Load() && f.fired.CompareAndSwap(false, true)
}

func (f *remainingCancellationFixture) serve(ctx context.Context, method, target, auth string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, body).WithContext(ctx)
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	f.armed.Store(true)
	f.router.ServeHTTP(response, request)
	f.armed.Store(false)
	return response
}

func (f *remainingCancellationFixture) serveJSON(ctx context.Context, method, target, auth, body string) *httptest.ResponseRecorder {
	if body == "" {
		return f.serve(ctx, method, target, auth, nil, "")
	}
	return f.serve(ctx, method, target, auth, strings.NewReader(body), "application/json")
}

func (f *remainingCancellationFixture) requireFired(t *testing.T, label string, ctx context.Context) {
	t.Helper()
	if !f.fired.Load() || ctx.Err() == nil {
		t.Fatalf("%s: the cancellation point was not reached (fired=%v, ctx=%v)", label, f.fired.Load(), ctx.Err())
	}
}

func (f *remainingCancellationFixture) events(userID uint) <-chan []byte {
	return f.server.hub.AddClient(userID, nil).Send
}

func statementOn(tx *gorm.DB, table string) bool {
	return tx.Statement != nil && tx.Statement.Table == table
}

func statementInTransaction(tx *gorm.DB) bool {
	_, ok := tx.Statement.ConnPool.(*sql.Tx)
	return ok
}

func statementSelects(tx *gorm.DB, columns ...string) bool {
	if len(tx.Statement.Selects) != len(columns) {
		return false
	}
	for index, column := range columns {
		if tx.Statement.Selects[index] != column {
			return false
		}
	}
	return true
}

func statementSelectsColumn(tx *gorm.DB, column string) bool {
	for _, selected := range tx.Statement.Selects {
		if selected == column {
			return true
		}
	}
	return false
}

func statementLoadsUser(tx *gorm.DB) bool {
	_, ok := tx.Statement.Dest.(*models.User)
	return ok
}

func statementCounts(tx *gorm.DB) bool {
	_, ok := tx.Statement.Dest.(*int64)
	return ok
}

// requireAdmin loads the whole caller row outside any transaction; the auth
// middleware selects only id and auth_version.
func requireAdminStatement(tx *gorm.DB) bool {
	return statementOn(tx, "users") && statementLoadsUser(tx) && len(tx.Statement.Selects) == 0 && !statementInTransaction(tx)
}

func compactRemainingBody(response *httptest.ResponseRecorder) string {
	body := response.Body.String()
	if len(body) > 240 {
		body = body[:240] + "..."
	}
	return body
}

// assertRemainingCancelledSilently checks the handler wrote nothing: Gin then
// sends its default empty 200, which is what the access log records.
func assertRemainingCancelledSilently(t *testing.T, label string, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Errorf("%s answered a client that went away: %d %s", label, response.Code, compactRemainingBody(response))
	}
}

func assertRemainingCancellationLogQuiet(t *testing.T, label string, output *remainingCancellationLog) {
	t.Helper()
	text := output.String()
	for _, ended := range []string{"context canceled", "context deadline exceeded", "transaction has already been committed or rolled back"} {
		if strings.Contains(text, ended) {
			t.Errorf("%s logged %q:\n%s", label, ended, text)
		}
	}
}

func assertRemainingNoEvents(t *testing.T, label string, channels ...<-chan []byte) {
	t.Helper()
	for _, channel := range channels {
		if drained := drainSettingContractEvents(channel); len(drained) != 0 {
			t.Errorf("%s broadcast events: %q", label, drained)
		}
	}
}

func remainingUser(t *testing.T, server *Server, username string) (models.User, bool) {
	t.Helper()
	var users []models.User
	if err := server.db.Where("username = ?", username).Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	if len(users) == 0 {
		return models.User{}, false
	}
	return users[0], true
}

func remainingSessionCount(t *testing.T, server *Server, userID uint) int64 {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.UserSession{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func remainingCount(t *testing.T, server *Server, model any, where string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := server.db.Model(model).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func remainingMeStatus(f *remainingCancellationFixture, auth string) int {
	return f.serveJSON(context.Background(), http.MethodGet, "/api/me", auth, "").Code
}

func remainingDefaultMirrorExists(t *testing.T, server *Server) bool {
	t.Helper()
	_, err := os.Stat(server.defaultBookSourcesPath())
	if err == nil {
		return true
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return false
}

// assertRemainingDefaultSnapshot checks SQLite's default namespace holds
// exactly the given source URLs and that defaultBookSources.json is the
// canonical mirror of that same snapshot.
func assertRemainingDefaultSnapshot(t *testing.T, label string, server *Server, sourceURLs ...string) {
	t.Helper()
	defaults, err := booksources.New(server.db).ListExistingActive(0)
	if err != nil {
		t.Fatalf("%s: load SQLite default sources: %v", label, err)
	}
	stored := make([]string, 0, len(defaults))
	for _, source := range defaults {
		stored = append(stored, source.BaseURL)
	}
	if strings.Join(stored, ",") != strings.Join(sourceURLs, ",") {
		t.Errorf("%s: SQLite default sources = %v, want %v", label, stored, sourceURLs)
	}
	if !remainingDefaultMirrorExists(t, server) {
		t.Errorf("%s: defaultBookSources.json was not written", label)
		return
	}
	mirror, err := os.ReadFile(server.defaultBookSourcesPath())
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.MarshalIndent(exportBookSources(defaults), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(mirror), bytes.TrimSpace(canonical)) {
		t.Errorf("%s: defaultBookSources.json is not the mirror of the SQLite default:\n%s", label, mirror)
	}
}

func remainingLocalStoreUpload(t *testing.T, directory string) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if directory != "" {
		if err := writer.WriteField("path", directory); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "book.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("cancelled upload")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func remainingPathExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return false
}

func writeRemainingFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func remainingSearchSource(t *testing.T, server *Server) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: "取消搜索源", Enabled: true, Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "https://cancel-search.example/search?q={keyword}&page={page}",
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	return source
}

func remainingSearchBody(sourceID uint) string {
	return `{"keyword":"取消","sourceIds":[` + strconv.FormatUint(uint64(sourceID), 10) + `],"page":1}`
}

// C2: a token issue cancelled before the session commits.
func TestRemainingCancellationTokenIssueBeforeCommitIsSilent(t *testing.T) {
	for _, test := range []struct {
		name     string
		register bool
		hook     remainingCancellationHook
		match    func(*gorm.DB) bool
	}{
		{
			name: "login before the session transaction",
			hook: remainingBeforeQuery,
			match: func(tx *gorm.DB) bool {
				return statementOn(tx, "users") && statementLoadsUser(tx) && len(tx.Statement.Selects) == 0
			},
		},
		{
			name:  "login inside the session insert",
			hook:  remainingBeforeCreate,
			match: func(tx *gorm.DB) bool { return statementOn(tx, "user_sessions") },
		},
		{
			name:     "register before the session transaction",
			register: true,
			hook:     remainingBeforeCreate,
			match:    func(tx *gorm.DB) bool { return statementOn(tx, "users") },
		},
		{
			name:     "register inside the session insert",
			register: true,
			hook:     remainingBeforeCreate,
			match:    func(tx *gorm.DB) bool { return statementOn(tx, "user_sessions") },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRemainingCancellationFixture(t)
			username, password, target := "testuser", "test1234", "/api/auth/login"
			if test.register {
				username, password, target = "cancelregister", "cancel-register-1", "/api/auth/register"
			}
			sessionsBefore := int64(0)
			if !test.register {
				sessionsBefore = remainingSessionCount(t, fixture.server, fixture.admin.ID)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.install(t, test.hook, test.match, func(*gorm.DB) { cancel() })

			response := fixture.serveJSON(ctx, http.MethodPost, target, "", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password))
			fixture.requireFired(t, test.name, ctx)
			assertRemainingCancelledSilently(t, test.name, response)
			user, found := remainingUser(t, fixture.server, username)
			if !found {
				t.Fatalf("%s: account %s is missing; registration commits without the request context", test.name, username)
			}
			if sessions := remainingSessionCount(t, fixture.server, user.ID); sessions != sessionsBefore {
				t.Errorf("%s left %d sessions, want %d: a cancelled issue must not commit one", test.name, sessions, sessionsBefore)
			}
			assertRemainingCancellationLogQuiet(t, test.name, fixture.log)
		})
	}
}

// C3: a logout cancelled before its DELETE commits.
func TestRemainingCancellationLogoutBeforeCommitIsSilent(t *testing.T) {
	for _, test := range []struct {
		name string
		hook remainingCancellationHook
	}{
		{name: "before the delete begins", hook: remainingBeforeDeleteBegins},
		{name: "inside the delete", hook: remainingBeforeDelete},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRemainingCancellationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.install(t, test.hook, func(tx *gorm.DB) bool { return statementOn(tx, "user_sessions") }, func(*gorm.DB) { cancel() })

			response := fixture.serveJSON(ctx, http.MethodPost, "/api/auth/logout", fixture.auth, "")
			fixture.requireFired(t, test.name, ctx)
			assertRemainingCancelledSilently(t, "logout cancelled "+test.name, response)
			if status := remainingMeStatus(fixture, fixture.auth); status != http.StatusOK {
				t.Errorf("logout cancelled %s revoked the session: /api/me = %d", test.name, status)
			}
			assertRemainingCancellationLogQuiet(t, test.name, fixture.log)
		})
	}
}

// C4: the sync upgrade's authentication cancelled; never a 401 or a 500.
func TestRemainingCancellationSyncAuthenticationIsSilent(t *testing.T) {
	for _, test := range []struct {
		name  string
		stale bool
		hook  remainingCancellationHook
		match func(*gorm.DB) bool
	}{
		{
			name: "user lookup",
			hook: remainingBeforeQuery,
			match: func(tx *gorm.DB) bool {
				return statementOn(tx, "users") && statementSelects(tx, "id", "auth_version")
			},
		},
		{
			name:  "session renewal",
			stale: true,
			hook:  remainingBeforeUpdate,
			match: func(tx *gorm.DB) bool { return statementOn(tx, "user_sessions") },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRemainingCancellationFixture(t)
			if test.stale {
				// A renewal older than RenewInterval makes Authenticate write.
				if err := fixture.server.db.Model(&models.UserSession{}).Where("user_id = ?", fixture.admin.ID).
					Update("last_seen_at", time.Now().UTC().Add(-2*time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.install(t, test.hook, test.match, func(*gorm.DB) { cancel() })

			response := fixture.serveJSON(ctx, http.MethodGet, "/ws/sync?token="+url.QueryEscape(bearerToken(fixture.auth)), "", "")
			fixture.requireFired(t, test.name, ctx)
			assertRemainingCancelledSilently(t, "sync authentication cancelled at the "+test.name, response)
			if test.stale {
				var session models.UserSession
				if err := fixture.server.db.Where("user_id = ?", fixture.admin.ID).First(&session).Error; err != nil {
					t.Fatal(err)
				}
				if time.Since(session.LastSeenAt) < time.Minute {
					t.Errorf("cancelled renewal committed: last_seen_at=%s", session.LastSeenAt)
				}
			}
			assertRemainingCancellationLogQuiet(t, test.name, fixture.log)
		})
	}
}

// C5 / R3: requireAdmin must not turn a cancelled lookup into 403.
func TestRemainingCancellationAdminPermissionIsNotForbidden(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/users"},
		{http.MethodPost, "/api/admin/cleanup-inactive"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			fixture := newRemainingCancellationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.install(t, remainingBeforeQuery, requireAdminStatement, func(*gorm.DB) { cancel() })

			response := fixture.serveJSON(ctx, route.method, route.path, fixture.auth, "")
			fixture.requireFired(t, route.path, ctx)
			assertRemainingCancelledSilently(t, "admin check cancelled for "+route.path, response)
			assertRemainingCancellationLogQuiet(t, route.path, fixture.log)
		})
	}
}

// C5: administrator writes cancelled before they commit.
func TestRemainingCancellationAdminWritesBeforeCommitAreSilent(t *testing.T) {
	type adminCase struct {
		name    string
		prepare func(t *testing.T, f *remainingCancellationFixture, target sourceContractAccount)
		method  string
		path    func(target sourceContractAccount) string
		body    func(target sourceContractAccount) string
		hook    remainingCancellationHook
		match   func(*gorm.DB) bool
		verify  func(t *testing.T, f *remainingCancellationFixture, target sourceContractAccount)
	}
	targetIDs := func(target sourceContractAccount) string {
		return `{"ids":[` + strconv.FormatUint(uint64(target.ID), 10) + `]}`
	}
	noDefault := func(t *testing.T, f *remainingCancellationFixture, _ sourceContractAccount) {
		assertNoDefaultSourceState(t, f.server.db)
		if remainingDefaultMirrorExists(t, f.server) {
			t.Error("cancelled default save wrote defaultBookSources.json")
		}
	}
	targetExists := func(t *testing.T, f *remainingCancellationFixture, target sourceContractAccount) {
		if remainingCount(t, f.server, &models.User{}, "id = ?", target.ID) != 1 {
			t.Error("cancelled deletion removed the target account")
		}
	}
	cases := []adminCase{
		{
			name:   "set as default: target lookup",
			method: http.MethodPost,
			path: func(target sourceContractAccount) string {
				return "/api/admin/users/" + strconv.FormatUint(uint64(target.ID), 10) + "/sources/default"
			},
			hook:   remainingBeforeQuery,
			match:  func(tx *gorm.DB) bool { return statementOn(tx, "users") && statementSelects(tx, "id") },
			verify: noDefault,
		},
		{
			name: "set as default: save transaction",
			prepare: func(t *testing.T, f *remainingCancellationFixture, target sourceContractAccount) {
				createSourceThroughAPI(t, f.router, target.Auth, `{"name":"Target default","baseUrl":"https://cancel-target-default.example","enabled":true}`)
			},
			method: http.MethodPost,
			path: func(target sourceContractAccount) string {
				return "/api/admin/users/" + strconv.FormatUint(uint64(target.ID), 10) + "/sources/default"
			},
			hook:   remainingBeforeDelete,
			match:  func(tx *gorm.DB) bool { return statementOn(tx, "user_book_sources") && statementInTransaction(tx) },
			verify: noDefault,
		},
		{
			name:   "reset sources: target count",
			method: http.MethodPost,
			path:   func(sourceContractAccount) string { return "/api/admin/users/sources/reset" },
			body:   targetIDs,
			hook:   remainingBeforeQuery,
			match:  func(tx *gorm.DB) bool { return statementOn(tx, "users") && statementCounts(tx) },
		},
		{
			name: "reset sources: reset transaction",
			prepare: func(t *testing.T, f *remainingCancellationFixture, _ sourceContractAccount) {
				createSourceThroughAPI(t, f.router, f.auth, `{"name":"Admin default","baseUrl":"https://cancel-admin-default.example","enabled":true}`)
				if response := f.serveJSON(context.Background(), http.MethodPost, "/api/sources/default/save", f.auth, ""); response.Code != http.StatusOK {
					t.Fatalf("save default = %d %s", response.Code, compactRemainingBody(response))
				}
			},
			method: http.MethodPost,
			path:   func(sourceContractAccount) string { return "/api/admin/users/sources/reset" },
			body:   targetIDs,
			hook:   remainingBeforeDelete,
			match:  func(tx *gorm.DB) bool { return statementOn(tx, "source_failures") && statementInTransaction(tx) },
			verify: func(t *testing.T, f *remainingCancellationFixture, target sourceContractAccount) {
				if count := remainingCount(t, f.server, &models.BookSourceNamespace{}, "user_id = ?", target.ID); count != 0 {
					t.Errorf("cancelled reset initialized the target namespace (%d rows)", count)
				}
			},
		},
		{
			name:   "reset password",
			method: http.MethodPut,
			path: func(target sourceContractAccount) string {
				return "/api/admin/users/" + strconv.FormatUint(uint64(target.ID), 10) + "/password"
			},
			body:  func(sourceContractAccount) string { return `{"password":"cancel-reset-2"}` },
			hook:  remainingBeforeDelete,
			match: func(tx *gorm.DB) bool { return statementOn(tx, "user_sessions") && statementInTransaction(tx) },
		},
		{
			name:   "delete users",
			method: http.MethodPost,
			path:   func(sourceContractAccount) string { return "/api/admin/users/batch-delete" },
			body:   targetIDs,
			hook:   remainingBeforeDelete,
			match:  func(tx *gorm.DB) bool { return statementOn(tx, "users") && statementInTransaction(tx) },
			verify: targetExists,
		},
		{
			name: "clean up inactive users",
			prepare: func(t *testing.T, f *remainingCancellationFixture, target sourceContractAccount) {
				if err := f.server.db.Model(&models.User{}).Where("id = ?", target.ID).
					Update("last_active_at", time.Now().Add(-100*24*time.Hour)).Error; err != nil {
					t.Fatal(err)
				}
			},
			method: http.MethodPost,
			path:   func(sourceContractAccount) string { return "/api/admin/cleanup-inactive" },
			hook:   remainingBeforeDelete,
			match:  func(tx *gorm.DB) bool { return statementOn(tx, "users") && statementInTransaction(tx) },
			verify: targetExists,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRemainingCancellationFixture(t)
			target := registerSourceContractAccount(t, fixture.router, "canceltarget")
			if test.prepare != nil {
				test.prepare(t, fixture, target)
			}
			var before models.User
			if err := fixture.server.db.First(&before, target.ID).Error; err != nil {
				t.Fatal(err)
			}
			sessionsBefore := remainingSessionCount(t, fixture.server, target.ID)
			adminEvents, targetEvents := fixture.events(fixture.admin.ID), fixture.events(target.ID)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.install(t, test.hook, test.match, func(*gorm.DB) { cancel() })

			body := ""
			if test.body != nil {
				body = test.body(target)
			}
			response := fixture.serveJSON(ctx, test.method, test.path(target), fixture.auth, body)
			fixture.requireFired(t, test.name, ctx)
			assertRemainingCancelledSilently(t, test.name, response)
			assertRemainingNoEvents(t, test.name, adminEvents, targetEvents)
			var after models.User
			if err := fixture.server.db.First(&after, target.ID).Error; err == nil {
				if after.PasswordHash != before.PasswordHash || after.AuthVersion != before.AuthVersion {
					t.Errorf("%s changed the target credentials", test.name)
				}
				if sessions := remainingSessionCount(t, fixture.server, target.ID); sessions != sessionsBefore {
					t.Errorf("%s changed the target sessions: %d, want %d", test.name, sessions, sessionsBefore)
				}
			}
			if test.verify != nil {
				test.verify(t, fixture, target)
			}
			assertRemainingCancellationLogQuiet(t, test.name, fixture.log)
		})
	}
}

// C6 / R3: requireSourceEdit must not turn a cancelled lookup into 500.
func TestRemainingCancellationSourceEditPermissionIsSilent(t *testing.T) {
	fixture := newRemainingCancellationFixture(t)
	events := fixture.events(fixture.admin.ID)
	sourcesBefore := remainingCount(t, fixture.server, &models.BookSource{}, "1 = 1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture.install(t, remainingBeforeQuery, func(tx *gorm.DB) bool {
		return statementOn(tx, "users") && statementSelects(tx, "can_edit_sources")
	}, func(*gorm.DB) { cancel() })

	response := fixture.serveJSON(ctx, http.MethodPost, "/api/sources", fixture.auth, `{"name":"取消新建","baseUrl":"https://cancel-create.example","enabled":true}`)
	fixture.requireFired(t, "source edit check", ctx)
	assertRemainingCancelledSilently(t, "source edit check cancelled", response)
	if sources := remainingCount(t, fixture.server, &models.BookSource{}, "1 = 1"); sources != sourcesBefore {
		t.Errorf("cancelled source edit check created a source: %d, want %d", sources, sourcesBefore)
	}
	assertRemainingNoEvents(t, "source edit check cancelled", events)
	assertRemainingCancellationLogQuiet(t, "source edit check cancelled", fixture.log)
}

// C6: default source status, save and restore cancelled before they commit.
func TestRemainingCancellationDefaultSourcesBeforeCommitAreSilent(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixture.install(t, remainingBeforeQuery, func(tx *gorm.DB) bool {
			return statementOn(tx, "book_source_namespaces") && !statementInTransaction(tx)
		}, func(*gorm.DB) { cancel() })

		response := fixture.serveJSON(ctx, http.MethodGet, "/api/sources/default", fixture.auth, "")
		fixture.requireFired(t, "default status", ctx)
		assertRemainingCancelledSilently(t, "default status cancelled", response)
		assertNoDefaultSourceState(t, fixture.server.db)
		assertRemainingCancellationLogQuiet(t, "default status cancelled", fixture.log)
	})

	t.Run("save transaction", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		createSourceThroughAPI(t, fixture.router, fixture.auth, `{"name":"Admin default","baseUrl":"https://cancel-save-default.example","enabled":true}`)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixture.install(t, remainingBeforeDelete, func(tx *gorm.DB) bool {
			return statementOn(tx, "user_book_sources") && statementInTransaction(tx)
		}, func(*gorm.DB) { cancel() })

		response := fixture.serveJSON(ctx, http.MethodPost, "/api/sources/default/save", fixture.auth, "")
		fixture.requireFired(t, "default save", ctx)
		assertRemainingCancelledSilently(t, "default save cancelled", response)
		assertNoDefaultSourceState(t, fixture.server.db)
		if remainingDefaultMirrorExists(t, fixture.server) {
			t.Error("cancelled default save wrote defaultBookSources.json")
		}
		assertRemainingCancellationLogQuiet(t, "default save cancelled", fixture.log)
	})

	t.Run("restore transaction", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		createSourceThroughAPI(t, fixture.router, fixture.auth, `{"name":"Admin default","baseUrl":"https://cancel-restore-default.example","enabled":true}`)
		if response := fixture.serveJSON(context.Background(), http.MethodPost, "/api/sources/default/save", fixture.auth, ""); response.Code != http.StatusOK {
			t.Fatalf("save default = %d %s", response.Code, compactRemainingBody(response))
		}
		reader := registerSourceContractAccount(t, fixture.router, "cancelrestore")
		createSourceThroughAPI(t, fixture.router, reader.Auth, `{"name":"Own source","baseUrl":"https://cancel-own-source.example","enabled":true}`)
		before := remainingCount(t, fixture.server, &models.UserBookSource{}, "user_id = ? AND detached = ?", reader.ID, false)
		events := fixture.events(reader.ID)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixture.install(t, remainingBeforeDelete, func(tx *gorm.DB) bool {
			return statementOn(tx, "source_failures") && statementInTransaction(tx)
		}, func(*gorm.DB) { cancel() })

		response := fixture.serveJSON(ctx, http.MethodPost, "/api/sources/default/restore", reader.Auth, "")
		fixture.requireFired(t, "default restore", ctx)
		assertRemainingCancelledSilently(t, "default restore cancelled", response)
		if after := remainingCount(t, fixture.server, &models.UserBookSource{}, "user_id = ? AND detached = ?", reader.ID, false); after != before {
			t.Errorf("cancelled restore changed the user's sources: %d, want %d", after, before)
		}
		assertRemainingNoEvents(t, "default restore cancelled", events)
		assertRemainingCancellationLogQuiet(t, "default restore cancelled", fixture.log)
	})
}

// C6-M / R2: once the default snapshot is committed, the mirror follows it
// even though the client has gone away.
func TestRemainingCancellationDefaultSnapshotAfterCommitStillMirrors(t *testing.T) {
	mirrorRead := func(tx *gorm.DB) bool { return statementOn(tx, "book_sources") && !statementInTransaction(tx) }

	t.Run("save default", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		createSourceThroughAPI(t, fixture.router, fixture.auth, `{"name":"Admin default","baseUrl":"https://cancel-mirror-save.example","enabled":true}`)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixture.install(t, remainingBeforeQuery, mirrorRead, func(*gorm.DB) { cancel() })

		response := fixture.serveJSON(ctx, http.MethodPost, "/api/sources/default/save", fixture.auth, "")
		fixture.requireFired(t, "default save mirror", ctx)
		if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"count":1}` {
			t.Errorf("default save cancelled after commit = %d %s, want the committed 200 {\"count\":1}", response.Code, compactRemainingBody(response))
		}
		assertRemainingDefaultSnapshot(t, "default save cancelled after commit", fixture.server, "https://cancel-mirror-save.example")
		assertRemainingCancellationLogQuiet(t, "default save cancelled after commit", fixture.log)
	})

	t.Run("set user sources as default", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		target := registerSourceContractAccount(t, fixture.router, "cancelmirror")
		createSourceThroughAPI(t, fixture.router, target.Auth, `{"name":"Target default","baseUrl":"https://cancel-mirror-target.example","enabled":true}`)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixture.install(t, remainingBeforeQuery, mirrorRead, func(*gorm.DB) { cancel() })

		path := "/api/admin/users/" + strconv.FormatUint(uint64(target.ID), 10) + "/sources/default"
		response := fixture.serveJSON(ctx, http.MethodPost, path, fixture.auth, "")
		fixture.requireFired(t, "set as default mirror", ctx)
		if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"count":1}` {
			t.Errorf("set as default cancelled after commit = %d %s, want the committed 200 {\"count\":1}", response.Code, compactRemainingBody(response))
		}
		assertRemainingDefaultSnapshot(t, "set as default cancelled after commit", fixture.server, "https://cancel-mirror-target.example")
		assertRemainingCancellationLogQuiet(t, "set as default cancelled after commit", fixture.log)
	})

	t.Run("first initialization from the legacy file", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		// Written after startup, so the first status request seeds the default namespace.
		writeDefaultSourceFixture(t, fixture.server.defaultBookSourcesPath(), "取消初始化", "https://cancel-mirror-initialize.example")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// Before the commit, DefaultStatus stops at the empty namespace count;
		// the first user_book_sources read outside a transaction is the read-back.
		fixture.install(t, remainingBeforeQuery, func(tx *gorm.DB) bool {
			return statementOn(tx, "user_book_sources") && !statementInTransaction(tx)
		}, func(*gorm.DB) { cancel() })

		response := fixture.serveJSON(ctx, http.MethodGet, "/api/sources/default", fixture.auth, "")
		fixture.requireFired(t, "default initialization read-back", ctx)
		if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"configured":true,"count":1}` {
			t.Errorf("default initialization cancelled after commit = %d %s, want the committed 200", response.Code, compactRemainingBody(response))
		}
		assertRemainingDefaultSnapshot(t, "default initialization cancelled after commit", fixture.server, "https://cancel-mirror-initialize.example")
		assertRemainingCancellationLogQuiet(t, "default initialization cancelled after commit", fixture.log)
	})
}

// C7: the single-source page search cancelled while the source is fetched.
func TestRemainingCancellationSingleSourceSearchIsSilent(t *testing.T) {
	fixture := newRemainingCancellationFixture(t)
	source := remainingSearchSource(t, fixture.server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requested atomic.Bool
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested.Store(true)
		cancel()
		<-request.Context().Done()
		return nil, request.Context().Err()
	})})
	defer restore()

	response := fixture.serveJSON(ctx, http.MethodPost, "/api/search", fixture.auth, remainingSearchBody(source.ID))
	if !requested.Load() || ctx.Err() == nil {
		t.Fatal("the search did not reach the source request")
	}
	assertRemainingCancelledSilently(t, "single-source search cancelled", response)
	if failures := remainingCount(t, fixture.server, &models.SourceFailure{}, "user_id = ?", fixture.admin.ID); failures != 0 {
		t.Errorf("cancelled search recorded %d source failures", failures)
	}
	assertRemainingCancellationLogQuiet(t, "single-source search cancelled", fixture.log)
}

// C8: every LocalStore management route cancelled during root initialization.
func TestRemainingCancellationLocalStoreRootInitializationIsSilent(t *testing.T) {
	for _, route := range []struct {
		name   string
		method string
		path   string
		body   func(t *testing.T) (io.Reader, string)
	}{
		{name: "list", method: http.MethodGet, path: "/api/local-store"},
		{name: "upload", method: http.MethodPost, path: "/api/local-store/upload", body: func(t *testing.T) (io.Reader, string) {
			return remainingLocalStoreUpload(t, "")
		}},
		{name: "download", method: http.MethodGet, path: "/api/local-store/download?path=book.txt"},
		{name: "new directory", method: http.MethodPost, path: "/api/local-store/directory", body: func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"name":"folder"}`), "application/json"
		}},
		{name: "rename", method: http.MethodPut, path: "/api/local-store/rename", body: func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"path":"book.txt","name":"renamed.txt"}`), "application/json"
		}},
		{name: "delete", method: http.MethodDelete, path: "/api/local-store?path=book.txt"},
	} {
		t.Run(route.name, func(t *testing.T) {
			fixture := newRemainingCancellationFixture(t)
			root := fixture.server.cfg.LocalStoreDir
			rootExisted := remainingPathExists(t, root)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// requireStorageAccess reads the store permission without the request
			// context; cancelling there makes the root initialization see it.
			fixture.install(t, remainingBeforeQuery, func(tx *gorm.DB) bool {
				return statementOn(tx, "users") && statementSelectsColumn(tx, "can_access_store")
			}, func(*gorm.DB) { cancel() })

			var body io.Reader
			contentType := ""
			if route.body != nil {
				body, contentType = route.body(t)
			}
			response := fixture.serve(ctx, route.method, route.path, fixture.auth, body, contentType)
			fixture.requireFired(t, route.name, ctx)
			assertRemainingCancelledSilently(t, "LocalStore "+route.name+" cancelled at root initialization", response)
			if !rootExisted && remainingPathExists(t, root) {
				t.Errorf("LocalStore %s cancelled at root initialization created the root", route.name)
			}
		})
	}
}

// C8: LocalStore reads and writes cancelled after root initialization.
func TestRemainingCancellationLocalStoreOperationsAreSilent(t *testing.T) {
	type storeCase struct {
		name    string
		read    string // beforeStoreReadTestHook action
		write   string // beforeLocalStoreWriteTestHook action
		seed    func(t *testing.T, root string)
		method  string
		path    string
		body    func(t *testing.T) (io.Reader, string)
		absent  []string
		present []string
	}
	jsonBody := func(text string) func(*testing.T) (io.Reader, string) {
		return func(*testing.T) (io.Reader, string) { return strings.NewReader(text), "application/json" }
	}
	seedOriginal := func(t *testing.T, root string) {
		writeRemainingFile(t, filepath.Join(root, "nested", "original.txt"), "original")
	}
	cases := []storeCase{
		{name: "list", read: "local-list", seed: seedOriginal, method: http.MethodGet, path: "/api/local-store?path=nested"},
		{name: "recursive list", read: "local-list", seed: seedOriginal, method: http.MethodGet, path: "/api/local-store?path=nested&recursive=1"},
		{name: "download", read: "local-download", seed: seedOriginal, method: http.MethodGet, path: "/api/local-store/download?path=nested/original.txt"},
		{
			name: "upload into a new directory", write: "local-upload", method: http.MethodPost, path: "/api/local-store/upload",
			body:   func(t *testing.T) (io.Reader, string) { return remainingLocalStoreUpload(t, "incoming") },
			absent: []string{"incoming"},
		},
		{
			name: "upload into the root", write: "local-upload", method: http.MethodPost, path: "/api/local-store/upload",
			body:   func(t *testing.T) (io.Reader, string) { return remainingLocalStoreUpload(t, "") },
			absent: []string{"book.txt"},
		},
		{
			name: "new directory with a parent", write: "local-directory", method: http.MethodPost, path: "/api/local-store/directory",
			body: jsonBody(`{"path":"parent","name":"child"}`), absent: []string{"parent"},
		},
		{
			name: "new directory in the root", write: "local-directory", method: http.MethodPost, path: "/api/local-store/directory",
			body: jsonBody(`{"name":"child"}`), absent: []string{"child"},
		},
		{
			name: "rename", write: "local-rename", method: http.MethodPut, path: "/api/local-store/rename",
			seed: func(t *testing.T, root string) { writeRemainingFile(t, filepath.Join(root, "book.txt"), "original") },
			body: jsonBody(`{"path":"book.txt","name":"renamed.txt"}`), absent: []string{"renamed.txt"}, present: []string{"book.txt"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRemainingCancellationFixture(t)
			root := fixture.server.cfg.LocalStoreDir
			if test.seed != nil {
				test.seed(t, root)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.read != "" {
				beforeStoreReadTestHook = func(action string) {
					if action == test.read && fixture.fireOnce() {
						cancel()
					}
				}
				t.Cleanup(func() { beforeStoreReadTestHook = nil })
			}
			if test.write != "" {
				beforeLocalStoreWriteTestHook = func(action string) {
					if action == test.write && fixture.fireOnce() {
						cancel()
					}
				}
				t.Cleanup(func() { beforeLocalStoreWriteTestHook = nil })
			}

			var body io.Reader
			contentType := ""
			if test.body != nil {
				body, contentType = test.body(t)
			}
			response := fixture.serve(ctx, test.method, test.path, fixture.auth, body, contentType)
			fixture.requireFired(t, test.name, ctx)
			assertRemainingCancelledSilently(t, "LocalStore "+test.name+" cancelled", response)
			for _, relative := range test.absent {
				if remainingPathExists(t, filepath.Join(root, relative)) {
					t.Errorf("LocalStore %s cancelled before writing created %s", test.name, relative)
				}
			}
			for _, relative := range test.present {
				if !remainingPathExists(t, filepath.Join(root, relative)) {
					t.Errorf("LocalStore %s cancelled before writing removed %s", test.name, relative)
				}
			}
			if test.read != "" {
				if data, err := os.ReadFile(filepath.Join(root, "nested", "original.txt")); err != nil || string(data) != "original" {
					t.Fatalf("cancelled read changed the fixture: %q %v", data, err)
				}
			}
		})
	}
}

// Controls: these pass before and after the fix.
func TestRemainingCancellationControls(t *testing.T) {
	t.Run("login and register issue a session", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		before := remainingSessionCount(t, fixture.server, fixture.admin.ID)
		login := fixture.serveJSON(context.Background(), http.MethodPost, "/api/auth/login", "", `{"username":"testuser","password":"test1234"}`)
		if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"token"`) {
			t.Fatalf("login = %d %s", login.Code, compactRemainingBody(login))
		}
		if after := remainingSessionCount(t, fixture.server, fixture.admin.ID); after != before+1 {
			t.Errorf("login sessions = %d, want %d", after, before+1)
		}
		account := registerSourceContractAccount(t, fixture.router, "controlregister")
		if sessions := remainingSessionCount(t, fixture.server, account.ID); sessions != 1 {
			t.Errorf("register sessions = %d, want 1", sessions)
		}
		assertRemainingCancellationLogQuiet(t, "login and register", fixture.log)
	})

	t.Run("token issue database failure", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		before := remainingSessionCount(t, fixture.server, fixture.admin.ID)
		fixture.install(t, remainingBeforeCreateCommit, func(tx *gorm.DB) bool { return statementOn(tx, "user_sessions") }, func(tx *gorm.DB) {
			_ = tx.AddError(errRemainingCancellationInjected)
		})
		response := fixture.serveJSON(context.Background(), http.MethodPost, "/api/auth/login", "", `{"username":"testuser","password":"test1234"}`)
		if !fixture.fired.Load() {
			t.Fatal("failure hook did not reach the session insert")
		}
		if response.Code != http.StatusInternalServerError || strings.TrimSpace(response.Body.String()) != `{"error":"failed to create token"}` {
			t.Errorf("token issue failure = %d %s", response.Code, compactRemainingBody(response))
		}
		if after := remainingSessionCount(t, fixture.server, fixture.admin.ID); after != before {
			t.Errorf("failed token issue left %d sessions, want %d", after, before)
		}
		if !strings.Contains(fixture.log.String(), errRemainingCancellationInjected.Error()) {
			t.Errorf("token issue failure was not logged:\n%s", fixture.log.String())
		}
	})

	t.Run("logout", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		response := fixture.serveJSON(context.Background(), http.MethodPost, "/api/auth/logout", fixture.auth, "")
		if response.Code != http.StatusNoContent {
			t.Fatalf("logout = %d %s", response.Code, compactRemainingBody(response))
		}
		if status := remainingMeStatus(fixture, fixture.auth); status != http.StatusUnauthorized {
			t.Errorf("revoked session still authenticates: /api/me = %d", status)
		}
	})

	t.Run("logout cancelled after the delete commits", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixture.install(t, remainingAfterDeleteCommit, func(tx *gorm.DB) bool {
			return statementOn(tx, "user_sessions") && tx.Error == nil
		}, func(*gorm.DB) { cancel() })
		response := fixture.serveJSON(ctx, http.MethodPost, "/api/auth/logout", fixture.auth, "")
		fixture.requireFired(t, "logout after commit", ctx)
		if response.Code != http.StatusNoContent {
			t.Errorf("logout cancelled after commit = %d %s, want 204", response.Code, compactRemainingBody(response))
		}
		if status := remainingMeStatus(fixture, fixture.auth); status != http.StatusUnauthorized {
			t.Errorf("committed logout left the session valid: /api/me = %d", status)
		}
	})

	t.Run("logout database failure", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		fixture.install(t, remainingBeforeDeleteCommit, func(tx *gorm.DB) bool { return statementOn(tx, "user_sessions") }, func(tx *gorm.DB) {
			_ = tx.AddError(errRemainingCancellationInjected)
		})
		response := fixture.serveJSON(context.Background(), http.MethodPost, "/api/auth/logout", fixture.auth, "")
		if !fixture.fired.Load() {
			t.Fatal("failure hook did not reach the session delete")
		}
		if response.Code != http.StatusInternalServerError || strings.TrimSpace(response.Body.String()) != `{"error":"failed to revoke session"}` {
			t.Errorf("logout failure = %d %s", response.Code, compactRemainingBody(response))
		}
		if status := remainingMeStatus(fixture, fixture.auth); status != http.StatusOK {
			t.Errorf("failed logout revoked the session: /api/me = %d", status)
		}
	})

	t.Run("sync handshake", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		httpServer := httptest.NewServer(fixture.router)
		defer httpServer.Close()
		connection, response, err := dialSyncWebSocket(httpServer.URL, bearerToken(fixture.auth), httpServer.URL)
		if err != nil {
			t.Fatalf("sync handshake: status=%v err=%v", responseStatus(response), err)
		}
		_ = connection.Close()
	})

	t.Run("sync invalid token and database failure", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		invalid := fixture.serveJSON(context.Background(), http.MethodGet, "/ws/sync?token=not-a-token", "", "")
		if invalid.Code != http.StatusUnauthorized || strings.TrimSpace(invalid.Body.String()) != `{"error":"invalid token"}` {
			t.Errorf("invalid sync token = %d %s", invalid.Code, compactRemainingBody(invalid))
		}
		fixture.install(t, remainingBeforeQuery, func(tx *gorm.DB) bool {
			return statementOn(tx, "users") && statementSelects(tx, "id", "auth_version")
		}, func(tx *gorm.DB) { _ = tx.AddError(errRemainingCancellationInjected) })
		failed := fixture.serveJSON(context.Background(), http.MethodGet, "/ws/sync?token="+url.QueryEscape(bearerToken(fixture.auth)), "", "")
		if !fixture.fired.Load() {
			t.Fatal("failure hook did not reach the sync user lookup")
		}
		if failed.Code != http.StatusInternalServerError || strings.TrimSpace(failed.Body.String()) != `{"error":"failed to authenticate sync connection"}` {
			t.Errorf("sync authentication failure = %d %s", failed.Code, compactRemainingBody(failed))
		}
	})

	t.Run("admin permission and writes", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		target := registerSourceContractAccount(t, fixture.router, "controltarget")
		if response := fixture.serveJSON(context.Background(), http.MethodGet, "/api/admin/users", target.Auth, ""); response.Code != http.StatusForbidden {
			t.Errorf("non-admin list users = %d %s, want 403", response.Code, compactRemainingBody(response))
		}

		adminEvents := fixture.events(fixture.admin.ID)
		reset := fixture.serveJSON(context.Background(), http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10)+"/password", fixture.auth, `{"password":"control-reset-2"}`)
		if reset.Code != http.StatusOK || strings.TrimSpace(reset.Body.String()) != `{"success":true}` {
			t.Errorf("reset password = %d %s", reset.Code, compactRemainingBody(reset))
		}
		if events := drainSettingContractEvents(adminEvents); len(events) != 1 || !strings.Contains(events[0], `"users_update"`) {
			t.Errorf("reset password events = %q, want one users_update", events)
		}
		if sessions := remainingSessionCount(t, fixture.server, target.ID); sessions != 0 {
			t.Errorf("reset password left %d target sessions", sessions)
		}

		fixture.install(t, remainingBeforeDelete, func(tx *gorm.DB) bool {
			return statementOn(tx, "users") && statementInTransaction(tx)
		}, func(tx *gorm.DB) { _ = tx.AddError(errRemainingCancellationInjected) })
		deleted := fixture.serveJSON(context.Background(), http.MethodPost, "/api/admin/users/batch-delete", fixture.auth, `{"ids":[`+strconv.FormatUint(uint64(target.ID), 10)+`]}`)
		if !fixture.fired.Load() {
			t.Fatal("failure hook did not reach the user delete")
		}
		if deleted.Code != http.StatusInternalServerError || !strings.Contains(deleted.Body.String(), "failed to delete users") {
			t.Errorf("delete users failure = %d %s", deleted.Code, compactRemainingBody(deleted))
		}
		if remainingCount(t, fixture.server, &models.User{}, "id = ?", target.ID) != 1 {
			t.Error("failed deletion removed the target account")
		}
		assertRemainingNoEvents(t, "failed deletion", adminEvents)
	})

	t.Run("source edit permission", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		target := registerSourceContractAccount(t, fixture.router, "controleditor")
		if err := fixture.server.db.Model(&models.User{}).Where("id = ?", target.ID).Update("can_edit_sources", false).Error; err != nil {
			t.Fatal(err)
		}
		response := fixture.serveJSON(context.Background(), http.MethodPost, "/api/sources", target.Auth, `{"name":"Denied","baseUrl":"https://control-denied.example","enabled":true}`)
		if response.Code != http.StatusForbidden {
			t.Errorf("source edit without permission = %d %s, want 403", response.Code, compactRemainingBody(response))
		}
	})

	t.Run("default sources", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		status := fixture.serveJSON(context.Background(), http.MethodGet, "/api/sources/default", fixture.auth, "")
		if status.Code != http.StatusOK || strings.TrimSpace(status.Body.String()) != `{"configured":false,"count":0}` {
			t.Errorf("default status = %d %s", status.Code, compactRemainingBody(status))
		}
		createSourceThroughAPI(t, fixture.router, fixture.auth, `{"name":"Admin default","baseUrl":"https://control-default.example","enabled":true}`)
		save := fixture.serveJSON(context.Background(), http.MethodPost, "/api/sources/default/save", fixture.auth, "")
		if save.Code != http.StatusOK || strings.TrimSpace(save.Body.String()) != `{"count":1}` {
			t.Errorf("default save = %d %s", save.Code, compactRemainingBody(save))
		}
		assertRemainingDefaultSnapshot(t, "default save", fixture.server, "https://control-default.example")

		reader := registerSourceContractAccount(t, fixture.router, "controlrestore")
		createSourceThroughAPI(t, fixture.router, reader.Auth, `{"name":"Own source","baseUrl":"https://control-own.example","enabled":true}`)
		events := fixture.events(reader.ID)
		restore := fixture.serveJSON(context.Background(), http.MethodPost, "/api/sources/default/restore", reader.Auth, "")
		if restore.Code != http.StatusOK {
			t.Errorf("default restore = %d %s", restore.Code, compactRemainingBody(restore))
		}
		if drained := drainSettingContractEvents(events); len(drained) != 1 || !strings.Contains(drained[0], `"sources_update"`) {
			t.Errorf("default restore events = %q, want one sources_update", drained)
		}
		assertRemainingCancellationLogQuiet(t, "default sources", fixture.log)
	})

	t.Run("single-source search", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		source := remainingSearchSource(t, fixture.server)
		var fail atomic.Bool
		restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if fail.Load() {
				return nil, errors.New("control upstream failure")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<article class="book"><a class="name" href="/book/1">取消对照书</a></article>`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		})})
		defer restore()

		found := fixture.serveJSON(context.Background(), http.MethodPost, "/api/search", fixture.auth, remainingSearchBody(source.ID))
		if found.Code != http.StatusOK || !strings.Contains(found.Body.String(), "取消对照书") {
			t.Errorf("single-source search = %d %s", found.Code, compactRemainingBody(found))
		}
		fail.Store(true)
		failed := fixture.serveJSON(context.Background(), http.MethodPost, "/api/search", fixture.auth, remainingSearchBody(source.ID))
		if failed.Code != http.StatusBadGateway || !strings.Contains(failed.Body.String(), `"failed to search source"`) {
			t.Errorf("single-source search failure = %d %s, want 502", failed.Code, compactRemainingBody(failed))
		}
		if failures := remainingCount(t, fixture.server, &models.SourceFailure{}, "user_id = ? AND source_id = ?", fixture.admin.ID, source.ID); failures != 1 {
			t.Errorf("live search failure recorded %d source failures, want 1", failures)
		}
	})

	t.Run("local store", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		root := fixture.server.cfg.LocalStoreDir
		background := context.Background()
		body, contentType := remainingLocalStoreUpload(t, "")
		if upload := fixture.serve(background, http.MethodPost, "/api/local-store/upload", fixture.auth, body, contentType); upload.Code != http.StatusCreated {
			t.Fatalf("upload = %d %s", upload.Code, compactRemainingBody(upload))
		}
		if list := fixture.serveJSON(background, http.MethodGet, "/api/local-store", fixture.auth, ""); list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"book.txt"`) {
			t.Errorf("list = %d %s", list.Code, compactRemainingBody(list))
		}
		if download := fixture.serveJSON(background, http.MethodGet, "/api/local-store/download?path=book.txt", fixture.auth, ""); download.Code != http.StatusOK || download.Body.String() != "cancelled upload" {
			t.Errorf("download = %d %s", download.Code, compactRemainingBody(download))
		}
		if directory := fixture.serveJSON(background, http.MethodPost, "/api/local-store/directory", fixture.auth, `{"path":"parent","name":"child"}`); directory.Code != http.StatusCreated {
			t.Errorf("new directory = %d %s", directory.Code, compactRemainingBody(directory))
		}
		if existing := fixture.serveJSON(background, http.MethodPost, "/api/local-store/directory", fixture.auth, `{"path":"parent","name":"child"}`); existing.Code != http.StatusConflict {
			t.Errorf("existing directory = %d %s, want 409", existing.Code, compactRemainingBody(existing))
		}
		if rename := fixture.serveJSON(background, http.MethodPut, "/api/local-store/rename", fixture.auth, `{"path":"book.txt","name":"renamed.txt"}`); rename.Code != http.StatusOK {
			t.Errorf("rename = %d %s", rename.Code, compactRemainingBody(rename))
		}
		if !remainingPathExists(t, filepath.Join(root, "renamed.txt")) {
			t.Error("rename did not move the file")
		}
		if missing := fixture.serveJSON(background, http.MethodGet, "/api/local-store?path=missing", fixture.auth, ""); missing.Code != http.StatusNotFound {
			t.Errorf("missing path = %d %s, want 404", missing.Code, compactRemainingBody(missing))
		}
		if regular := fixture.serveJSON(background, http.MethodGet, "/api/local-store?path=renamed.txt", fixture.auth, ""); regular.Code != http.StatusInternalServerError {
			t.Errorf("listing a regular file = %d %s, want the existing 500", regular.Code, compactRemainingBody(regular))
		}
	})

	t.Run("local store import keeps its cancellation 500", func(t *testing.T) {
		fixture := newRemainingCancellationFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixture.install(t, remainingBeforeQuery, func(tx *gorm.DB) bool {
			return statementOn(tx, "users") && statementSelectsColumn(tx, "can_access_store")
		}, func(*gorm.DB) { cancel() })
		response := fixture.serveJSON(ctx, http.MethodPost, "/api/local-store/import-preview", fixture.auth, `{"paths":["book.txt"]}`)
		fixture.requireFired(t, "import preview root initialization", ctx)
		if response.Code != http.StatusInternalServerError {
			t.Errorf("import preview cancelled at root initialization = %d %s, want the import flows' 500", response.Code, compactRemainingBody(response))
		}
	})
}
