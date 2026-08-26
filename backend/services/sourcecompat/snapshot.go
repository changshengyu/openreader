package sourcecompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const (
	DefaultSnapshotMaxBytes   int64 = 16 << 20
	DefaultSnapshotMaxEntries       = 300
)

var (
	ErrUnsafeSnapshot         = errors.New("unsafe book source snapshot")
	ErrSnapshotTooLarge       = errors.New("book source snapshot is too large")
	ErrSnapshotTooManyEntries = errors.New("too many book sources in snapshot")
	ErrInvalidSnapshot        = errors.New("invalid book source snapshot")
)

// ReadSnapshotFile reads one fixed compatibility file without following a
// symlink or accepting a special filesystem object.
func ReadSnapshotFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entry, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() || entry.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafeSnapshot
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, ErrUnsafeSnapshot
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(entry, opened) {
		return nil, ErrUnsafeSnapshot
	}

	reader := io.LimitReader(&contextReader{ctx: ctx, reader: file}, DefaultSnapshotMaxBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > DefaultSnapshotMaxBytes {
		return nil, ErrSnapshotTooLarge
	}
	if err := ValidateSnapshot(data); err != nil {
		return nil, err
	}
	return data, nil
}

// ValidateSnapshot bounds the raw compatibility shape before the richer API
// decoder allocates model and rule structures.
func ValidateSnapshot(data []byte) error {
	if !utf8.Valid(data) {
		return ErrInvalidSnapshot
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ErrInvalidSnapshot
	}

	switch trimmed[0] {
	case '[':
		var entries []json.RawMessage
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return ErrInvalidSnapshot
		}
		return validateSnapshotEntries(entries)
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return ErrInvalidSnapshot
		}
		foundWrapper := false
		totalEntries := 0
		for _, key := range []string{"bookSources", "sources"} {
			raw, exists := object[key]
			if !exists {
				continue
			}
			foundWrapper = true
			var entries []json.RawMessage
			if err := json.Unmarshal(raw, &entries); err != nil {
				return ErrInvalidSnapshot
			}
			if err := validateSnapshotEntries(entries); err != nil {
				return err
			}
			totalEntries += len(entries)
			if totalEntries > DefaultSnapshotMaxEntries {
				return ErrSnapshotTooManyEntries
			}
		}
		if foundWrapper {
			return nil
		}
		return nil
	default:
		return ErrInvalidSnapshot
	}
}

func validateSnapshotEntries(entries []json.RawMessage) error {
	if len(entries) > DefaultSnapshotMaxEntries {
		return ErrSnapshotTooManyEntries
	}
	for _, entry := range entries {
		trimmed := bytes.TrimSpace(entry)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return ErrInvalidSnapshot
		}
	}
	return nil
}

// WriteSnapshotFile publishes a private, synced regular file by same-directory
// rename. An existing special entry is rejected instead of replaced silently.
func WriteSnapshotFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry, err := os.Lstat(path); err == nil {
		if !entry.Mode().IsRegular() || entry.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafeSnapshot
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrUnsafeSnapshot
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".default-book-sources-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, &contextReader{ctx: ctx, reader: bytes.NewReader(data)}); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	if err := directoryHandle.Sync(); err != nil {
		_ = directoryHandle.Close()
		return err
	}
	return directoryHandle.Close()
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
