package rootedfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ReadScope owns one admitted directory and its original ancestor chain.
// Subsequent selections are relative to that held directory, never a new
// absolute-path admission. Close the scope AND each returned ReadHandle.
// Like ReadHandle, admission/walk/close on a scope are sequential operations.
type ReadScope struct {
	anchor    *ReadHandle
	directory *readDirectory
	parents   map[string]*readDirectory
	owned     []*readDirectory
}

// NewReadScope transfers ownership of anchor on success only.
func NewReadScope(anchor *ReadHandle) (*ReadScope, error) {
	if err := anchor.Validate(); err != nil {
		return nil, err
	}
	if !anchor.Info.IsDir() {
		return nil, ErrNotDirectory
	}
	directory, err := openReadDirectory(anchor.parent, anchor.name, anchor.Info)
	if err != nil {
		return nil, err
	}
	scope := &ReadScope{anchor: anchor, directory: directory,
		parents: map[string]*readDirectory{".": directory}, owned: []*readDirectory{directory}}
	if err := anchor.Validate(); err != nil {
		_ = closeReadDirectory(directory)
		return nil, err
	}
	return scope, nil
}

func (s *ReadScope) Close() error {
	if s.anchor == nil {
		return nil
	}
	var result error
	for i := len(s.owned) - 1; i >= 0; i-- {
		result = errors.Join(result, closeReadDirectory(s.owned[i]))
	}
	result = errors.Join(result, s.anchor.Close())
	s.anchor, s.directory, s.parents, s.owned = nil, nil, nil, nil
	return result
}

func (s *ReadScope) Admit(relative string) (*ReadHandle, error) {
	if s.anchor == nil {
		return nil, ErrUnsafePath
	}
	if err := s.anchor.Validate(); err != nil {
		return nil, err
	}
	clean := "."
	if relative != "" && relative != "." {
		var err error
		clean, err = cleanRelative(relative)
		if err != nil {
			return nil, err
		}
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	parent, parentPath := s.directory, "."
	for _, name := range parts[:len(parts)-1] {
		if err := s.validateParent(parent); err != nil {
			return nil, err
		}
		path := filepath.ToSlash(filepath.Join(parentPath, name))
		child := s.parents[path]
		if child == nil {
			info, err := parent.meta.Lstat(name)
			if err != nil {
				if changed := s.validateParent(parent); changed != nil {
					return nil, changed
				}
				return nil, err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, ErrUnsafePath
			}
			child, err = openReadDirectory(parent, name, info)
			if err != nil {
				return nil, err
			}
			s.parents[path], s.owned = child, append(s.owned, child)
		}
		parent, parentPath = child, path
	}
	if err := s.validateParent(parent); err != nil {
		return nil, err
	}
	name := parts[len(parts)-1]
	info, err := parent.meta.Lstat(name)
	if err != nil {
		if changed := s.validateParent(parent); changed != nil {
			return nil, changed
		}
		return nil, err
	}
	if !safeReadKind(info) {
		return nil, ErrUnsafePath
	}
	h := s.anchor.retainEntry(parent, name, filepath.ToSlash(filepath.Join(s.anchor.relative, clean)), info)
	if err := h.Validate(); err != nil {
		_ = h.Close()
		return nil, err
	}
	return h, nil
}

func (s *ReadScope) validateParent(parent *readDirectory) error {
	if err := s.anchor.Validate(); err != nil {
		return err
	}
	for current := parent; current != nil; current = current.parent {
		if err := checkReadDirectory(current); err != nil {
			return err
		}
	}
	return s.anchor.ctx.Err()
}

func (h *ReadHandle) retainEntry(parent *readDirectory, name, relative string, info os.FileInfo) *ReadHandle {
	dirs := []*readDirectory{}
	for current := parent; current != nil; current = current.parent {
		current.refs.Add(1)
		dirs = append(dirs, current)
	}
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return &ReadHandle{ctx: h.ctx, boundary: h.boundary, dirs: dirs, parent: parent,
		name: name, relative: relative, Info: info}
}

// ValidateAncestors deliberately does not reopen/revalidate the final name.
// An already opened regular file may legally be renamed and still supplies its
// original bytes; substitution of any original containing directory is rejected.
func (h *ReadHandle) ValidateAncestors() error { return h.validateDirectories() }

func (h *ReadHandle) RelativePath() string { return h.relative }

type ReadWalkVisitor struct {
	Select func(ReadEntry) bool
	// Visit takes ownership on success. On error the walker closes the handle.
	Visit      func(*ReadHandle) error
	BeforeScan func(relative string)
}

// WalkFiles admits selected regular files while their original recursive parent
// stack is held. Stable links/special entries are skipped; changes after native
// metadata admission are errors. Hidden entries are intentionally included.
func (h *ReadHandle) WalkFiles(visitor ReadWalkVisitor) error {
	if err := h.Validate(); err != nil {
		return err
	}
	if !h.Info.IsDir() || visitor.Select == nil || visitor.Visit == nil {
		return ErrNotDirectory
	}
	directory, err := openReadDirectory(h.parent, h.name, h.Info)
	if err != nil {
		return err
	}
	defer closeReadDirectory(directory)
	options := ReadListOptions{Depth: -1, SkipUnsafe: true}
	if err := h.scan(directory, h.relative, -1, options, nil, &visitor); err != nil {
		return err
	}
	return h.Validate()
}
