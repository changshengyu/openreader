package rootedfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

// Nonparallel tests inject work at actual directory/entry admission boundaries.
var beforeReadDirectoryScanTestHook func(relative string)
var afterReadEntryAdmissionTestHook func(relative string)

type ReadEntry struct {
	RelativePath string
	Info         os.FileInfo
}

type ReadListOptions struct {
	// Zero is metadata only, one is one level, negative is recursive.
	Depth         int
	HideDot       bool
	SkipUnsafe    bool
	SkipReadError bool
}

type readDirectory struct {
	name   string
	parent *readDirectory
	file   *os.File
	meta   *os.Root
	info   os.FileInfo
	refs   atomic.Int64
}

// ReadHandle owns the admitted boundary and every original target ancestor.
// Metadata uses native Root.Lstat against the original parent fd, including
// unreadable regular files. Only OpenRegular reads content. Callers must Close.
type ReadHandle struct {
	ctx      context.Context
	boundary string
	dirs     []*readDirectory
	parent   *readDirectory
	name     string
	relative string
	Info     os.FileInfo
}

func AdmitRead(ctx context.Context, boundary, relative string) (*ReadHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(boundary) == "" {
		return nil, ErrUnsafePath
	}
	clean := "."
	if relative != "." {
		var err error
		clean, err = cleanRelative(relative)
		if err != nil {
			return nil, err
		}
	}
	path, err := filepath.Abs(filepath.Clean(boundary))
	if err != nil {
		return nil, ErrUnsafePath
	}
	expected, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !expected.IsDir() || expected.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafePath
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, readOpenError(err)
	}
	file := os.NewFile(uintptr(fd), path)
	root, err := bindReadDirectory(file, expected)
	if err != nil {
		return nil, err
	}
	h := &ReadHandle{ctx: ctx, boundary: path, dirs: []*readDirectory{root}, relative: filepath.ToSlash(clean)}
	complete := false
	defer func() {
		if !complete {
			_ = h.Close()
		}
	}()
	parent := root
	parts := strings.Split(filepath.ToSlash(clean), "/")
	for _, part := range parts[:len(parts)-1] {
		if err := h.validateDirectories(); err != nil {
			return nil, err
		}
		info, err := parent.meta.Lstat(part)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrUnsafePath
		}
		if !info.IsDir() {
			return nil, ErrNotDirectory
		}
		child, err := openReadDirectory(parent, part, info)
		if err != nil {
			return nil, err
		}
		h.dirs = append(h.dirs, child)
		parent = child
	}
	h.parent, h.name = parent, parts[len(parts)-1]
	h.Info, err = parent.meta.Lstat(h.name)
	if err != nil {
		return nil, err
	}
	if !safeReadKind(h.Info) {
		return nil, ErrUnsafePath
	}
	if err := h.Validate(); err != nil {
		return nil, err
	}
	complete = true
	return h, nil
}

func (h *ReadHandle) Close() error {
	var result error
	for index := len(h.dirs) - 1; index >= 0; index-- {
		result = errors.Join(result, closeReadDirectory(h.dirs[index]))
	}
	h.dirs = nil
	return result
}

func (h *ReadHandle) validateDirectories() error {
	if err := h.ctx.Err(); err != nil {
		return err
	}
	if len(h.dirs) == 0 {
		return ErrUnsafePath
	}
	current, err := os.Lstat(h.boundary)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(h.dirs[0].info, current) {
		return ErrUnsafePath
	}
	for _, directory := range h.dirs[1:] {
		if err := checkReadDirectory(directory); err != nil {
			return err
		}
	}
	return h.ctx.Err()
}

func (h *ReadHandle) Validate() error {
	if err := h.validateDirectories(); err != nil {
		return err
	}
	return checkReadEntry(h.parent, h.name, h.Info)
}

