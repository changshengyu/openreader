package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"openreader/backend/models"
	"openreader/backend/services/authsession"
)

type bookPatchConcurrentCommitFixture struct {
	router *gin.Engine
	server *Server
	auth   string
	token  string
	book   models.Book
	// sessions authenticates like another request, but on its own
	// busy_timeout=0 connection: its session renew commits at once when the
	// write lock is free and fails with SQLITE_BUSY instead of waiting.
	sessions *authsession.Service
}

type bookPatchConcurrentCommitCase struct {
	name    string
	prepare func(*testing.T, bookPatchConcurrentCommitFixture) (string, func(*testing.T, models.Book))
}

func bookPatchConcurrentCommitCases() []bookPatchConcurrentCommitCase {
	return []bookPatchConcurrentCommitCase{
		{
			name: "follow",
			prepare: func(t *testing.T, fixture bookPatchConcurrentCommitFixture) (string, func(*testing.T, models.Book)) {
				return `{"canUpdate":false}`, func(t *testing.T, book models.Book) {
					if book.CanUpdate {
						t.Errorf("stored canUpdate = true, want false")
					}
				}
			},
		},
		{
			name: "cover",
			prepare: func(t *testing.T, fixture bookPatchConcurrentCommitFixture) (string, func(*testing.T, models.Book)) {
				coverURL := uploadBookInfoCover(t, fixture.router, fixture.auth, "concurrent-cover.png")
				return fmt.Sprintf(`{"customCoverUrl":%q}`, coverURL), func(t *testing.T, book models.Book) {
					if book.CustomCoverURL != coverURL {
						t.Errorf("stored customCoverUrl = %q, want %q", book.CustomCoverURL, coverURL)
					}
				}
			},
		},
	}
}

func newBookPatchConcurrentCommitFixture(t *testing.T, username string) bookPatchConcurrentCommitFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth, user := registerBookInfoAssetUser(t, router, username)
	book := models.Book{UserID: user.ID, Title: "并发提交书", CanUpdate: true}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	other, err := gorm.Open(sqlite.Open(server.cfg.DatabasePath+"?_busy_timeout=0"), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherSQL, err := other.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherSQL.Close() })
	return bookPatchConcurrentCommitFixture{
		router:   router,
		server:   server,
		auth:     auth,
		token:    strings.TrimPrefix(auth, "Bearer "),
		book:     book,
		sessions: authsession.New(other, server.cfg.JWTSecret, server.cfg.JWTPreviousSecrets...),
	}
}

func performBookPatchConcurrentCommitRequest(fixture bookPatchConcurrentCommitFixture, body string) *httptest.ResponseRecorder {
	path := "/api/books/" + strconv.FormatUint(uint64(fixture.book.ID), 10)
	return performUserAssetLifecycleJSON(fixture.router, fixture.auth, http.MethodPut, path, body, context.Background())
}

func loadBookPatchConcurrentCommitBook(t *testing.T, fixture bookPatchConcurrentCommitFixture) models.Book {
	t.Helper()
	var book models.Book
	if err := fixture.server.db.First(&book, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	return book
}

func isBookPatchSQLiteBusy(err error) bool {
	var sqliteError sqlite3.Error
	return errors.As(err, &sqliteError) && sqliteError.Code == sqlite3.ErrBusy
}

type bookPatchInTransactionRenew struct {
	attempted bool
	err       error
}

// installBookPatchInTransactionRenew renews the fixture session from another
// connection right after PUT /api/books/:id reads the book inside its
// transaction, the moment another authenticated request commits in practice.
func installBookPatchInTransactionRenew(t *testing.T, fixture bookPatchConcurrentCommitFixture) *bookPatchInTransactionRenew {
	t.Helper()
	renew := &bookPatchInTransactionRenew{}
	const callbackName = "test:book-patch-in-transaction-session-renew"
	if err := fixture.server.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if renew.attempted || tx.Statement.Table != "books" {
			return
		}
		if _, inTransaction := tx.Statement.ConnPool.(*sql.Tx); !inTransaction {
			return
		}
		renew.attempted = true
		_, renew.err = fixture.sessions.Authenticate(context.Background(), fixture.token)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.server.db.Callback().Query().Remove(callbackName) })
	return renew
}

func TestBookPatchSurvivesConcurrentSessionRenew(t *testing.T) {
	for _, testCase := range bookPatchConcurrentCommitCases() {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newBookPatchConcurrentCommitFixture(t, "concurrentrenew"+testCase.name)
			body, check := testCase.prepare(t, fixture)
			renew := installBookPatchInTransactionRenew(t, fixture)

			response := performBookPatchConcurrentCommitRequest(fixture, body)
			if !renew.attempted {
				t.Fatal("PUT /api/books/:id never read the book inside its transaction; the concurrent renew did not run")
			}
			if response.Code != http.StatusOK {
				t.Fatalf("PUT %s while another authenticated request renewed its session = %d %s, want 200 (concurrent renew error: %v)",
					body, response.Code, response.Body.String(), renew.err)
			}
			check(t, loadBookPatchConcurrentCommitBook(t, fixture))
			if renew.err != nil {
				if !isBookPatchSQLiteBusy(renew.err) {
					t.Fatalf("concurrent renew = %v, want success or SQLITE_BUSY while the PUT held the write lock", renew.err)
				}
				if _, err := fixture.sessions.Authenticate(context.Background(), fixture.token); err != nil {
					t.Fatalf("concurrent renew after the PUT committed: %v", err)
				}
			}
		})
	}
}

func TestBookPatchConcurrentCommitControls(t *testing.T) {
	for _, testCase := range bookPatchConcurrentCommitCases() {
		t.Run(testCase.name+"/no-concurrent-commit", func(t *testing.T) {
			fixture := newBookPatchConcurrentCommitFixture(t, "concurrentnone"+testCase.name)
			body, check := testCase.prepare(t, fixture)

			response := performBookPatchConcurrentCommitRequest(fixture, body)
			if response.Code != http.StatusOK {
				t.Fatalf("PUT %s = %d %s, want 200", body, response.Code, response.Body.String())
			}
			check(t, loadBookPatchConcurrentCommitBook(t, fixture))
		})
		t.Run(testCase.name+"/commit-before-transaction", func(t *testing.T) {
			fixture := newBookPatchConcurrentCommitFixture(t, "concurrentbefore"+testCase.name)
			body, check := testCase.prepare(t, fixture)
			renewed := false
			var renewErr error
			bookPatchWriteLifecycleTestHook = func(action string) {
				if action != "metadata" || renewed {
					return
				}
				renewed = true
				_, renewErr = fixture.sessions.Authenticate(context.Background(), fixture.token)
			}
			t.Cleanup(func() { bookPatchWriteLifecycleTestHook = nil })

			response := performBookPatchConcurrentCommitRequest(fixture, body)
			if !renewed {
				t.Fatal("PUT /api/books/:id did not reach the pre-transaction hook")
			}
			if renewErr != nil {
				t.Fatalf("renew committed before the transaction: %v", renewErr)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("PUT %s after a committed renew = %d %s, want 200", body, response.Code, response.Body.String())
			}
			check(t, loadBookPatchConcurrentCommitBook(t, fixture))
		})
	}
}
