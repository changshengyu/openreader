package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

var (
	errInvalidBackupArchive  = errors.New("invalid backup archive")
	errBackupArchiveLimit    = errors.New("backup archive exceeds safety limits")
	errBackupRestoreTooLarge = errors.New("backup file exceeds size limit")
)

type backupRestoreLimits struct {
	MaxCompressedBytes int64
	MaxEntries         int
	MaxEntryBytes      int64
	MaxExpandedBytes   int64
}

type backupRestoreArchive struct {
	entries []backupRestoreEntry
	files   map[*zip.File][]byte
}

type backupRestoreEntry struct {
	file *zip.File
	name string
}

func defaultBackupRestoreLimits() backupRestoreLimits {
	return backupRestoreLimits{
		MaxCompressedBytes: 128 * 1024 * 1024,
		MaxEntries:         5_000,
		MaxEntryBytes:      16 * 1024 * 1024,
		MaxExpandedBytes:   128 * 1024 * 1024,
	}
}

func (s *Server) backupRestoreLimits() backupRestoreLimits {
	defaults := defaultBackupRestoreLimits()
	limits := backupRestoreLimits{
		MaxCompressedBytes: s.cfg.MaxBackupRestoreBytes,
		MaxEntries:         s.cfg.MaxBackupArchiveEntries,
		MaxEntryBytes:      s.cfg.MaxBackupArchiveBytes,
		MaxExpandedBytes:   s.cfg.MaxBackupArchiveTotal,
	}
	return limits.normalized(defaults)
}

func (limits backupRestoreLimits) normalized(fallback backupRestoreLimits) backupRestoreLimits {
	if limits.MaxCompressedBytes <= 0 {
		limits.MaxCompressedBytes = fallback.MaxCompressedBytes
	}
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = fallback.MaxEntries
	}
	if limits.MaxEntryBytes <= 0 {
		limits.MaxEntryBytes = fallback.MaxEntryBytes
	}
	if limits.MaxExpandedBytes <= 0 {
		limits.MaxExpandedBytes = fallback.MaxExpandedBytes
	}
	return limits
}

func readBoundedBackup(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = defaultBackupRestoreLimits().MaxCompressedBytes
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errBackupRestoreTooLarge
	}
	return data, nil
}

func newBackupRestoreArchive(data []byte, limits backupRestoreLimits) (*backupRestoreArchive, error) {
	limits = limits.normalized(defaultBackupRestoreLimits())
	if int64(len(data)) > limits.MaxCompressedBytes {
		return nil, errBackupRestoreTooLarge
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid zip data", errInvalidBackupArchive)
	}
	if len(reader.File) > limits.MaxEntries {
		return nil, fmt.Errorf("%w: too many entries", errBackupArchiveLimit)
	}

	archive := &backupRestoreArchive{
		entries: make([]backupRestoreEntry, 0, len(reader.File)),
		files:   make(map[*zip.File][]byte, len(reader.File)),
	}
	seen := make(map[string]struct{}, len(reader.File))
	var expanded int64
	for _, file := range reader.File {
		name, err := normalizeBackupArchivePath(file.Name)
		if err != nil {
			return nil, err
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: symlink entry", errInvalidBackupArchive)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate entry", errInvalidBackupArchive)
		}
		seen[key] = struct{}{}
		if file.UncompressedSize64 > uint64(limits.MaxEntryBytes) {
			return nil, fmt.Errorf("%w: entry exceeds size limit", errBackupArchiveLimit)
		}
		if expanded > limits.MaxExpandedBytes-int64(file.UncompressedSize64) {
			return nil, fmt.Errorf("%w: archive exceeds expanded size limit", errBackupArchiveLimit)
		}
		expanded += int64(file.UncompressedSize64)

		fileReader, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: cannot read entry", errInvalidBackupArchive)
		}
		contents, readErr := io.ReadAll(io.LimitReader(fileReader, limits.MaxEntryBytes+1))
		closeErr := fileReader.Close()
		if readErr != nil || closeErr != nil || int64(len(contents)) > limits.MaxEntryBytes {
			if int64(len(contents)) > limits.MaxEntryBytes {
				return nil, fmt.Errorf("%w: entry exceeds size limit", errBackupArchiveLimit)
			}
			return nil, fmt.Errorf("%w: cannot read entry", errInvalidBackupArchive)
		}
		archive.files[file] = contents
		archive.entries = append(archive.entries, backupRestoreEntry{file: file, name: name})
	}
	return archive, nil
}

func normalizeBackupArchivePath(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: unsafe entry path", errInvalidBackupArchive)
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", fmt.Errorf("%w: unsafe entry path", errInvalidBackupArchive)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != name && strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: unsafe entry path", errInvalidBackupArchive)
	}
	return clean, nil
}

func (archive *backupRestoreArchive) dataFor(file *zip.File) ([]byte, error) {
	data, ok := archive.files[file]
	if !ok {
		return nil, fmt.Errorf("%w: missing archive entry", errInvalidBackupArchive)
	}
	return data, nil
}

func readBackupZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	maxBytes := defaultBackupRestoreLimits().MaxEntryBytes
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errBackupArchiveLimit
	}
	return data, nil
}