func (h *ReadHandle) OpenRegular() (*os.File, os.FileInfo, error) {
	if err := h.Validate(); err != nil {
		return nil, nil, err
	}
	if h.Info.IsDir() {
		return nil, h.Info, ErrIsDirectory
	}
	fd, err := unix.Openat(int(h.parent.file.Fd()), h.name,
		unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if changed := h.Validate(); changed != nil {
			return nil, nil, changed
		}
		return nil, nil, readOpenError(err)
	}
	// Name is the original complete label used by adjacent Reader identity
	// checks. It does not resolve or reopen the path: fd came from Openat above.
	file := os.NewFile(uintptr(fd), filepath.Join(h.boundary, filepath.FromSlash(h.relative)))
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(h.Info, info) {
		_ = file.Close()
		return nil, nil, ErrUnsafePath
	}
	if err := h.Validate(); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

func (h *ReadHandle) List(options ReadListOptions) ([]ReadEntry, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}
	entries := []ReadEntry{{RelativePath: h.relative, Info: h.Info}}
	if options.Depth == 0 || !h.Info.IsDir() {
		return entries, nil
	}
	directory, err := openReadDirectory(h.parent, h.name, h.Info)
	if err != nil {
		if options.Depth < 0 && options.SkipReadError && errors.Is(err, os.ErrPermission) {
			return entries, h.Validate()
		}
		return nil, err
	}
	defer closeReadDirectory(directory)
	if err := h.scan(directory, h.relative, options.Depth, options, &entries, nil); err != nil {
		return nil, err
	}
	if err := h.Validate(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (h *ReadHandle) scan(directory *readDirectory, relative string, depth int, options ReadListOptions, result *[]ReadEntry, visitor *ReadWalkVisitor) error {
	if visitor != nil && visitor.BeforeScan != nil {
		visitor.BeforeScan(relative)
	}
	if beforeReadDirectoryScanTestHook != nil {
		beforeReadDirectoryScanTestHook(relative)
	}
	validate := func() error {
		if err := h.Validate(); err != nil {
			return err
		}
		for current := directory; current != nil; current = current.parent {
			if err := checkReadDirectory(current); err != nil {
				return err
			}
		}
		return h.ctx.Err()
	}
	if err := validate(); err != nil {
		return err
	}
	names := []string{}
	for {
		if err := validate(); err != nil {
			return err
		}
		batch, err := directory.file.ReadDir(128)
		for _, entry := range batch {
			names = append(names, entry.Name())
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if changed := validate(); changed != nil {
				return changed
			}
			if options.Depth < 0 && options.SkipReadError {
				return nil
			}
			return err
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if options.HideDot && strings.HasPrefix(name, ".") {
			continue
		}
		if err := validate(); err != nil {
			return err
		}
		info, err := directory.meta.Lstat(name)
		if err != nil {
			if options.SkipReadError {
				continue
			}
			return err
		}
		if !safeReadKind(info) {
			if options.SkipUnsafe {
				continue
			}
			return ErrUnsafePath
		}
		childRelative := filepath.ToSlash(filepath.Join(relative, name))
		if afterReadEntryAdmissionTestHook != nil {
			afterReadEntryAdmissionTestHook(childRelative)
		}
		if err := validate(); err != nil {
			return err
		}
		if err := checkReadEntry(directory, name, info); err != nil {
			return err
		}
		entry := ReadEntry{RelativePath: childRelative, Info: info}
		if result != nil {
			*result = append(*result, entry)
		}
		if visitor != nil && info.Mode().IsRegular() && visitor.Select(entry) {
			child := h.retainEntry(directory, name, childRelative, info)
			if err := child.Validate(); err != nil {
				_ = child.Close()
				return err
			}
			if err := visitor.Visit(child); err != nil {
				_ = child.Close()
				return err
			}
		}
		if info.IsDir() && (depth < 0 || depth > 1) {
			child, err := openReadDirectory(directory, name, info)
			if err != nil {
				if options.SkipReadError && errors.Is(err, os.ErrPermission) {
					if changed := checkReadEntry(directory, name, info); changed != nil {
						return changed
					}
					continue
				}
				return err
			}
			err = h.scan(child, childRelative, depth-1, options, result, visitor)
			_ = closeReadDirectory(child)
			if err != nil {
				return err
			}
			if err := checkReadEntry(directory, name, info); err != nil {
				return err
			}
		}
	}
	return validate()
}

func bindReadDirectory(file *os.File, expected os.FileInfo) (*readDirectory, error) {
	if file == nil {
		return nil, ErrUnsafePath
	}
	info, err := file.Stat()
	if err != nil || !info.IsDir() || !os.SameFile(expected, info) {
		_ = file.Close()
		return nil, ErrUnsafePath
	}
	// The fd is held by this request. This server-generated descriptor name
	// bootstraps native Lstat-at; it never resolves a client-selected path.
	meta, err := os.OpenRoot(fmt.Sprintf("/dev/fd/%d", file.Fd()))
	if err != nil {
		_ = file.Close()
		return nil, ErrUnsafePath
	}
	bound, err := meta.Lstat(".")
	if err != nil || !bound.IsDir() || !os.SameFile(info, bound) {
		_ = meta.Close()
		_ = file.Close()
		return nil, ErrUnsafePath
	}
	directory := &readDirectory{file: file, meta: meta, info: info}
	directory.refs.Store(1)
	return directory, nil
}

func openReadDirectory(parent *readDirectory, name string, expected os.FileInfo) (*readDirectory, error) {
	fd, err := unix.Openat(int(parent.file.Fd()), name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if changed := checkReadEntry(parent, name, expected); changed != nil {
			return nil, changed
		}
		return nil, readOpenError(err)
	}
	directory, err := bindReadDirectory(os.NewFile(uintptr(fd), name), expected)
	if err != nil {
		return nil, err
	}
	directory.parent, directory.name = parent, name
	return directory, nil
}

func closeReadDirectory(directory *readDirectory) error {
	if directory.refs.Add(-1) != 0 {
		return nil
	}
	return errors.Join(directory.meta.Close(), directory.file.Close())
}

func checkReadDirectory(directory *readDirectory) error {
	if directory.parent == nil {
		return nil
	}
	return checkReadEntry(directory.parent, directory.name, directory.info)
}

func checkReadEntry(parent *readDirectory, name string, expected os.FileInfo) error {
	current, err := parent.meta.Lstat(name)
	if err != nil || !safeReadKind(current) || current.IsDir() != expected.IsDir() || !os.SameFile(expected, current) {
		return ErrUnsafePath
	}
	return nil
}

func safeReadKind(info os.FileInfo) bool {
	return info != nil && info.Mode()&os.ModeSymlink == 0 && (info.IsDir() || info.Mode().IsRegular())
}

func readOpenError(err error) error {
	if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
		return ErrUnsafePath
	}
	return err
}
