package rootedfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	ErrIsDirectory  = errors.New("rooted file target is a directory")
	ErrNotDirectory = errors.New("rooted file parent is not a directory")
)

// Tests inject a competing final name after the original has been detached.
// Tests installing this hook must not run in parallel.
var afterWriteDetachTestHook func(rootPath, relative string)

type writeDirectory struct {
	relative string
	file     *os.File
	info     os.FileInfo
}

// ReplaceRegular writes a private stage through the original opened parent,
// then publishes it only if the path still names the validated directories and
// target. A failed writer never changes the original final file. The callback
// owns content admission; this function owns permissions, sync/close and commit.
func ReplaceRegular(ctx context.Context, rootPath, relative string, write func(*os.File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, clean, err := open(rootPath, relative)
	if err != nil {
		return err
	}
	defer root.Close()
	directories, err := openWriteDirectories(root, filepath.Dir(filepath.FromSlash(clean)))
	if err != nil {
		return err
	}
	defer func() {
		for _, directory := range directories {
			_ = directory.file.Close()
		}
	}()
	parent := directories[len(directories)-1].file
	finalName := filepath.Base(filepath.FromSlash(clean))
	expected, err := statWriteName(parent, finalName)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if exists {
		switch expected.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			return ErrIsDirectory
		case unix.S_IFREG:
		default:
			return ErrUnsafePath
		}
	}
	stageName, err := randomName(".openreader-write-", 12)
	if err != nil {
		return err
	}
	fd, err := unix.Openat(int(parent.Fd()), stageName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	output := os.NewFile(uintptr(fd), stageName)
	if output == nil {
		_ = unix.Close(fd)
		return ErrUnsafePath
	}
	var stage unix.Stat_t
	if err := unix.Fstat(fd, &stage); err != nil {
		_ = output.Close()
		_ = unix.Unlinkat(int(parent.Fd()), stageName, 0)
		return err
	}
	defer func() {
		_ = output.Close()
		// An attacker replacing the random stage does not grant us ownership of
		// the replacement. Cleanup stays in the original opened parent.
		if current, err := statWriteName(parent, stageName); err == nil && sameWriteFile(stage, current) {
			_ = unix.Unlinkat(int(parent.Fd()), stageName, 0)
		}
	}()
	if err := write(output); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := output.Chmod(0o644); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := validateWriteDirectories(root, directories); err != nil {
		return err
	}
	currentStage, err := statWriteName(parent, stageName)
	if err != nil || !sameWriteFile(stage, currentStage) || currentStage.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrUnsafePath
	}
	current, err := statWriteName(parent, finalName)
	if exists {
		if err != nil || !sameWriteFile(expected, current) || current.Mode&unix.S_IFMT != unix.S_IFREG {
			return ErrUnsafePath
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrUnsafePath
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	quarantine := ""
	if exists {
		quarantine, err = randomName(".openreader-write-old-", 12)
		if err != nil {
			return err
		}
		if err := renameWriteNoReplace(parent, finalName, quarantine); err != nil {
			return writePublicationError(err)
		}
		// The rename is the identity claim. If a replacement won the race,
		// restore that entity without overwriting a later final name.
		moved, err := statWriteName(parent, quarantine)
		if err != nil || !sameWriteFile(expected, moved) || moved.Mode&unix.S_IFMT != unix.S_IFREG {
			_ = renameWriteNoReplace(parent, quarantine, finalName)
			return ErrUnsafePath
		}
		if afterWriteDetachTestHook != nil {
			afterWriteDetachTestHook(root.Name(), clean)
		}
	}
	restoreOriginal := func() {
		if quarantine != "" {
			// A concurrently installed final is never overwritten. If it wins,
			// retain the old file under its private quarantine for recovery.
			_ = renameWriteNoReplace(parent, quarantine, finalName)
		}
	}
	if err := ctx.Err(); err != nil {
		restoreOriginal()
		return err
	}
	if err := validateWriteDirectories(root, directories); err != nil {
		restoreOriginal()
		return err
	}
	currentStage, err = statWriteName(parent, stageName)
	if err != nil || !sameWriteFile(stage, currentStage) || currentStage.Mode&unix.S_IFMT != unix.S_IFREG {
		restoreOriginal()
		return ErrUnsafePath
	}
	if err := renameWriteNoReplace(parent, stageName, finalName); err != nil {
		restoreOriginal()
		return writePublicationError(err)
	}
	published, err := statWriteName(parent, finalName)
	if err != nil || !sameWriteFile(stage, published) || published.Mode&unix.S_IFMT != unix.S_IFREG {
		// Do not delete an unknown final entity during rollback.
		restoreOriginal()
		return ErrUnsafePath
	}
	if quarantine != "" {
		if old, err := statWriteName(parent, quarantine); err == nil && sameWriteFile(expected, old) && old.Mode&unix.S_IFMT == unix.S_IFREG {
			if err := unix.Unlinkat(int(parent.Fd()), quarantine, 0); err != nil {
				return err
			}
		} else {
			return ErrUnsafePath
		}
	}
	return nil
}

func openWriteDirectories(root *os.Root, parentRelative string) ([]writeDirectory, error) {
	rootFile, err := root.Open(".")
	if err != nil {
		return nil, ErrUnsafePath
	}
	rootInfo, err := rootFile.Stat()
	if err != nil {
		_ = rootFile.Close()
		return nil, err
	}
	directories := []writeDirectory{{relative: ".", file: rootFile, info: rootInfo}}
	fail := func(err error) ([]writeDirectory, error) {
		for _, directory := range directories {
			_ = directory.file.Close()
		}
		return nil, err
	}
	if parentRelative != "." {
		current := ""
		for _, part := range strings.Split(filepath.ToSlash(parentRelative), "/") {
			parent := directories[len(directories)-1].file
			info, err := statWriteName(parent, part)
			if err != nil {
				return fail(err)
			}
			if info.Mode&unix.S_IFMT == unix.S_IFLNK {
				return fail(ErrUnsafePath)
			}
			if info.Mode&unix.S_IFMT != unix.S_IFDIR {
				return fail(ErrNotDirectory)
			}
			child, openedInfo, err := openDirectoryAt(parent, part)
			if err != nil {
				return fail(ErrUnsafePath)
			}
			var opened unix.Stat_t
			if err := unix.Fstat(int(child.Fd()), &opened); err != nil || !sameWriteFile(info, opened) {
				_ = child.Close()
				return fail(ErrUnsafePath)
			}
			current = filepath.Join(current, part)
			directories = append(directories, writeDirectory{relative: current, file: child, info: openedInfo})
		}
	}
	if err := validateWriteDirectories(root, directories); err != nil {
		return fail(err)
	}
	return directories, nil
}

func validateWriteDirectories(root *os.Root, directories []writeDirectory) error {
	currentRoot, err := os.Lstat(root.Name())
	if err != nil || currentRoot.Mode()&os.ModeSymlink != 0 || !currentRoot.IsDir() || !os.SameFile(directories[0].info, currentRoot) {
		return ErrUnsafePath
	}
	for _, directory := range directories {
		current, err := root.Lstat(directory.relative)
		if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(directory.info, current) {
			return ErrUnsafePath
		}
	}
	return nil
}

func statWriteName(parent *os.File, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		return stat, os.ErrNotExist
	}
	return stat, err
}

func sameWriteFile(left, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino
}

func writePublicationError(err error) error {
	if errors.Is(err, syscall.EEXIST) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.ENOTSUP) {
		return ErrUnsafePath
	}
	return err
}
