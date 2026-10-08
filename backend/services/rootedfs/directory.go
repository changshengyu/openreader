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

var ErrDirectoryConflict = errors.New("rooted directory target is a regular file")

// Package contract tests installing these seams must not run in parallel.
var beforeDirectoryCreateTestHook func(anchor, relative string)
var afterDirectoryStageTestHook func(anchor, relative, stage string)
var afterDirectoryInstallTestHook func(anchor, relative string)

type createdDirectory struct {
	parent *os.File
	name   string
	stat   unix.Stat_t
}

// CreateDirectories preserves mkdir-all semantics without following a changed
// absolute parent. rootPath is server configuration, never a client-selected
// anchor. A missing configured root is created beneath its original existing
// configuration ancestor. relative "." is reserved for root initialization.
func CreateDirectories(ctx context.Context, rootPath, relative string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(rootPath) == "" {
		return ErrUnsafePath
	}
	if relative != "." {
		clean, err := cleanRelative(relative)
		if err != nil {
			return err
		}
		relative = clean
	}
	configuredRoot, err := filepath.Abs(filepath.Clean(rootPath))
	if err != nil {
		return ErrUnsafePath
	}
	anchor := configuredRoot
	var anchorInfo os.FileInfo
	for {
		info, err := os.Lstat(anchor)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return ErrUnsafePath
			}
			anchorInfo = info
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		parent := filepath.Dir(anchor)
		if parent == anchor {
			return ErrUnsafePath
		}
		anchor = parent
	}
	root, err := openDirectoryAnchor(anchor, anchorInfo)
	if err != nil {
		return err
	}
	defer root.Close()
	target := filepath.Join(configuredRoot, relative)
	path, err := filepath.Rel(anchor, target)
	if err != nil || path == ".." || strings.HasPrefix(path, ".."+string(os.PathSeparator)) {
		return ErrUnsafePath
	}
	directories, err := openWriteDirectories(root, ".")
	if err != nil {
		return err
	}
	defer func() { closeCopyDirectories(directories) }()
	heldStages := []*os.File{}
	defer func() {
		for _, file := range heldStages {
			_ = file.Close()
		}
	}()
	created := []*createdDirectory{}
	complete := false
	defer func() {
		if !complete {
			for index := len(created) - 1; index >= 0; index-- {
				removeCreatedEmptyDirectory(created[index])
			}
		}
	}()
	validate := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return validateWriteDirectories(root, directories)
	}
	if path == "." {
		return validate()
	}
	current := ""
	var finalParent *os.File
	var finalName string
	var finalExpected unix.Stat_t
	parts := strings.Split(filepath.ToSlash(path), "/")
	for index, part := range parts {
		if err := validate(); err != nil {
			return err
		}
		parent := directories[len(directories)-1].file
		current = filepath.Join(current, part)
		stat, err := statWriteName(parent, part)
		if errors.Is(err, os.ErrNotExist) {
			if beforeDirectoryCreateTestHook != nil {
				beforeDirectoryCreateTestHook(anchor, current)
			}
			if err := validate(); err != nil {
				return err
			}
			stage, nameErr := randomName(".openreader-directory-", 12)
			if nameErr != nil {
				return nameErr
			}
			if err := unix.Mkdirat(int(parent.Fd()), stage, 0o755); err != nil {
				return err
			}
			child, info, openErr := openDirectoryAt(parent, stage)
			if openErr != nil {
				// Without an opened identity there is no cleanup ownership.
				return openErr
			}
			heldStages = append(heldStages, child)
			var opened unix.Stat_t
			if err := unix.Fstat(int(child.Fd()), &opened); err != nil {
				return err
			}
			owned := &createdDirectory{parent: parent, name: stage, stat: opened}
			created = append(created, owned)
			if afterDirectoryStageTestHook != nil {
				afterDirectoryStageTestHook(anchor, current, stage)
			}
			if err := validate(); err != nil {
				return err
			}
			if err := validateCreatedDirectory(owned); err != nil {
				return err
			}
			err = renameWriteNoReplace(parent, stage, part)
			if errors.Is(err, syscall.EEXIST) {
				// A competing safe directory can satisfy mkdir's idempotent
				// contract. Never claim its inode for cleanup or overwrite it.
				removeCreatedEmptyDirectory(owned)
				stat, err = statWriteName(parent, part)
			} else if err != nil {
				return writePublicationError(err)
			} else {
				owned.name = part
				if err := validateCreatedDirectory(owned); err != nil {
					return err
				}
				directories = append(directories, writeDirectory{relative: current, file: child, info: info})
				if afterDirectoryInstallTestHook != nil {
					afterDirectoryInstallTestHook(anchor, current)
				}
				if err := validate(); err != nil {
					return err
				}
				continue
			}
		}
		if err != nil {
			return err
		}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
		case unix.S_IFREG:
			if index == len(parts)-1 {
				return ErrDirectoryConflict
			}
			return ErrNotDirectory
		default:
			return ErrUnsafePath
		}
		if index == len(parts)-1 {
			// An existing final directory needs no content-read permission.
			if err := validate(); err != nil {
				return err
			}
			final, err := statWriteName(parent, part)
			if err != nil || !sameWriteFile(stat, final) || final.Mode&unix.S_IFMT != unix.S_IFDIR {
				return ErrUnsafePath
			}
			finalParent, finalName, finalExpected = parent, part, stat
			break
		}
		child, info, err := openDirectoryAt(parent, part)
		if err != nil {
			return err
		}
		var opened unix.Stat_t
		if err := unix.Fstat(int(child.Fd()), &opened); err != nil || !sameWriteFile(stat, opened) {
			_ = child.Close()
			return ErrUnsafePath
		}
		directories = append(directories, writeDirectory{relative: current, file: child, info: info})
	}
	if err := validate(); err != nil {
		return err
	}
	if finalParent != nil {
		current, err := statWriteName(finalParent, finalName)
		if err != nil || current.Mode&unix.S_IFMT != unix.S_IFDIR || !sameWriteFile(finalExpected, current) {
			return ErrUnsafePath
		}
	}
	complete = true
	return nil
}

func openDirectoryAnchor(path string, expected os.FileInfo) (*os.Root, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnsafePath
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, opened) {
		_ = root.Close()
		return nil, ErrUnsafePath
	}
	return root, nil
}

func validateCreatedDirectory(node *createdDirectory) error {
	current, err := statWriteName(node.parent, node.name)
	if err != nil || current.Mode&unix.S_IFMT != unix.S_IFDIR || !sameWriteFile(node.stat, current) {
		return ErrUnsafePath
	}
	return nil
}

func removeCreatedEmptyDirectory(node *createdDirectory) {
	if validateCreatedDirectory(node) == nil {
		_ = unix.Unlinkat(int(node.parent.Fd()), node.name, unix.AT_REMOVEDIR)
	}
}
