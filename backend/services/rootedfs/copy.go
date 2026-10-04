package rootedfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	ErrCopySourceMissing     = errors.New("rooted copy source missing")
	ErrCopyParentMissing     = errors.New("rooted copy destination parent missing")
	ErrCopyDestinationExists = errors.New("rooted copy destination exists")
	ErrCopyCleanupPending    = errors.New("rooted copy committed with cleanup pending")
)

// Nonparallel package tests exercise recovery after detaching an old target.
var afterCopyDetachTestHook func(rootPath, relative string)
var afterCopyPublishTestHook func(rootPath, relative, stageName string)

type copyEntry struct {
	name     string
	stat     unix.Stat_t
	children []*copyEntry
}

// CopyTree admits a source tree and the original target before work, copies
// only through opened directories, then claims/publishes with no-overwrite
// renames. No absolute-path walk or cleanup is used after admission.
func CopyTree(ctx context.Context, rootPath, sourceRelative, destinationRelative string, overwrite bool) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, sourceClean, err := open(rootPath, sourceRelative)
	if err != nil {
		return err
	}
	defer root.Close()
	destinationClean, err := cleanRelative(destinationRelative)
	if err != nil {
		return err
	}
	if sourceClean == destinationClean || copyContains(sourceClean, destinationClean) || copyContains(destinationClean, sourceClean) {
		return ErrUnsafePath
	}
	sourceDirs, err := openWriteDirectories(root, filepath.Dir(filepath.FromSlash(sourceClean)))
	if errors.Is(err, os.ErrNotExist) {
		return ErrCopySourceMissing
	}
	if err != nil {
		return err
	}
	defer closeCopyDirectories(sourceDirs)
	destinationDirs, err := openWriteDirectories(root, filepath.Dir(filepath.FromSlash(destinationClean)))
	if errors.Is(err, os.ErrNotExist) {
		return ErrCopyParentMissing
	}
	if err != nil {
		return err
	}
	defer closeCopyDirectories(destinationDirs)
	sourceParent := sourceDirs[len(sourceDirs)-1].file
	destinationParent := destinationDirs[len(destinationDirs)-1].file
	source, err := snapshotCopyEntry(ctx, sourceParent, filepath.Base(sourceClean), false)
	if errors.Is(err, os.ErrNotExist) {
		return ErrCopySourceMissing
	}
	if err != nil {
		return err
	}
	finalName := filepath.Base(destinationClean)
	var old *copyEntry
	if stat, err := statWriteName(destinationParent, finalName); err == nil {
		if stat.Mode&unix.S_IFMT != unix.S_IFREG && stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			return ErrUnsafePath
		}
		if !overwrite {
			return ErrCopyDestinationExists
		}
		old, err = snapshotCopyEntry(ctx, destinationParent, finalName, true)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stageName, err := randomName(".openreader-copy-stage-", 12)
	if err != nil {
		return err
	}
	var staged *copyEntry
	publishedComplete := false
	defer func() {
		// A stage is ours only while its inode occupies the admitted parent.
		// The published tree keeps another name and is never a cleanup target.
		if staged != nil {
			if err := removeOwnedCopyEntry(destinationParent, staged); err != nil && publishedComplete {
				resultErr = ErrCopyCleanupPending
			}
		}
	}()
	validateLocation := func() error {
		if err := validateWriteDirectories(root, sourceDirs); err != nil {
			return err
		}
		if err := validateWriteDirectories(root, destinationDirs); err != nil {
			return err
		}
		if staged != nil {
			current, err := statWriteName(destinationParent, stageName)
			if err != nil || !sameWriteFile(staged.stat, current) || current.Mode&unix.S_IFMT != staged.stat.Mode&unix.S_IFMT {
				return ErrUnsafePath
			}
		}
		return nil
	}
	// Staging the top node directly in its final parent keeps the directory's
	// parent unchanged on publication. Read-only source directory permissions
	// can therefore be preserved without a writable final permission window.
	staged, err = cloneCopyEntry(ctx, sourceParent, source, destinationParent, stageName, validateLocation, func(node *copyEntry) { staged = node })
	if err != nil {
		return err
	}
	validate := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateLocation(); err != nil {
			return err
		}
		if err := validateCopyEntry(ctx, sourceParent, source); err != nil {
			return err
		}
		if err := validateCopyEntry(ctx, destinationParent, staged); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return validateLocation()
	}
	if err := validate(); err != nil {
		return err
	}
	if old != nil {
		if err := validateCopyEntry(ctx, destinationParent, old); err != nil {
			return err
		}
	} else if _, err := statWriteName(destinationParent, finalName); !errors.Is(err, os.ErrNotExist) {
		return ErrUnsafePath
	}
	quarantine := ""
	if err := ctx.Err(); err != nil {
		return err
	}
	if old != nil {
		quarantine, err = randomName(".openreader-copy-old-", 12)
		if err != nil {
			return err
		}
		if err := renameWriteNoReplace(destinationParent, finalName, quarantine); err != nil {
			return writePublicationError(err)
		}
		moved, err := statWriteName(destinationParent, quarantine)
		if err != nil || !sameWriteFile(old.stat, moved) || moved.Mode != old.stat.Mode || moved.Size != old.stat.Size || moved.Mtim != old.stat.Mtim {
			_ = renameWriteNoReplace(destinationParent, quarantine, finalName)
			return ErrUnsafePath
		}
		refreshCopyOwnRename(source, old.stat, moved)
		old.name, old.stat = quarantine, moved
		if afterCopyDetachTestHook != nil {
			afterCopyDetachTestHook(root.Name(), destinationClean)
		}
	}
	restoreOld := func() {
		if quarantine != "" {
			_ = renameWriteNoReplace(destinationParent, quarantine, finalName)
		}
	}
	if err := validate(); err != nil {
		restoreOld()
		return err
	}
	if old != nil {
		if err := validateCopyEntry(ctx, destinationParent, old); err != nil {
			restoreOld()
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		restoreOld()
		return err
	}
	if err := validateLocation(); err != nil {
		restoreOld()
		return err
	}
	expectedPublished := staged.stat
	if err := renameWriteNoReplace(destinationParent, stageName, finalName); err != nil {
		restoreOld()
		return writePublicationError(err)
	}
	published, err := statWriteName(destinationParent, finalName)
	if err != nil || !sameWriteFile(expectedPublished, published) {
		restoreOld()
		return ErrUnsafePath
	}
	publishedComplete = true
	if afterCopyPublishTestHook != nil {
		afterCopyPublishTestHook(root.Name(), destinationClean, stageName)
	}
	if old != nil {
		if err := validateCopyEntry(ctx, destinationParent, old); err != nil {
			return ErrCopyCleanupPending
		}
		if err := removeOwnedCopyEntry(destinationParent, old); err != nil {
			return ErrCopyCleanupPending
		}
	}
	return nil
}

