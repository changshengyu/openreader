package rootedfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCopyFixture(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func TestCopyTreeFilesDirectoriesPermissionsAndOverwrite(t *testing.T) {
	for _, kind := range []string{"file", "empty file", "directory", "read-only directory", "space-only file"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			source := "source"
			data := "file content"
			if kind == "space-only file" {
				source = " "
			}
			if kind == "empty file" {
				data = ""
			}
			if strings.Contains(kind, "directory") {
				writeCopyFixture(t, filepath.Join(root, source, "nested", "file.txt"), data, 0o640)
				if err := os.Mkdir(filepath.Join(root, source, "empty"), 0o755); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_ = os.Chmod(filepath.Join(root, source), 0o755)
					_ = os.Chmod(filepath.Join(root, "target"), 0o755)
				})
				if kind == "read-only directory" {
					if err := os.Chmod(filepath.Join(root, source), 0o555); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				writeCopyFixture(t, filepath.Join(root, source), data, 0o640)
			}
			for _, overwrite := range []bool{false, true} {
				if overwrite {
					writeCopyFixture(t, filepath.Join(root, "target"), "replace this", 0o600)
				}
				if err := CopyTree(context.Background(), root, source, "target", overwrite); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(root, "target")
				if strings.Contains(kind, "directory") {
					target = filepath.Join(target, "nested", "file.txt")
				}
				got, err := os.ReadFile(target)
				if err != nil || string(got) != data {
					t.Fatalf("copied bytes %q %v", got, err)
				}
				info, err := os.Stat(target)
				if err != nil || info.Mode().Perm() != 0o640 {
					t.Fatalf("file permissions %v %v", info, err)
				}
				if strings.Contains(kind, "directory") {
					if info, err := os.Stat(filepath.Join(root, "target", "empty")); err != nil || !info.IsDir() {
						t.Fatal("empty directory not preserved")
					}
					if kind == "read-only directory" {
						if info, err := os.Stat(filepath.Join(root, "target")); err != nil || info.Mode().Perm() != 0o555 {
							t.Fatal("directory permissions changed")
						}
					}
				}
				if kind == "read-only directory" {
					if err := os.Chmod(filepath.Join(root, "target"), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if err := RemovePath(root, "target"); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 || entries[0].Name() != source {
				t.Fatalf("copy leaked stage/quarantine: %v %v", entries, err)
			}
		})
	}
}

func TestCopyTreeRecoveryKeepsOriginalAndNewcomer(t *testing.T) {
	for _, kind := range []string{"cancel", "newcomer file", "newcomer directory", "source changes", "stage changes"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			writeCopyFixture(t, filepath.Join(root, "source"), "source", 0o644)
			writeCopyFixture(t, filepath.Join(root, "target", "old.txt"), "old bytes", 0o644)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			afterCopyDetachTestHook = func(_, _ string) {
				switch kind {
				case "cancel":
					cancel()
				case "newcomer file":
					writeCopyFixture(t, filepath.Join(root, "target"), "newcomer", 0o644)
				case "newcomer directory":
					writeCopyFixture(t, filepath.Join(root, "target", "new.txt"), "newcomer", 0o644)
				case "source changes":
					writeCopyFixture(t, filepath.Join(root, "source"), "later source", 0o644)
				case "stage changes":
					entries, _ := os.ReadDir(root)
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), ".openreader-copy-stage-") {
							writeCopyFixture(t, filepath.Join(root, entry.Name()), "stage impostor", 0o644)
						}
					}
				}
			}
			t.Cleanup(func() { afterCopyDetachTestHook = nil })
			err := CopyTree(ctx, root, "source", "target", true)
			if kind == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("expected unsafe, got %v", err)
			}
			oldPath := filepath.Join(root, "target", "old.txt")
			if strings.HasPrefix(kind, "newcomer") {
				entries, _ := os.ReadDir(root)
				oldPath = ""
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".openreader-copy-old-") {
						oldPath = filepath.Join(root, entry.Name(), "old.txt")
					}
				}
				newPath := filepath.Join(root, "target")
				if kind == "newcomer directory" {
					newPath = filepath.Join(newPath, "new.txt")
				}
				got, err := os.ReadFile(newPath)
				if err != nil || string(got) != "newcomer" {
					t.Fatalf("newcomer overwritten %q %v", got, err)
				}
			}
			got, err := os.ReadFile(oldPath)
			if err != nil || string(got) != "old bytes" {
				t.Fatalf("old bytes lost %q %v", got, err)
			}
		})
	}
}

