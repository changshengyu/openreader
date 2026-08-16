package backup

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
)

type contextBackupGenerator interface {
	RunNowForUserContext(context.Context, uint, string) (string, error)
	RunPortableV2ForUserContext(context.Context, uint, string, string) (PortableResult, error)
}

func requireContextBackupGenerator(t *testing.T, service *Service) contextBackupGenerator {
	t.Helper()
	generator, ok := any(service).(contextBackupGenerator)
	if !ok {
		t.Fatal("backup service must expose context-aware logical and portable generation entry points")
	}
	return generator
}

func TestBackupGenerationRejectsCanceledContextBeforeWork(t *testing.T) {
	database := portableBackupTestDB(t)
	root := t.TempDir()
	service := New(database, filepath.Join(root, "webdav"), config.Config{LibraryDir: filepath.Join(root, "library")})
	generator := requireContextBackupGenerator(t, service)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := generator.RunNowForUserContext(ctx, 1, "canceled-logical"); !errors.Is(err, context.Canceled) {
		t.Fatalf("logical pre-cancel error = %v, want context.Canceled", err)
	}
	portableDir := filepath.Join(root, "webdav", "users", "canceled-portable")
	if _, err := generator.RunPortableV2ForUserContext(ctx, 1, "canceled-portable", portableDir); !errors.Is(err, context.Canceled) {
		t.Fatalf("portable pre-cancel error = %v, want context.Canceled", err)
	}
	assertNoBackupGenerationArtifacts(t, root)
}

func TestCanceledBackupGenerationWaiterDoesNotStartAfterLockRelease(t *testing.T) {
	database := portableBackupTestDB(t)
	root := t.TempDir()
	service := New(database, filepath.Join(root, "webdav"), config.Config{LibraryDir: filepath.Join(root, "library")})
	generator := requireContextBackupGenerator(t, service)
	started := make(chan struct{})
	release := make(chan struct{})
	var blocked atomic.Bool
	if err := database.Callback().Query().Before("gorm:query").Register("test:block-first-backup", func(tx *gorm.DB) {
		if tx.Statement.Table == "rss_sources" && blocked.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := generator.RunNowForUserContext(context.Background(), 1, "first-backup")
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first backup did not reach the blocking query")
	}

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := generator.RunNowForUserContext(ctx, 2, "canceled-waiter")
		secondDone <- err
	}()
	cancel()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled waiter remained blocked behind active backup")
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first backup failed after release: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(root, "webdav", "users", "canceled-waiter", "backup_*.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled waiter started after lock release: %v", entries)
	}
}

func TestBackupGenerationSuccessLogDoesNotExposeHostPath(t *testing.T) {
	database := portableBackupTestDB(t)
	root := t.TempDir()
	service := New(database, filepath.Join(root, "private-mounted-webdav"))
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	path, err := service.RunNowForUser(1, "log-redaction")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), root) || strings.Contains(output.String(), filepath.Dir(path)) {
		t.Fatalf("backup success log exposed host path: %s", output.String())
	}
	if !strings.Contains(output.String(), filepath.Base(path)) {
		t.Fatalf("backup success log omitted safe basename: %s", output.String())
	}
}

func assertNoBackupGenerationArtifacts(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, "backup_") || strings.HasPrefix(name, "portable_backup_") ||
			strings.HasPrefix(name, ".backup-") || strings.HasPrefix(name, ".portable-backup-") {
			t.Fatalf("canceled generation left artifact %s", path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
