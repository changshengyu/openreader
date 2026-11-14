package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"openreader/backend/config"
	"openreader/backend/models"
)

// Contract: docs/compat/user-setting-write-request-cancellation-p2-contract.md section 5.2.

const loggerContractTestFile = "logger_contract_test.go:"

var loggerContractEndedTexts = []string{
	"context canceled",
	"context deadline exceeded",
	"transaction has already been committed or rolled back",
}

type loggerContractOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (o *loggerContractOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.Write(data)
}

func (o *loggerContractOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func (o *loggerContractOutput) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.buffer.Reset()
}

func openLoggerContractDatabase(t *testing.T) (*gorm.DB, *loggerContractOutput) {
	t.Helper()
	database, err := Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "logger.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDatabase, err := database.DB(); err == nil {
			_ = sqlDatabase.Close()
		}
	})
	if err := database.AutoMigrate(&models.UserSetting{}); err != nil {
		t.Fatal(err)
	}
	output := &loggerContractOutput{}
	database.Logger = NewLogger(log.New(output, "", 0))
	return database, output
}

func assertLoggerContractSilent(t *testing.T, label string, output *loggerContractOutput) {
	t.Helper()
	if text := output.String(); text != "" {
		t.Errorf("%s printed a statement whose own context ended:\n%s", label, text)
	}
}

func TestLoggerDropsStatementsWhoseOwnContextEnded(t *testing.T) {
	t.Run("query on a cancelled context", func(t *testing.T) {
		database, output := openLoggerContractDatabase(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := database.WithContext(ctx).Where("user_id = ? AND key = ?", 1, "shelf").First(&models.UserSetting{}).Error
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled query error = %v, want context.Canceled", err)
		}
		assertLoggerContractSilent(t, "cancelled query", output)
	})

	t.Run("insert after the transaction began", func(t *testing.T) {
		database, output := openLoggerContractDatabase(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		const hook = "test:logger-contract-cancel-insert"
		if err := database.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
			cancel()
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Callback().Create().Remove(hook) })
		setting := models.UserSetting{UserID: 1, Key: "shelf", Value: `{"marker":"cancelled"}`, UpdatedAt: time.Now()}
		err := database.WithContext(ctx).Create(&setting).Error
		if err == nil || ctx.Err() == nil {
			t.Fatalf("insert after cancellation: err=%v ctx=%v, want a failed statement on an ended context", err, ctx.Err())
		}
		assertLoggerContractSilent(t, "cancelled insert", output)
		var count int64
		if err := database.Model(&models.UserSetting{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("cancelled insert left %d rows", count)
		}
	})

	t.Run("GORM's joined rollback error and an expired deadline", func(t *testing.T) {
		output := &loggerContractOutput{}
		logger := NewLogger(log.New(output, "", 0))
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer stop()
		statement := func() (string, int64) { return "INSERT INTO `user_settings` (`value`) VALUES (?)", 0 }
		logger.Trace(cancelled, time.Now(), statement, fmt.Errorf("%v; %w", context.Canceled, sql.ErrTxDone))
		logger.Trace(cancelled, time.Now(), statement, context.Canceled)
		logger.Trace(expired, time.Now(), statement, context.DeadlineExceeded)
		logger.Trace(expired, time.Now().Add(-time.Second), statement, fmt.Errorf("%v; %w", context.DeadlineExceeded, sql.ErrTxDone))
		assertLoggerContractSilent(t, "ended-context traces", output)
	})
}

func TestLoggerKeepsOtherStatementOutput(t *testing.T) {
	statement := func() (string, int64) { return "SELECT * FROM `user_settings` WHERE id = ?", 0 }

	t.Run("live context errors keep the caller's file and line", func(t *testing.T) {
		output := &loggerContractOutput{}
		logger := NewLogger(log.New(output, "", 0))
		logger.Trace(context.Background(), time.Now(), statement, fmt.Errorf("%v; %w", errors.New("disk I/O error"), sql.ErrTxDone))
		text := output.String()
		if !strings.Contains(text, "disk I/O error") || !strings.Contains(text, "transaction has already been committed or rolled back") {
			t.Errorf("live-context rollback error was not printed: %q", text)
		}
		if !strings.Contains(text, loggerContractTestFile) {
			t.Errorf("error line does not name the calling file: %q", text)
		}
	})

	t.Run("a cancelled context does not hide unrelated errors", func(t *testing.T) {
		output := &loggerContractOutput{}
		logger := NewLogger(log.New(output, "", 0))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		logger.Trace(ctx, time.Now(), statement, errors.New("UNIQUE constraint failed: user_settings.user_id, user_settings.key"))
		if text := output.String(); !strings.Contains(text, "UNIQUE constraint failed") {
			t.Errorf("unrelated error on an ended context was not printed: %q", text)
		}
	})

	t.Run("record not found, slow SQL and warnings", func(t *testing.T) {
		database, output := openLoggerContractDatabase(t)
		err := database.Where("user_id = ? AND key = ?", 7, "search").First(&models.UserSetting{}).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("missing row error = %v", err)
		}
		text := output.String()
		if !strings.Contains(text, "record not found") || !strings.Contains(text, loggerContractTestFile) {
			t.Errorf("record not found was not printed with the caller: %q", text)
		}
		for _, ended := range loggerContractEndedTexts {
			if strings.Contains(text, ended) {
				t.Errorf("live query printed %q: %q", ended, text)
			}
		}

		output.Reset()
		database.Logger.Trace(context.Background(), time.Now().Add(-time.Second), statement, nil)
		if text := output.String(); !strings.Contains(text, "SLOW SQL >= 200ms") || !strings.Contains(text, loggerContractTestFile) {
			t.Errorf("slow statement was not printed with the caller: %q", text)
		}

		output.Reset()
		database.Logger.Warn(context.Background(), "logger contract %s", "warning")
		if text := output.String(); !strings.Contains(text, "logger contract warning") || !strings.Contains(text, loggerContractTestFile) {
			t.Errorf("warning was not printed with the caller: %q", text)
		}

		output.Reset()
		database.Logger.LogMode(gormlogger.Silent).Trace(context.Background(), time.Now(), statement, errors.New("silenced"))
		if text := output.String(); text != "" {
			t.Errorf("silent logger printed: %q", text)
		}
	})

	t.Run("statement parameters stay out of the log", func(t *testing.T) {
		database, output := openLoggerContractDatabase(t)
		secret := strings.Repeat("b2", 32)
		if err := database.Exec("INSERT INTO missing_table (secret) VALUES (?)", secret).Error; err == nil {
			t.Fatal("insert into a missing table succeeded")
		}
		text := output.String()
		if strings.Contains(text, secret) {
			t.Errorf("log leaked a statement parameter: %q", text)
		}
		if !strings.Contains(text, "missing_table") || !strings.Contains(text, loggerContractTestFile) {
			t.Errorf("failed statement was not printed with the caller: %q", text)
		}
	})
}
