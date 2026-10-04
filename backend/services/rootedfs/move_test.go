package rootedfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMoveTreePreservesInodePermissionsAndUnreadableSource(t *testing.T) {
	for _, kind := range []string{"file", "empty file", "directory", "read-only directory", "unreadable file", "unreadable directory", "space-only file"} {
		for _, crossParent := range []bool{false, true} {
			if kind == "unreadable directory" && crossParent {
				continue // Moving .. across parents may require source-directory write permission.
			}
			t.Run(kind+map[bool]string{false: "/same parent", true: "/other parent"}[crossParent], func(t *testing.T) {
				root := t.TempDir()
				source := "source"
				if kind == "space-only file" {
					source = " "
				}
				destination := "target"
				if crossParent {
					destination = "other/target"
					if err := os.Mkdir(filepath.Join(root, "other"), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				path := filepath.Join(root, source)
				data := "historical source bytes"
				if kind == "empty file" {
					data = ""
				}
				if strings.Contains(kind, "directory") {
					writeCopyFixture(t, filepath.Join(path, "nested", "file.txt"), data, 0o640)
				} else {
					writeCopyFixture(t, path, data, 0o640)
				}
				mode := os.FileMode(0o640)
				if strings.Contains(kind, "directory") {
					mode = 0o755
				}
				if strings.HasPrefix(kind, "read-only") {
					mode = 0o555
				}
				if strings.HasPrefix(kind, "unreadable") {
					mode = 0
				}
				if err := os.Chmod(path, mode); err != nil {
					t.Fatal(err)
				}
				final := filepath.Join(root, destination)
				t.Cleanup(func() { _ = os.Chmod(path, 0o755); _ = os.Chmod(final, 0o755) })
				before, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				writeCopyFixture(t, final, "old target", 0o600)
				err = MoveTree(context.Background(), root, source, destination, true)
				if os.IsPermission(err) && kind == "read-only directory" && crossParent {
					after, statErr := os.Lstat(path)
					old, oldErr := os.ReadFile(final)
					if statErr != nil || !os.SameFile(before, after) || after.Mode().Perm() != mode || oldErr != nil || string(old) != "old target" {
						t.Fatalf("permission failure lost source/old target: %v %v %q %v", after, statErr, old, oldErr)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					after, err := os.Lstat(final)
					if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != mode {
						t.Fatalf("MOVE changed inode/permissions: %v %v", after, err)
					}
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatal("original source still exists")
					}
					_ = os.Chmod(final, 0o755)
					read := final
					if strings.Contains(kind, "directory") {
						read = filepath.Join(final, "nested", "file.txt")
					}
					got, err := os.ReadFile(read)
					if err != nil || string(got) != data {
						t.Fatalf("MOVE changed bytes: %q %v", got, err)
					}
				}
				assertNoMoveArtifacts(t, root)
			})
		}
	}
}

func assertNoMoveArtifacts(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".openreader-move-") {
			t.Errorf("unexpected MOVE artifact %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMoveTreeAdmissionChangesNeverTouchReplacement(t *testing.T) {
	for _, action := range []string{"source", "source parent", "destination parent", "target", "target symlink"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			writeCopyFixture(t, filepath.Join(root, "from", "source"), "original source", 0o644)
			writeCopyFixture(t, filepath.Join(root, "to", "target"), "original target", 0o644)
			beforeMoveClaimTestHook = func(_, _, _ string) {
				name := map[string]string{"source": "from/source", "source parent": "from", "destination parent": "to", "target": "to/target", "target symlink": "to/target"}[action]
				path := filepath.Join(root, name)
				if err := os.Rename(path, path+"-held"); err != nil {
					t.Fatal(err)
				}
				if action == "target symlink" {
					if err := os.Symlink(path+"-held", path); err != nil {
						t.Fatal(err)
					}
				} else if strings.Contains(action, "parent") {
					writeCopyFixture(t, filepath.Join(path, "unknown"), "replacement", 0o644)
				} else {
					writeCopyFixture(t, path, "replacement", 0o644)
				}
			}
			t.Cleanup(func() { beforeMoveClaimTestHook = nil })
			if err := MoveTree(context.Background(), root, "from/source", "to/target", true); !errors.Is(err, ErrUnsafePath) {
				t.Fatal(err)
			}
			source := filepath.Join(root, "from", "source")
			if action == "source" {
				source += "-held"
			} else if action == "source parent" {
				source = filepath.Join(root, "from-held", "source")
			}
			got, err := os.ReadFile(source)
			if err != nil || string(got) != "original source" {
				t.Fatalf("original source lost %q %v", got, err)
			}
			assertNoMoveArtifacts(t, root)
		})
	}
}

func TestMoveTreeRecoveryNeverOverwritesNewSourceOrFinal(t *testing.T) {
	for _, action := range []string{"cancel", "new source", "new final", "source claim replacement", "old claim replacement"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			writeCopyFixture(t, filepath.Join(root, "source"), "original source", 0o644)
			writeCopyFixture(t, filepath.Join(root, "target", "old.txt"), "original target", 0o644)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			unknown := ""
			afterMoveDetachTestHook = func(_, _, sourceClaim, oldClaim string) {
				switch action {
				case "cancel":
					cancel()
				case "new source", "new final":
					name := map[string]string{"new source": "source", "new final": "target"}[action]
					unknown = filepath.Join(root, name)
					writeCopyFixture(t, unknown, "newcomer", 0o644)
				case "source claim replacement", "old claim replacement":
					name := sourceClaim
					if action == "old claim replacement" {
						name = oldClaim
					}
					unknown = filepath.Join(root, name)
					if err := os.Rename(unknown, unknown+"-held"); err != nil {
						t.Fatal(err)
					}
					writeCopyFixture(t, unknown, "newcomer", 0o644)
				}
			}
			t.Cleanup(func() { afterMoveDetachTestHook = nil })
			err := MoveTree(ctx, root, "source", "target", true)
			if action == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrUnsafePath) {
				t.Fatal(err)
			}
			if unknown != "" {
				got, err := os.ReadFile(unknown)
				if err != nil || string(got) != "newcomer" {
					t.Fatalf("compensation overwrote newcomer %q %v", got, err)
				}
			}
			foundSource, foundTarget := false, false
			err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					got, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}
					foundSource = foundSource || string(got) == "original source"
					foundTarget = foundTarget || string(got) == "original target"
				}
				return nil
			})
			if err != nil || !foundSource || !foundTarget {
				t.Fatalf("recovery lost admitted bytes source=%v target=%v err=%v", foundSource, foundTarget, err)
			}
		})
	}
}

