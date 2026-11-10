package api

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/services/booksources"
)

// beginReportingPool reports each BEGIN once armed, then hands it to the
// application pool. A source write that takes a per-user lock takes it before
// it begins, so the report also means the write holds that lock.
type beginReportingPool struct {
	*sql.DB
	armed  *atomic.Bool
	begins chan<- struct{}
}

func (p beginReportingPool) BeginTx(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error) {
	if p.armed.Load() {
		select {
		case p.begins <- struct{}{}:
		default:
		}
	}
	return p.DB.BeginTx(ctx, options)
}

func (p beginReportingPool) GetDBConn() (*sql.DB, error) { return p.DB, nil }

func restoreSourceWriteInTransaction(tx *gorm.DB) bool {
	committer, ok := tx.Statement.ConnPool.(gorm.TxCommitter)
	return ok && committer != nil
}

func sourceImportFileRequest(t *testing.T, auth, data string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bookSources.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sources/import", &body)
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

// A backup restore holds SQLite's write lock from BEGIN IMMEDIATE until it
// commits. While it runs, the same user creates or imports a book source, and
// that request waits at its own BEGIN for the lock. The restore must not in
// turn wait for anything that request holds: otherwise both wait until the
// request's busy_timeout (5 s) fails it with a 500, and every other write in
// the process waits behind the restore meanwhile.
//
// Both requests use a session issued moments earlier. Session renewal and
// activity writes happen at most once a minute, so neither request writes
// before its handler, and the first BEGIN after the restore stops is the
// source write's own.
func TestBackupRestoreDoesNotStallBehindTheSameUsersSourceWrite(t *testing.T) {
	for _, operation := range []string{"create", "import"} {
		t.Run(operation, func(t *testing.T) {
			router, server := setupTestServer(t)
			sqlDB, err := server.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			var armed atomic.Bool
			begins := make(chan struct{}, 1)
			pool := beginReportingPool{DB: sqlDB, armed: &armed, begins: begins}
			server.db.ConnPool = pool
			server.db.Statement.ConnPool = pool

			account := registerSourceContractAccount(t, router, "restoresourcewrite"+operation)
			// Initialize the namespace now, so the concurrent write's only
			// BEGIN is the one of its source transaction.
			_ = sourceList(t, router, account.Auth)

			const restoredName, writtenName = "恢复的书源", "同时写入的书源"
			archive := makeBackupRestoreZIP(t, map[string]string{
				"bookSource.json": `[{"bookSourceName":"` + restoredName + `","bookSourceUrl":"https://restore-source-write-restored.example","enabled":true}]`,
			})
			restoreRequest := multipartBackupRestoreRequest(t, "openreader-backup.zip", archive)
			restoreRequest.Header.Set("Authorization", account.Auth)
			var writeRequest *http.Request
			wantWrite := http.StatusOK
			switch operation {
			case "create":
				writeRequest = httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(`{"name":"`+writtenName+`","baseUrl":"https://restore-source-write-create.example"}`))
				writeRequest.Header.Set("Authorization", account.Auth)
				writeRequest.Header.Set("Content-Type", "application/json")
				wantWrite = http.StatusCreated
			case "import":
				writeRequest = sourceImportFileRequest(t, account.Auth, `[{"bookSourceName":"`+writtenName+`","bookSourceUrl":"https://restore-source-write-import.example"}]`)
			}

			// Stop the restore at its first write, the namespace row that
			// ReplaceActive keeps. Its transaction already holds the write lock.
			restoreLocked := make(chan struct{})
			resumeRestore := make(chan struct{})
			resume := sync.OnceFunc(func() { close(resumeRestore) })
			defer resume()
			var restoreStopped atomic.Bool
			const hook = "openreader:test:restore-same-user-source-write"
			if err := server.db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table != "book_source_namespaces" || !restoreSourceWriteInTransaction(tx) || !restoreStopped.CompareAndSwap(false, true) {
					return
				}
				close(restoreLocked)
				select {
				case <-resumeRestore:
				case <-time.After(10 * time.Second):
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = server.db.Callback().Create().Remove(hook) })

			restoreResponse := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, restoreRequest)
				restoreResponse <- response
			}()
			select {
			case <-restoreLocked:
			case <-time.After(5 * time.Second):
				t.Fatal("the restore never reached its first source write inside its transaction")
			}

			armed.Store(true)
			writeResponse := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, writeRequest)
				writeResponse <- response
			}()
			select {
			case <-begins:
			case <-time.After(5 * time.Second):
				t.Fatal("the same user's source " + operation + " never reached BEGIN")
			}
			resumed := time.Now()
			resume()

			var restored, written *httptest.ResponseRecorder
			deadline := time.After(2 * time.Second)
			for restored == nil || written == nil {
				select {
				case restored = <-restoreResponse:
				case written = <-writeResponse:
				case <-deadline:
					t.Errorf("2s after the restore resumed, the restore (done %v) and the same user's source %s (done %v) were still waiting on each other", restored != nil, operation, written != nil)
					// Keep waiting so that the database outlives both requests.
					deadline = nil
				}
			}
			if restored.Code != http.StatusOK || written.Code != wantWrite {
				t.Fatalf("restore = %d %s, same user's source %s = %d %s after %s, want %d and %d",
					restored.Code, strings.TrimSpace(restored.Body.String()), operation, written.Code, strings.TrimSpace(written.Body.String()),
					time.Since(resumed).Round(time.Millisecond), http.StatusOK, wantWrite)
			}

			// The write waited for the restore to commit: had it committed
			// first, the restore would have removed its unused source.
			active, err := booksources.New(server.db).ListExistingActive(account.ID)
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, 0, len(active))
			for _, source := range active {
				names = append(names, source.Name)
			}
			slices.Sort(names)
			want := []string{restoredName, writtenName}
			slices.Sort(want)
			if !slices.Equal(names, want) {
				t.Fatalf("active sources after the restore and the %s = %v, want %v", operation, names, want)
			}
		})
	}
}
