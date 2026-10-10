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

// PrivateScope retains a configured boundary and original directory chain for
// several related private-file actions. It is sequential, owns its handles, and
// never re-admits an absolute pathname during read/publication/cleanup.
type PrivateScope struct {
	read            *ReadScope
	parent          *readDirectory
	ctx             context.Context
	label           string
	created         []*createdDirectory
	keepDirectories bool
}

// PrivateEntry records original identity OR original absence. Reading an opened
// file may tolerate a legitimate final rename; replacing/deleting never does.
type PrivateEntry struct {
	scope *PrivateScope
	Name  string
	Info  os.FileInfo
	Read  bool
}

// Nil outside native ownership fixtures; observes a newly opened, owned
// initialization directory before its first publication/context check.
var privateScopeDirectoryCreatedTestHook func(*os.File)

func OpenPrivateScope(ctx context.Context, boundary, relative string, create bool) (*PrivateScope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(boundary) == "" {
		return nil, ErrUnsafePath
	}
	if relative != "." {
		var err error
		relative, err = cleanRelative(relative)
		if err != nil {
			return nil, err
		}
	}
	configured, err := filepath.Abs(filepath.Clean(boundary))
	if err != nil {
		return nil, ErrUnsafePath
	}
	anchor := configured
	for {
		info, err := os.Lstat(anchor)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return nil, ErrUnsafePath
			}
			break
		}
		if !create || !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		next := filepath.Dir(anchor)
		if next == anchor {
			return nil, ErrUnsafePath
		}
		anchor = next
	}
	handle, err := AdmitRead(ctx, anchor, ".")
	if err != nil {
		return nil, err
	}
	read, err := NewReadScope(handle)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}
	s := &PrivateScope{read: read, parent: read.directory, ctx: ctx, label: filepath.Join(configured, relative)}
	complete := false
	defer func() {
		if !complete {
			_ = s.Close()
		}
	}()
	path, err := filepath.Rel(anchor, s.label)
	if err != nil {
		return nil, ErrUnsafePath
	}
	if path != "." {
		for _, part := range strings.Split(filepath.ToSlash(path), "/") {
			if part == "" || part == "." || part == ".." {
				return nil, ErrUnsafePath
			}
			if err := s.Validate(); err != nil {
				return nil, err
			}
			info, err := s.parent.meta.Lstat(part)
			if errors.Is(err, os.ErrNotExist) && create {
				name, err := randomName(".openreader-directory-", 12)
				if err != nil {
					return nil, err
				}
				if err := unix.Mkdirat(int(s.parent.file.Fd()), name, 0o700); err != nil {
					return nil, err
				}
				stageInfo, err := s.parent.meta.Lstat(name)
				if err != nil {
					return nil, err
				}
				stage, err := openReadDirectory(s.parent, name, stageInfo)
				if err != nil {
					return nil, err
				}
				var stat unix.Stat_t
				if err := unix.Fstat(int(stage.file.Fd()), &stat); err != nil {
					_ = closeReadDirectory(stage)
					return nil, err
				}
				owned := &createdDirectory{parent: s.parent.file, name: name, stat: stat}
				s.created = append(s.created, owned)
				if privateScopeDirectoryCreatedTestHook != nil {
					privateScopeDirectoryCreatedTestHook(stage.file)
				}
				if err := s.Validate(); err != nil {
					_ = closeReadDirectory(stage)
					return nil, err
				}
				install := renameWriteNoReplace(s.parent.file, name, part)
				if install == nil {
					owned.name = part
					stage.name = part
					if err := checkReadDirectory(stage); err != nil {
						_ = closeReadDirectory(stage)
						return nil, err
					}
					s.read.owned = append(s.read.owned, stage)
					s.parent = stage
					continue
				}
				_ = closeReadDirectory(stage)
				removeCreatedEmptyDirectory(owned)
				if !errors.Is(install, unix.EEXIST) {
					return nil, writePublicationError(install)
				}
				// Another legitimate initializer may win. Bind it once through the
				// still-original parent; never restart admission at the absolute root.
				info, err = s.parent.meta.Lstat(part)
			}
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return nil, ErrUnsafePath
			}
			child, err := openReadDirectory(s.parent, part, info)
			if err != nil {
				return nil, err
			}
			s.read.owned = append(s.read.owned, child)
			s.parent = child
		}
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	complete = true
	return s, nil
}

