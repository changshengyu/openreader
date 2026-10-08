package webdavfs

import (
	"context"
	"path/filepath"

	"openreader/backend/services/rootedfs"
)

// ReadScope holds the original configured boundary and caller root for a whole
// source plan. Token-only callers must not create one.
type ReadScope struct {
	handle  *rootedfs.ReadScope
	service *Service
}

func (s *Service) AdmitReadScope(ctx context.Context) (*ReadScope, error) {
	root, err := s.AdmitRead(ctx, "")
	if err != nil {
		return nil, err
	}
	handle, err := rootedfs.NewReadScope(root.handle)
	if err != nil {
		_ = root.Close()
		return nil, readServiceError(err)
	}
	return &ReadScope{handle: handle, service: s}, nil
}

func (s *ReadScope) Close() error { return s.handle.Close() }

func (s *ReadScope) Admit(rawPath string) (*Reader, error) {
	relative, err := cleanRelative(rawPath)
	if err != nil {
		return nil, err
	}
	handle, err := s.handle.Admit(relative)
	if err != nil {
		return nil, readServiceError(err)
	}
	prefix, _ := filepath.Rel(s.service.boundary, s.service.root)
	base := filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(relative)))
	return &Reader{handle: handle, service: s.service, relative: relative, base: base,
		Resource: Resource{RelativePath: relative, Info: handle.Info}}, nil
}

func (r *Reader) ValidateAncestors() error { return readServiceError(r.handle.ValidateAncestors()) }

func (r *Reader) childRelative(base string) string {
	suffix, _ := filepath.Rel(filepath.FromSlash(r.base), filepath.FromSlash(base))
	relative := filepath.ToSlash(filepath.Join(filepath.FromSlash(r.relative), suffix))
	if relative == "." {
		return ""
	}
	return relative
}

func (r *Reader) WalkFiles(selectFile func(Resource) bool, visit func(*Reader) error, beforeScan func(string)) error {
	err := r.handle.WalkFiles(rootedfs.ReadWalkVisitor{
		Select: func(entry rootedfs.ReadEntry) bool {
			return selectFile(Resource{RelativePath: r.childRelative(entry.RelativePath), Info: entry.Info})
		},
		Visit: func(handle *rootedfs.ReadHandle) error {
			// The selected path was supplied by native metadata in the original
			// traversal; it is never reconstructed through Service.AdmitRead.
			relative := r.childRelative(handle.RelativePath())
			reader := &Reader{handle: handle, service: r.service, relative: relative,
				base:     handle.RelativePath(),
				Resource: Resource{RelativePath: relative, Info: handle.Info}}
			return visit(reader)
		},
		BeforeScan: func(base string) {
			if beforeScan != nil {
				beforeScan(r.childRelative(base))
			}
		},
	})
	return readServiceError(err)
}
