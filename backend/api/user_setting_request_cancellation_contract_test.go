package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

// Contract: docs/compat/user-setting-write-request-cancellation-p2-contract.md sections 5 and 6.

var userSettingCancellationKeys = []string{"shelf", "reader", "search"}

var errUserSettingCancellationInjected = errors.New("injected user setting failure")

type userSettingCancellationLog struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (l *userSettingCancellationLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.Write(data)
}

func (l *userSettingCancellationLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.String()
}

type userSettingCancellationFixture struct {
	router *gin.Engine
	server *Server
	auth   string
	user   models.User
	events <-chan []byte
	log    *userSettingCancellationLog
	armed  atomic.Bool
	fired  atomic.Bool
}

func newUserSettingCancellationFixture(t *testing.T) *userSettingCancellationFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := loadSettingContractUser(t, server)
	output := &userSettingCancellationLog{}
	// Handlers copy the root Config on WithContext, so every later statement uses this logger.
	server.db.Logger = readerdb.NewLogger(log.New(output, "", 0))
	return &userSettingCancellationFixture{
		router: router,
		server: server,
		auth:   auth,
		user:   user,
		events: server.hub.AddClient(user.ID, nil).Send,
		log:    output,
	}
}

// fireOnce reports whether the armed request reached the hook for the first time.
func (f *userSettingCancellationFixture) fireOnce(tx *gorm.DB) bool {
	return tx.Statement.Table == "user_settings" && f.armed.Load() && f.fired.CompareAndSwap(false, true)
}

type userSettingCancellationHook struct {
	name     string
	register func(*gorm.DB, string, func(*gorm.DB)) error
	remove   func(*gorm.DB, string) error
}

var (
	userSettingCancelBeforeRead = userSettingCancellationHook{
		name: "test:user-setting-cancel-before-read",
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Query().Before("gorm:query").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Query().Remove(name) },
	}
	userSettingCancelInsideUpsert = userSettingCancellationHook{
		name: "test:user-setting-cancel-inside-upsert",
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Create().Before("gorm:create").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Create().Remove(name) },
	}
	userSettingCancelAfterCommit = userSettingCancellationHook{
		name: "test:user-setting-cancel-after-commit",
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Create().After("gorm:commit_or_rollback_transaction").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Create().Remove(name) },
	}
	// GORM appends an After("gorm:create") callback after the commit, so the
	// failure is injected right before the commit instead: the INSERT has run
	// inside the transaction and the error makes GORM roll it back.
	userSettingFailBeforeCommit = userSettingCancellationHook{
		name: "test:user-setting-fail-before-commit",
		register: func(database *gorm.DB, name string, fn func(*gorm.DB)) error {
			return database.Callback().Create().Before("gorm:commit_or_rollback_transaction").Register(name, fn)
		},
		remove: func(database *gorm.DB, name string) error { return database.Callback().Create().Remove(name) },
	}
)

func (f *userSettingCancellationFixture) install(t *testing.T, hook userSettingCancellationHook, fn func(*gorm.DB)) {
	t.Helper()
	if err := hook.register(f.server.db, hook.name, func(tx *gorm.DB) {
		if f.fireOnce(tx) {
			fn(tx)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hook.remove(f.server.db, hook.name) })
}

func (f *userSettingCancellationFixture) put(ctx context.Context, key string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPut, "/api/settings/"+key, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Authorization", f.auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.armed.Store(true)
	f.router.ServeHTTP(response, request)
	f.armed.Store(false)
	return response
}

func userSettingCancellationBody(marker string) string {
	return fmt.Sprintf(`{"value":{"marker":%q,"theme":"night"},"force":true}`, marker)
}

func (f *userSettingCancellationFixture) loadRow(t *testing.T, key string) (models.UserSetting, bool) {
	t.Helper()
	var rows []models.UserSetting
	if err := f.server.db.Where("user_id = ? AND key = ?", f.user.ID, key).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return models.UserSetting{}, false
	}
	return rows[0], true
}

