package epubreader

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func epubZip(t *testing.T, files map[string]string, symlinks map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range symlinks {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(os.ModeSymlink | 0o777)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(target)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExtractArchivePreservesRelativeResources(t *testing.T) {
	root := t.TempDir()
	data := epubZip(t, map[string]string{
		"META-INF/container.xml": "<container/>",
		"OPS/Text/one.xhtml":     "<html><body>正文</body></html>",
		"OPS/Styles/book.css":    "body { color: #333; }",
		"OPS/Images/cover.svg":   "<svg/>",
	}, nil)

	if err := extractArchive(data, root, defaultExtractionLimits()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"META-INF/container.xml",
		"OPS/Text/one.xhtml",
		"OPS/Styles/book.css",
		"OPS/Images/cover.svg",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatalf("missing extracted resource %q: %v", name, err)
		}
	}
}

func TestExtractArchiveRejectsUnsafeEntriesWithoutEscape(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		symlinks map[string]string
	}{
		{name: "parent traversal", files: map[string]string{"../outside.txt": "escape"}},
		{name: "absolute path", files: map[string]string{"/outside.txt": "escape"}},
		{name: "windows drive", files: map[string]string{`C:\outside.txt`: "escape"}},
		{name: "nul name", files: map[string]string{"OPS/\x00bad.xhtml": "escape"}},
		{name: "symlink", symlinks: map[string]string{"OPS/link": "../../outside.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "extract")
			err := extractArchive(epubZip(t, tt.files, tt.symlinks), root, defaultExtractionLimits())
			if err == nil {
				t.Fatal("unsafe archive unexpectedly extracted")
			}
			if _, statErr := os.Stat(filepath.Join(parent, "outside.txt")); !os.IsNotExist(statErr) {
				t.Fatalf("archive escaped extraction root: %v", statErr)
			}
		})
	}
}

func TestExtractArchiveRejectsDuplicateAndBoundedExpansion(t *testing.T) {
	t.Run("duplicate", func(t *testing.T) {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, content := range []string{"one", "two"} {
			entry, err := writer.Create("OPS/one.xhtml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := extractArchive(buffer.Bytes(), t.TempDir(), defaultExtractionLimits()); err == nil {
			t.Fatal("duplicate archive path unexpectedly accepted")
		}
	})

	t.Run("entry count", func(t *testing.T) {
		limits := defaultExtractionLimits()
		limits.MaxEntries = 1
		err := extractArchive(epubZip(t, map[string]string{
			"one.txt": "1",
			"two.txt": "2",
		}, nil), t.TempDir(), limits)
		if err == nil {
			t.Fatal("entry-count limit was not enforced")
		}
	})

	t.Run("entry size", func(t *testing.T) {
		limits := defaultExtractionLimits()
		limits.MaxEntryBytes = 4
		err := extractArchive(epubZip(t, map[string]string{
			"one.txt": strings.Repeat("x", 5),
		}, nil), t.TempDir(), limits)
		if err == nil {
			t.Fatal("per-entry size limit was not enforced")
		}
	})

	t.Run("total size", func(t *testing.T) {
		limits := defaultExtractionLimits()
		limits.MaxTotalBytes = 7
		err := extractArchive(epubZip(t, map[string]string{
			"one.txt": "1234",
			"two.txt": "5678",
		}, nil), t.TempDir(), limits)
		if err == nil {
			t.Fatal("total expanded-size limit was not enforced")
		}
	})
}

func TestJoinUnderRejectsExistingSymlinkAncestorOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "redirect")); err != nil {
		t.Fatal(err)
	}

	_, err := joinUnder(root, filepath.Join("redirect", "new", "resource.xhtml"))
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("joinUnder() error = %v, want ErrUnsafePath", err)
	}
}
