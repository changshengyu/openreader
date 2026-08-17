package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"log"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
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
	var queries atomic.Int64
	if err := database.Callback().Query().Before("gorm:query").Register("test:count-pre-canceled-queries", func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := generator.RunNowForUserContext(ctx, 1, "canceled-logical"); !errors.Is(err, context.Canceled) {
		t.Fatalf("logical pre-cancel error = %v, want context.Canceled", err)
	}
	portableDir := filepath.Join(root, "webdav", "users", "canceled-portable")
	if _, err := generator.RunPortableV2ForUserContext(ctx, 1, "canceled-portable", portableDir); !errors.Is(err, context.Canceled) {
		t.Fatalf("portable pre-cancel error = %v, want context.Canceled", err)
	}
	if got := queries.Load(); got != 0 {
		t.Fatalf("pre-canceled backup generation executed %d database queries, want 0", got)
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

func TestLogicalBackupGenerationCancelsInFlightAndCleansTemporaryFile(t *testing.T) {
	database := portableBackupTestDB(t)
	root := t.TempDir()
	service := New(database, filepath.Join(root, "webdav"))
	started := make(chan struct{})
	if err := database.Callback().Query().Before("gorm:query").Register("test:block-logical-backup", func(tx *gorm.DB) {
		if tx.Statement.Table != "rss_sources" {
			return
		}
		close(started)
		<-tx.Statement.Context.Done()
		tx.AddError(tx.Statement.Context.Err())
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.RunNowForUserContext(ctx, 1, "in-flight-logical")
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("logical backup did not reach the blocking snapshot query")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("in-flight logical cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("logical backup did not stop after cancellation")
	}
	assertNoBackupGenerationArtifacts(t, root)
}

func TestPortableBackupGenerationCancelsDuringArchiveCopyAndCleansTemporaryFile(t *testing.T) {
	database := portableBackupTestDB(t)
	root := t.TempDir()
	libraryDir := filepath.Join(root, "library")
	backupDir := filepath.Join(root, "webdav", "users", "portable-copy")
	service := New(database, filepath.Join(root, "webdav"), config.Config{LibraryDir: libraryDir})
	user := models.User{Username: "portable-copy", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(libraryDir, "data", user.Username, "large-book", "large.txt")
	writePseudoRandomArchive(t, archivePath, 8*1024*1024)
	book := models.Book{
		UserID:       user.ID,
		SourceID:     0,
		Title:        "large local book",
		URL:          "local://cancel-copy",
		LibraryPath:  filepath.Join("data", user.Username, "large-book"),
		OriginalFile: filepath.Join("data", user.Username, "large-book", "large.txt"),
	}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.RunPortableV2ForUserContext(ctx, user.ID, user.Username, backupDir)
		done <- err
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	waiting := true
	for waiting {
		select {
		case err := <-done:
			cancel()
			t.Fatalf("portable backup completed before cancellation boundary: %v", err)
		case <-ticker.C:
			temporary, err := filepath.Glob(filepath.Join(backupDir, ".portable-backup-*.tmp"))
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			waiting = len(temporary) == 0
		case <-timeout.C:
			cancel()
			t.Fatal("portable backup did not create its private temporary file")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("portable copy cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("portable archive copy did not stop after cancellation")
	}
	assertNoBackupGenerationArtifacts(t, root)
}

func TestBackupGenerationKeepsPackageWhenCanceledAfterDurableRename(t *testing.T) {
	database := portableBackupTestDB(t)
	root := t.TempDir()
	service := New(database, filepath.Join(root, "webdav"))
	writer := &blockingBackupLogWriter{started: make(chan struct{}), release: make(chan struct{})}
	previous := log.Writer()
	log.SetOutput(writer)
	t.Cleanup(func() { log.SetOutput(previous) })

	ctx, cancel := context.WithCancel(context.Background())
	type backupResult struct {
		path string
		err  error
	}
	done := make(chan backupResult, 1)
	go func() {
		path, err := service.RunNowForUserContext(ctx, 1, "durable-rename")
		done <- backupResult{path: path, err: err}
	}()
	select {
	case <-writer.started:
	case <-time.After(2 * time.Second):
		cancel()
		close(writer.release)
		t.Fatal("backup did not reach its post-rename success log")
	}
	cancel()
	close(writer.release)
	result := <-done
	if result.err != nil {
		t.Fatalf("post-rename cancellation changed durable success to error: %v", result.err)
	}
	reader, err := zip.OpenReader(result.path)
	if err != nil {
		t.Fatalf("durable backup is not readable after request cancellation: %v", err)
	}
	_ = reader.Close()
	if strings.HasPrefix(filepath.Base(result.path), ".backup-") {
		t.Fatalf("durable result still uses a temporary name: %s", result.path)
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

type blockingBackupLogWriter struct {
	started chan struct{}
	release chan struct{}
	wrote   atomic.Bool
}

func (w *blockingBackupLogWriter) Write(data []byte) (int, error) {
	if w.wrote.CompareAndSwap(false, true) {
		close(w.started)
	}
	<-w.release
	return len(data), nil
}

func writePseudoRandomArchive(t *testing.T, path string, size int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	random := mathrand.New(mathrand.NewSource(1))
	buffer := make([]byte, 64*1024)
	remaining := size
	for remaining > 0 {
		chunk := int64(len(buffer))
		if remaining < chunk {
			chunk = remaining
		}
		if _, err := random.Read(buffer[:chunk]); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if _, err := file.Write(buffer[:chunk]); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		remaining -= chunk
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
