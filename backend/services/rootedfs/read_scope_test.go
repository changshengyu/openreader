package rootedfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReadScopeKeepsOriginalCallerAcrossSelections(t *testing.T) {
	for _, level := range []string{"boundary", "users", "user", "parent"} {
		for _, kind := range []string{"directory", "symlink"} {
			t.Run(level+"/"+kind, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "boundary")
				original := filepath.Join(root, "users/alice/parent/book.txt")
				if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(original, []byte("original"), 0o600); err != nil {
					t.Fatal(err)
				}
				anchor, err := AdmitRead(context.Background(), root, "users/alice")
				if err != nil {
					t.Fatal(err)
				}
				scope, err := NewReadScope(anchor)
				if err != nil {
					anchor.Close()
					t.Fatal(err)
				}
				defer scope.Close()
				first, err := scope.Admit("parent/book.txt")
				if err != nil {
					t.Fatal(err)
				}
				defer first.Close()
				replaced := map[string]string{"boundary": root, "users": filepath.Join(root, "users"),
					"user": filepath.Join(root, "users/alice"), "parent": filepath.Dir(original)}[level]
				suffix, _ := filepath.Rel(replaced, original)
				outside := filepath.Join(t.TempDir(), "foreign")
				if err := os.MkdirAll(filepath.Dir(filepath.Join(outside, suffix)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(outside, suffix), []byte("foreign"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replaced, replaced+"-held"); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					if err := os.Symlink(outside, replaced); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Rename(outside, replaced); err != nil {
					t.Fatal(err)
				}
				second, err := scope.Admit("parent/book.txt")
				if second != nil {
					second.Close()
				}
				if !errors.Is(err, ErrUnsafePath) {
					t.Fatalf("second selection re-admitted caller: %v", err)
				}
				file, _, err := first.OpenRegular()
				if file != nil {
					file.Close()
				}
				if !errors.Is(err, ErrUnsafePath) {
					t.Fatalf("first selection lost original parents: %v", err)
				}
			})
		}
	}
}

func TestReadScopeWalkOwnershipClosesSuccessAndFailureWithoutPerFileAncestorCopies(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "selected/deep"), 0o755); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 201; i++ {
				if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("selected/deep/%03d.txt", i)), []byte("original"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			anchor, err := AdmitRead(context.Background(), root, ".")
			if err != nil {
				t.Fatal(err)
			}
			scope, err := NewReadScope(anchor)
			if err != nil {
				anchor.Close()
				t.Fatal(err)
			}
			directory, err := scope.Admit("selected")
			if err != nil {
				scope.Close()
				t.Fatal(err)
			}
			var handles []*ReadHandle
			all := map[*readDirectory]bool{}
			limitErr := errors.New("fixture limit")
			err = directory.WalkFiles(ReadWalkVisitor{Select: func(ReadEntry) bool { return true }, Visit: func(h *ReadHandle) error {
				for _, d := range h.dirs {
					all[d] = true
				}
				if fail && len(handles) == 200 {
					return limitErr
				}
				handles = append(handles, h)
				return nil
			}})
			if fail && !errors.Is(err, limitErr) || !fail && err != nil {
				t.Errorf("walk result: %v", err)
			}
			// Shared original ancestors, not hundreds of duplicated fd stacks.
			if len(all) > 6 {
				t.Errorf("per-file directory descriptors: %d", len(all))
			}
			_ = directory.Close()
			_ = scope.Close()
			for _, h := range handles {
				file, _, err := h.OpenRegular()
				if err != nil {
					t.Errorf("caller close revoked independent selected handle: %v", err)
				} else {
					data, readErr := io.ReadAll(file)
					file.Close()
					if readErr != nil || string(data) != "original" {
						t.Errorf("original bytes: %q %v", data, readErr)
					}
				}
				_ = h.Close()
			}
			for d := range all {
				if d.refs.Load() != 0 {
					t.Errorf("unreleased ancestor refs=%d", d.refs.Load())
				}
				if _, err := d.file.Stat(); !errors.Is(err, os.ErrClosed) {
					t.Errorf("directory fd not closed: %v", err)
				}
				if _, err := d.meta.Lstat("."); !errors.Is(err, os.ErrClosed) {
					t.Errorf("metadata fd not closed: %v", err)
				}
			}
		})
	}
}

func TestReadScopeWalkRejectsReplacementAfterDeepEntryAdmission(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, reader := readFixture(t)
			defer reader.Close()
			foreign := t.TempDir()
			if err := os.WriteFile(filepath.Join(foreign, "foreign-secret.txt"), []byte("foreign"), 0o600); err != nil {
				t.Fatal(err)
			}
			fired := false
			afterReadEntryAdmissionTestHook = func(relative string) {
				if fired || relative != "nested/child/deep" {
					return
				}
				fired = true
				original := filepath.Join(root, relative)
				if err := os.Rename(original, original+"-held"); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					if err := os.Symlink(foreign, original); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Rename(foreign, original); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { afterReadEntryAdmissionTestHook = nil })
			var handles []*ReadHandle
			defer func() {
				for _, h := range handles {
					_ = h.Close()
				}
			}()
			err := reader.WalkFiles(ReadWalkVisitor{Select: func(ReadEntry) bool { return true }, Visit: func(h *ReadHandle) error {
				handles = append(handles, h)
				return nil
			}})
			if !fired || !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("deep changed admission skipped: fired=%v err=%v", fired, err)
			}
			for _, h := range handles {
				if filepath.Base(h.relative) == "foreign-secret.txt" {
					t.Error("foreign entry accepted")
				}
			}
		})
	}
}