func (s *PrivateScope) PathLabel() string { return s.label }
func (s *PrivateScope) KeepDirectories()  { s.keepDirectories = true }
func (s *PrivateScope) Close() error {
	if s.read == nil {
		return nil
	}
	if !s.keepDirectories {
		for i := len(s.created) - 1; i >= 0; i-- {
			removeCreatedEmptyDirectory(s.created[i])
		}
	}
	err := s.read.Close()
	s.read, s.parent, s.created = nil, nil, nil
	return err
}

// Cleanup can ignore cancellation ONLY after independently validating original
// directory identities. It does not mutate an unsafe/replaced namespace.
func (s *PrivateScope) identity() error {
	if s.read == nil || s.parent == nil {
		return ErrUnsafePath
	}
	a := s.read.anchor
	current, err := os.Lstat(a.boundary)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(a.dirs[0].info, current) {
		return ErrUnsafePath
	}
	for p := s.parent; p != nil; p = p.parent {
		if err := checkReadDirectory(p); err != nil {
			return err
		}
	}
	return nil
}

func (s *PrivateScope) Validate() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if err := s.identity(); err != nil {
		return err
	}
	return s.ctx.Err()
}

func privateName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, `/\\`) && !strings.ContainsRune(name, 0)
}

func (s *PrivateScope) Snapshot(name string) (*PrivateEntry, error) {
	if !privateName(name) {
		return nil, ErrUnsafePath
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	info, err := s.parent.meta.Lstat(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if info != nil && !info.Mode().IsRegular() {
		return nil, ErrUnsafePath
	}
	e := &PrivateEntry{scope: s, Name: name, Info: info}
	return e, e.Validate()
}

func (s *PrivateScope) SnapshotOriginal(name string, info os.FileInfo) (*PrivateEntry, error) {
	if !privateName(name) || (info != nil && !info.Mode().IsRegular()) {
		return nil, ErrUnsafePath
	}
	e := &PrivateEntry{scope: s, Name: name, Info: info}
	return e, e.Validate()
}

func (e *PrivateEntry) identity() error {
	if err := e.scope.identity(); err != nil {
		return err
	}
	current, err := e.scope.parent.meta.Lstat(e.Name)
	if e.Info == nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return ErrUnsafePath
	}
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(e.Info, current) {
		return ErrUnsafePath
	}
	return nil
}

func (e *PrivateEntry) Validate() error {
	if err := e.scope.Validate(); err != nil {
		return err
	}
	return e.identity()
}

func (e *PrivateEntry) Open() (*os.File, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if e.Info == nil {
		return nil, os.ErrNotExist
	}
	h := e.scope.read.anchor.retainEntry(e.scope.parent, e.Name, ".", e.Info)
	defer h.Close()
	file, _, err := h.OpenRegular()
	if err == nil {
		e.Read = true
	}
	return file, err
}

// Entries snapshots native names+metadata against the held directory in bounded
// batches. Callers classify safe regular entries; special/unknown entries remain.
func (s *PrivateScope) Entries() ([]ReadEntry, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(s.parent.file.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), s.label)
	defer file.Close()
	result := []ReadEntry{}
	for {
		if err := s.Validate(); err != nil {
			return nil, err
		}
		entries, scanErr := file.ReadDir(64)
		for _, entry := range entries {
			if err := s.Validate(); err != nil {
				return nil, err
			}
			info, err := s.parent.meta.Lstat(entry.Name())
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			result = append(result, ReadEntry{RelativePath: entry.Name(), Info: info})
		}
		if errors.Is(scanErr, io.EOF) {
			break
		}
		if scanErr != nil {
			return nil, scanErr
		}
	}
	return result, s.Validate()
}

func (s *PrivateScope) Child(name string, info os.FileInfo) (*PrivateScope, error) {
	if !privateName(name) || info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafePath
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	h := s.read.anchor.retainEntry(s.parent, name, ".", info)
	read, err := NewReadScope(h)
	if err != nil {
		_ = h.Close()
		return nil, err
	}
	return &PrivateScope{read: read, parent: read.directory, ctx: s.ctx, label: filepath.Join(s.label, name), keepDirectories: true}, nil
}

