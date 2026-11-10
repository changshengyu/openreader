package booksources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

// immediateSourceDatabase opens the application database through db.Open, so
// every transaction begins with BEGIN IMMEDIATE, plus a second connection with
// busy_timeout 0. The second connection stands in for another request: its
// write fails at once, instead of waiting, while a book-source transaction
// holds SQLite's write lock.
func immediateSourceDatabase(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "openreader.db")
	database, err := readerdb.Open(config.Config{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite3", path+"?_busy_timeout=0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	return database, other
}

func isSQLiteBusy(err error) bool {
	var sqliteError sqlite3.Error
	return errors.As(err, &sqliteError) && sqliteError.Code == sqlite3.ErrBusy
}

type sourceTestCallerKey struct{}

// callerSourceService tags every statement the service issues, including the
// ones inside its transactions, so a query hook can tell callers apart.
func callerSourceService(database *gorm.DB, caller string) *Service {
	return New(database.WithContext(context.WithValue(context.Background(), sourceTestCallerKey{}, caller)))
}

func sourceTestCaller(tx *gorm.DB) string {
	caller, _ := tx.Statement.Context.Value(sourceTestCallerKey{}).(string)
	return caller
}

func inSourceTransaction(tx *gorm.DB) bool {
	committer, ok := tx.Statement.ConnPool.(gorm.TxCommitter)
	return ok && committer != nil
}

func afterSourceQuery(t *testing.T, database *gorm.DB, name string, hook func(*gorm.DB)) {
	t.Helper()
	if err := database.Callback().Query().After("gorm:query").Register(name, hook); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(name) })
}

// overlapFirstTwo keeps the first caller waiting until a second caller reaches
// the same point, or until timeout. Two deferred transactions would both read
// there before either writes. A transaction that holds the write lock from
// BEGIN keeps the second one out, so the first goes on alone after timeout.
func overlapFirstTwo(timeout time.Duration) func() {
	var arrivals atomic.Int32
	partner := make(chan struct{})
	return func() {
		switch arrivals.Add(1) {
		case 1:
			select {
			case <-partner:
			case <-time.After(timeout):
			}
		case 2:
			close(partner)
		}
	}
}

func activeSourceIDs(t *testing.T, database *gorm.DB, userID uint) []uint {
	t.Helper()
	var sourceIDs []uint
	if err := database.Model(&models.UserBookSource{}).
		Where("user_id = ? AND detached = ?", userID, false).
		Order("source_id asc").
		Pluck("source_id", &sourceIDs).Error; err != nil {
		t.Fatal(err)
	}
	return sourceIDs
}

func namespaceRows(t *testing.T, database *gorm.DB, userID uint) int64 {
	t.Helper()
	var count int64
	if err := database.Model(&models.BookSourceNamespace{}).
		Where("user_id = ?", userID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func setUserSourceLimit(t *testing.T, database *gorm.DB, userID uint, limit int) {
	t.Helper()
	if err := database.Model(&models.User{}).Where("id = ?", userID).Update("source_limit", limit).Error; err != nil {
		t.Fatal(err)
	}
}

func TestEnsureNamespaceConcurrentFirstAccessCopiesDefaultsOnce(t *testing.T) {
	database := sourceServiceDatabase(t)
	defaultIDs := make([]uint, 0, 3)
	for index := 1; index <= 3; index++ {
		source := createSourceSnapshot(t, database, fmt.Sprintf("并发默认源 %d", index), fmt.Sprintf("https://first-access-default-%d.example", index))
		attachSource(t, database, 0, source.ID, false)
		defaultIDs = append(defaultIDs, source.ID)
	}
	markSourceNamespace(t, database, 0)
	users := []models.User{
		createSourceUser(t, database, "source-first-access-a"),
		createSourceUser(t, database, "source-first-access-b"),
	}

	overlap := overlapFirstTwo(200 * time.Millisecond)
	afterSourceQuery(t, database, "openreader:test:first-access-overlap", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_book_sources" && inSourceTransaction(tx) {
			overlap()
		}
	})

	const requestsPerUser = 4
	start := make(chan struct{})
	failures := make(chan error, len(users)*requestsPerUser)
	var requests sync.WaitGroup
	for _, user := range users {
		for range requestsPerUser {
			requests.Add(1)
			go func() {
				defer requests.Done()
				<-start
				if _, err := New(database).ListActive(user.ID); err != nil {
					failures <- fmt.Errorf("user %d: %w", user.ID, err)
				}
			}()
		}
	}
	close(start)
	requests.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent first access returned %v", err)
	}

	for _, user := range users {
		namespaces := namespaceRows(t, database, user.ID)
		sourceIDs := activeSourceIDs(t, database, user.ID)
		if namespaces != 1 || !slices.Equal(sourceIDs, defaultIDs) {
			t.Errorf("user %d after concurrent first access: namespace rows = %d, active sources = %v, want 1 and %v", user.ID, namespaces, sourceIDs, defaultIDs)
		}
	}
}