func TestMoveTreeNestedLinksAndHardLinkedTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeCopyFixture(t, filepath.Join(root, "source", "file"), "source", 0o640)
	writeCopyFixture(t, filepath.Join(outside, "keep"), "outside", 0o640)
	if err := os.Symlink(outside, filepath.Join(root, "source", "link")); err != nil {
		t.Fatal(err)
	}
	if err := MoveTree(context.Background(), root, "source", "target", false); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(filepath.Join(root, "target", "link")); err != nil || got != outside {
		t.Fatal("nested link was followed or changed")
	}
	if got, err := os.ReadFile(filepath.Join(outside, "keep")); err != nil || string(got) != "outside" {
		t.Fatal("outside content changed")
	}
	for _, nestedOld := range []bool{false, true} {
		t.Run(map[bool]string{false: "file hard link", true: "old tree hard link"}[nestedOld], func(t *testing.T) {
			root := t.TempDir()
			writeCopyFixture(t, filepath.Join(root, "source"), "hard-linked", 0o640)
			old := filepath.Join(root, "target")
			if nestedOld {
				old = filepath.Join(old, "nested", "old")
				if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Link(filepath.Join(root, "source"), old); err != nil {
				t.Fatal(err)
			}
			before, _ := os.Lstat(filepath.Join(root, "source"))
			if err := MoveTree(context.Background(), root, "source", "target", true); err != nil {
				t.Fatal(err)
			}
			after, err := os.Lstat(filepath.Join(root, "target"))
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("hard-linked source inode lost")
			}
			if got, err := os.ReadFile(filepath.Join(root, "target")); err != nil || string(got) != "hard-linked" {
				t.Fatal("hard-linked bytes lost")
			}
			assertNoMoveArtifacts(t, root)
		})
	}
}

func TestMoveTreePostCommitUnknownCleanupIsPending(t *testing.T) {
	root := t.TempDir()
	writeCopyFixture(t, filepath.Join(root, "source"), "published source", 0o644)
	writeCopyFixture(t, filepath.Join(root, "target", "old"), "old bytes", 0o644)
	unknown := ""
	afterMovePublishTestHook = func(_, _, old string) {
		unknown = filepath.Join(root, old, "unknown")
		writeCopyFixture(t, unknown, "new entity", 0o644)
	}
	t.Cleanup(func() { afterMovePublishTestHook = nil })
	if err := MoveTree(context.Background(), root, "source", "target", true); !errors.Is(err, ErrMoveCleanupPending) {
		t.Fatal(err)
	}
	for path, want := range map[string]string{filepath.Join(root, "target"): "published source", unknown: "new entity"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("postcommit bytes changed %q %v", got, err)
		}
	}
}

