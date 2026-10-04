package rootedfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	ErrMoveSourceMissing     = errors.New("rooted move source missing")
	ErrMoveParentMissing     = errors.New("rooted move destination parent missing")
	ErrMoveDestinationExists = errors.New("rooted move destination exists")
	ErrMoveCleanupPending    = errors.New("rooted move committed with cleanup pending")
)

// These seams are used only by nonparallel package contract tests.
var beforeMoveClaimTestHook func(rootPath, source, destination string)
var afterMoveDetachTestHook func(rootPath, destination, sourceClaim, oldClaim string)
var afterMovePublishTestHook func(rootPath, destination, oldClaim string)

// MoveTree moves the admitted top inode without copying, reading or chmodding
// its content. All names are relative to the original opened parents. A claimed
// source/old target is restored only to an empty name, never over a newcomer.
func MoveTree(ctx context.Context, rootPath, sourceRelative, destinationRelative string, overwrite bool) error {
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
		return ErrMoveSourceMissing
	}
	if err != nil {
		return err
	}
	defer closeCopyDirectories(sourceDirs)
	destinationDirs, err := openWriteDirectories(root, filepath.Dir(filepath.FromSlash(destinationClean)))
	if errors.Is(err, os.ErrNotExist) {
		return ErrMoveParentMissing
	}
	if err != nil {
		return err
	}
	defer closeCopyDirectories(destinationDirs)
	sourceParent := sourceDirs[len(sourceDirs)-1].file
	destinationParent := destinationDirs[len(destinationDirs)-1].file
	sourceName, finalName := filepath.Base(sourceClean), filepath.Base(destinationClean)
	sourceStat, err := statWriteName(sourceParent, sourceName)
	if errors.Is(err, os.ErrNotExist) {
		return ErrMoveSourceMissing
	}
	if err != nil {
		return err
	}
	if !moveTopKind(sourceStat) {
		return ErrUnsafePath
	}
	source := &copyEntry{name: sourceName, stat: sourceStat}
	var destinationParentStat unix.Stat_t
	if err := unix.Fstat(int(destinationParent.Fd()), &destinationParentStat); err != nil {
		return err
	}
	if sourceStat.Dev != destinationParentStat.Dev {
		return syscall.EXDEV
	}
	var old *copyEntry
	if stat, err := statWriteName(destinationParent, finalName); err == nil {
		if !moveTopKind(stat) {
			return ErrUnsafePath
		}
		if !overwrite {
			return ErrMoveDestinationExists
		}
		old, err = snapshotMoveCleanupEntry(ctx, destinationParent, finalName)
		if err != nil {
			return err
		}
		if !sameCopyStat(stat, old.stat) {
			return ErrUnsafePath
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	validateLocations := func() error {
		if err := validateWriteDirectories(root, sourceDirs); err != nil {
			return err
		}
		return validateWriteDirectories(root, destinationDirs)
	}
	validateSource := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateLocations(); err != nil {
			return err
		}
		current, err := statWriteName(sourceParent, source.name)
		if err != nil || !sameCopyStat(source.stat, current) {
			return ErrUnsafePath
		}
		return nil
	}
	validateTarget := func() error {
		if old != nil {
			return validateCopyEntry(ctx, destinationParent, old)
		}
		if _, err := statWriteName(destinationParent, finalName); !errors.Is(err, os.ErrNotExist) {
			return ErrUnsafePath
		}
		return nil
	}
	if beforeMoveClaimTestHook != nil {
		beforeMoveClaimTestHook(root.Name(), sourceClean, destinationClean)
	}
	if err := validateSource(); err != nil {
		return err
	}
	if err := validateTarget(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateLocations(); err != nil {
		return err
	}
	sourceClaim, err := randomName(".openreader-move-source-", 12)
	if err != nil {
		return err
	}
	if err := renameWriteNoReplace(sourceParent, sourceName, sourceClaim); err != nil {
		return writePublicationError(err)
	}
	claimed, err := statWriteName(sourceParent, sourceClaim)
	if err != nil || !sameMoveRenamedStat(source.stat, claimed) {
		if err == nil {
			restoreMoveName(sourceParent, sourceClaim, sourceName, claimed)
		}
		return ErrUnsafePath
	}
	if old != nil {
		refreshCopyOwnRename(old, source.stat, claimed)
	}
	source.name, source.stat = sourceClaim, claimed
	oldClaim := ""
	published := false
	defer func() {
		if !published {
			if oldClaim != "" {
				restoreMoveName(destinationParent, oldClaim, finalName, old.stat)
			}
			restoreMoveName(sourceParent, sourceClaim, sourceName, source.stat)
		}
	}()
	validateClaimed := func() error {
		if err := validateSource(); err != nil {
			return err
		}
		if _, err := statWriteName(sourceParent, sourceName); !errors.Is(err, os.ErrNotExist) {
			return ErrUnsafePath
		}
		if err := validateTarget(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return validateLocations()
	}
	if err := validateClaimed(); err != nil {
		return err
	}
	if old != nil {
		name, err := randomName(".openreader-move-old-", 12)
		if err != nil {
			return err
		}
		if err := renameWriteNoReplace(destinationParent, finalName, name); err != nil {
			return writePublicationError(err)
		}
		moved, err := statWriteName(destinationParent, name)
		if err != nil || !sameMoveRenamedStat(old.stat, moved) {
			if err == nil {
				restoreMoveName(destinationParent, name, finalName, moved)
			}
			return ErrUnsafePath
		}
		refreshCopyOwnRename(source, old.stat, moved)
		old.name, old.stat = name, moved
		oldClaim = name
	}
	if afterMoveDetachTestHook != nil {
		afterMoveDetachTestHook(root.Name(), destinationClean, sourceClaim, oldClaim)
	}
	if err := validateClaimed(); err != nil {
		return err
	}
	if _, err := statWriteName(destinationParent, finalName); !errors.Is(err, os.ErrNotExist) {
		return ErrUnsafePath
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateLocations(); err != nil {
		return err
	}
	if err := renameBetweenNoReplace(sourceParent, sourceClaim, destinationParent, finalName); err != nil {
		return writePublicationError(err)
	}
	current, err := statWriteName(destinationParent, finalName)
	if err != nil || !sameMoveRenamedStat(source.stat, current) {
		// If the renamed inode is still ours, return it without overwriting a
		// later source claim/name. Unknown replacement final is never moved.
		if err == nil && sameWriteFile(source.stat, current) {
			_ = renameBetweenNoReplace(destinationParent, finalName, sourceParent, sourceClaim)
		}
		return ErrUnsafePath
	}
	published = true
	if old != nil {
		refreshCopyOwnRename(old, source.stat, current)
	}
	if afterMovePublishTestHook != nil {
		afterMovePublishTestHook(root.Name(), destinationClean, oldClaim)
	}
	if old != nil {
		if err := validateCopyEntry(ctx, destinationParent, old); err != nil {
			return ErrMoveCleanupPending
		}
		if err := removeOwnedCopyEntry(destinationParent, old); err != nil {
			return ErrMoveCleanupPending
		}
	}
	return nil
}

func moveTopKind(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG || stat.Mode&unix.S_IFMT == unix.S_IFDIR
}

// ctime may change on a verified rename; bytes, mode and mtime must not.
func sameMoveRenamedStat(before, current unix.Stat_t) bool {
	return sameWriteFile(before, current) && before.Mode == current.Mode && before.Size == current.Size && before.Mtim == current.Mtim
}

func restoreMoveName(parent *os.File, from, to string, expected unix.Stat_t) {
	current, err := statWriteName(parent, from)
	if err == nil && sameWriteFile(expected, current) && expected.Mode&unix.S_IFMT == current.Mode&unix.S_IFMT {
		_ = renameWriteNoReplace(parent, from, to)
	}
}

// Only the old target will be deleted, so only that tree needs descendant
// admission. Regular/symlink/special leaves need no read permission; directories
// are opened no-follow and checked before any source/target is detached.
func snapshotMoveCleanupEntry(ctx context.Context, parent *os.File, name string) (*copyEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stat, err := statWriteName(parent, name)
	if err != nil {
		return nil, err
	}
	node := &copyEntry{name: name, stat: stat}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return node, nil
	}
	file, _, err := openDirectoryAt(parent, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &opened); err != nil || !sameCopyStat(stat, opened) {
		return nil, ErrUnsafePath
	}
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		child, err := snapshotMoveCleanupEntry(ctx, file, entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrUnsafePath
		}
		if err != nil {
			return nil, err
		}
		node.children = append(node.children, child)
	}
	if current, err := statWriteName(parent, name); err != nil || !sameCopyStat(stat, current) {
		return nil, ErrUnsafePath
	}
	return node, nil
}
