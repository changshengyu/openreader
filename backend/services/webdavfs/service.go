package webdavfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"openreader/backend/services/rootedfs"
)

var (
	ErrUnsafePath         = errors.New("unsafe WebDAV path")
	ErrNotFound           = errors.New("WebDAV path not found")
	ErrConflict           = errors.New("WebDAV path conflict")
	ErrPrecondition       = errors.New("WebDAV precondition failed")
	ErrIsDirectory        = errors.New("WebDAV path is a directory")
	ErrNotDirectory       = errors.New("WebDAV parent is not a directory")
	ErrTooLarge           = errors.New("WebDAV upload exceeds size limit")
	ErrCopyCleanupPending = errors.New("WebDAV copy committed with cleanup pending")
	ErrMoveCleanupPending = errors.New("WebDAV move committed with cleanup pending")
)

const maxImportPathBytes = 4096

// Tests use this hook to replace a target ancestor after lexical validation.
// Package tests are not parallel while the hook is installed.
var beforeRemoveTestHook func(root, relative string)

// A nonparallel test seam after identity admission, before validation/read.
// Nil in production; fixtures cannot replace the fd-relative implementation.
var afterReadAdmissionTestHook func(operation, root, relative string)

type Service struct {
	boundary string
	root     string
}

type Resource struct {
	RelativePath string
	Info         os.FileInfo
}

func New(root string) (*Service, error) {
	return NewScoped(root, root)
}

// NormalizeImportPath applies the stricter JSON import boundary without
// changing the raw DAV protocol's established path contract.
func NormalizeImportPath(value string) (string, error) {
	if !utf8.ValidString(value) || len(value) > maxImportPathBytes {
		return "", ErrUnsafePath
	}
	return cleanRelative(value)
}

