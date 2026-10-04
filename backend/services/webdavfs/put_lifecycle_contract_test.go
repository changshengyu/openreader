package webdavfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type putMutationReader struct {
	mutate func()
	done   bool
}

func (r *putMutationReader) Read(data []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	r.mutate()
	return copy(data, "uploaded"), io.EOF
}

func putFixtureFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPutFile(t *testing.T, name, content string) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil || string(data) != content {
		t.Fatalf("file %s = %q, %v; want %q", name, data, err, content)
	}
}

func assertNoPutStage(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".webdav-upload-") || strings.HasPrefix(entry.Name(), ".webdav-replace-") || strings.HasPrefix(entry.Name(), ".openreader-write-") {
			t.Errorf("upload left request staging entry %q", entry.Name())
		}
	}
}

func TestPutRejectsAncestorReplacementDuringBodyRead(t *testing.T) {
	for _, component := range []string{"root", "parent"} {
		for _, replacement := range []string{"symlink", "directory"} {
			t.Run(component+"/"+replacement, func(t *testing.T) {
				service := newTestService(t)
				parent := filepath.Join(service.Root(), "collection")
				if err := os.Mkdir(parent, 0o700); err != nil {
					t.Fatal(err)
				}
				putFixtureFile(t, filepath.Join(parent, "target"), "original")
				outside := t.TempDir()
				outsideParent := outside
				changed := parent
				if component == "root" {
					changed = service.Root()
					outsideParent = filepath.Join(outside, "collection")
					if err := os.Mkdir(outsideParent, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				putFixtureFile(t, filepath.Join(outsideParent, "target"), "outside")
				original := changed + "-original"
				reader := &putMutationReader{mutate: func() {
					if err := os.Rename(changed, original); err != nil {
						t.Fatal(err)
					}
					if replacement == "symlink" {
						if err := os.Symlink(outside, changed); err != nil {
							t.Fatal(err)
						}
					} else {
						newParent := changed
						if component == "root" {
							newParent = filepath.Join(changed, "collection")
						}
						if err := os.MkdirAll(newParent, 0o700); err != nil {
							t.Fatal(err)
						}
						putFixtureFile(t, filepath.Join(newParent, "target"), "replacement")
					}
				}}
				err := service.Put(context.Background(), "collection/target", reader, 1024)
				if !errors.Is(err, ErrUnsafePath) {
					t.Errorf("upload after ancestor replacement = %v, want ErrUnsafePath", err)
				}
				originalParent := original
				if component == "root" {
					originalParent = filepath.Join(original, "collection")
				}
				assertPutFile(t, filepath.Join(originalParent, "target"), "original")
				assertPutFile(t, filepath.Join(outsideParent, "target"), "outside")
				assertNoPutStage(t, originalParent)
				if replacement == "directory" {
					assertPutFile(t, filepath.Join(parent, "target"), "replacement")
				}
			})
		}
	}
}

func TestPutRejectsTargetReplacementDuringBodyRead(t *testing.T) {
	for _, replacement := range []string{"regular", "directory", "symlink", "new-target"} {
		t.Run(replacement, func(t *testing.T) {
			service := newTestService(t)
			target := filepath.Join(service.Root(), "target")
			original := filepath.Join(service.Root(), "original")
			outside := filepath.Join(t.TempDir(), "sentinel")
			putFixtureFile(t, outside, "outside")
			if replacement != "new-target" {
				putFixtureFile(t, target, "original")
			}
			reader := &putMutationReader{mutate: func() {
				if replacement != "new-target" {
					if err := os.Rename(target, original); err != nil {
						t.Fatal(err)
					}
				}
				switch replacement {
				case "regular", "new-target":
					putFixtureFile(t, target, "replacement")
				case "directory":
					if err := os.Mkdir(target, 0o700); err != nil {
						t.Fatal(err)
					}
					putFixtureFile(t, filepath.Join(target, "sentinel"), "replacement")
				case "symlink":
					if err := os.Symlink(outside, target); err != nil {
						t.Fatal(err)
					}
				}
			}}
			if err := service.Put(context.Background(), "target", reader, 1024); !errors.Is(err, ErrUnsafePath) {
				t.Errorf("upload after target replacement = %v, want ErrUnsafePath", err)
			}
			if replacement != "new-target" {
				assertPutFile(t, original, "original")
			}
			assertPutFile(t, outside, "outside")
			switch replacement {
			case "regular", "new-target":
				assertPutFile(t, target, "replacement")
			case "directory":
				assertPutFile(t, filepath.Join(target, "sentinel"), "replacement")
			case "symlink":
				if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("upload replaced new symlink: info=%v err=%v", info, err)
				}
			}
			assertNoPutStage(t, service.Root())
		})
	}
}

func TestPutRejectsStagingReplacementDuringBodyRead(t *testing.T) {
	service := newTestService(t)
	putFixtureFile(t, filepath.Join(service.Root(), "target"), "original")
	reader := &putMutationReader{mutate: func() {
		entries, err := os.ReadDir(service.Root())
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".webdav-upload-") || strings.HasPrefix(entry.Name(), ".openreader-write-") {
				stage := filepath.Join(service.Root(), entry.Name())
				if err := os.Rename(stage, stage+"-moved"); err != nil {
					t.Fatal(err)
				}
				putFixtureFile(t, stage, "imposter")
				return
			}
		}
		t.Fatal("upload reader did not observe a request stage")
	}}
	if err := service.Put(context.Background(), "target", reader, 1024); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("upload with replaced stage = %v, want ErrUnsafePath", err)
	}
	assertPutFile(t, filepath.Join(service.Root(), "target"), "original")
}