func TestEnsureNamespaceDoesNotRecopyDefaultsAfterAnotherFirstAccessWon(t *testing.T) {
	database := sourceServiceDatabase(t)
	kept := createSourceSnapshot(t, database, "保留的默认源", "https://kept-default.example")
	removed := createSourceSnapshot(t, database, "被删除的默认源", "https://removed-default.example")
	for _, source := range []models.BookSource{kept, removed} {
		attachSource(t, database, 0, source.ID, false)
	}
	markSourceNamespace(t, database, 0)
	user := createSourceUser(t, database, "source-first-access-loser")

	// Stop the losing request right after it found no namespace, before it
	// starts initializing one.
	missing := make(chan struct{})
	resume := make(chan struct{})
	var stopped atomic.Bool
	afterSourceQuery(t, database, "openreader:test:first-access-loser", func(tx *gorm.DB) {
		if tx.Statement.Table != "book_source_namespaces" || sourceTestCaller(tx) != "loser" || !stopped.CompareAndSwap(false, true) {
			return
		}
		close(missing)
		select {
		case <-resume:
		case <-time.After(5 * time.Second):
		}
	})
	loser := make(chan error, 1)
	go func() { loser <- callerSourceService(database, "loser").EnsureNamespace(user.ID) }()
	select {
	case <-missing:
	case <-time.After(5 * time.Second):
		t.Fatal("the losing first access never checked the namespace")
	}

	winner := New(database)
	if err := winner.EnsureNamespace(user.ID); err != nil {
		t.Fatal(err)
	}
	// The user removes one inherited source before the losing request resumes.
	if err := winner.Delete(user.ID, removed.ID); err != nil {
		t.Fatal(err)
	}
	close(resume)
	if err := <-loser; err != nil {
		t.Fatalf("the losing first access returned %v", err)
	}

	if sourceIDs := activeSourceIDs(t, database, user.ID); !slices.Equal(sourceIDs, []uint{kept.ID}) {
		t.Fatalf("active sources after the losing first access = %v, want only %d; it copied back default %d that the user had removed", sourceIDs, kept.ID, removed.ID)
	}
	if namespaces := namespaceRows(t, database, user.ID); namespaces != 1 {
		t.Fatalf("namespace rows = %d, want 1", namespaces)
	}
}

