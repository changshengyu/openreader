package rootedfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func privateTestWrite(data string) func(io.Writer) error {
	return func(writer io.Writer) error { _, err := io.WriteString(writer, data); return err }
}
func privateTestBytes(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, []byte(want)) {
		t.Fatalf("fixture bytes: %q %v", got, err)
	}
}

func TestPrivateScopePartialInitializationCloseKeepsUnknownMember(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "unknown-member"}[unknown], func(t *testing.T) {
			root := t.TempDir()
			scope, err := OpenPrivateScope(context.Background(), root, "import-previews/1", true)
			if err != nil {
				t.Fatal(err)
			}
			if unknown {
				if err := os.WriteFile(filepath.Join(scope.PathLabel(), "unknown"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := scope.Close(); err != nil {
				t.Fatal(err)
			}
			if err := scope.Close(); err != nil {
				t.Fatal(err)
			}
			if unknown {
				privateTestBytes(t, filepath.Join(root, "import-previews/1/unknown"), "keep")
			} else if _, err := os.Lstat(filepath.Join(root, "import-previews")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("owned empty directories retained: %v", err)
			}
		})
	}
}

func TestPrivateScopeWriteCancellationRollsBackOriginalButKeepsReplacement(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "newcomer"}[replacement], func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			scope, err := OpenPrivateScope(ctx, root, ".", false)
			if err != nil {
				t.Fatal(err)
			}
			defer scope.Close()
			entry, err := scope.Snapshot("original.book")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, entry.Name)
			err = scope.WriteNew(entry, func(writer io.Writer) error {
				if _, err := io.WriteString(writer, "owned"); err != nil {
					return err
				}
				if replacement {
					if err := os.Rename(path, path+"-held"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("unknown"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				cancel()
				return ctx.Err()
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("write returned %v", err)
			}
			if replacement {
				privateTestBytes(t, path, "unknown")
				privateTestBytes(t, path+"-held", "owned")
			} else if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("owned partial file remains: %v", err)
			}
		})
	}
}

func TestPrivateScopePreparedReplacementCancellationKeepsOriginal(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope, err := OpenPrivateScope(ctx, root, ".", false)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	entry, err := scope.Snapshot("original.parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := scope.WriteNew(entry, privateTestWrite("original")); err != nil {
		t.Fatal(err)
	}
	err = scope.Replace(entry, "original.parsed-", privateTestWrite("replacement"), func(_ string) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("replace returned %v", err)
	}
	privateTestBytes(t, filepath.Join(root, entry.Name), "original")
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary resources retained: %v %v", entries, err)
	}
}

func TestPrivateScopeStableSymlinkAndExistingModesRemainUnchanged(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if scope, err := OpenPrivateScope(context.Background(), root, "link/1", true); !errors.Is(err, ErrUnsafePath) || scope != nil {
		t.Fatalf("link accepted: %v %v", scope, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("existing mode changed: %v %v", info, err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatal("link target initialized")
	}
}

func TestPrivateScopeCanceledPartialInitializationOwnsCloseAndUnknownMembers(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "unknown-member"}[unknown], func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fired := 0
			privateScopeDirectoryCreatedTestHook = func(directory *os.File) {
				fired++
				if fired != 2 {
					return
				}
				if unknown {
					fd, err := unix.Openat(int(directory.Fd()), "unknown", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
					if err != nil {
						t.Fatal(err)
					}
					file := os.NewFile(uintptr(fd), "unknown")
					_, err = file.WriteString("keep")
					closeErr := file.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("fixture write: %v %v", err, closeErr)
					}
				}
				cancel()
			}
			t.Cleanup(func() { privateScopeDirectoryCreatedTestHook = nil })
			scope, err := OpenPrivateScope(ctx, root, "import-previews/1", true)
			if scope != nil {
				_ = scope.Close()
			}
			if fired != 2 || !errors.Is(err, context.Canceled) || scope != nil {
				t.Fatalf("partial initialization accepted: %d %v", fired, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if !unknown {
				if len(entries) != 0 {
					t.Fatalf("owned empty initialization remains: %v", entries)
				}
				return
			}
			children, err := os.ReadDir(filepath.Join(root, "import-previews"))
			if err != nil || len(children) != 1 {
				t.Fatalf("unknown private directory lost: %v %v", children, err)
			}
			privateTestBytes(t, filepath.Join(root, "import-previews", children[0].Name(), "unknown"), "keep")
		})
	}
}
