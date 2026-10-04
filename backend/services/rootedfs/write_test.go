package rootedfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceRegularPreservesCompetingFinalAfterDetach(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterWriteDetachTestHook = func(_, _ string) {
		if err := os.WriteFile(target, []byte("newcomer"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { afterWriteDetachTestHook = nil })
	err := ReplaceRegular(context.Background(), root, "target", func(file *os.File) error {
		_, err := file.WriteString("uploaded")
		return err
	})
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("competing final publication = %v, want unsafe", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "newcomer" {
		t.Fatalf("replacement final = %q, %v", data, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	foundOriginal := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".openreader-write-old-") {
			data, err := os.ReadFile(filepath.Join(root, entry.Name()))
			if err != nil || string(data) != "original" {
				t.Fatalf("preserved original = %q, %v", data, err)
			}
			foundOriginal = true
		} else if strings.HasPrefix(entry.Name(), ".openreader-write-") {
			t.Fatalf("failed publication leaked upload stage %q", entry.Name())
		}
	}
	if !foundOriginal {
		t.Fatal("competing final caused loss of the original file")
	}
}

func TestReplaceRegularCancellationAfterDetachRestoresOriginal(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	afterWriteDetachTestHook = func(_, _ string) { cancel() }
	t.Cleanup(func() { afterWriteDetachTestHook = nil })
	err := ReplaceRegular(ctx, root, "target", func(file *os.File) error {
		_, err := file.WriteString("uploaded")
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel after detach = %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original" {
		t.Fatalf("restored original = %q, %v", data, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "target" {
		t.Fatalf("cancel cleanup = %v, %v", entries, err)
	}
}