func TestCopyTreePostCommitUnknownCleanupIsPending(t *testing.T) {
	for _, kind := range []string{"unowned stage name", "old new member"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			writeCopyFixture(t, filepath.Join(root, "source"), "copied source", 0o644)
			writeCopyFixture(t, filepath.Join(root, "target", "old.txt"), "old bytes", 0o644)
			unowned := ""
			afterCopyPublishTestHook = func(_, _, stageName string) {
				switch kind {
				case "unowned stage name":
					unowned = filepath.Join(root, stageName, "unowned")
				case "old new member":
					entries, _ := os.ReadDir(root)
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), ".openreader-copy-old-") {
							unowned = filepath.Join(root, entry.Name(), "unowned")
						}
					}
				}
				writeCopyFixture(t, unowned, "keep new entity", 0o644)
			}
			t.Cleanup(func() { afterCopyPublishTestHook = nil })
			err := CopyTree(context.Background(), root, "source", "target", true)
			if !errors.Is(err, ErrCopyCleanupPending) {
				t.Fatalf("missing postcommit diagnostic %v", err)
			}
			got, err := os.ReadFile(filepath.Join(root, "target"))
			if err != nil || string(got) != "copied source" {
				t.Fatalf("completed copy rolled back %q %v", got, err)
			}
			got, err = os.ReadFile(unowned)
			if err != nil || string(got) != "keep new entity" {
				t.Fatalf("cleanup removed unowned entity %q %v", got, err)
			}
		})
	}
}

func TestCopyTreeContainmentAndSourceSymlinks(t *testing.T) {
	root := t.TempDir()
	writeCopyFixture(t, filepath.Join(root, "source", "file"), "source", 0o644)
	for _, dest := range []string{"source/.hidden", "source/new", "source"} {
		if err := CopyTree(context.Background(), root, "source", dest, false); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("containment %s %v", dest, err)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "source", "link")); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(context.Background(), root, "source", "target", false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("source symlink admitted %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "target")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial tree published")
	}
}

func TestCopyTreeReplacingOldTreeNeverFollowsItsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeCopyFixture(t, filepath.Join(root, "source", "new.txt"), "new", 0o644)
	writeCopyFixture(t, filepath.Join(root, "target", "old.txt"), "old", 0o644)
	writeCopyFixture(t, filepath.Join(outside, "keep.txt"), "outside", 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "target", "link")); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(context.Background(), root, "source", "target", true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "target", "new.txt"))
	if err != nil || string(got) != "new" {
		t.Fatal("new tree not installed")
	}
	got, err = os.ReadFile(filepath.Join(outside, "keep.txt"))
	if err != nil || string(got) != "outside" {
		t.Fatal("old target cleanup followed symlink")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		t.Fatalf("old tree leaked %v %v", entries, err)
	}
}

func TestCopyTreePreservesSourceHardLinkedToOldTarget(t *testing.T) {
	for _, tree := range []bool{false, true} {
		t.Run(map[bool]string{false: "source file", true: "source tree"}[tree], func(t *testing.T) {
			root := t.TempDir()
			source := "source"
			original := filepath.Join(root, source)
			if tree {
				original = filepath.Join(original, "nested", "original")
			}
			writeCopyFixture(t, original, "historical hard-link bytes", 0o644)
			if err := os.Link(original, filepath.Join(root, "target")); err != nil {
				t.Fatal(err)
			}
			if err := CopyTree(context.Background(), root, source, "target", true); err != nil {
				t.Fatalf("COPY rejected a legal source hard link: %v", err)
			}
			target := filepath.Join(root, "target")
			if tree {
				target = filepath.Join(target, "nested", "original")
			}
			for _, path := range []string{original, target} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != "historical hard-link bytes" {
					t.Fatalf("hard-link bytes lost: %q %v", got, err)
				}
			}
		})
	}
}

func TestCopyTreeRejectsHardLinkChangesAfterOwnRename(t *testing.T) {
	for _, change := range []string{"bytes", "permissions"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			original := filepath.Join(root, "source", "nested", "original")
			writeCopyFixture(t, original, "historical hard-link bytes", 0o644)
			if err := os.Link(original, filepath.Join(root, "target")); err != nil {
				t.Fatal(err)
			}
			afterCopyDetachTestHook = func(_, _ string) {
				if change == "bytes" {
					writeCopyFixture(t, original, "later actor bytes", 0o644)
				} else if err := os.Chmod(original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { afterCopyDetachTestHook = nil })
			if err := CopyTree(context.Background(), root, "source", "target", true); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("external hard-link change admitted: %v", err)
			}
			for _, path := range []string{original, filepath.Join(root, "target")} {
				got, err := os.ReadFile(path)
				expected := "historical hard-link bytes"
				if change == "bytes" {
					expected = "later actor bytes"
				}
				if err != nil || string(got) != expected {
					t.Fatalf("failed copy lost original/current bytes: %q %v", got, err)
				}
			}
		})
	}
}