// WriteNew exclusively owns the final name; unknown name replacements are never
// removed by compensation. The writer must bound bytes and observe its context.
func (s *PrivateScope) WriteNew(e *PrivateEntry, write func(io.Writer) error) (err error) {
	if e.scope != s || e.Info != nil {
		return ErrUnsafePath
	}
	if err := e.Validate(); err != nil {
		return err
	}
	fd, err := unix.Openat(int(s.parent.file.Fd()), e.Name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return writePublicationError(err)
	}
	file := os.NewFile(uintptr(fd), e.Name)
	defer file.Close()
	e.Info, err = file.Stat()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.RemoveOwned(e)
		}
	}()
	if err = write(file); err != nil {
		return err
	}
	if err = e.Validate(); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = e.Validate(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return e.Validate()
}

// RemoveOwned ignores request cancellation only for already-owned compensation;
// still-original ancestors and the final inode are required before detachment.
func (s *PrivateScope) RemoveOwned(e *PrivateEntry) error { return s.remove(e, false) }
func (s *PrivateScope) Remove(e *PrivateEntry) error      { return s.remove(e, true) }
func (s *PrivateScope) remove(e *PrivateEntry, observeContext bool) error {
	if e.scope != s {
		return ErrUnsafePath
	}
	if observeContext {
		if err := s.Validate(); err != nil {
			return err
		}
	}
	if err := e.identity(); err != nil {
		return err
	}
	if e.Info == nil {
		return nil
	}
	quarantine, err := randomName(".openreader-write-old-", 12)
	if err != nil {
		return err
	}
	if err := renameWriteNoReplace(s.parent.file, e.Name, quarantine); err != nil {
		return writePublicationError(err)
	}
	moved, err := s.parent.meta.Lstat(quarantine)
	if err != nil || !moved.Mode().IsRegular() || !os.SameFile(e.Info, moved) {
		_ = renameWriteNoReplace(s.parent.file, quarantine, e.Name)
		return ErrUnsafePath
	}
	if observeContext {
		err = s.Validate()
	} else {
		err = s.identity()
	}
	if err != nil {
		_ = renameWriteNoReplace(s.parent.file, quarantine, e.Name)
		return err
	}
	current, err := s.parent.meta.Lstat(quarantine)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(e.Info, current) {
		return ErrUnsafePath
	}
	if err := unix.Unlinkat(int(s.parent.file.Fd()), quarantine, 0); err != nil {
		return err
	}
	e.Info, e.Read = nil, false
	return nil
}

func (s *PrivateScope) Replace(e *PrivateEntry, prefix string, write func(io.Writer) error, ready func(string)) error {
	if e.scope != s || !privateName(prefix) {
		return ErrUnsafePath
	}
	if err := e.Validate(); err != nil {
		return err
	}
	name, err := randomName(prefix, 12)
	if err != nil {
		return err
	}
	temp, err := s.Snapshot(name)
	if err != nil {
		return err
	}
	if err := s.WriteNew(temp, write); err != nil {
		return err
	}
	defer func() { _ = s.RemoveOwned(temp) }()
	if ready != nil {
		ready(filepath.Join(s.label, name))
	}
	if err := temp.Validate(); err != nil {
		return err
	}
	if err := e.Validate(); err != nil {
		return err
	}
	quarantine := ""
	original := e.Info
	if e.Info != nil {
		quarantine, err = randomName(".openreader-write-old-", 12)
		if err != nil {
			return err
		}
		if err := renameWriteNoReplace(s.parent.file, e.Name, quarantine); err != nil {
			return writePublicationError(err)
		}
		moved, err := s.parent.meta.Lstat(quarantine)
		if err != nil || !moved.Mode().IsRegular() || !os.SameFile(e.Info, moved) {
			_ = renameWriteNoReplace(s.parent.file, quarantine, e.Name)
			return ErrUnsafePath
		}
	}
	restore := func() {
		if quarantine != "" {
			_ = renameWriteNoReplace(s.parent.file, quarantine, e.Name)
		}
	}
	if err := temp.Validate(); err != nil {
		restore()
		return err
	}
	if err := renameWriteNoReplace(s.parent.file, temp.Name, e.Name); err != nil {
		restore()
		return writePublicationError(err)
	}
	e.Info, e.Read, temp.Info = temp.Info, false, nil
	// Publication must be checked before retiring the previous snapshot. If the
	// namespace changed, retain the quarantine instead of deleting through it.
	if err := e.Validate(); err != nil {
		return err
	}
	if quarantine != "" {
		old, err := s.parent.meta.Lstat(quarantine)
		if err == nil && old.Mode().IsRegular() && os.SameFile(original, old) {
			_ = unix.Unlinkat(int(s.parent.file.Fd()), quarantine, 0)
		}
	}
	return nil
}
