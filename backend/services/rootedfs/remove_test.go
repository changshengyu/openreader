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
