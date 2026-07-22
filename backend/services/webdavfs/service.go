package webdavfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var (
	ErrUnsafePath   = errors.New("unsafe WebDAV path")
	ErrNotFound     = errors.New("WebDAV path not found")
	ErrConflict     = errors.New("WebDAV path conflict")
	ErrPrecondition = errors.New("WebDAV precondition failed")
	ErrIsDirectory  = errors.New("WebDAV path is a directory")
	ErrNotDirectory = errors.New("WebDAV parent is not a directory")
	ErrTooLarge     = errors.New("WebDAV upload exceeds size limit")
)

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
	if err := s.rejectSymlinks(s.root); err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("create WebDAV root: %w", err)
	}
	return s.rejectSymlinks(s.root)
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
	target, relative, err := s.Resolve(rawPath)
	if err != nil {
		return Resource{}, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return Resource{}, ErrNotFound
	}
	if err != nil {
		return Resource{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Resource{}, ErrUnsafePath
	}
	return Resource{RelativePath: relative, Info: info}, nil
}

func (s *Service) List(rawPath string, depth int) ([]Resource, error) {
	target, relative, err := s.Resolve(rawPath)
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
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafePath
	}
	resources := []Resource{{RelativePath: relative, Info: info}}
	if depth <= 0 || !info.IsDir() {
		return resources, nil
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		entryPath := filepath.Join(target, entry.Name())
		entryInfo, err := os.Lstat(entryPath)
		if err != nil {
			return nil, err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return nil, ErrUnsafePath
		}
		entryRelative := filepath.ToSlash(filepath.Join(filepath.FromSlash(relative), entry.Name()))
		resources = append(resources, Resource{RelativePath: entryRelative, Info: entryInfo})
	}
	return resources, nil
}

func (s *Service) Open(rawPath string) (*os.File, os.FileInfo, error) {
	target, _, err := s.Resolve(rawPath)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, ErrUnsafePath
	}
	if info.IsDir() {
		return nil, info, ErrIsDirectory
	}
	if !info.Mode().IsRegular() {
		return nil, nil, ErrUnsafePath
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, nil, ErrUnsafePath
	}
	return file, openedInfo, nil
}

func (s *Service) Put(ctx context.Context, rawPath string, source io.Reader, maxBytes int64) error {
	target, relative, err := s.Resolve(rawPath)
	if err != nil {
		return err
	}
	if relative == "" {
		return ErrUnsafePath
	}
	parent := filepath.Dir(target)
	parentInfo, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return ErrNotDirectory
	}
	if targetInfo, statErr := os.Lstat(target); statErr == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if targetInfo.IsDir() {
			return ErrIsDirectory
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	staged, err := os.CreateTemp(parent, ".webdav-upload-")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)

	reader := source
	if maxBytes > 0 {
		reader = io.LimitReader(source, maxBytes+1)
	}
	written, copyErr := copyContext(ctx, staged, reader)
	if closeErr := staged.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	if maxBytes > 0 && written > maxBytes {
		return ErrTooLarge
	}
	if err := os.Chmod(stagedPath, 0o644); err != nil {
		return err
	}
	return replaceWithStaged(target, stagedPath)
}

func (s *Service) Mkdir(rawPath string) error {
	target, relative, err := s.Resolve(rawPath)
	if err != nil {
		return err
	}
	if relative == "" {
		return ErrUnsafePath
	}
	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if info.IsDir() {
			return nil
		}
		return ErrConflict
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return s.rejectSymlinks(target)
}

func (s *Service) Remove(rawPath string) error {
	target, relative, err := s.Resolve(rawPath)
	if err != nil {
		return err
	}
	if relative == "" {
		return ErrUnsafePath
	}
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return os.RemoveAll(target)
}

func (s *Service) Copy(ctx context.Context, sourceRaw, destinationRaw string, overwrite bool) error {
	source, destination, err := s.validTransfer(sourceRaw, destinationRaw, overwrite)
	if err != nil {
		return err
	}
	stageDir, err := os.MkdirTemp(filepath.Dir(destination), ".webdav-copy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)
	staged := filepath.Join(stageDir, "new")
	if err := copyTree(ctx, source, staged); err != nil {
		return err
	}
	return installTransfer(destination, staged, overwrite)
}

func (s *Service) Move(sourceRaw, destinationRaw string, overwrite bool) error {
	source, destination, err := s.validTransfer(sourceRaw, destinationRaw, overwrite)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(destination); errors.Is(err, os.ErrNotExist) {
		return os.Rename(source, destination)
	} else if err != nil {
		return err
	}
	return replaceByRename(source, destination)
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
		if info.Mode()&os.ModeSymlink != 0 {
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

func copyTree(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return ErrUnsafePath
	}
	if info.IsDir() {
		if err := os.Mkdir(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTree(ctx, filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	openedInfo, err := input.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return ErrUnsafePath
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := copyContext(ctx, output, input)
	if closeErr := output.Close(); copyErr == nil {
		copyErr = closeErr
	}
	return copyErr
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

func replaceWithStaged(target, staged string) error {
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return os.Rename(staged, target)
	} else if err != nil {
		return err
	}
	return replaceByRename(staged, target)
}

func installTransfer(target, staged string, overwrite bool) error {
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return os.Rename(staged, target)
	} else if err != nil {
		return err
	}
	if !overwrite {
		return ErrPrecondition
	}
	return replaceByRename(staged, target)
}

func replaceByRename(source, target string) error {
	backupDir, err := os.MkdirTemp(filepath.Dir(target), ".webdav-replace-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(backupDir)
	backup := filepath.Join(backupDir, "old")
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	return nil
}

func within(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	return target == root || strings.HasPrefix(target, root+string(os.PathSeparator))
}
