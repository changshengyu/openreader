package api

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/webdavfs"
)

type localBookArchive struct {
	root    string
	info    os.FileInfo
	storage *webdavfs.Service
}

type openedLocalBookSource struct {
	file    *os.File
	info    os.FileInfo
	name    string
	archive *localBookArchive
}

// localBookArchiveOpenTestHook deterministically replaces a mounted entry
// after rooted same-file open. API tests are not parallel.
var localBookArchiveOpenTestHook func(string)

func (source *openedLocalBookSource) close() {
	if source != nil && source.file != nil {
		_ = source.file.Close()
	}
}

func (s *Server) resolveLocalBookArchive(book models.Book) (*localBookArchive, bool) {
	if book.SourceID != 0 || strings.TrimSpace(book.LibraryPath) == "" {
		return nil, false
	}
	var owner models.User
	if err := s.db.Select("username").First(&owner, book.UserID).Error; err != nil {
		return nil, false
	}
	candidate, ok := s.privateImportedBookDirectory(owner.Username, book.LibraryPath)
	if !ok {
		return nil, false
	}
	ownerRoot, ok := s.trustedLocalBookOwnerRoot(owner.Username)
	if !ok {
		return nil, false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil || !pathInside(ownerRoot, resolved) {
		return nil, false
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	storage, err := webdavfs.NewScoped(ownerRoot, resolved)
	if err != nil {
		return nil, false
	}
	return &localBookArchive{root: storage.Root(), info: info, storage: storage}, true
}

func (archive *localBookArchive) current() bool {
	if archive == nil || archive.info == nil {
		return false
	}
	current, err := os.Lstat(archive.root)
	return err == nil && current.IsDir() && current.Mode()&os.ModeSymlink == 0 && os.SameFile(archive.info, current)
}

func (s *Server) trustedLocalBookOwnerRoot(username string) (string, bool) {
	ownerName := engine.SafeFilename(username)
	if ownerName == "" {
		return "", false
	}
	configuredRoot := filepath.Join(s.cfg.LibraryDir, "data", ownerName)
	storage, err := webdavfs.NewScoped(s.cfg.LibraryDir, configuredRoot)
	if err != nil {
		return "", false
	}
	resource, err := storage.Stat("")
	if err != nil || !resource.Info.IsDir() {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(storage.Root())
	if err != nil {
		return "", false
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	return resolved, true
}

func (s *Server) openLocalBookSource(book models.Book) (*openedLocalBookSource, bool) {
	archive, ok := s.resolveLocalBookArchive(book)
	if !ok {
		return nil, false
	}
	for _, relative := range localBookSourceRelativeCandidates(book) {
		if source, ok := openLocalBookSourceCandidate(archive, relative); ok {
			return source, true
		}
	}
	entries, err := os.ReadDir(archive.root)
	if err != nil {
		return nil, false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if source, ok := openLocalBookSourceCandidate(archive, entry.Name()); ok {
			return source, true
		}
	}
	return nil, false
}

func openLocalBookSourceCandidate(archive *localBookArchive, relative string) (*openedLocalBookSource, bool) {
	relative = strings.TrimSpace(relative)
	if archive == nil || relative == "" || filepath.IsAbs(relative) || !isSupportedLocalBookFile(relative) {
		return nil, false
	}
	if !archive.current() {
		return nil, false
	}
	file, info, err := archive.storage.Open(filepath.ToSlash(relative))
	if err != nil {
		return nil, false
	}
	if !archive.current() {
		_ = file.Close()
		return nil, false
	}
	if localBookArchiveOpenTestHook != nil {
		localBookArchiveOpenTestHook(file.Name())
	}
	return &openedLocalBookSource{
		file: file, info: info, name: filepath.Base(file.Name()), archive: archive,
	}, true
}

func localBookSourceRelativeCandidates(book models.Book) []string {
	original := strings.TrimSpace(book.OriginalFile)
	libraryPath := strings.TrimSpace(book.LibraryPath)
	candidates := make([]string, 0, 5)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || filepath.IsAbs(value) {
			return
		}
		clean := filepath.Clean(value)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return
		}
		for _, existing := range candidates {
			if existing == clean {
				return
			}
		}
		candidates = append(candidates, clean)
	}

	if filepath.IsAbs(original) {
		if suffix, ok := suffixAfterPathSegment(original, libraryPath); ok {
			add(suffix)
		}
	} else if original != "" {
		if libraryPath != "" {
			if relative, err := filepath.Rel(filepath.Clean(libraryPath), filepath.Clean(original)); err == nil {
				add(relative)
			}
			if suffix, ok := suffixAfterPathSegment(original, libraryPath); ok {
				add(suffix)
			}
		}
		add(original)
	}
	add(filepath.Base(original))
	return candidates
}

func (s *Server) openLocalChapterCache(book models.Book, cachePath string) (*os.File, os.FileInfo, bool) {
	cachePath = strings.TrimSpace(cachePath)
	if cachePath == "" {
		return nil, nil, false
	}
	if archive, ok := s.resolveLocalBookArchive(book); ok {
		for _, relative := range localChapterCacheRelativeCandidates(book, cachePath) {
			if !archive.current() {
				return nil, nil, false
			}
			file, info, err := archive.storage.Open(filepath.ToSlash(relative))
			if err == nil {
				if !archive.current() {
					_ = file.Close()
					return nil, nil, false
				}
				if localBookArchiveOpenTestHook != nil {
					localBookArchiveOpenTestHook(file.Name())
				}
				return file, info, true
			}
		}
	} else if strings.TrimSpace(book.LibraryPath) != "" && book.Type != 1 {
		return nil, nil, false
	}
	if filepath.IsAbs(cachePath) {
		return nil, nil, false
	}
	cacheStorage, err := webdavfs.New(s.cfg.CacheDir)
	if err != nil {
		return nil, nil, false
	}
	file, info, err := cacheStorage.Open(filepath.ToSlash(cachePath))
	if err != nil {
		return nil, nil, false
	}
	if localBookArchiveOpenTestHook != nil {
		localBookArchiveOpenTestHook(file.Name())
	}
	return file, info, true
}

func localChapterCacheRelativeCandidates(book models.Book, cachePath string) []string {
	candidates := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || filepath.IsAbs(value) {
			return
		}
		clean := filepath.Clean(value)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return
		}
		for _, existing := range candidates {
			if existing == clean {
				return
			}
		}
		candidates = append(candidates, clean)
	}

	if filepath.IsAbs(cachePath) {
		if suffix, ok := suffixAfterPathSegment(cachePath, "content"); ok {
			add(filepath.Join("content", suffix))
		}
		if suffix, ok := suffixAfterPathSegment(cachePath, book.LibraryPath); ok {
			add(suffix)
		}
	} else {
		add(cachePath)
		add(filepath.Join("content", cachePath))
	}
	return candidates
}

func readOpenedFile(file *os.File) ([]byte, error) {
	if file == nil {
		return nil, os.ErrInvalid
	}
	return io.ReadAll(file)
}
