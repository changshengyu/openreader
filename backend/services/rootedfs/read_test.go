package rootedfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadReturnedFileKeepsOriginalCompletePathLabelAfterRename(t *testing.T) {
	root, initial := readFixture(t)
	_ = initial.Close()
	relative := "nested/neighbor.txt"
	reader, err := AdmitRead(context.Background(), root, relative)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	file, _, err := reader.OpenRegular()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	originalPath := filepath.Join(root, filepath.FromSlash(relative))
	if file.Name() != originalPath {
		t.Fatalf("complete original path label = %q, want %q", file.Name(), originalPath)
	}
	if err := os.Rename(originalPath, originalPath+"-held"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "original" || file.Name() != originalPath {
		t.Fatalf("returned file reopened label: name=%q bytes=%q err=%v", file.Name(), data, err)
	}
}

func readFixture(t *testing.T) (string, *ReadHandle) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"nested/child/deep/original.txt", "nested/neighbor.txt", "nested/.hidden/secret.txt"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("original"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := AdmitRead(context.Background(), root, "nested")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	return root, reader
}

func TestReadRecursiveCancellationAtSecondDirectoryReturnsNoPartialList(t *testing.T) {
	root, original := readFixture(t)
	_ = original.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, err := AdmitRead(ctx, root, "nested")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	fired := false
	beforeReadDirectoryScanTestHook = func(relative string) {
		if relative == "nested/child/deep" {
			fired = true
			cancel()
		}
	}
	t.Cleanup(func() { beforeReadDirectoryScanTestHook = nil })
	entries, err := reader.List(ReadListOptions{Depth: -1, HideDot: true, SkipUnsafe: true, SkipReadError: true})
	if !fired || !errors.Is(err, context.Canceled) || entries != nil {
		t.Fatalf("deep canceled scan returned partial success: fired=%v err=%v entries=%v", fired, err, entries)
	}
}

func TestReadRecursiveChildReplacementFailsBeforeReturningNames(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		for _, stage := range []string{"before-scan", "after-entry"} {
			t.Run(kind+"/"+stage, func(t *testing.T) {
				root, reader := readFixture(t)
				outside := t.TempDir()
				if err := os.WriteFile(filepath.Join(outside, "foreign-secret.txt"), []byte("foreign"), 0o600); err != nil {
					t.Fatal(err)
				}
				fired := false
				replace := func(relative string) {
					if fired || relative != "nested/child/deep" {
						return
					}
					fired = true
					target := filepath.Join(root, relative)
					if err := os.Rename(target, target+"-held"); err != nil {
						t.Fatal(err)
					}
					if kind == "symlink" {
						if err := os.Symlink(outside, target); err != nil {
							t.Fatal(err)
						}
					} else if err := os.Rename(outside, target); err != nil {
						t.Fatal(err)
					}
				}
				if stage == "before-scan" {
					beforeReadDirectoryScanTestHook = replace
				} else {
					afterReadEntryAdmissionTestHook = replace
				}
				t.Cleanup(func() {
					beforeReadDirectoryScanTestHook = nil
					afterReadEntryAdmissionTestHook = nil
				})
				entries, err := reader.List(ReadListOptions{Depth: -1, HideDot: true, SkipUnsafe: true, SkipReadError: true})
				if !fired || !errors.Is(err, ErrUnsafePath) || entries != nil {
					t.Fatalf("deep replacement returned names: fired=%v err=%v entries=%v", fired, err, entries)
				}
				data, err := os.ReadFile(filepath.Join(root, "nested", "child", "deep-held", "original.txt"))
				if err != nil || string(data) != "original" {
					t.Fatalf("read altered original: bytes=%q err=%v", data, err)
				}
			})
		}
	}
}

func TestReadRecursiveEntryReplacementAndCancellationAreNotSkipped(t *testing.T) {
	for _, outcome := range []string{"cancel", "regular", "symlink"} {
		t.Run(outcome, func(t *testing.T) {
			root, initial := readFixture(t)
			_ = initial.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader, err := AdmitRead(ctx, root, "nested")
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			fired := false
			afterReadEntryAdmissionTestHook = func(relative string) {
				if relative != "nested/neighbor.txt" {
					return
				}
				fired = true
				if outcome == "cancel" {
					cancel()
					return
				}
				target := filepath.Join(root, relative)
				if err := os.Rename(target, target+"-held"); err != nil {
					t.Fatal(err)
				}
				if outcome == "symlink" {
					if err := os.Symlink(target+"-held", target); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(target, []byte("new entity"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { afterReadEntryAdmissionTestHook = nil })
			entries, err := reader.List(ReadListOptions{Depth: -1, HideDot: true, SkipUnsafe: true, SkipReadError: true})
			want := ErrUnsafePath
			if outcome == "cancel" {
				want = context.Canceled
			}
			if !fired || !errors.Is(err, want) || entries != nil {
				t.Fatalf("unsafe/cancel entry skipped: fired=%v err=%v entries=%v", fired, err, entries)
			}
		})
	}
}

func TestReadHiddenAndUnsafePoliciesDoNotConverge(t *testing.T) {
	root, reader := readFixture(t)
	if err := os.Symlink(filepath.Join(root, "nested", "neighbor.txt"), filepath.Join(root, "nested", "linked")); err != nil {
		t.Fatal(err)
	}
	entries, err := reader.List(ReadListOptions{Depth: -1, HideDot: true, SkipUnsafe: true, SkipReadError: true})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		if strings.Contains(entry.RelativePath, ".hidden") || strings.Contains(entry.RelativePath, "linked") {
			t.Fatalf("local hidden entity returned: %q", entry.RelativePath)
		}
		names = append(names, entry.RelativePath)
	}
	if strings.Join(names, ",") != "nested,nested/child,nested/child/deep,nested/child/deep/original.txt,nested/neighbor.txt" {
		t.Fatalf("normal recursive neighbors changed: %q", names)
	}
	if entries, err := reader.List(ReadListOptions{Depth: 1}); !errors.Is(err, ErrUnsafePath) || entries != nil {
		t.Fatalf("DAV unsafe entity was hidden: err=%v entries=%v", err, entries)
	}
}