func TestAdminSourceResetDoesNotStallBehindAnotherFirstAccess(t *testing.T) {
	database := sourceServiceDatabase(t)
	defaultSource := createSourceSnapshot(t, database, "重置默认源", "https://reset-default.example")
	attachSource(t, database, 0, defaultSource.ID, false)
	markSourceNamespace(t, database, 0)
	target := createSourceUser(t, database, "source-reset-uninitialized")
	newcomer := createSourceUser(t, database, "source-reset-newcomer")

	// The admin reset holds the write lock from BEGIN and stops at its first
	// namespace check. Another new user's first access then starts and, where
	// it can, gets past its own recheck before the reset goes on.
	resetLocked := make(chan struct{})
	resumeReset := make(chan struct{})
	newcomerRechecked := make(chan struct{})
	var resetStopped atomic.Bool
	var newcomerChecks atomic.Int32
	afterSourceQuery(t, database, "openreader:test:reset-and-first-access", func(tx *gorm.DB) {
		if tx.Statement.Table != "book_source_namespaces" {
			return
		}
		switch sourceTestCaller(tx) {
		case "reset":
			if inSourceTransaction(tx) && resetStopped.CompareAndSwap(false, true) {
				close(resetLocked)
				select {
				case <-resumeReset:
				case <-time.After(5 * time.Second):
				}
			}
		case "newcomer":
			if newcomerChecks.Add(1) == 2 {
				close(newcomerRechecked)
			}
		}
	})

	started := time.Now()
	reset := make(chan error, 1)
	go func() {
		_, err := callerSourceService(database, "reset").RestoreDefaultForUsers([]uint{target.ID})
		reset <- err
	}()
	select {
	case <-resetLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("the admin reset never checked the target namespace inside its transaction")
	}
	firstAccess := make(chan error, 1)
	go func() { firstAccess <- callerSourceService(database, "newcomer").EnsureNamespace(newcomer.ID) }()
	select {
	case <-newcomerRechecked:
	case <-time.After(300 * time.Millisecond):
	}
	close(resumeReset)

	var resetErr, firstAccessErr error
	resetDone, firstAccessDone := false, false
	deadline := time.After(4 * time.Second)
	for !resetDone || !firstAccessDone {
		select {
		case resetErr = <-reset:
			resetDone = true
		case firstAccessErr = <-firstAccess:
			firstAccessDone = true
		case <-deadline:
			t.Errorf("after 4s the admin reset (done %v) and the first access (done %v) were still waiting on each other", resetDone, firstAccessDone)
			// Keep waiting so that the database outlives both calls.
			deadline = nil
		}
	}
	if resetErr != nil || firstAccessErr != nil {
		t.Fatalf("admin reset = %v, first access = %v after %s, want both to succeed", resetErr, firstAccessErr, time.Since(started).Round(time.Millisecond))
	}
	for _, user := range []models.User{target, newcomer} {
		if sourceIDs := activeSourceIDs(t, database, user.ID); !slices.Equal(sourceIDs, []uint{defaultSource.ID}) {
			t.Errorf("user %d active sources = %v, want [%d]", user.ID, sourceIDs, defaultSource.ID)
		}
	}
}

func TestSourceLimitWritesHoldTheLimitAgainstAConcurrentCommit(t *testing.T) {
	for _, operation := range []string{"create", "import"} {
		t.Run(operation, func(t *testing.T) {
			database, other := immediateSourceDatabase(t)
			user := createSourceUser(t, database, "source-limit-commit-"+operation)
			setUserSourceLimit(t, database, user.ID, 3)
			existing := createSourceSnapshot(t, database, "已有书源", "https://limit-existing.example")
			attachSource(t, database, user.ID, existing.ID, false)
			markSourceNamespace(t, database, user.ID)

			// An administrator lowers the limit between the write's limit read
			// and its insert.
			lowerLimit := func() error {
				_, err := other.Exec("UPDATE users SET source_limit = 1 WHERE id = ?", user.ID)
				return err
			}
			var concurrentErr error
			var intercepted atomic.Bool
			afterSourceQuery(t, database, "openreader:test:source-limit-concurrent-commit", func(tx *gorm.DB) {
				if tx.Statement.Table == "users" && inSourceTransaction(tx) && intercepted.CompareAndSwap(false, true) {
					concurrentErr = lowerLimit()
				}
			})

			service := New(database)
			added := 1
			var err error
			switch operation {
			case "create":
				_, err = service.Create(user.ID, models.BookSource{Name: "并发新书源", BaseURL: "https://limit-new.example", Enabled: true})
			case "import":
				added = 2
				_, err = service.Import(user.ID, []models.BookSource{
					{Name: "并发导入一", BaseURL: "https://limit-import-1.example", Enabled: true},
					{Name: "并发导入二", BaseURL: "https://limit-import-2.example", Enabled: true},
				})
			}
			if err != nil {
				t.Fatalf("%s failed once another connection tried to commit between its source-limit read and its write: %v (concurrent commit error: %v)", operation, err, concurrentErr)
			}
			if !isSQLiteBusy(concurrentErr) {
				t.Fatalf("lowering the source limit while the %s transaction was open = %v, want SQLITE_BUSY until it commits", operation, concurrentErr)
			}
			if active := len(activeSourceIDs(t, database, user.ID)); active != 1+added {
				t.Fatalf("active sources after %s = %d, want %d", operation, active, 1+added)
			}
			if err := lowerLimit(); err != nil {
				t.Fatalf("the waiting limit change after the %s committed: %v", operation, err)
			}
			if _, err := service.Create(user.ID, models.BookSource{Name: "超出新限额", BaseURL: "https://limit-over.example", Enabled: true}); !errors.Is(err, ErrSourceLimitExceeded) {
				t.Fatalf("create after the limit was lowered = %v, want ErrSourceLimitExceeded", err)
			}
		})
	}
}

