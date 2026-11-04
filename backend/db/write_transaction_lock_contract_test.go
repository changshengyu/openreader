package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"openreader/backend/config"
)

// openWriteTransactionLockFixture opens the application database and a second,
// independent connection with busy_timeout 0. The second connection stands in
// for another request: its write fails at once, instead of waiting, whenever
// an open transaction holds SQLite's single write lock.
func openWriteTransactionLockFixture(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "openreader.db")
	database, err := Open(config.Config{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	if err := database.Exec("CREATE TABLE write_lock_probe(id INTEGER PRIMARY KEY, value TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO write_lock_probe(id, value) VALUES (1, ?), (2, ?)", "原值", "旁路原值").Error; err != nil {
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

func writeLockProbeValue(t *testing.T, database *sql.DB, id int) string {
	t.Helper()
	var value string
	if err := database.QueryRow("SELECT value FROM write_lock_probe WHERE id = ?", id).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestOpenWriteTransactionsHoldTheWriteLockFromBegin(t *testing.T) {
	database, other := openWriteTransactionLockFixture(t)
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := sqlDatabase.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := sqlDatabase.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	for index, connection := range []*sql.Conn{first, second} {
		tx, err := connection.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("connection %d begin: %v", index+1, err)
		}
		var current string
		if err := tx.QueryRowContext(ctx, "SELECT value FROM write_lock_probe WHERE id = 1").Scan(&current); err != nil {
			_ = tx.Rollback()
			t.Fatalf("connection %d read: %v", index+1, err)
		}
		concurrent := fmt.Sprintf("连接 %d 期间的并发提交", index+1)
		_, concurrentErr := other.ExecContext(ctx, "UPDATE write_lock_probe SET value = ? WHERE id = 2", concurrent)
		if _, err := tx.ExecContext(ctx, "UPDATE write_lock_probe SET value = ? WHERE id = 1", fmt.Sprintf("连接 %d 写入", index+1)); err != nil {
			_ = tx.Rollback()
			t.Fatalf("connection %d: write after read failed once another connection tried to commit: %v (concurrent commit error: %v)", index+1, err, concurrentErr)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("connection %d commit: %v", index+1, err)
		}
		if !isSQLiteBusy(concurrentErr) {
			t.Fatalf("connection %d: another connection's write during the open transaction = %v, want SQLITE_BUSY until the transaction ends", index+1, concurrentErr)
		}
		if _, err := other.ExecContext(ctx, "UPDATE write_lock_probe SET value = ? WHERE id = 2", concurrent); err != nil {
			t.Fatalf("connection %d: the waiting write after commit: %v", index+1, err)
		}
	}
	if got := writeLockProbeValue(t, other, 1); got != "连接 2 写入" {
		t.Errorf("transaction row = %q, want 连接 2 写入", got)
	}
	if got := writeLockProbeValue(t, other, 2); got != "连接 2 期间的并发提交" {
		t.Errorf("concurrent row = %q, want 连接 2 期间的并发提交", got)
	}
}

func TestOpenGormTransactionSurvivesConcurrentCommitBetweenReadAndWrite(t *testing.T) {
	database, other := openWriteTransactionLockFixture(t)
	ctx := context.Background()

	var concurrentErr error
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current string
		if err := tx.Raw("SELECT value FROM write_lock_probe WHERE id = 1").Scan(&current).Error; err != nil {
			return err
		}
		_, concurrentErr = other.ExecContext(ctx, "UPDATE write_lock_probe SET value = ? WHERE id = 2", "事务期间的并发提交")
		return tx.Exec("UPDATE write_lock_probe SET value = ? WHERE id = 1", current+"，事务写入").Error
	})
	if err != nil {
		t.Fatalf("read-then-write transaction failed once another connection tried to commit: %v (concurrent commit error: %v)", err, concurrentErr)
	}
	if !isSQLiteBusy(concurrentErr) {
		t.Fatalf("another connection's write during the open transaction = %v, want SQLITE_BUSY until the transaction ends", concurrentErr)
	}
	if _, err := other.ExecContext(ctx, "UPDATE write_lock_probe SET value = ? WHERE id = 2", "事务期间的并发提交"); err != nil {
		t.Fatalf("the waiting write after commit: %v", err)
	}
	if got := writeLockProbeValue(t, other, 1); got != "原值，事务写入" {
		t.Errorf("transaction row = %q, want 原值，事务写入", got)
	}
	if got := writeLockProbeValue(t, other, 2); got != "事务期间的并发提交" {
		t.Errorf("concurrent row = %q, want 事务期间的并发提交", got)
	}
}

func TestOpenKeepsConfiguredDSNParametersAndImmediateTransactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "openreader.db")
	database, err := Open(config.Config{DatabasePath: path + "?_cache_size=-4096"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDatabase.Close()
	if err := database.Exec("CREATE TABLE write_lock_probe(id INTEGER PRIMARY KEY, value TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite3", path+"?_busy_timeout=0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	ctx := context.Background()
	connection, err := sqlDatabase.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var cacheSize int
	if err := connection.QueryRowContext(ctx, "PRAGMA cache_size;").Scan(&cacheSize); err != nil {
		t.Fatal(err)
	}
	if cacheSize != -4096 {
		t.Errorf("configured cache size = %d, want -4096", cacheSize)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := other.ExecContext(ctx, "INSERT INTO write_lock_probe(id, value) VALUES (1, ?)", "并发写入"); !isSQLiteBusy(err) {
		t.Fatalf("another connection's write while a transaction is open = %v, want SQLITE_BUSY when the configured path carries its own parameters", err)
	}
}

func TestOpenWriteTransactionDoesNotBlockReaders(t *testing.T) {
	database, other := openWriteTransactionLockFixture(t)
	ctx := context.Background()

	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE write_lock_probe SET value = ? WHERE id = 1", "未提交").Error; err != nil {
			return err
		}
		var pooled string
		if err := database.WithContext(ctx).Raw("SELECT value FROM write_lock_probe WHERE id = 1").Scan(&pooled).Error; err != nil {
			return fmt.Errorf("pooled reader during the write transaction: %w", err)
		}
		var outside string
		if err := other.QueryRowContext(ctx, "SELECT value FROM write_lock_probe WHERE id = 1").Scan(&outside); err != nil {
			return fmt.Errorf("busy_timeout=0 reader during the write transaction: %w", err)
		}
		if pooled != "原值" || outside != "原值" {
			return fmt.Errorf("readers saw %q and %q during the write transaction, want the committed 原值", pooled, outside)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := writeLockProbeValue(t, other, 1); got != "未提交" {
		t.Errorf("committed row = %q, want 未提交", got)
	}
}