func TestMoveTreeNestedSpecialsNeverReadSourceOrFollowOldLinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeCopyFixture(t, filepath.Join(root, "source", "file"), "source", 0o640)
	writeCopyFixture(t, filepath.Join(root, "target", "old"), "old", 0o640)
	writeCopyFixture(t, filepath.Join(outside, "keep"), "outside", 0o640)
	if err := unix.Mkfifo(filepath.Join(root, "source", "stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "target", "old-stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "target", "old-link")); err != nil {
		t.Fatal(err)
	}
	if err := MoveTree(context.Background(), root, "source", "target", true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(root, "target", "stream"))
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("source FIFO was opened, copied or changed")
	}
	if got, err := os.ReadFile(filepath.Join(outside, "keep")); err != nil || string(got) != "outside" {
		t.Fatal("old cleanup followed outside link")
	}
	assertNoMoveArtifacts(t, root)
}

func TestMoveTreeUnreadableOldTargetAdmission(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file needs no read", true: "directory rejects before detach"}[directory], func(t *testing.T) {
			if directory && os.Geteuid() == 0 {
				t.Skip("root bypasses unreadable-directory admission; exercised in non-root Linux runtime")
			}
			root := t.TempDir()
			writeCopyFixture(t, filepath.Join(root, "source"), "source", 0o640)
			old := filepath.Join(root, "target")
			if directory {
				old = filepath.Join(old, "old")
			}
			writeCopyFixture(t, old, "old bytes", 0o600)
			if err := os.Chmod(filepath.Join(root, "target"), 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "target"), 0o755) })
			before, _ := os.Lstat(filepath.Join(root, "target"))
			err := MoveTree(context.Background(), root, "source", "target", true)
			if !directory {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			// The shared no-follow directory opener deliberately collapses open
			// failures to unsafe; ReadDir may retain its permission error instead.
			if !os.IsPermission(err) && !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("unreadable old tree not rejected: %v", err)
			}
			after, err := os.Lstat(filepath.Join(root, "target"))
			if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != 0 {
				t.Fatal("failed admission mutated live old directory")
			}
			if got, err := os.ReadFile(filepath.Join(root, "source")); err != nil || string(got) != "source" {
				t.Fatal("failed admission detached source")
			}
			_ = os.Chmod(filepath.Join(root, "target"), 0o755)
			if got, err := os.ReadFile(old); err != nil || string(got) != "old bytes" {
				t.Fatal("old bytes lost")
			}
			assertNoMoveArtifacts(t, root)
		})
	}
}

// The diagnostic container mounts nested tmpfs at ROOT/to, making Dev differ
// without privileges in the test process or access to any production volume.
func TestMoveTreeCrossDeviceFailsBeforeMutatingEitherSide(t *testing.T) {
	root := os.Getenv("OPENREADER_TEST_MOVE_CROSS_DEVICE_ROOT")
	if root == "" {
		t.Skip("requires isolated two-filesystem fixture")
	}
	sourceDir, err := os.MkdirTemp(root, "source-")
	if err != nil {
		t.Fatal(err)
	}
	destinationDir, err := os.MkdirTemp(filepath.Join(root, "to"), "target-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sourceDir); _ = os.RemoveAll(destinationDir) })
	sourcePath, targetPath := filepath.Join(sourceDir, "source"), filepath.Join(destinationDir, "target")
	writeCopyFixture(t, sourcePath, "source bytes", 0o640)
	writeCopyFixture(t, targetPath, "old bytes", 0o600)
	sourceInfo, _ := os.Lstat(sourcePath)
	targetInfo, _ := os.Lstat(targetPath)
	sourceRelative, _ := filepath.Rel(root, sourcePath)
	targetRelative, _ := filepath.Rel(root, targetPath)
	if err := MoveTree(context.Background(), root, sourceRelative, targetRelative, true); !errors.Is(err, syscall.EXDEV) {
		t.Fatalf("cross-device move: %v", err)
	}
	for path, want := range map[string]string{sourcePath: "source bytes", targetPath: "old bytes"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatal("cross-device move changed bytes")
		}
	}
	afterSource, _ := os.Lstat(sourcePath)
	afterTarget, _ := os.Lstat(targetPath)
	if !os.SameFile(sourceInfo, afterSource) || !os.SameFile(targetInfo, afterTarget) {
		t.Fatal("cross-device move changed original inode")
	}
	assertNoMoveArtifacts(t, sourceDir)
	assertNoMoveArtifacts(t, destinationDir)
}
