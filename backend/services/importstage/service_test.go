package importstage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/services/localbook"
)

func testStageService(t *testing.T, hook Hook) *Service {
	t.Helper()
	return New(t.TempDir(), Limits{Source: 1 << 20, Prepared: 2 << 20}, hook)
}
func testPrepared() localbook.PreparedImport {
	return localbook.NewPreparedImport(localbook.ImportRequest{Extension: ".txt", Data: []byte("original")},
		engine.ParsedBook{Title: "original", Chapters: []engine.TXTChapter{{Title: "chapter", Content: "original"}}})
}
func createTestStage(t *testing.T, service *Service) *Session {
	t.Helper()
	x, err := service.Create(context.Background(), 1, "original.txt", ".txt", []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(x.Close)
	return x
}
func assertStageBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("fixture bytes changed: %q %v", got, err)
	}
}

func TestStageActualWriteCancellationCompensatesOnlyOwned(t *testing.T) {
	for _, phase := range []string{"create-write", "metadata-write", "prepared-write"} {
		t.Run(phase, func(t *testing.T) {
			service := testStageService(t, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fired := false
			service.hook = func(current, _, _, _ string) {
				if current == phase {
					fired = true
					cancel()
				}
			}
			if phase == "prepared-write" {
				first := createTestStage(t, service)
				path := first.path(".book")
				token := first.Token
				first.Close()
				x, err := service.Open(ctx, 1, token)
				if err != nil {
					t.Fatal(err)
				}
				defer x.Close()
				if err := x.SavePrepared(testPrepared()); !errors.Is(err, context.Canceled) {
					t.Fatalf("write returned %v", err)
				}
				assertStageBytes(t, path, []byte("original"))
				entries, err := os.ReadDir(filepath.Dir(path))
				if err != nil || len(entries) != 2 {
					t.Fatalf("partial temporary retained: %v %v", entries, err)
				}
			} else {
				if x, err := service.Create(ctx, 1, "original.txt", ".txt", bytes.Repeat([]byte("x"), 128<<10)); !errors.Is(err, context.Canceled) || x != nil {
					t.Fatalf("create returned %v %v", x, err)
				}
				entries, err := os.ReadDir(service.cache)
				if err != nil || len(entries) != 0 {
					t.Fatalf("owned partial directories/files retained: %v %v", entries, err)
				}
			}
			if !fired {
				t.Fatal("actual Write fixture did not fire")
			}
		})
	}
}

func TestStageOpenedRenameKeepsOriginalBytesButNotNewcomerConsumption(t *testing.T) {
	service := testStageService(t, nil)
	first := createTestStage(t, service)
	token, path := first.Token, first.path(".book")
	first.Close()
	fired := false
	service.hook = func(phase, _, _, _ string) {
		if phase != "load-raw-read" || fired {
			return
		}
		fired = true
		if err := os.Rename(path, path+"-held"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("newcomer"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	x, err := service.Open(context.Background(), 1, token)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if !fired || !bytes.Equal(x.Data, []byte("original")) {
		t.Fatal("opened original bytes not retained")
	}
	if err := x.SavePrepared(testPrepared()); err != nil {
		t.Fatal(err)
	}
	if err := x.Consume(); err == nil {
		t.Fatal("newcomer falsely consumed")
	}
	assertStageBytes(t, path, []byte("newcomer"))
	assertStageBytes(t, path+"-held", []byte("original"))
}

func TestStageMetadataWriteCannotHandOffReplacedRawBundle(t *testing.T) {
	service := testStageService(t, nil)
	fired := false
	var raw, metadata string
	service.hook = func(phase, dir, token, _ string) {
		if phase != "metadata-write" || fired {
			return
		}
		fired = true
		raw, metadata = filepath.Join(dir, token+".book"), filepath.Join(dir, token+".json")
		if err := os.Rename(raw, raw+"-held"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(raw, []byte("unknown"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	x, err := service.Create(context.Background(), 1, "original.txt", ".txt", []byte("original"))
	if x != nil {
		x.Close()
	}
	if !fired || !errors.Is(err, ErrStageWrite) || x != nil {
		t.Fatalf("changed bundle handed off: fired=%v stage=%v error=%v", fired, x != nil, err)
	}
	assertStageBytes(t, raw, []byte("unknown"))
	assertStageBytes(t, raw+"-held", []byte("original"))
	if _, err := os.Lstat(metadata); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("own failed metadata retained: %v", err)
	}
}

func TestStageLateSameInodeSymlinkIsNotRegular(t *testing.T) {
	service := testStageService(t, nil)
	first := createTestStage(t, service)
	token, path := first.Token, first.path(".book")
	first.Close()
	fired := false
	service.hook = func(phase, _, _, _ string) {
		if phase != "load-metadata" {
			return
		}
		fired = true
		if err := os.Rename(path, path+"-held"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path+"-held", path); err != nil {
			t.Fatal(err)
		}
	}
	if x, err := service.Open(context.Background(), 1, token); !errors.Is(err, ErrInvalidToken) || x != nil {
		t.Fatalf("link accepted: %v %v", x, err)
	}
	if !fired {
		t.Fatal("fixture did not fire")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("link was changed")
	}
	assertStageBytes(t, path+"-held", []byte("original"))
}

func TestStageCleanupSkipsActiveLeaseAndReclaimsAfterClose(t *testing.T) {
	service := testStageService(t, nil)
	x := createTestStage(t, service)
	path := x.path(".json")
	x.Metadata.CreatedAt = time.Now().Add(-25 * time.Hour)
	encoded, err := json.Marshal(x.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	service.Cleanup(context.Background())
	assertStageBytes(t, path, encoded)
	x.Close()
	service.Cleanup(context.Background())
	for _, suffix := range []string{".book", ".json", ".parsed.json"} {
		if _, err := os.Lstat(filepath.Join(filepath.Dir(path), x.Token+suffix)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expired token remains: %v", err)
		}
	}
	leases.Lock()
	count := len(leases.values)
	leases.Unlock()
	if count != 0 {
		t.Fatalf("idle leases retained: %d", count)
	}
}

func TestStagePermissionsNewPrivateOldUnchanged(t *testing.T) {
	service := testStageService(t, nil)
	x := createTestStage(t, service)
	if err := x.SavePrepared(testPrepared()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Dir(x.path(".book")), filepath.Join(service.cache, "import-previews")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("new directory mode: %v %v", info, err)
		}
	}
	for _, suffix := range []string{".book", ".json", ".parsed.json"} {
		info, err := os.Stat(x.path(suffix))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("new file mode: %v %v", info, err)
		}
	}
	parent, raw, token := filepath.Dir(x.path(".book")), x.path(".book"), x.Token
	x.Close()
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(raw, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(raw, 0o600) })
	loaded, err := service.Open(context.Background(), 1, token)
	if loaded != nil {
		loaded.Close()
	}
	if os.Geteuid() != 0 && !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("unreadable raw accepted: %v", err)
	}
	info, err := os.Stat(raw)
	if err != nil || info.Mode().Perm() != 0 {
		t.Fatalf("unreadable file repaired: %v %v", info, err)
	}
	info, err = os.Stat(parent)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("legacy parent chmod: %v %v", info, err)
	}
}

func TestStageRepeatedSuccessFailureCloseReclaimsDescriptorsAndLeases(t *testing.T) {
	service := testStageService(t, nil)
	first := createTestStage(t, service)
	token := first.Token
	first.Close()
	dir := "/proc/self/fd"
	if runtime.GOOS == "darwin" {
		dir = "/dev/fd"
	}
	fdNames := func() ([]string, error) {
		file, err := os.Open(dir)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		// Darwin /dev/fd is a descriptor namespace, not a directory on which
		// ReadDir's fstatat metadata lookup is valid. Count names only.
		return file.Readdirnames(-1)
	}
	before, err := fdNames()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		x, err := service.Open(context.Background(), 1, token)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := x.Prepared(localbook.ImportRequest{Extension: ".txt", Data: x.Data}); err != nil {
			t.Fatal(err)
		}
		x.Close()
		x.Close()
		if failed, err := service.Open(context.Background(), 1, "ffffffffffffffffffffffffffffffffffffffffffffffff"); err == nil || failed != nil {
			t.Fatal("missing token accepted")
		}
		service.Cleanup(context.Background())
	}
	after, err := fdNames()
	if err != nil || len(after) > len(before)+3 {
		t.Fatalf("FD count grew: before=%d after=%d %v", len(before), len(after), err)
	}
	leases.Lock()
	count := len(leases.values)
	leases.Unlock()
	if count != 0 {
		t.Fatalf("idle leases retained: %d", count)
	}
}
