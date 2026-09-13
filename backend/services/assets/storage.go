package assets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	ErrUnsafePath = errors.New("unsafe asset path")
	ErrNotFound   = errors.New("asset not found")
	ErrConflict   = errors.New("asset path conflict")
)

type Store struct {
	rootPath string
}

type OpenedFile struct {
	File *os.File
	Info os.FileInfo
	Path string
}

func NewStore(dataDir string) *Store {
	return &Store{rootPath: filepath.Join(dataDir, "uploads")}
}

func (s *Store) Open(userID uint, kind, name string) (OpenedFile, error) {
	relative, err := assetRelativePath(userID, kind, name)
	if err != nil {
		return OpenedFile{}, err
	}
	root, err := s.openRoot(false)
	if err != nil {
		return OpenedFile{}, err
	}
	defer root.Close()

	entryInfo, err := validateComponents(root, relative, true)
	if err != nil {
		return OpenedFile{}, err
	}
	file, err := root.Open(relative)
	if errors.Is(err, os.ErrNotExist) {
		return OpenedFile{}, ErrNotFound
	}
	if err != nil {
		return OpenedFile{}, ErrUnsafePath
	}
	openedInfo, statErr := file.Stat()
	currentInfo, currentErr := root.Lstat(relative)
	if statErr != nil || currentErr != nil || !openedInfo.Mode().IsRegular() ||
		!os.SameFile(entryInfo, openedInfo) || !os.SameFile(openedInfo, currentInfo) {
		_ = file.Close()
		return OpenedFile{}, ErrUnsafePath
	}
	if _, err := validateComponents(root, relative, true); err != nil {
		_ = file.Close()
		return OpenedFile{}, err
	}
	return OpenedFile{File: file, Info: openedInfo, Path: filepath.Join(s.rootPath, filepath.FromSlash(relative))}, nil
}

func (s *Store) Publish(
	ctx context.Context,
	userID uint,
	kind string,
	name string,
	source io.Reader,
	expectedSize int64,
) error {
	if source == nil || expectedSize <= 0 {
		return ErrUnsafePath
	}
	relative, err := assetRelativePath(userID, kind, name)
	if err != nil {
		return err
	}
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)))
	root, err := s.openRoot(true)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := ensureDirectory(root, directory); err != nil {
		return err
	}
	directoryHandle, err := root.Open(directory)
	if err != nil {
		return ErrUnsafePath
	}
	defer directoryHandle.Close()
	directoryInfo, err := directoryHandle.Stat()
	if err != nil || !directoryInfo.IsDir() {
		return ErrUnsafePath
	}
	finalName := filepath.Base(filepath.FromSlash(relative))

	nonce, err := secureHex(12)
	if err != nil {
		return err
	}
	stageName := ".asset-stage-" + nonce
	stageFD, err := unix.Openat(
		int(directoryHandle.Fd()),
		stageName,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return err
	}
	output := os.NewFile(uintptr(stageFD), stageName)
	if output == nil {
		_ = unix.Close(stageFD)
		return ErrUnsafePath
	}
	stageExists := true
	defer func() {
		_ = output.Close()
		if stageExists {
			_ = unix.Unlinkat(int(directoryHandle.Fd()), stageName, 0)
		}
	}()

	written, copyErr := copyContext(ctx, output, io.LimitReader(source, expectedSize+1))
	if copyErr != nil || written != expectedSize {
		if copyErr != nil {
			return copyErr
		}
		return ErrUnsafePath
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	stageInfo, err := statAtRegular(directoryHandle, stageName)
	if err != nil || !stageInfo.Mode().IsRegular() {
		return ErrUnsafePath
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := unix.Linkat(int(directoryHandle.Fd()), stageName, int(directoryHandle.Fd()), finalName, 0); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrConflict
		}
		return err
	}
	finalInfo, err := statAtRegular(directoryHandle, finalName)
	if err != nil || !finalInfo.Mode().IsRegular() || !os.SameFile(stageInfo, finalInfo) {
		if err == nil && os.SameFile(stageInfo, finalInfo) {
			_ = unix.Unlinkat(int(directoryHandle.Fd()), finalName, 0)
		}
		return ErrUnsafePath
	}
	if err := unix.Unlinkat(int(directoryHandle.Fd()), stageName, 0); err != nil {
		_ = unix.Unlinkat(int(directoryHandle.Fd()), finalName, 0)
		return err
	}
	stageExists = false
	return nil
}

