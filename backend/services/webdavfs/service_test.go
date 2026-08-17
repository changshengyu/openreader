package webdavfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, err := New(filepath.Join(t.TempDir(), "webdav"))
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if err := service.EnsureRoot(); err != nil {
		t.Fatalf("create root: %v", err)
	}
	return service
}

func TestResolvePreservesEncodedFilenameWhitespaceAndRejectsPortableVolumes(t *testing.T) {
	service := newTestService(t)
	target, relative, err := service.Resolve(" folder / spaced file.txt ")
	if err != nil {
		t.Fatalf("resolve spaced path: %v", err)
	}
	if relative != " folder / spaced file.txt " {
		t.Fatalf("relative path lost filename whitespace: %q", relative)
	}
	if filepath.Base(target) != " spaced file.txt " {
		t.Fatalf("target path lost filename whitespace: %q", target)
	}

	for _, unsafe := range []string{
		"C:/Windows/system.ini",
		`C:\Windows\system.ini`,
		"z:",
		"../outside.txt",
		"folder/../../outside.txt",
		"folder/\x00outside.txt",
	} {
		t.Run(unsafe, func(t *testing.T) {
			if _, _, err := service.Resolve(unsafe); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("Resolve(%q) error = %v, want ErrUnsafePath", unsafe, err)
			}
		})
	}
}

func TestNormalizeImportPathAddsUTF8AndLengthAdmission(t *testing.T) {
	valid := " folder / spaced file.txt "
	if normalized, err := NormalizeImportPath(valid); err != nil || normalized != valid {
		t.Fatalf("NormalizeImportPath(%q) = %q, %v", valid, normalized, err)
	}
	exact := strings.Repeat("a", maxImportPathBytes)
	if normalized, err := NormalizeImportPath(exact); err != nil || normalized != exact {
		t.Fatalf("exact import path limit = %d bytes, %v", len(normalized), err)
	}
	for _, unsafe := range []string{
		strings.Repeat("a", maxImportPathBytes+1),
		string([]byte{0xff}),
		"../outside.txt",
		"//server/share.txt",
		`C:\outside.txt`,
		`/C:/outside.txt`,
		"bad\x00path.txt",
	} {
		if _, err := NormalizeImportPath(unsafe); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("NormalizeImportPath(%q) error = %v, want ErrUnsafePath", unsafe, err)
		}
	}
}

func TestOpenReturnsOnlySameRegularFileAndRejectsUnsafeKinds(t *testing.T) {
	service := newTestService(t)
	regularPath := filepath.Join(service.Root(), "regular.txt")
	if err := os.WriteFile(regularPath, []byte("regular"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, info, err := service.Open("regular.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(data) != "regular" || !info.Mode().IsRegular() {
		t.Fatalf("opened regular file = %q mode=%v read=%v close=%v", data, info.Mode(), readErr, closeErr)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(service.Root(), "linked.txt")); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	if _, _, err := service.Open("linked.txt"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("Open symlink error = %v, want ErrUnsafePath", err)
	}

	fifoPath := filepath.Join(service.Root(), "blocked.txt")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	if _, _, err := service.Open("blocked.txt"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("Open FIFO error = %v, want ErrUnsafePath", err)
	}
	if _, _, err := service.Open(""); !errors.Is(err, ErrIsDirectory) {
		t.Fatalf("Open directory error = %v, want ErrIsDirectory", err)
	}
}

func TestRemoveRegularDeletesOnlyVerifiedRegularFiles(t *testing.T) {
	service := newTestService(t)
	regularPath := filepath.Join(service.Root(), "regular.txt")
	if err := os.WriteFile(regularPath, []byte("regular"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := service.RemoveRegular("regular.txt")
	if err != nil || info.Size() != int64(len("regular")) {
		t.Fatalf("remove regular info=%v err=%v", info, err)
	}
	if _, err := os.Lstat(regularPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("regular file still exists: %v", err)
	}

	directoryPath := filepath.Join(service.Root(), "directory")
	if err := os.Mkdir(directoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RemoveRegular("directory"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("remove directory error=%v", err)
	}
	if _, err := os.Lstat(directoryPath); err != nil {
		t.Fatalf("remove regular changed directory: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(service.Root(), "linked.txt")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	if _, err := service.RemoveRegular("linked.txt"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("remove symlink error=%v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatalf("remove regular changed symlink target: data=%q err=%v", data, err)
	}
}

func TestMkdirReportsAFileParentAsNotDirectory(t *testing.T) {
	service := newTestService(t)
	if err := os.WriteFile(filepath.Join(service.Root(), "parent"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := service.Mkdir("parent/child"); !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("Mkdir below file error = %v, want ErrNotDirectory", err)
	}
}

func TestNewScopedRejectsSymlinkBetweenWebDAVRootAndPrivateUserRoot(t *testing.T) {
	base := filepath.Join(t.TempDir(), "webdav")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "users")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := NewScoped(base, filepath.Join(base, "users", "member")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("NewScoped through private-root symlink error = %v, want ErrUnsafePath", err)
	}
}

func TestRecursiveCopyFailurePreservesExistingDestination(t *testing.T) {
	service := newTestService(t)
	if err := os.MkdirAll(filepath.Join(service.Root(), "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(service.Root(), "source", "good.txt"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(service.Root(), "source", "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(service.Root(), "destination.txt"), []byte("keep destination"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := service.Copy(context.Background(), "source", "destination.txt", true)
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("copy symlink tree error = %v, want ErrUnsafePath", err)
	}
	content, readErr := os.ReadFile(filepath.Join(service.Root(), "destination.txt"))
	if readErr != nil || string(content) != "keep destination" {
		t.Fatalf("failed copy changed destination: content=%q err=%v", content, readErr)
	}
	entries, readDirErr := os.ReadDir(service.Root())
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".webdav-copy-") || strings.HasPrefix(entry.Name(), ".webdav-replace-") {
			t.Fatalf("failed copy left staging entry %q", entry.Name())
		}
	}
}