func closeCopyDirectories(dirs []writeDirectory) {
	for _, dir := range dirs {
		_ = dir.file.Close()
	}
}

func copyContains(parent, child string) bool {
	relative, err := filepath.Rel(filepath.FromSlash(parent), filepath.FromSlash(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func snapshotCopyEntry(ctx context.Context, parent *os.File, name string, allowUnsafeChildren bool) (*copyEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stat, err := statWriteName(parent, name)
	if err != nil {
		return nil, err
	}
	node := &copyEntry{name: name, stat: stat}
	kind := stat.Mode & unix.S_IFMT
	if kind != unix.S_IFDIR && kind != unix.S_IFREG {
		if allowUnsafeChildren {
			return node, nil
		}
		return nil, ErrUnsafePath
	}
	file, _, err := openEntryAt(parent, name, kind == unix.S_IFDIR)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &opened); err != nil || !sameCopyStat(stat, opened) {
		return nil, ErrUnsafePath
	}
	if kind == unix.S_IFDIR {
		entries, err := file.ReadDir(-1)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			child, err := snapshotCopyEntry(ctx, file, entry.Name(), allowUnsafeChildren)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil, ErrUnsafePath
				}
				return nil, err
			}
			node.children = append(node.children, child)
		}
	}
	if current, err := statWriteName(parent, name); err != nil || !sameCopyStat(stat, current) {
		return nil, ErrUnsafePath
	}
	return node, nil
}

func sameCopyStat(expected, current unix.Stat_t) bool {
	return sameWriteFile(expected, current) && expected.Mode == current.Mode && expected.Size == current.Size && expected.Mtim == current.Mtim && expected.Ctim == current.Ctim
}

// A historical source file can share the old target inode. Our verified rename
// changes that inode's ctime, but not its bytes, permissions or mtime. Refresh
// only the matching admitted snapshot before any post-detach work, so later
// external changes still fail the full metadata check.
func refreshCopyOwnRename(node *copyEntry, before, moved unix.Stat_t) {
	if sameCopyStat(node.stat, before) {
		node.stat.Ctim = moved.Ctim
	}
	for _, child := range node.children {
		refreshCopyOwnRename(child, before, moved)
	}
}

func validateCopyEntry(ctx context.Context, parent *os.File, node *copyEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stat, err := statWriteName(parent, node.name)
	if err != nil || !sameCopyStat(node.stat, stat) {
		return ErrUnsafePath
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil
	}
	file, _, err := openDirectoryAt(parent, node.name)
	if err != nil {
		return err
	}
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &opened); err != nil || !sameWriteFile(stat, opened) {
		return ErrUnsafePath
	}
	entries, err := file.ReadDir(-1)
	if err != nil {
		return err
	}
	if len(entries) != len(node.children) {
		return ErrUnsafePath
	}
	for _, child := range node.children {
		if err := validateCopyEntry(ctx, file, child); err != nil {
			return err
		}
	}
	return nil
}

