package webdavfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadListRejectsAdmittedDirectoryReplacement(t *testing.T) {
	for _, operation := range []string{"stat", "list"} {
		for _, level := range []string{"boundary", "users", "user", "parent", "target"} {
			for _, replacement := range []string{"directory", "symlink"} {
				t.Run(operation+"/"+level+"/"+replacement, func(t *testing.T) {
					boundary := filepath.Join(t.TempDir(), "webdav")
					root := filepath.Join(boundary, "users", "alice")
					relative := "parent/target"
					if err := os.MkdirAll(filepath.Join(root, relative), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(root, relative, "original.txt"), []byte("original"), 0o640); err != nil {
						t.Fatal(err)
					}
					service, err := NewScoped(boundary, root)
					if err != nil {
						t.Fatal(err)
					}
					replaced := map[string]string{
						"boundary": boundary,
						"users":    filepath.Join(boundary, "users"),
						"user":     root,
						"parent":   filepath.Join(root, "parent"),
						"target":   filepath.Join(root, relative),
					}[level]
					targetSuffix, err := filepath.Rel(replaced, filepath.Join(root, relative))
					if err != nil {
						t.Fatal(err)
					}
					outside := t.TempDir()
					if err := os.MkdirAll(filepath.Join(outside, targetSuffix), 0o755); err != nil {
						t.Fatal(err)
					}
					bait := filepath.Join(outside, targetSuffix, "foreign-secret.txt")
					if err := os.WriteFile(bait, []byte("foreign bytes"), 0o600); err != nil {
						t.Fatal(err)
					}
					fired := false
					afterReadAdmissionTestHook = func(op, _, _ string) {
						if fired || op != operation {
							return
						}
						fired = true
						if err := os.Rename(replaced, replaced+"-held"); err != nil {
							t.Fatal(err)
						}
						if replacement == "symlink" {
							if err := os.Symlink(outside, replaced); err != nil {
								t.Fatal(err)
							}
						} else if err := os.Rename(outside, replaced); err != nil {
							t.Fatal(err)
						}
					}
					t.Cleanup(func() { afterReadAdmissionTestHook = nil })
					var resources []Resource
					if operation == "stat" {
						var resource Resource
						resource, err = service.Stat(relative)
						resources = []Resource{resource}
					} else {
						resources, err = service.List(relative, 1)
					}
					if !fired || !errors.Is(err, ErrUnsafePath) {
						names := make([]string, 0, len(resources))
						for _, resource := range resources {
							names = append(names, resource.RelativePath)
						}
						t.Errorf("admitted replacement accepted: fired=%v err=%v names=%q", fired, err, names)
					}
					preservedBait := bait
					if replacement == "directory" {
						preservedBait = filepath.Join(replaced, targetSuffix, "foreign-secret.txt")
					}
					for _, expected := range []struct{ path, bytes string }{
						{preservedBait, "foreign bytes"},
						{filepath.Join(replaced+"-held", targetSuffix, "original.txt"), "original"},
					} {
						data, err := os.ReadFile(expected.path)
						if err != nil || string(data) != expected.bytes {
							t.Fatalf("read changed fixture bytes: got=%q err=%v", data, err)
						}
					}
				})
			}
		}
	}
}

func TestReadOpenRejectsLateSameInodeSymlink(t *testing.T) {
	service := newTestService(t)
	final := filepath.Join(service.Root(), "original.txt")
	if err := os.WriteFile(final, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(t.TempDir(), "held.txt")
	fired := false
	afterReadAdmissionTestHook = func(operation, _, _ string) {
		if fired || operation != "open" {
			return
		}
		fired = true
		if err := os.Rename(final, held); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(held, final); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { afterReadAdmissionTestHook = nil })
	file, _, err := service.Open("original.txt")
	var data []byte
	if file != nil {
		data, _ = io.ReadAll(file)
		_ = file.Close()
	}
	if !fired || !errors.Is(err, ErrUnsafePath) {
		t.Errorf("late symlink accepted: fired=%v err=%v bytes=%q", fired, err, data)
	}
	if data, err := os.ReadFile(held); err != nil || string(data) != "original" {
		t.Fatalf("held fixture changed: bytes=%q err=%v", data, err)
	}
}

func TestReadOpenLateFIFOIsRejectedWithoutBlocking(t *testing.T) {
	service := newTestService(t)
	final := filepath.Join(service.Root(), "original.txt")
	if err := os.WriteFile(final, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	var fixtureErr error
	var fired atomic.Bool
	afterReadAdmissionTestHook = func(operation, _, _ string) {
		if operation != "open" || fired.Swap(true) {
			return
		}
		defer close(ready)
		if fixtureErr = os.Rename(final, final+"-held"); fixtureErr != nil {
			return
		}
		fixtureErr = unix.Mkfifo(final, 0o600)
	}
	t.Cleanup(func() { afterReadAdmissionTestHook = nil })
	result := make(chan error, 1)
	go func() {
		file, _, err := service.Open("original.txt")
		if file != nil {
			_ = file.Close()
		}
		result <- err
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("FIFO fixture boundary did not become ready")
	}
	if fixtureErr != nil {
		<-result
		t.Fatal(fixtureErr)
	}
	var err error
	select {
	case err = <-result:
	case <-time.After(500 * time.Millisecond):
		// Open a temporary nonblocking peer ONLY to release the old blocking
		// os.Open. Always join the worker before clearing the nonparallel hook.
		fd, releaseErr := unix.Open(final, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if releaseErr != nil {
			t.Fatalf("could not release old FIFO reader: %v", releaseErr)
		}
		defer unix.Close(fd)
		select {
		case err = <-result:
		case <-time.After(time.Second):
			t.Fatal("old FIFO reader did not join after peer release")
		}
		t.Errorf("read blocked on late FIFO instead of rejecting it at open")
	}
	if !fired.Load() || !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("late FIFO error: fired=%v err=%v", fired.Load(), err)
	}
	if data, err := os.ReadFile(final + "-held"); err != nil || string(data) != "original" {
		t.Fatalf("FIFO fixture changed original: bytes=%q err=%v", data, err)
	}
}

func TestReadStatUnreadableRegularFileDoesNotOpenOrChmod(t *testing.T) {
	service := newTestService(t)
	final := filepath.Join(service.Root(), "metadata-only.txt")
	if err := os.WriteFile(final, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(final, 0); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(final)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := service.Stat("metadata-only.txt")
	if err != nil || !os.SameFile(before, resource.Info) || resource.Info.Mode().Perm() != 0 || resource.Info.Size() != 8 {
		t.Fatalf("metadata-only read regressed: info=%v err=%v", resource.Info, err)
	}
}

func TestReadOpenedFileNeverSwitchesAfterReturn(t *testing.T) {
	service := newTestService(t)
	final := filepath.Join(service.Root(), "original.txt")
	if err := os.WriteFile(final, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	file, info, err := service.Open("original.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Rename(final, final+"-held"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "original" || info.Size() != int64(len(data)) {
		t.Fatalf("opened handle switched: bytes=%q size=%d err=%v", data, info.Size(), err)
	}
}
