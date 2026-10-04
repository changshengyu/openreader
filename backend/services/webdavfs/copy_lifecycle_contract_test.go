package webdavfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Trigger only once a copy stage exists: this is after admission on BOTH the
// old absolute-path implementation and the opened-handle replacement.
type copyBoundaryContext struct {
	context.Context
	mutate func() bool
	fired  bool
}

func (c *copyBoundaryContext) Err() error {
	if !c.fired && c.mutate() {
		c.fired = true
		return nil
	}
	return c.Context.Err()
}

func copyStage(parent string) string {
	entries, _ := os.ReadDir(parent)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".webdav-copy-") || strings.HasPrefix(entry.Name(), ".openreader-copy-stage-") {
			return filepath.Join(parent, entry.Name())
		}
	}
	return ""
}

func TestCopyRejectsWorkingPhaseIdentityChanges(t *testing.T) {
	for _, kind := range []string{"source ancestor symlink", "source real directory", "destination parent symlink", "destination real directory", "target regular", "target directory", "stage replacement", "initially missing target"} {
		t.Run(kind, func(t *testing.T) {
			s := newTestService(t)
			root := s.Root()
			sourceParent := filepath.Join(root, "source-parent")
			destinationParent := filepath.Join(root, "destination-parent")
			if err := os.MkdirAll(sourceParent, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(destinationParent, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sourceParent, "source.txt"), []byte("admitted source"), 0o644); err != nil {
				t.Fatal(err)
			}
			final := filepath.Join(destinationParent, "final.txt")
			if kind != "initially missing target" {
				if err := os.WriteFile(final, []byte("old final"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "source.txt"), []byte("outside secret"), 0o644); err != nil {
				t.Fatal(err)
			}
			ctx := &copyBoundaryContext{Context: context.Background()}
			ctx.mutate = func() bool {
				stage := copyStage(destinationParent)
				if stage == "" {
					return false
				}
				switch kind {
				case "source ancestor symlink", "source real directory":
					if err := os.Rename(sourceParent, sourceParent+"-held"); err != nil {
						t.Fatal(err)
					}
					if kind == "source ancestor symlink" {
						if err := os.Symlink(outside, sourceParent); err != nil {
							t.Fatal(err)
						}
					} else {
						if err := os.Mkdir(sourceParent, 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(sourceParent, "source.txt"), []byte("replacement source"), 0o644); err != nil {
							t.Fatal(err)
						}
					}
				case "destination parent symlink", "destination real directory":
					if err := os.Rename(destinationParent, destinationParent+"-held"); err != nil {
						t.Fatal(err)
					}
					newParent := destinationParent
					if kind == "destination parent symlink" {
						if err := os.Symlink(outside, destinationParent); err != nil {
							t.Fatal(err)
						}
						newParent = outside
					} else if err := os.Mkdir(destinationParent, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(filepath.Join(newParent, filepath.Base(stage)), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(newParent, "final.txt"), []byte("new parent final"), 0o644); err != nil {
						t.Fatal(err)
					}
				case "target regular", "target directory":
					if err := os.Rename(final, final+"-held"); err != nil {
						t.Fatal(err)
					}
					if kind == "target directory" {
						if err := os.Mkdir(final, 0o755); err != nil {
							t.Fatal(err)
						}
					} else if err := os.WriteFile(final, []byte("new final"), 0o644); err != nil {
						t.Fatal(err)
					}
				case "initially missing target":
					if err := os.WriteFile(final, []byte("new final"), 0o644); err != nil {
						t.Fatal(err)
					}
				case "stage replacement":
					if err := os.Rename(stage, stage+"-held"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(stage, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(stage, "unowned.txt"), []byte("keep unrelated stage"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return true
			}
			err := s.Copy(ctx, "source-parent/source.txt", "destination-parent/final.txt", true)
			if !ctx.fired {
				t.Fatal("copy boundary mutation never fired")
			}
			if !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("COPY admitted replacement: error=%v kind=%s", err, kind)
			}
			if kind == "target directory" {
				if info, err := os.Stat(final); err != nil || !info.IsDir() {
					t.Fatalf("new directory final changed: %v %v", info, err)
				}
			} else {
				want := "old final"
				if strings.HasPrefix(kind, "destination") {
					want = "new parent final"
				}
				if kind == "target regular" || kind == "initially missing target" {
					want = "new final"
				}
				got, err := os.ReadFile(final)
				if err != nil || string(got) != want {
					t.Fatalf("failed COPY changed final bytes: got=%q want=%q err=%v", got, want, err)
				}
			}
			if kind == "stage replacement" {
				stage := copyStage(destinationParent)
				data, err := os.ReadFile(filepath.Join(stage, "unowned.txt"))
				if err != nil || string(data) != "keep unrelated stage" {
					t.Fatalf("cleanup removed unowned replacement %q %v", data, err)
				}
			}
			if kind == "destination parent symlink" {
				data, err := os.ReadFile(filepath.Join(outside, "final.txt"))
				if err != nil || string(data) != "new parent final" {
					t.Fatalf("outside final changed %q %v", data, err)
				}
			}
		})
	}
}

func TestCopyLastReadCancellationCannotPublish(t *testing.T) {
	s := newTestService(t)
	if err := os.WriteFile(filepath.Join(s.Root(), "source.txt"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root(), "final.txt"), []byte("keep final"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &copyBoundaryContext{Context: base, mutate: func() bool {
		stage := copyStage(s.Root())
		if stage == "" {
			return false
		}
		info, err := os.Stat(filepath.Join(stage, "new"))
		if err != nil {
			info, err = os.Stat(stage)
		}
		if err != nil || info.IsDir() || info.Size() == 0 {
			return false
		}
		cancel()
		return true
	}}
	err := s.Copy(ctx, "source.txt", "final.txt", true)
	if !ctx.fired || !errors.Is(err, context.Canceled) {
		t.Fatalf("last-read cancellation published: fired=%v err=%v", ctx.fired, err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root(), "final.txt"))
	if err != nil || string(data) != "keep final" {
		t.Fatalf("cancelled COPY changed final: %q %v", data, err)
	}
}