func (s *Store) Remove(userID uint, kind, name string) error {
	relative, err := assetRelativePath(userID, kind, name)
	if err != nil {
		return err
	}
	root, err := s.openRoot(false)
	if err != nil {
		return err
	}
	defer root.Close()
	entryInfo, err := validateComponents(root, relative, true)
	if err != nil {
		return err
	}
	file, err := root.Open(relative)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return ErrUnsafePath
	}
	openedInfo, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(entryInfo, openedInfo) {
		return ErrUnsafePath
	}
	nonce, err := secureHex(12)
	if err != nil {
		return err
	}
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)))
	directoryHandle, err := root.Open(directory)
	if err != nil {
		return ErrUnsafePath
	}
	defer directoryHandle.Close()
	entryName := filepath.Base(filepath.FromSlash(relative))
	quarantine := ".asset-delete-" + nonce
	if err := unix.Renameat(int(directoryHandle.Fd()), entryName, int(directoryHandle.Fd()), quarantine); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	movedInfo, err := statAtRegular(directoryHandle, quarantine)
	if err != nil || !movedInfo.Mode().IsRegular() || !os.SameFile(openedInfo, movedInfo) {
		restoreQuarantinedEntry(directoryHandle, quarantine, entryName)
		return ErrUnsafePath
	}
	if err := unix.Unlinkat(int(directoryHandle.Fd()), quarantine, 0); err != nil {
		restoreQuarantinedEntry(directoryHandle, quarantine, entryName)
		return err
	}
	return nil
}

func restoreQuarantinedEntry(directory *os.File, quarantine, name string) {
	if err := unix.Linkat(int(directory.Fd()), quarantine, int(directory.Fd()), name, 0); err == nil {
		_ = unix.Unlinkat(int(directory.Fd()), quarantine, 0)
	}
}

func (s *Store) openRoot(create bool) (*os.Root, error) {
	abs, err := filepath.Abs(filepath.Clean(s.rootPath))
	if err != nil || strings.TrimSpace(abs) == "" {
		return nil, ErrUnsafePath
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := os.Mkdir(abs, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, err = os.Lstat(abs)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ErrUnsafePath
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, ErrUnsafePath
	}
	openedInfo, err := root.Stat(".")
	if err != nil || !openedInfo.IsDir() || !os.SameFile(info, openedInfo) {
		_ = root.Close()
		return nil, ErrUnsafePath
	}
	return root, nil
}

func assetRelativePath(userID uint, kind, name string) (string, error) {
	if userID == 0 || !IsKindDirectory(kind) || name == "" || name != filepath.Base(name) ||
		strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return "", ErrUnsafePath
	}
	return filepath.ToSlash(filepath.Join("users", strconv.FormatUint(uint64(userID), 10), kind, name)), nil
}

func ensureDirectory(root *os.Root, relative string) error {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	current := ""
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ErrUnsafePath
		}
		current = filepath.ToSlash(filepath.Join(filepath.FromSlash(current), part))
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := root.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return ErrUnsafePath
			}
			info, err = root.Lstat(current)
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrUnsafePath
		}
	}
	return nil
}

func statAtRegular(directory *os.File, name string) (os.FileInfo, error) {
	fd, err := unix.Openat(int(directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, ErrUnsafePath
	}
	defer file.Close()
	return file.Stat()
}

func validateComponents(root *os.Root, relative string, finalRegular bool) (os.FileInfo, error) {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	current := ""
	var last os.FileInfo
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, ErrUnsafePath
		}
		current = filepath.ToSlash(filepath.Join(filepath.FromSlash(current), part))
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrUnsafePath
		}
		last = info
		if index < len(parts)-1 && !info.IsDir() {
			return nil, ErrUnsafePath
		}
	}
	if last == nil || (finalRegular && !last.Mode().IsRegular()) || (!finalRegular && !last.IsDir()) {
		return nil, ErrUnsafePath
	}
	return last, nil
}

func secureHex(bytesLen int) (string, error) {
	if bytesLen <= 0 {
		return "", ErrUnsafePath
	}
	data := make([]byte, bytesLen)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