func TestConcurrentSourceWritesStayWithinEachUsersLimit(t *testing.T) {
	database := sourceServiceDatabase(t)
	limits := make(map[uint]int, 3)
	for index, limit := range []int{2, 3, 4} {
		user := createSourceUser(t, database, fmt.Sprintf("source-limit-concurrent-%d", index+1))
		setUserSourceLimit(t, database, user.ID, limit)
		markSourceNamespace(t, database, user.ID)
		limits[user.ID] = limit
	}

	overlap := overlapFirstTwo(200 * time.Millisecond)
	afterSourceQuery(t, database, "openreader:test:source-limit-overlap", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && inSourceTransaction(tx) {
			overlap()
		}
	})

	const writesPerUser = 6
	type outcome struct {
		userID uint
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, len(limits)*writesPerUser)
	var writers sync.WaitGroup
	for userID := range limits {
		for index := range writesPerUser {
			writers.Add(1)
			go func() {
				defer writers.Done()
				<-start
				source := models.BookSource{
					Name:    fmt.Sprintf("用户 %d 并发书源 %d", userID, index),
					BaseURL: fmt.Sprintf("https://user-%d-concurrent-%d.example", userID, index),
					Enabled: true,
				}
				var err error
				if index%2 == 0 {
					_, err = New(database).Create(userID, source)
				} else {
					_, err = New(database).Import(userID, []models.BookSource{source})
				}
				outcomes <- outcome{userID: userID, err: err}
			}()
		}
	}
	close(start)
	writers.Wait()
	close(outcomes)

	accepted := make(map[uint]int, len(limits))
	for outcome := range outcomes {
		switch {
		case outcome.err == nil:
			accepted[outcome.userID]++
		case errors.Is(outcome.err, ErrSourceLimitExceeded):
		default:
			t.Errorf("user %d concurrent source write returned %v, want success or ErrSourceLimitExceeded", outcome.userID, outcome.err)
		}
	}
	for userID, limit := range limits {
		active := len(activeSourceIDs(t, database, userID))
		if accepted[userID] != limit || active != limit {
			t.Errorf("user %d: %d writes accepted and %d active sources, want both to equal the limit %d", userID, accepted[userID], active, limit)
		}
	}
}

// Two writes of one user must not both read the user row before either
// writes: a deferred transaction that read first fails its later write once
// the other commits. Book source writes used to take a per-user process lock
// for that; BEGIN IMMEDIATE now keeps the second transaction at BEGIN until
// the first commits, so the first writer waits in vain for a partner here.
func TestSameUserSourceWritesSerializeWithoutAProcessLock(t *testing.T) {
	database := sourceServiceDatabase(t)
	user := createSourceUser(t, database, "source-same-user-concurrent")
	markSourceNamespace(t, database, user.ID)

	overlap := overlapFirstTwo(200 * time.Millisecond)
	afterSourceQuery(t, database, "openreader:test:same-user-source-overlap", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && inSourceTransaction(tx) {
			overlap()
		}
	})

	const writes = 6
	start := make(chan struct{})
	failures := make(chan error, writes)
	var writers sync.WaitGroup
	for index := range writes {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			source := models.BookSource{
				Name:    fmt.Sprintf("同用户并发书源 %d", index),
				BaseURL: fmt.Sprintf("https://same-user-concurrent-%d.example", index),
				Enabled: true,
			}
			var err error
			if index%2 == 0 {
				_, err = New(database).Create(user.ID, source)
			} else {
				_, err = New(database).Import(user.ID, []models.BookSource{source})
			}
			if err != nil {
				failures <- fmt.Errorf("write %d: %w", index, err)
			}
		}()
	}
	close(start)
	writers.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent same-user source write returned %v, want success", err)
	}
	if active := len(activeSourceIDs(t, database, user.ID)); active != writes {
		t.Fatalf("active sources after %d concurrent same-user writes = %d, want %d", writes, active, writes)
	}
}
