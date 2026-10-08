package webdavfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"openreader/backend/services/rootedfs"
)

// Reader is one admitted read operation, not a second path lookup. Close
// releases ancestors; a returned opened regular file has its own lifetime.
type Reader struct {
	handle   *rootedfs.ReadHandle
	service  *Service
	relative string
	base     string
	Resource Resource
}

func (s *Service) AdmitRead(ctx context.Context, rawPath string) (*Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relative, err := cleanRelative(rawPath)
	if err != nil {
		return nil, err
	}
	prefix, err := filepath.Rel(s.boundary, s.root)
	if err != nil || !within(s.boundary, s.root) {
		return nil, ErrUnsafePath
	}
	base := filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(relative)))
	handle, err := rootedfs.AdmitRead(ctx, s.boundary, base)
	if err != nil {
		return nil, readServiceError(err)
	}
	return &Reader{handle: handle, service: s, relative: relative, base: base,
		Resource: Resource{RelativePath: relative, Info: handle.Info}}, nil
}

func (r *Reader) Close() error { return r.handle.Close() }

func (r *Reader) Validate() error { return readServiceError(r.handle.Validate()) }

func (r *Reader) hook(operation string) {
	if afterReadAdmissionTestHook != nil {
		afterReadAdmissionTestHook(operation, r.service.boundary, r.relative)
	}
}

func (r *Reader) Open() (*os.File, os.FileInfo, error) {
	r.hook("open")
	file, info, err := r.handle.OpenRegular()
	if errors.Is(err, rootedfs.ErrIsDirectory) {
		return nil, info, ErrIsDirectory
	}
	return file, info, readServiceError(err)
}

func (r *Reader) List(depth int) ([]Resource, error) {
	if depth > 0 {
		depth = 1
	} else {
		depth = 0
	}
	return r.list(rootedfs.ReadListOptions{Depth: depth})
}

func (r *Reader) ListLocal(recursive bool) ([]Resource, error) {
	depth := 1
	if recursive {
		depth = -1
	}
	return r.list(rootedfs.ReadListOptions{Depth: depth, HideDot: true, SkipUnsafe: true, SkipReadError: true})
}

func (r *Reader) list(options rootedfs.ReadListOptions) ([]Resource, error) {
	r.hook("list")
	entries, err := r.handle.List(options)
	if err != nil {
		return nil, readServiceError(err)
	}
	resources := make([]Resource, 0, len(entries))
	for _, entry := range entries {
		suffix, err := filepath.Rel(filepath.FromSlash(r.base), filepath.FromSlash(entry.RelativePath))
		if err != nil {
			return nil, ErrUnsafePath
		}
		relative := filepath.ToSlash(filepath.Join(filepath.FromSlash(r.relative), suffix))
		if relative == "." {
			relative = ""
		}
		resources = append(resources, Resource{RelativePath: relative, Info: entry.Info})
	}
	return resources, nil
}

func (s *Service) StatContext(ctx context.Context, rawPath string) (Resource, error) {
	reader, err := s.AdmitRead(ctx, rawPath)
	if err != nil {
		return Resource{}, err
	}
	defer reader.Close()
	reader.hook("stat")
	if err := reader.Validate(); err != nil {
		return Resource{}, err
	}
	return reader.Resource, nil
}

func (s *Service) ListContext(ctx context.Context, rawPath string, depth int) ([]Resource, error) {
	reader, err := s.AdmitRead(ctx, rawPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.List(depth)
}

func (s *Service) OpenContext(ctx context.Context, rawPath string) (*os.File, os.FileInfo, error) {
	reader, err := s.AdmitRead(ctx, rawPath)
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()
	return reader.Open()
}

func readServiceError(err error) error {
	switch {
	case errors.Is(err, rootedfs.ErrUnsafePath):
		return ErrUnsafePath
	case errors.Is(err, rootedfs.ErrNotDirectory):
		return ErrNotDirectory
	case errors.Is(err, os.ErrNotExist):
		return ErrNotFound
	default:
		return err
	}
}
