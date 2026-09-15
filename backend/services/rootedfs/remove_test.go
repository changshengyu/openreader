package rootedfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveDirectoryRejectsUnsafeComponentsAndPreservesExternalData(t *testing.T) {
	t.Run("root-symlink", func(t *testing.T) {
		outside := t.TempDir()
		rootLink := filepath.Join(t.TempDir(), "root")
		if err := os.Symlink(outside, rootLink); err != nil {
			t.Skipf("symlink fixture unavailable: %v", err)
		}
		if err := RemoveDirectory(rootLink, "users/target", nil); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("root symlink error=%v, want ErrUnsafePath", err)
		}
	})

	for _, fixture := range []struct {
		name  string
		setup func(t *testing.T, root, outside string)
	}{
		{
			name: "ancestor-symlink",
			setup: func(t *testing.T, root, outside string) {
				t.Helper()
				if err := os.Symlink(outside, filepath.Join(root, "users")); err != nil {
					t.Skipf("symlink fixture unavailable: %v", err)
				}
			},
		},
		{
			name: "target-symlink",
			setup: func(t *testing.T, root, outside string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, "users"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "target"), filepath.Join(root, "users", "target")); err != nil {
					t.Skipf("symlink fixture unavailable: %v", err)
				}
			},
		},
		{
			name: "target-regular-file",
			setup: func(t *testing.T, root, _ string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, "users"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "users", "target"), []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			outsideTarget := filepath.Join(outside, "target")
			if err := os.MkdirAll(outsideTarget, 0o700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(outsideTarget, "sentinel")
			if err := os.WriteFile(sentinel, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixture.setup(t, root, outside)

			if err := RemoveDirectory(root, filepath.Join("users", "target"), nil); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("unsafe target error=%v, want ErrUnsafePath", err)
			}
			if data, err := os.ReadFile(sentinel); err != nil || string(data) != "outside" {
				t.Fatalf("unsafe removal touched external data: data=%q err=%v", data, err)
			}
		})
	}
}

func TestRemoveDirectoryDetachesValidatedIdentityAndConfinesRecursiveRemoval(t *testing.T) {
	t.Run("internal-symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		target := filepath.Join(root, "users", "target")
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(outside, "sentinel")
		if err := os.WriteFile(sentinel, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(target, "external")); err != nil {
			t.Skipf("symlink fixture unavailable: %v", err)
		}

		if err := RemoveDirectory(root, filepath.Join("users", "target"), nil); err != nil {
			t.Fatalf("remove rooted tree with internal symlink: %v", err)
		}
		if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("target remains after cleanup: %v", err)
		}
		if data, err := os.ReadFile(sentinel); err != nil || string(data) != "outside" {
			t.Fatalf("recursive cleanup escaped rooted parent: data=%q err=%v", data, err)
		}
	})

	t.Run("replacement-after-validation", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		target := filepath.Join(root, "users", "target")
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		original := filepath.Join(root, "users", "original")
		sentinel := filepath.Join(outside, "sentinel")
		if err := os.WriteFile(sentinel, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		beforeDetachTestHook = func(_ string, _ string) {
			if err := os.Rename(target, original); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, target); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { beforeDetachTestHook = nil })

		if err := RemoveDirectory(root, filepath.Join("users", "target"), nil); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("replaced target error=%v, want ErrUnsafePath", err)
		}
		if data, err := os.ReadFile(sentinel); err != nil || string(data) != "outside" {
			t.Fatalf("replacement cleanup touched external data: data=%q err=%v", data, err)
		}
		if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("replacement link changed: info=%v err=%v", info, err)
		}
		if info, err := os.Stat(original); err != nil || !info.IsDir() {
			t.Fatalf("validated original identity changed: info=%v err=%v", info, err)
		}
	})

	t.Run("missing-is-idempotent", func(t *testing.T) {
		if err := RemoveDirectory(t.TempDir(), filepath.Join("users", "missing"), nil); err != nil {
			t.Fatalf("missing cleanup error=%v", err)
		}
	})
}

func TestRemovePathSupportsFilesAndDirectoriesWithVisibleMissing(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "files", "item.txt")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemovePath(root, "files/item.txt"); err != nil {
		t.Fatalf("remove regular file: %v", err)
	}
	if _, err := os.Lstat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("regular file remains: %v", err)
	}

	directory := filepath.Join(root, "trees", "target", "nested")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "item.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemovePath(root, "trees/target"); err != nil {
		t.Fatalf("remove directory: %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(directory)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory remains: %v", err)
	}
	if err := RemovePath(root, "trees/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error=%v, want os.ErrNotExist", err)
	}
}

func TestRemovePathRejectsParentReplacementAfterValidation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(root, "users", "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	originalParent := filepath.Join(root, "original-users")
	outsideTarget := filepath.Join(outside, "target")
	if err := os.MkdirAll(outsideTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outsideTarget, "sentinel")
	if err := os.WriteFile(sentinel, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeDetachTestHook = func(_, _ string) {
		if err := os.Rename(filepath.Join(root, "users"), originalParent); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "users")); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { beforeDetachTestHook = nil })

	if err := RemovePath(root, "users/target"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("replaced parent error=%v, want ErrUnsafePath", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "outside" {
		t.Fatalf("parent replacement touched external data: data=%q err=%v", data, err)
	}
	if info, err := os.Stat(filepath.Join(originalParent, "target")); err != nil || !info.IsDir() {
		t.Fatalf("validated target changed: info=%v err=%v", info, err)
	}
}

func TestRemovePathRejectsFileReplacementAfterValidation(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "files")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(parent, "original.txt")
	beforeDetachTestHook = func(_, _ string) {
		if err := os.Rename(target, original); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { beforeDetachTestHook = nil })

	if err := RemovePath(root, "files/target.txt"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("replaced file error=%v, want ErrUnsafePath", err)
	}
	if data, err := os.ReadFile(original); err != nil || string(data) != "original" {
		t.Fatalf("validated file changed: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "replacement" {
		t.Fatalf("replacement file changed: data=%q err=%v", data, err)
	}
}

func TestRemovePathRejectsRootReplacementAfterValidation(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	originalRoot := filepath.Join(base, "original-root")
	outside := t.TempDir()
	outsideTarget := filepath.Join(outside, "target")
	if err := os.MkdirAll(outsideTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outsideTarget, "sentinel")
	if err := os.WriteFile(sentinel, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeDetachTestHook = func(_, _ string) {
		if err := os.Rename(root, originalRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, root); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { beforeDetachTestHook = nil })

	if err := RemovePath(root, "target"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("replaced root error=%v, want ErrUnsafePath", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "outside" {
		t.Fatalf("root replacement touched external data: data=%q err=%v", data, err)
	}
	if info, err := os.Stat(filepath.Join(originalRoot, "target")); err != nil || !info.IsDir() {
		t.Fatalf("validated root target changed: info=%v err=%v", info, err)
	}
}
