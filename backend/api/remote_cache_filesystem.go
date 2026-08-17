package api

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"openreader/backend/services/webdavfs"
)

const defaultRemoteChapterCacheReadLimit = 16 * 1024 * 1024

var errRemoteChapterCacheTooLarge = errors.New("chapter cache exceeds configured size limit")

type remoteCacheFile struct {
	relative string
	fullPath string
	file     *os.File
	info     os.FileInfo
}

func (s *Server) remoteCacheStorage() (*webdavfs.Service, error) {
	if strings.TrimSpace(s.cfg.CacheDir) == "" {
		return nil, webdavfs.ErrUnsafePath
	}
	return webdavfs.New(s.cfg.CacheDir)
}

func (s *Server) remoteCacheRelativePath(cachePath string) (string, error) {
	cachePath = strings.TrimSpace(cachePath)
	if cachePath == "" || strings.ContainsRune(cachePath, '\x00') {
		return "", webdavfs.ErrUnsafePath
	}
	storage, err := s.remoteCacheStorage()
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(cachePath) {
		absolutePath, err := filepath.Abs(filepath.Clean(cachePath))
		if err != nil {
			return "", webdavfs.ErrUnsafePath
		}
		relative, err := filepath.Rel(storage.Root(), absolutePath)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return "", webdavfs.ErrUnsafePath
		}
		cachePath = relative
	}
	fullPath, relative, err := storage.Resolve(filepath.ToSlash(cachePath))
	if err != nil {
		return "", err
	}
	if relative == "" || fullPath == storage.Root() {
		return "", webdavfs.ErrUnsafePath
	}
	return relative, nil
}

func (s *Server) openRemoteCacheFile(cachePath string) (remoteCacheFile, error) {
	storage, err := s.remoteCacheStorage()
	if err != nil {
		return remoteCacheFile{}, err
	}
	relative, err := s.remoteCacheRelativePath(cachePath)
	if err != nil {
		return remoteCacheFile{}, err
	}
	fullPath, _, err := storage.Resolve(relative)
	if err != nil {
		return remoteCacheFile{}, err
	}
	file, info, err := storage.Open(relative)
	if err != nil {
		if errors.Is(err, webdavfs.ErrNotFound) {
			return remoteCacheFile{}, os.ErrNotExist
		}
		return remoteCacheFile{}, err
	}
	return remoteCacheFile{relative: relative, fullPath: fullPath, file: file, info: info}, nil
}

func (s *Server) remoteChapterCacheReadLimit() int64 {
	if s.cfg.MaxSourceResponseBytes > 0 {
		return s.cfg.MaxSourceResponseBytes
	}
	return defaultRemoteChapterCacheReadLimit
}

func (s *Server) readRemoteChapterCache(cachePath string) ([]byte, string, error) {
	opened, err := s.openRemoteCacheFile(cachePath)
	if err != nil {
		return nil, "", err
	}
	defer opened.file.Close()
	limit := s.remoteChapterCacheReadLimit()
	if opened.info.Size() > limit {
		return nil, "", errRemoteChapterCacheTooLarge
	}
	content, err := io.ReadAll(io.LimitReader(opened.file, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(content)) > limit {
		return nil, "", errRemoteChapterCacheTooLarge
	}
	return content, opened.fullPath, nil
}

func (s *Server) removeRemoteCacheFile(storage *webdavfs.Service, relative string) (bool, int64) {
	info, err := storage.RemoveRegular(relative)
	if err != nil {
		return false, 0
	}
	return true, info.Size()
}