// NewScoped creates a service rooted at root while also checking every
// existing component between the trusted WebDAV boundary and that root. This
// matters for private roots such as webdav/users/<name>: checking only the
// final user root would otherwise miss a symlink substituted at users.
func NewScoped(boundary, root string) (*Service, error) {
	if strings.TrimSpace(boundary) == "" || strings.TrimSpace(root) == "" {
		return nil, ErrUnsafePath
	}
	absoluteBoundary, err := filepath.Abs(filepath.Clean(boundary))
	if err != nil {
		return nil, ErrUnsafePath
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil || !within(absoluteBoundary, absoluteRoot) {
		return nil, ErrUnsafePath
	}
	service := &Service{boundary: absoluteBoundary, root: absoluteRoot}
	if err := service.rejectSymlinks(absoluteRoot); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Root() string {
	return s.root
}

func (s *Service) EnsureRoot() error {
	return s.EnsureRootContext(context.Background())
}

func (s *Service) EnsureRootContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.rejectSymlinks(s.root); err != nil {
		return err
	}
	if info, err := os.Lstat(s.root); err == nil && !info.IsDir() {
		return ErrUnsafePath
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	relative, err := filepath.Rel(s.boundary, s.root)
	if err != nil {
		return ErrUnsafePath
	}
	return directoryCreationError(rootedfs.CreateDirectories(ctx, s.boundary, relative))
}

func (s *Service) Resolve(rawPath string) (string, string, error) {
	relative, err := cleanRelative(rawPath)
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(s.root, filepath.FromSlash(relative))
	target, err = filepath.Abs(target)
	if err != nil || !within(s.root, target) {
		return "", "", ErrUnsafePath
	}
	if err := s.rejectSymlinks(target); err != nil {
		return "", "", err
	}
	return target, relative, nil
}

func (s *Service) Stat(rawPath string) (Resource, error) {
	return s.StatContext(context.Background(), rawPath)
}

func (s *Service) List(rawPath string, depth int) ([]Resource, error) {
	return s.ListContext(context.Background(), rawPath, depth)
}

func (s *Service) Open(rawPath string) (*os.File, os.FileInfo, error) {
	return s.OpenContext(context.Background(), rawPath)
}

func (s *Service) Put(ctx context.Context, rawPath string, source io.Reader, maxBytes int64) error {
	target, relative, err := s.Resolve(rawPath)
	if err != nil {
		return err
	}
	if relative == "" {
		return ErrUnsafePath
	}
	boundaryRelative, err := filepath.Rel(s.boundary, target)
	if err != nil {
		return ErrUnsafePath
	}
	err = rootedfs.ReplaceRegular(ctx, s.boundary, boundaryRelative, func(staged *os.File) error {
		reader := source
		if maxBytes > 0 {
			reader = io.LimitReader(source, maxBytes+1)
		}
		written, err := copyContext(ctx, staged, reader)
		if err != nil {
			return err
		}
		if maxBytes > 0 && written > maxBytes {
			return ErrTooLarge
		}
		return nil
	})
	switch {
	case errors.Is(err, rootedfs.ErrUnsafePath):
		return ErrUnsafePath
	case errors.Is(err, rootedfs.ErrIsDirectory):
		return ErrIsDirectory
	case errors.Is(err, rootedfs.ErrNotDirectory):
		return ErrNotDirectory
	case errors.Is(err, os.ErrNotExist):
		return ErrConflict
	default:
		return err
	}
}

func (s *Service) Mkdir(rawPath string) error {
	return s.MkdirContext(context.Background(), rawPath)
}

func (s *Service) MkdirContext(ctx context.Context, rawPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, relative, err := s.Resolve(rawPath)
	if err != nil {
		return err
	}
	if relative == "" {
		return ErrUnsafePath
	}
	boundaryRelative, err := filepath.Rel(s.boundary, target)
	if err != nil {
		return ErrUnsafePath
	}
	return directoryCreationError(rootedfs.CreateDirectories(ctx, s.boundary, boundaryRelative))
}

func directoryCreationError(err error) error {
	switch {
	case errors.Is(err, rootedfs.ErrUnsafePath):
		return ErrUnsafePath
	case errors.Is(err, rootedfs.ErrNotDirectory):
		return ErrNotDirectory
	case errors.Is(err, rootedfs.ErrDirectoryConflict):
		return ErrConflict
	default:
		return err
	}
}

func (s *Service) Remove(rawPath string) error {
	target, relative, err := s.Resolve(rawPath)
	if err != nil {
		return err
	}
	if relative == "" {
		return ErrUnsafePath
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return ErrUnsafePath
	}
	if beforeRemoveTestHook != nil {
		beforeRemoveTestHook(s.root, relative)
	}
	boundaryRelative, err := filepath.Rel(s.boundary, target)
	if err != nil || boundaryRelative == "." || strings.HasPrefix(boundaryRelative, ".."+string(os.PathSeparator)) {
		return ErrUnsafePath
	}
	if err := rootedfs.RemovePath(s.boundary, boundaryRelative); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return ErrNotFound
		case errors.Is(err, rootedfs.ErrUnsafePath):
			return ErrUnsafePath
		default:
			return err
		}
	}
	return nil
}

