package api

import (
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mattn/go-sqlite3"

	"openreader/backend/models"
)

// sqliteCommitCounters maps a canonical database file path to the number of
// write transactions committed on that file by any connection of this test
// binary. Only databases a test registers with countSQLiteCommits are counted.
var sqliteCommitCounters sync.Map

// init gives every SQLite connection this test binary opens a commit hook,
// before any test runs. SQLite calls the hook whenever a write transaction
// commits, including a BEGIN IMMEDIATE or an UPDATE that changed no row, and
// never for reads, so the count is the number of times the write lock was
// taken and committed.
func init() {
	probe, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	driver, ok := probe.Driver().(*sqlite3.SQLiteDriver)
	_ = probe.Close()
	if !ok {
		panic("the sqlite3 database/sql driver is not mattn/go-sqlite3")
	}
	driver.ConnectHook = func(conn *sqlite3.SQLiteConn) error {
		path := canonicalSQLitePath(conn.GetFilename(""))
		conn.RegisterCommitHook(func() int {
			if counter, ok := sqliteCommitCounters.Load(path); ok {
				counter.(*atomic.Int64).Add(1)
			}
			return 0
		})
		return nil
	}
}

// canonicalSQLitePath resolves symbolic links the way SQLite reports a
// database filename (macOS temporary directories live under a /var link).
func canonicalSQLitePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func countSQLiteCommits(t *testing.T, databasePath string) *atomic.Int64 {
	t.Helper()
	path := canonicalSQLitePath(databasePath)
	counter := &atomic.Int64{}
	if _, loaded := sqliteCommitCounters.LoadOrStore(path, counter); loaded {
		t.Fatalf("commit counter for %s is already registered", path)
	}
	t.Cleanup(func() { sqliteCommitCounters.Delete(path) })
	return counter
}

type authenticatedCommitRequest struct {
	name    string
	status  int
	perform func() *httptest.ResponseRecorder
}

func authenticatedCommitRequests(router *gin.Engine, username, password, token string) []authenticatedCommitRequest {
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	return []authenticatedCommitRequest{
		{
			name:   "REST GET /api/me",
			status: http.StatusOK,
			perform: func() *httptest.ResponseRecorder {
				return sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", token)
			},
		},
		{
			name:   "WebDAV Bearer GET /webdav/",
			status: http.StatusMultiStatus,
			perform: func() *httptest.ResponseRecorder {
				return sessionContractJSONRequest(router, http.MethodGet, "/webdav/", "", token)
			},
		},
		{
			name:   "WebDAV Basic GET /webdav/",
			status: http.StatusMultiStatus,
			perform: func() *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodGet, "/webdav/", nil)
				request.Header.Set("Authorization", basic)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				return response
			},
		},
	}
}

func TestAuthenticatedRequestsWithinAMinuteCommitNothing(t *testing.T) {
	router, server := setupTestServer(t)
	issued := registerSessionContractUser(t, router, "commitwithin", "password8")
	commits := countSQLiteCommits(t, server.cfg.DatabasePath)

	for round := 1; round <= 3; round++ {
		for _, request := range authenticatedCommitRequests(router, "commitwithin", "password8", issued.Token) {
			before := commits.Load()
			response := request.perform()
			if response.Code != request.status {
				t.Fatalf("%s (round %d) = %d %s, want %d", request.name, round, response.Code, response.Body.String(), request.status)
			}
			if got := commits.Load() - before; got != 0 {
				t.Errorf("%s (round %d) committed %d SQLite write transactions within a minute of the session's last renewal, want 0",
					request.name, round, got)
			}
		}
	}
}

func TestAuthenticatedRequestAfterAMinuteRenewsSessionAndActivityOnce(t *testing.T) {
	router, server := setupTestServer(t)
	issued := registerSessionContractUser(t, router, "commitafter", "password8")
	staleAt := time.Now().UTC().Add(-2 * time.Minute)
	if result := server.db.Model(&models.UserSession{}).Where("user_id = ?", issued.User.ID).
		UpdateColumns(map[string]any{"last_seen_at": staleAt, "expires_at": staleAt.Add(7 * 24 * time.Hour)}); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("age session: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if result := server.db.Model(&models.User{}).Where("id = ?", issued.User.ID).
		UpdateColumn("last_active_at", staleAt); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("age activity: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	commits := countSQLiteCommits(t, server.cfg.DatabasePath)

	requestedAt := time.Now().UTC()
	first := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", issued.Token)
	if first.Code != http.StatusOK {
		t.Fatalf("GET /api/me two minutes after the last renewal = %d %s, want 200", first.Code, first.Body.String())
	}
	if got := commits.Load(); got != 2 {
		t.Errorf("GET /api/me two minutes after the last renewal committed %d SQLite write transactions, want 2 (session renew and last_active_at)", got)
	}
	var session models.UserSession
	if err := server.db.Where("user_id = ?", issued.User.ID).First(&session).Error; err != nil {
		t.Fatal(err)
	}
	if session.LastSeenAt.Before(requestedAt) || !session.ExpiresAt.Equal(session.LastSeenAt.Add(7*24*time.Hour)) {
		t.Errorf("renewed session last_seen_at=%s expires_at=%s, want last_seen_at >= %s and expires_at = last_seen_at + 7d",
			session.LastSeenAt, session.ExpiresAt, requestedAt)
	}
	var user models.User
	if err := server.db.First(&user, issued.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.LastActiveAt.Before(requestedAt) {
		t.Errorf("last_active_at=%s after the first request, want >= %s", user.LastActiveAt, requestedAt)
	}

	before := commits.Load()
	second := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", issued.Token)
	if second.Code != http.StatusOK {
		t.Fatalf("second GET /api/me = %d %s, want 200", second.Code, second.Body.String())
	}
	if got := commits.Load() - before; got != 0 {
		t.Errorf("GET /api/me right after a renewal committed %d SQLite write transactions, want 0", got)
	}
}
