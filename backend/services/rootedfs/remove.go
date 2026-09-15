package rootedfs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

var ErrUnsafePath = errors.New("unsafe rooted filesystem path")

// Tests use this hook to replace a target after identity validation. Package
// tests are not parallel while the hook is installed.
var beforeDetachTestHook func(rootPath, relative string)

// DirectoryExists checks a generated relative directory path without treating
// a symlink target as a new trust boundary.
func DirectoryExists(rootPath, relative string) (bool, error) {
	root, clean, err := open(rootPath, relative)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	info, err := validateDirectory(root, clean)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return info != nil, err
}

// RemovePath removes one existing regular file or directory from a trusted
// root. A missing target remains visible to callers as os.ErrNotExist.
func RemovePath(rootPath, relative string) error {
	return removeEntry(rootPath, relative, false, false, nil)
}

// RemoveDirectory detaches one validated directory inside an opened parent
// before recursively removing it. The rooted parent keeps the recursive walk
// confined even if a mounted entry changes after validation.
func RemoveDirectory(rootPath, relative string, beforeRemove func(string) error) error {
	return removeEntry(rootPath, relative, true, true, beforeRemove)
}

func removeEntry(rootPath, relative string, missingOK, directoryOnly bool, beforeRemove func(string) error) error {
	root, clean, err := open(rootPath, relative)
	if errors.Is(err, os.ErrNotExist) {
		return missingResult(missingOK)
	}
	if err != nil {
		return err
	}
	defer root.Close()

	expected, err := validateEntry(root, clean, directoryOnly)
	if errors.Is(err, os.ErrNotExist) {
		return missingResult(missingOK)
	}
	if err != nil {
		return err
	}
	parentName := filepath.ToSlash(filepath.Dir(filepath.FromSlash(clean)))
	entryName := filepath.Base(filepath.FromSlash(clean))
	parentExpected, err := root.Lstat(parentName)
	if err != nil || parentExpected.Mode()&os.ModeSymlink != 0 || !parentExpected.IsDir() {
		return ErrUnsafePath
	}
	parent, err := root.Open(parentName)
	if err != nil {
		return ErrUnsafePath
	}
	defer parent.Close()
	parentOpened, err := parent.Stat()
	if err != nil || !parentOpened.IsDir() || !os.SameFile(parentExpected, parentOpened) {
		return ErrUnsafePath
	}
	currentFile, current, err := openEntryAt(parent, entryName, directoryOnly)
	if errors.Is(err, os.ErrNotExist) {
		return missingResult(missingOK)
	}
	if err != nil || !os.SameFile(expected, current) {
		return ErrUnsafePath
	}
	defer currentFile.Close()
	if beforeDetachTestHook != nil {
		beforeDetachTestHook(root.Name(), clean)
	}
	rootOpened, openErr := root.Stat(".")
	rootCurrent, pathErr := os.Lstat(root.Name())
	if openErr != nil || pathErr != nil || rootCurrent.Mode()&os.ModeSymlink != 0 || !rootCurrent.IsDir() || !os.SameFile(rootOpened, rootCurrent) {
		return ErrUnsafePath
	}
	parentCurrent, err := root.Lstat(parentName)
	if err != nil || parentCurrent.Mode()&os.ModeSymlink != 0 || !parentCurrent.IsDir() || !os.SameFile(parentOpened, parentCurrent) {
		return ErrUnsafePath
	}

	quarantine, err := randomName(".openreader-delete-", 12)
	if err != nil {
		return err
	}
	if err := unix.Renameat(int(parent.Fd()), entryName, int(parent.Fd()), quarantine); err != nil {
		return err
	}
	movedFile, moved, err := openEntryAt(parent, quarantine, directoryOnly)
	if err != nil || !os.SameFile(current, moved) {
		restore(parent, quarantine, entryName)
		return ErrUnsafePath
	}
	defer movedFile.Close()
	if beforeRemove != nil && moved.IsDir() {
		absoluteTarget := filepath.Join(root.Name(), filepath.FromSlash(clean))
		if err := beforeRemove(absoluteTarget); err != nil {
			_ = movedFile.Close()
			restore(parent, quarantine, entryName)
			return err
		}
	}
	if moved.IsDir() {
		if err := removeOpenedDirectory(movedFile); err != nil {
			return err
		}
		if err := movedFile.Close(); err != nil {
			return err
		}
		return unix.Unlinkat(int(parent.Fd()), quarantine, unix.AT_REMOVEDIR)
	}
	if err := movedFile.Close(); err != nil {
		return err
	}
	return unix.Unlinkat(int(parent.Fd()), quarantine, 0)
}

