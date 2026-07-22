package webdavfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
