package rootedfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestCreateDirectoriesRecursiveFreshAndIdempotent(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-root", true: "fresh-configured-root"}[fresh], func(t *testing.T) {
			base := t.TempDir()
			root := base
			if fresh {
				root = filepath.Join(base, "new", "configured-root")
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := CreateDirectories(context.Background(), root, "users/用户/ 目录 /child"); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(root, "users", "用户", " 目录 ", "child")
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
			if err := CreateDirectories(context.Background(), root, "users/用户/ 目录 /child"); err != nil {
				t.Fatalf("unreadable existing final must be idempotent: %v", err)
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != 0 {
				t.Fatalf("changed existing inode/permissions: %v %v", after, err)
			}
		})
	}
}

func TestCreateDirectoriesRejectsWorkingAncestorReplacement(t *testing.T) {
	for _, name := range []string{"root", "parent", "users", "user", "fresh-anchor"} {
		for _, symlink := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/real", true: "/symlink"}[symlink], func(t *testing.T) {
				base := t.TempDir()
				root := filepath.Join(base, "root")
				if err := os.MkdirAll(filepath.Join(root, "users", "member", "parent"), 0o755); err != nil {
					t.Fatal(err)
				}
				changed := root
				relative := "users/member/parent/new/child"
				switch name {
				case "parent":
					changed = filepath.Join(root, "users", "member", "parent")
				case "users":
					changed = filepath.Join(root, "users")
				case "user":
					changed = filepath.Join(root, "users", "member")
				case "fresh-anchor":
					root = filepath.Join(base, "root", "fresh", "configured-root")
					relative = "new/child"
				}
				outside := t.TempDir()
				if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside"), 0o640); err != nil {
					t.Fatal(err)
				}
				fired := false
				beforeDirectoryCreateTestHook = func(_, _ string) {
					if fired {
						return
					}
					fired = true
					if err := os.Rename(changed, changed+"-held"); err != nil {
						t.Fatal(err)
					}
					var err error
					if symlink {
						err = os.Symlink(outside, changed)
					} else {
						err = os.Mkdir(changed, 0o750)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { beforeDirectoryCreateTestHook = nil })
				err := CreateDirectories(context.Background(), root, relative)
				if !fired || !errors.Is(err, ErrUnsafePath) {
					t.Fatalf("replacement not rejected: fired=%v err=%v", fired, err)
				}
				entries, err := os.ReadDir(outside)
				if err != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
					t.Fatalf("external destination changed: %v %v", entries, err)
				}
				if !symlink {
					entries, err := os.ReadDir(changed)
					if err != nil || len(entries) != 0 {
						t.Fatalf("replacement real directory changed: %v %v", entries, err)
					}
				}
			})
		}
	}
}

func TestCreateDirectoriesCancellationAndUnknownMembers(t *testing.T) {
	for _, phase := range []string{"stage", "installed"} {
		for _, unknown := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/empty", true: "/unknown"}[unknown], func(t *testing.T) {
				root := t.TempDir()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				fired := false
				kept := ""
				inject := func(directory string) {
					if fired {
						return
					}
					fired = true
					if unknown {
						kept = filepath.Join(directory, "unknown")
						if err := os.WriteFile(kept, []byte("not owned"), 0o640); err != nil {
							t.Fatal(err)
						}
					}
					cancel()
				}
				if phase == "stage" {
					afterDirectoryStageTestHook = func(anchor, relative, stage string) {
						if relative == filepath.Join("new", "parent") {
							inject(filepath.Join(anchor, filepath.Dir(relative), stage))
						}
					}
				} else {
					afterDirectoryInstallTestHook = func(anchor, relative string) {
						if relative == filepath.Join("new", "parent") {
							inject(filepath.Join(anchor, relative))
						}
					}
				}
				t.Cleanup(func() {
					afterDirectoryStageTestHook = nil
					afterDirectoryInstallTestHook = nil
				})
				err := CreateDirectories(ctx, root, "new/parent/child")
				if !fired || !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel: fired=%v err=%v", fired, err)
				}
				if unknown {
					data, err := os.ReadFile(kept)
					if err != nil || string(data) != "not owned" {
						t.Fatalf("unknown was removed: %q %v", data, err)
					}
				} else if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
					t.Fatalf("owned empty nodes leaked: %v %v", entries, err)
				}
			})
		}
	}
}

func TestCreateDirectoriesNewcomerAndStageReplacement(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "directory", "stage"} {
		t.Run(kind, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			fired := false
			kept := filepath.Join(root, "new")
			afterDirectoryStageTestHook = func(anchor, relative, stage string) {
				if fired {
					return
				}
				fired = true
				var err error
				switch kind {
				case "file":
					err = os.WriteFile(kept, []byte("newcomer"), 0o640)
				case "symlink":
					err = os.Symlink(outside, kept)
				case "directory":
					err = os.Mkdir(kept, 0o750)
				case "stage":
					kept = filepath.Join(anchor, filepath.Dir(relative), stage)
					if err := os.Rename(kept, kept+"-held"); err != nil {
						t.Fatal(err)
					}
					err = os.Mkdir(kept, 0o750)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { afterDirectoryStageTestHook = nil })
			err := CreateDirectories(context.Background(), root, "new")
			if !fired || (kind == "directory" && err != nil) || (kind == "file" && !errors.Is(err, ErrDirectoryConflict)) || (kind != "directory" && kind != "file" && !errors.Is(err, ErrUnsafePath)) {
				t.Fatalf("newcomer result: fired=%v err=%v", fired, err)
			}
			info, err := os.Lstat(kept)
			if err != nil || (kind == "directory" && info.Mode().Perm() != 0o750) {
				t.Fatalf("newcomer changed: %v %v", info, err)
			}
		})
	}
}

func TestCreateDirectoriesConcurrentIdempotence(t *testing.T) {
	root := t.TempDir()
	var group sync.WaitGroup
	errorsOut := make(chan error, 16)
	for index := 0; index < 16; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			errorsOut <- CreateDirectories(context.Background(), root, "users/member/parent/child")
		}()
	}
	group.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || strings.HasPrefix(entry.Name(), ".openreader-directory-") {
			t.Fatalf("concurrent stage leftover: %s %v", path, err)
		}
		return nil
	})
}

func TestCreateDirectoriesPermissionFailureKeepsExistingMode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the permission admission fixture")
	}
	root := t.TempDir()
	parent := filepath.Join(root, "readonly")
	if err := os.Mkdir(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	err := CreateDirectories(context.Background(), root, "readonly/child")
	if !errors.Is(err, syscall.EACCES) {
		t.Fatalf("permission error=%v", err)
	}
	info, err := os.Lstat(parent)
	if err != nil || info.Mode().Perm() != 0o555 {
		t.Fatalf("existing permission changed: %v %v", info, err)
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("permission failure created entries: %v %v", entries, err)
	}
}