// cloneCopyEntry returns its owned partial tree on error as well as success,
// so failure cleanup never has to guess ownership from directory names.
func cloneCopyEntry(ctx context.Context, parent *os.File, source *copyEntry, outParent *os.File, name string, validateLocation func() error, created func(*copyEntry)) (*copyEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateLocation(); err != nil {
		return nil, err
	}
	if current, err := statWriteName(parent, source.name); err != nil || !sameCopyStat(source.stat, current) {
		return nil, ErrUnsafePath
	}
	isDir := source.stat.Mode&unix.S_IFMT == unix.S_IFDIR
	input, _, err := openEntryAt(parent, source.name, isDir)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(int(input.Fd()), &opened); err != nil || !sameCopyStat(source.stat, opened) {
		return nil, ErrUnsafePath
	}
	var output *os.File
	if isDir {
		if err := unix.Mkdirat(int(outParent.Fd()), name, 0o700); err != nil {
			return nil, err
		}
		output, _, err = openDirectoryAt(outParent, name)
	} else {
		fd, openErr := unix.Openat(int(outParent.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
		err = openErr
		if err == nil {
			output = os.NewFile(uintptr(fd), name)
		}
	}
	if err != nil {
		return nil, err
	}
	defer output.Close()
	node := &copyEntry{name: name}
	if err := unix.Fstat(int(output.Fd()), &node.stat); err != nil {
		return node, err
	}
	if created != nil {
		created(node)
	}
	if isDir {
		for _, child := range source.children {
			cloned, err := cloneCopyEntry(ctx, input, child, output, child.name, validateLocation, nil)
			if cloned != nil {
				node.children = append(node.children, cloned)
			}
			if err != nil {
				return node, err
			}
		}
	} else {
		buffer := make([]byte, 32*1024)
		for {
			if err := ctx.Err(); err != nil {
				return node, err
			}
			if err := validateLocation(); err != nil {
				return node, err
			}
			n, readErr := input.Read(buffer)
			if n > 0 {
				if _, err := output.Write(buffer[:n]); err != nil {
					return node, err
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					return node, readErr
				}
				break
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return node, err
	}
	if err := validateLocation(); err != nil {
		return node, err
	}
	if err := unix.Fstat(int(input.Fd()), &opened); err != nil || !sameCopyStat(source.stat, opened) {
		return node, ErrUnsafePath
	}
	if err := output.Chmod(os.FileMode(source.stat.Mode & 0o777)); err != nil {
		return node, err
	}
	if err := output.Sync(); err != nil {
		return node, err
	}
	if err := unix.Fstat(int(output.Fd()), &node.stat); err != nil {
		return node, err
	}
	if err := output.Close(); err != nil {
		return node, err
	}
	return node, nil
}

// Cleanup first claims one entry by no-replace rename and verifies that claim.
// Unknown descendants are retained, not recursively deleted by a lexical walk.
func removeOwnedCopyEntry(parent *os.File, node *copyEntry) error {
	current, err := statWriteName(parent, node.name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !sameWriteFile(node.stat, current) || current.Mode&unix.S_IFMT != node.stat.Mode&unix.S_IFMT {
		return ErrUnsafePath
	}
	claimed, err := randomName(".openreader-copy-clean-", 12)
	if err != nil {
		return err
	}
	if err := renameWriteNoReplace(parent, node.name, claimed); err != nil {
		return err
	}
	restore := func() { _ = renameWriteNoReplace(parent, claimed, node.name) }
	current, err = statWriteName(parent, claimed)
	if err != nil || !sameWriteFile(node.stat, current) || current.Mode&unix.S_IFMT != node.stat.Mode&unix.S_IFMT {
		restore()
		return ErrUnsafePath
	}
	flags := 0
	if current.Mode&unix.S_IFMT == unix.S_IFDIR {
		file, _, err := openDirectoryAt(parent, claimed)
		if err != nil {
			restore()
			return err
		}
		defer file.Close()
		var opened unix.Stat_t
		if err := unix.Fstat(int(file.Fd()), &opened); err != nil || !sameWriteFile(current, opened) {
			restore()
			return ErrUnsafePath
		}
		entries, err := file.ReadDir(-1)
		if err != nil {
			restore()
			return err
		}
		if len(entries) != len(node.children) {
			restore()
			return ErrUnsafePath
		}
		for _, child := range node.children {
			stat, err := statWriteName(file, child.name)
			if err != nil || !sameWriteFile(child.stat, stat) || child.stat.Mode&unix.S_IFMT != stat.Mode&unix.S_IFMT {
				restore()
				return ErrUnsafePath
			}
		}
		// A copied read-only directory is mutable only AFTER its owned claim;
		// source directories and live final directories are never chmodded.
		if err := file.Chmod(0o700); err != nil {
			restore()
			return err
		}
		for _, child := range node.children {
			if err := removeOwnedCopyEntry(file, child); err != nil {
				restore()
				return err
			}
		}
		flags = unix.AT_REMOVEDIR
	}
	current, err = statWriteName(parent, claimed)
	if err != nil || !sameWriteFile(node.stat, current) {
		restore()
		return ErrUnsafePath
	}
	if err := unix.Unlinkat(int(parent.Fd()), claimed, flags); err != nil {
		restore()
		return err
	}
	return nil
}