func TestPutPreservesNormalUploadAndFailureContract(t *testing.T) {
	for _, name := range []string{"target", " spaced name ", " "} {
		t.Run(name, func(t *testing.T) {
			service := newTestService(t)
			for _, body := range []string{"created", "overwritten", ""} {
				if err := service.Put(context.Background(), name, strings.NewReader(body), 11); err != nil {
					t.Fatal(err)
				}
				assertPutFile(t, filepath.Join(service.Root(), name), body)
				info, err := os.Stat(filepath.Join(service.Root(), name))
				if err != nil || info.Mode().Perm() != 0o644 {
					t.Fatalf("upload permission = %v, %v", info, err)
				}
				assertNoPutStage(t, service.Root())
			}
		})
	}
	for _, failure := range []string{"over-limit", "read-error", "pre-cancel", "last-read-cancel"} {
		t.Run(failure, func(t *testing.T) {
			service := newTestService(t)
			target := filepath.Join(service.Root(), "target")
			putFixtureFile(t, target, "original")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var reader io.Reader = strings.NewReader("12345")
			var expected error
			limit := int64(4)
			switch failure {
			case "over-limit":
				expected = ErrTooLarge
			case "read-error":
				expected = errors.New("failed upload read")
				reader = putErrorReader{err: expected}
			case "pre-cancel":
				cancel()
				expected = context.Canceled
			case "last-read-cancel":
				reader = &putMutationReader{mutate: cancel}
				expected = context.Canceled
				limit = 16
			}
			if err := service.Put(ctx, "target", reader, limit); !errors.Is(err, expected) {
				t.Errorf("failed upload = %v, want %v", err, expected)
			}
			assertPutFile(t, target, "original")
			assertNoPutStage(t, service.Root())
		})
	}
	service := newTestService(t)
	if err := service.Put(context.Background(), "target", strings.NewReader("1234"), 4); err != nil {
		t.Fatalf("exact upload limit: %v", err)
	}
}

type putErrorReader struct{ err error }

func (r putErrorReader) Read(data []byte) (int, error) { return copy(data, "partial"), r.err }