func assertUserSettingCancelledWithoutResponse(t *testing.T, label string, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code == http.StatusInternalServerError || response.Body.Len() != 0 {
		t.Errorf("%s wrote a response after the client went away: %s", label, compactSettingContractResponse(response))
	}
}

// assertUserSettingCancellationLogQuiet checks the production GORM logger printed no
// error for the cancelled request. Two settings.go lines predate this contract and
// stay allowed: slow-statement warnings, and the first read's "record not found"
// when the key has no row yet.
func assertUserSettingCancellationLogQuiet(t *testing.T, label string, output *userSettingCancellationLog) {
	t.Helper()
	text := output.String()
	for _, ended := range []string{"context canceled", "context deadline exceeded", "transaction has already been committed or rolled back"} {
		if strings.Contains(text, ended) {
			t.Errorf("%s logged %q:\n%s", label, ended, text)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "settings.go:") && !strings.Contains(line, "SLOW SQL") && !strings.Contains(line, "record not found") {
			t.Errorf("%s logged a settings statement error: %q", label, line)
		}
	}
}

func assertUserSettingCancellationNoEvents(t *testing.T, label string, events <-chan []byte) {
	t.Helper()
	if drained := drainSettingContractEvents(events); len(drained) != 0 {
		t.Errorf("%s broadcast events: %q", label, drained)
	}
}