func (s *Service) RemoveRegular(rawPath string) (os.FileInfo, error) {
	target, _, err := s.Resolve(rawPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, ErrUnsafePath
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	openedInfo, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil || !os.SameFile(info, openedInfo) {
		return nil, ErrUnsafePath
	}
	currentInfo, err := os.Lstat(target)
	if err != nil || !currentInfo.Mode().IsRegular() || !os.SameFile(openedInfo, currentInfo) {
		return nil, ErrUnsafePath
	}
	if err := os.Remove(target); err != nil {
		return nil, err
	}
	return openedInfo, nil
}

func (s *Service) Copy(ctx context.Context, sourceRaw, destinationRaw string, overwrite bool) error {
	source, destination, err := s.validTransfer(sourceRaw, destinationRaw, overwrite)
	if err != nil {
		return err
	}
	sourceRelative, err := filepath.Rel(s.boundary, source)
	if err != nil {
		return ErrUnsafePath
	}
	destinationRelative, err := filepath.Rel(s.boundary, destination)
	if err != nil {
		return ErrUnsafePath
	}
	err = rootedfs.CopyTree(ctx, s.boundary, sourceRelative, destinationRelative, overwrite)
	switch {
	case errors.Is(err, rootedfs.ErrUnsafePath):
		return ErrUnsafePath
	case errors.Is(err, rootedfs.ErrCopySourceMissing):
		return ErrPrecondition
	case errors.Is(err, rootedfs.ErrCopyParentMissing):
		return ErrConflict
	case errors.Is(err, rootedfs.ErrCopyDestinationExists):
		return ErrPrecondition
	case errors.Is(err, rootedfs.ErrCopyCleanupPending):
		return ErrCopyCleanupPending
	case errors.Is(err, rootedfs.ErrNotDirectory):
		return ErrNotDirectory
	default:
		return err
	}
}

func (s *Service) Move(sourceRaw, destinationRaw string, overwrite bool) error {
	return s.MoveContext(context.Background(), sourceRaw, destinationRaw, overwrite)
}

func (s *Service) MoveContext(ctx context.Context, sourceRaw, destinationRaw string, overwrite bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, destination, err := s.validTransfer(sourceRaw, destinationRaw, overwrite)
	if err != nil {
		return err
	}
	sourceRelative, err := filepath.Rel(s.boundary, source)
	if err != nil {
		return ErrUnsafePath
	}
	destinationRelative, err := filepath.Rel(s.boundary, destination)
	if err != nil {
		return ErrUnsafePath
	}
	err = rootedfs.MoveTree(ctx, s.boundary, sourceRelative, destinationRelative, overwrite)
	switch {
	case errors.Is(err, rootedfs.ErrUnsafePath):
		return ErrUnsafePath
	case errors.Is(err, rootedfs.ErrMoveSourceMissing), errors.Is(err, rootedfs.ErrMoveDestinationExists):
		return ErrPrecondition
	case errors.Is(err, rootedfs.ErrMoveParentMissing):
		return ErrConflict
	case errors.Is(err, rootedfs.ErrNotDirectory):
		return ErrNotDirectory
	case errors.Is(err, rootedfs.ErrMoveCleanupPending):
		return ErrMoveCleanupPending
	default:
		return err
	}
}

func (s *Service) validTransfer(sourceRaw, destinationRaw string, overwrite bool) (string, string, error) {
	source, sourceRelative, err := s.Resolve(sourceRaw)
	if err != nil {
		return "", "", err
	}
	destination, destinationRelative, err := s.Resolve(destinationRaw)
	if err != nil {
		return "", "", err
	}
	if sourceRelative == "" || destinationRelative == "" || source == destination || within(source, destination) || within(destination, source) {
		return "", "", ErrUnsafePath
	}
	sourceInfo, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", ErrPrecondition
	} else if err != nil {
		return "", "", err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || (!sourceInfo.IsDir() && !sourceInfo.Mode().IsRegular()) {
		return "", "", ErrUnsafePath
	}
	parentInfo, err := os.Lstat(filepath.Dir(destination))
	if errors.Is(err, os.ErrNotExist) {
		return "", "", ErrConflict
	}
	if err != nil {
		return "", "", err
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return "", "", ErrNotDirectory
	}
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return "", "", ErrUnsafePath
		}
		if !overwrite {
			return "", "", ErrPrecondition
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return source, destination, nil
}

func cleanRelative(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.ContainsRune(value, '\x00') {
		return "", ErrUnsafePath
	}
	value = strings.TrimPrefix(value, "/")
	if value == "" || value == "." {
		return "", nil
	}
	if hasWindowsVolumePrefix(value) {
		return "", ErrUnsafePath
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", ErrUnsafePath
		}
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", ErrUnsafePath
	}
	return cleaned, nil
}

func hasWindowsVolumePrefix(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	first := value[0]
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
}

func (s *Service) rejectSymlinks(target string) error {
	if !within(s.root, target) {
		return ErrUnsafePath
	}
	current := s.boundary
	paths := []string{current}
	if target != s.boundary {
		relative, err := filepath.Rel(s.boundary, target)
		if err != nil {
			return ErrUnsafePath
		}
		for _, part := range strings.Split(relative, string(os.PathSeparator)) {
			current = filepath.Join(current, part)
			paths = append(paths, current)
		}
	}
	for _, candidate := range paths {
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if errors.Is(err, syscall.ENOTDIR) {
			return ErrNotDirectory
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
	}
	return nil
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

func within(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	return target == root || strings.HasPrefix(target, root+string(os.PathSeparator))
}
