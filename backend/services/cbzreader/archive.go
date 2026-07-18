package cbzreader

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"openreader/backend/config"
	"openreader/backend/engine"
)

type extractionLimits struct {
	MaxArchiveBytes int64
	MaxEntries      int
	MaxEntryBytes   int64
	MaxTotalBytes   int64
}

func extractionLimitsFromConfig(_ config.Config) extractionLimits {
	// New imports already pass the caller-configured strict parser limits before
	// an archive is allocated. Runtime extraction deliberately retains the
	// bounded legacy limits so an existing valid mounted CBZ does not become
	// unreadable merely because the default upload limit is lower.
	return extractionLimits{
		MaxArchiveBytes: maxCBZArchiveBytes,
		MaxEntries:      maxCBZEntries,
		MaxEntryBytes:   maxCBZEntryBytes,
		MaxTotalBytes:   maxCBZTotalBytes,
	}
}

type archiveEntry struct {
	file      *zip.File
	canonical string
	isDir     bool
	isImage   bool
}

func extractArchiveFile(sourcePath, destination string, limits extractionLimits) (string, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > limits.MaxArchiveBytes {
		return "", fmt.Errorf("%w: compressed archive is too large", ErrExtractionLimit)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return "", fmt.Errorf("%w: invalid zip archive", ErrInvalidArchive)
	}
	return extractZipReader(reader, destination, limits)
}

func extractZipReader(reader *zip.Reader, destination string, limits extractionLimits) (coverPath string, returnErr error) {
	if len(reader.File) > limits.MaxEntries {
		return "", fmt.Errorf("%w: too many archive entries", ErrExtractionLimit)
	}

	entries := make([]archiveEntry, 0, len(reader.File))
	seen := make(map[string]bool, len(reader.File))
	var total uint64
	for _, file := range reader.File {
		canonical, err := engine.NormalizeCBZResourcePath(file.Name)
		if err != nil {
			return "", fmt.Errorf("%w: malformed archive path", ErrUnsafePath)
		}
		if canonical == "" {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symbolic links are not allowed", ErrUnsafePath)
		}
		isDir := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
		key := strings.ToLower(canonical)
		if _, exists := seen[key]; exists {
			return "", fmt.Errorf("%w: duplicate archive path", ErrUnsafePath)
		}
		for parent := path.Dir(canonical); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if parentIsDir, exists := seen[strings.ToLower(parent)]; exists && !parentIsDir {
				return "", fmt.Errorf("%w: archive path conflicts with a file", ErrUnsafePath)
			}
		}
		if !isDir {
			prefix := key + "/"
			for existing := range seen {
				if strings.HasPrefix(existing, prefix) {
					return "", fmt.Errorf("%w: archive file conflicts with a directory", ErrUnsafePath)
				}
			}
		}
		seen[key] = isDir

		if !isDir {
			if file.UncompressedSize64 > uint64(limits.MaxEntryBytes) {
				return "", fmt.Errorf("%w: archive entry is too large", ErrExtractionLimit)
			}
			if ^uint64(0)-total < file.UncompressedSize64 {
				return "", fmt.Errorf("%w: archive size overflow", ErrExtractionLimit)
			}
			total += file.UncompressedSize64
			if total > uint64(limits.MaxTotalBytes) {
				return "", fmt.Errorf("%w: archive expands beyond the total limit", ErrExtractionLimit)
			}
		}
		_, isImage := engine.CBZImageContentType(canonical)
		if !isDir && isImage && coverPath == "" {
			coverPath = canonical
		}
		entries = append(entries, archiveEntry{file: file, canonical: canonical, isDir: isDir, isImage: isImage})
	}
	if coverPath == "" {
		return "", ErrNotFound
	}

	if err := os.RemoveAll(destination); err != nil {
		return "", err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", err
	}
	defer func() {
		if returnErr != nil {
			_ = os.RemoveAll(destination)
		}
	}()
	var actualTotal int64
	for _, entry := range entries {
		if entry.isDir || !entry.isImage {
			continue
		}
		target, err := extractionTarget(destination, entry.canonical)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		opened, err := entry.file.Open()
		if err != nil {
			return "", err
		}
		written, writeErr := writeBoundedFile(target, opened, limits.MaxEntryBytes)
		closeErr := opened.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if written > limits.MaxTotalBytes-actualTotal {
			return "", fmt.Errorf("%w: extracted files exceed the total limit", ErrExtractionLimit)
		}
		actualTotal += written
	}
	return coverPath, nil
}

func extractionTarget(root, archivePath string) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(archivePath))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: extraction path escaped root", ErrUnsafePath)
	}
	return target, nil
}

func writeBoundedFile(target string, source io.Reader, maxBytes int64) (written int64, returnErr error) {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer func() {
		closeErr := file.Close()
		if returnErr == nil && closeErr != nil {
			returnErr = closeErr
		}
		if returnErr != nil {
			_ = os.Remove(target)
		}
	}()
	written, err = io.Copy(file, io.LimitReader(source, maxBytes+1))
	if err != nil {
		return written, err
	}
	if written > maxBytes {
		return written, fmt.Errorf("%w: archive entry exceeded size limit", ErrExtractionLimit)
	}
	if err := file.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return written, err
	}
	return written, nil
}