func TestUserSettingWriteCancelledBeforeReadIsSilent(t *testing.T) {
	for _, key := range userSettingCancellationKeys {
		t.Run(key, func(t *testing.T) {
			fixture := newUserSettingCancellationFixture(t)
			baseline := replaceSettingContractRow(t, fixture.server, fixture.user.ID, key, `{"marker":"before"}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.install(t, userSettingCancelBeforeRead, func(*gorm.DB) { cancel() })

			response := fixture.put(ctx, key, userSettingCancellationBody("cancelled-before-read"))
			if !fixture.fired.Load() {
				t.Fatal("cancellation hook did not reach the setting read")
			}
			assertUserSettingCancelledWithoutResponse(t, "write cancelled before the read", response)
			assertSettingContractRowUnchanged(t, fixture.server, baseline)
			assertUserSettingCancellationNoEvents(t, "write cancelled before the read", fixture.events)
			assertUserSettingCancellationLogQuiet(t, "write cancelled before the read", fixture.log)
		})
	}
}

func TestUserSettingWriteCancelledInsideUpsertLeavesNoWrite(t *testing.T) {
	for _, key := range userSettingCancellationKeys {
		for _, existing := range []bool{false, true} {
			name := key + "/new-row"
			if existing {
				name = key + "/existing-row"
			}
			t.Run(name, func(t *testing.T) {
				fixture := newUserSettingCancellationFixture(t)
				var baseline models.UserSetting
				if existing {
					baseline = replaceSettingContractRow(t, fixture.server, fixture.user.ID, key, `{"marker":"before"}`)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				fixture.install(t, userSettingCancelInsideUpsert, func(*gorm.DB) { cancel() })

				response := fixture.put(ctx, key, userSettingCancellationBody("cancelled-inside-upsert"))
				if !fixture.fired.Load() {
					t.Fatal("cancellation hook did not reach the upsert")
				}
				assertUserSettingCancelledWithoutResponse(t, "write cancelled inside the upsert", response)
				if existing {
					assertSettingContractRowUnchanged(t, fixture.server, baseline)
				} else if row, found := fixture.loadRow(t, key); found {
					t.Errorf("write cancelled inside the upsert created a row: value=%q", settingContractValuePrefix(row.Value))
				}
				assertUserSettingCancellationNoEvents(t, "write cancelled inside the upsert", fixture.events)
				assertUserSettingCancellationLogQuiet(t, "write cancelled inside the upsert", fixture.log)
			})
		}
	}
}

func TestUserSettingWriteCancelledAfterCommitStillNotifies(t *testing.T) {
	for _, key := range userSettingCancellationKeys {
		t.Run(key, func(t *testing.T) {
			fixture := newUserSettingCancellationFixture(t)
			replaceSettingContractRow(t, fixture.server, fixture.user.ID, key, `{"marker":"before"}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.install(t, userSettingCancelAfterCommit, func(tx *gorm.DB) {
				if tx.Error == nil {
					cancel()
				}
			})

			response := fixture.put(ctx, key, userSettingCancellationBody("cancelled-after-commit"))
			if !fixture.fired.Load() || ctx.Err() == nil {
				t.Fatal("cancellation hook did not run after the upsert committed")
			}
			row, found := fixture.loadRow(t, key)
			if !found || !strings.Contains(row.Value, `"cancelled-after-commit"`) {
				t.Fatalf("committed write is not durable: found=%v value=%q", found, settingContractValuePrefix(row.Value))
			}
			if response.Code != http.StatusOK {
				t.Errorf("write cancelled after commit = %s, want the committed 200 response", compactSettingContractResponse(response))
			} else {
				assertSettingContractResponseValue(t, response.Body.Bytes(), `{"marker":"cancelled-after-commit","theme":"night"}`)
			}

			events := drainSettingContractEvents(fixture.events)
			if len(events) != 1 {
				t.Errorf("write cancelled after commit broadcast %d events, want one settings_update: %q", len(events), events)
			} else {
				var event struct {
					Type    string `json:"type"`
					Payload struct {
						Key       string    `json:"key"`
						UpdatedAt time.Time `json:"updatedAt"`
					} `json:"payload"`
				}
				if err := json.Unmarshal([]byte(events[0]), &event); err != nil {
					t.Fatalf("decode settings_update: %v", err)
				}
				if event.Type != "settings_update" || event.Payload.Key != key || !event.Payload.UpdatedAt.Equal(row.UpdatedAt) {
					t.Errorf("settings_update = %s, want key %s at the stored %s", events[0], key, row.UpdatedAt.Format(time.RFC3339Nano))
				}
			}
			assertUserSettingCancellationLogQuiet(t, "write cancelled after commit", fixture.log)
		})
	}
}

func TestUserSettingWriteCancellationControls(t *testing.T) {
	t.Run("uncancelled write", func(t *testing.T) {
		fixture := newUserSettingCancellationFixture(t)
		response := fixture.put(context.Background(), "shelf", userSettingCancellationBody("uncancelled"))
		if response.Code != http.StatusOK {
			t.Fatalf("uncancelled write = %s", compactSettingContractResponse(response))
		}
		assertSettingContractResponseValue(t, response.Body.Bytes(), `{"marker":"uncancelled","theme":"night"}`)
		if events := drainSettingContractEvents(fixture.events); len(events) != 1 || !strings.Contains(events[0], `"settings_update"`) {
			t.Errorf("uncancelled write events = %q, want one settings_update", events)
		}
		assertUserSettingCancellationLogQuiet(t, "uncancelled write", fixture.log)
	})

	t.Run("database failure on a live request", func(t *testing.T) {
		fixture := newUserSettingCancellationFixture(t)
		baseline := replaceSettingContractRow(t, fixture.server, fixture.user.ID, "reader", `{"marker":"before"}`)
		fixture.install(t, userSettingFailBeforeCommit, func(tx *gorm.DB) {
			_ = tx.AddError(errUserSettingCancellationInjected)
		})

		response := fixture.put(context.Background(), "reader", userSettingCancellationBody("failed"))
		if !fixture.fired.Load() {
			t.Fatal("failure hook did not reach the upsert")
		}
		assertSettingContractError(t, response, http.StatusInternalServerError, `{"error":"failed to save setting"}`)
		assertSettingContractRowUnchanged(t, fixture.server, baseline)
		assertUserSettingCancellationNoEvents(t, "failed write", fixture.events)
		text := fixture.log.String()
		if !strings.Contains(text, errUserSettingCancellationInjected.Error()) || !strings.Contains(text, "settings.go:") {
			t.Errorf("database failure was not logged against settings.go:\n%s", text)
		}
	})
}