func missingResult(missingOK bool) error {
	if missingOK {
		return nil
	}
	return os.ErrNotExist
}

func open(rootPath, relative string) (*os.Root, string, error) {
	if strings.TrimSpace(rootPath) == "" {
		return nil, "", ErrUnsafePath
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(rootPath))
	if err != nil {
		return nil, "", ErrUnsafePath
	}
	rootInfo, err := os.Lstat(absoluteRoot)
	if err != nil {
		return nil, "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, "", ErrUnsafePath
	}
	clean, err := cleanRelative(relative)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return nil, "", ErrUnsafePath
	}
	opened, err := root.Stat(".")
	if err != nil || !opened.IsDir() || !os.SameFile(rootInfo, opened) {
		_ = root.Close()
		return nil, "", ErrUnsafePath
	}
	return root, clean, nil
}

func validateDirectory(root *os.Root, relative string) (os.FileInfo, error) {
	return validateEntry(root, relative, true)
}

func validateEntry(root *os.Root, relative string, directoryOnly bool) (os.FileInfo, error) {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	current := ""
	var target os.FileInfo
	for index, part := range parts {
		current = filepath.ToSlash(filepath.Join(filepath.FromSlash(current), part))
		info, err := root.Lstat(current)
		if err != nil {
			return nil, err
		}
		isTarget := index == len(parts)-1
		if info.Mode()&os.ModeSymlink != 0 || (!isTarget && !info.IsDir()) {
			return nil, ErrUnsafePath
		}
		if isTarget {
			if directoryOnly && !info.IsDir() {
				return nil, ErrUnsafePath
			}
			if !directoryOnly && !info.IsDir() && !info.Mode().IsRegular() {
				return nil, ErrUnsafePath
			}
			target = info
		}
	}
	if target == nil {
		return nil, ErrUnsafePath
	}
	return target, nil
}

func cleanRelative(relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", ErrUnsafePath
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", ErrUnsafePath
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == "" || part == "." || part == ".." {
			return "", ErrUnsafePath
		}
	}
	return filepath.ToSlash(clean), nil
}

func randomName(prefix string, bytesLength int) (string, error) {
	buffer := make([]byte, bytesLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buffer), nil
}

func openDirectoryAt(parent *os.File, name string) (*os.File, os.FileInfo, error) {
	fd, err := unix.Openat(
		int(parent.Fd()),
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return nil, nil, os.ErrNotExist
		}
		return nil, nil, ErrUnsafePath
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, nil, ErrUnsafePath
	}
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		_ = file.Close()
		return nil, nil, ErrUnsafePath
	}
	return file, info, nil
}

func openEntryAt(parent *os.File, name string, directoryOnly bool) (*os.File, os.FileInfo, error) {
	if directoryOnly {
		return openDirectoryAt(parent, name)
	}
	fd, err := unix.Openat(
		int(parent.Fd()),
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return nil, nil, os.ErrNotExist
		}
		return nil, nil, ErrUnsafePath
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, nil, ErrUnsafePath
	}
	info, err := file.Stat()
	if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
		_ = file.Close()
		return nil, nil, ErrUnsafePath
	}
	return file, info, nil
}

func removeOpenedDirectory(directory *os.File) error {
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		child, _, openErr := openDirectoryAt(directory, name)
		if openErr == nil {
			if err := removeOpenedDirectory(child); err != nil {
				_ = child.Close()
				return err
			}
			if err := child.Close(); err != nil {
				return err
			}
			if err := unix.Unlinkat(int(directory.Fd()), name, unix.AT_REMOVEDIR); err != nil {
				return err
			}
			continue
		}
		if !errors.Is(openErr, ErrUnsafePath) {
			return openErr
		}
		if err := unix.Unlinkat(int(directory.Fd()), name, 0); err != nil {
			return err
		}
	}
	return nil
}

func restore(parent *os.File, quarantine, original string) {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), original, &stat, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, syscall.ENOENT) {
		return
	}
	_ = unix.Renameat(int(parent.Fd()), quarantine, int(parent.Fd()), original)
}
