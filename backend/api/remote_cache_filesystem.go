package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"openreader/backend/engine"
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

type stagedChapterCache struct {
	storage     *webdavfs.Service
	relative    string
	staged      string
	backup      string
	hadPrevious bool
	published   bool
}

func (s *Server) stageRemoteChapterCache(
	ctx context.Context,
	bookURL string,
	chapterURL string,
	content string,
) (*stagedChapterCache, error) {
	storage, err := s.remoteCacheStorage()
	if err != nil {
		return nil, err
	}
	if err := storage.EnsureRoot(); err != nil {
		return nil, err
	}
	relative := filepath.ToSlash(engine.ChapterCachePath(bookURL, chapterURL))
	if err := storage.Mkdir(filepath.ToSlash(filepath.Dir(relative))); err != nil {
		return nil, err
	}
	return stageChapterCacheFile(ctx, storage, relative, content)
}

func (s *Server) stageLocalChapterCache(
	ctx context.Context,
	archive *localBookArchive,
	bookURL string,
	chapterURL string,
	content string,
) (*stagedChapterCache, error) {
	if archive == nil || archive.storage == nil || !archive.current() {
		return nil, errReaderChapterContentStale
	}
	relative := filepath.ToSlash(filepath.Join("content", engine.ChapterCachePath(bookURL, chapterURL)))
	if err := archive.storage.Mkdir(filepath.ToSlash(filepath.Dir(relative))); err != nil {
		return nil, err
	}
	if !archive.current() {
		return nil, errReaderChapterContentStale
	}
	staged, err := stageChapterCacheFile(ctx, archive.storage, relative, content)
	if err != nil {
		return nil, err
	}
	if !archive.current() {
		staged.rollback()
		return nil, errReaderChapterContentStale
	}
	return staged, nil
}

func stageChapterCacheFile(
	ctx context.Context,
	storage *webdavfs.Service,
	relative string,
	content string,
) (*stagedChapterCache, error) {
	staged, err := remoteCacheSidecarPath(relative, "stage")
	if err != nil {
		return nil, err
	}
	backup, err := remoteCacheSidecarPath(relative, "backup")
	if err != nil {
		return nil, err
	}
	if err := storage.Put(ctx, staged, strings.NewReader(content), int64(len(content))); err != nil {
		return nil, err
	}
	return &stagedChapterCache{
		storage:  storage,
		relative: relative,
		staged:   staged,
		backup:   backup,
	}, nil
}

func remoteCacheSidecarPath(relative string, kind string) (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return relative + "." + kind + "-" + hex.EncodeToString(token[:]), nil
}

func (staged *stagedChapterCache) publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := staged.storage.Stat(staged.relative); err == nil {
		if err := staged.storage.Move(staged.relative, staged.backup, false); err != nil {
			return err
		}
		staged.hadPrevious = true
	} else if !errors.Is(err, webdavfs.ErrNotFound) {
		return err
	}
	if err := ctx.Err(); err != nil {
		staged.restorePrevious()
		return err
	}
	if err := staged.storage.Move(staged.staged, staged.relative, false); err != nil {
		staged.restorePrevious()
		return err
	}
	staged.staged = ""
	staged.published = true
	return nil
}

func (staged *stagedChapterCache) rollback() {
	if staged == nil {
		return
	}
	if staged.published {
		_, _ = staged.storage.RemoveRegular(staged.relative)
		staged.restorePrevious()
		staged.published = false
	}
	if staged.staged != "" {
		_, _ = staged.storage.RemoveRegular(staged.staged)
		staged.staged = ""
	}
}

func (staged *stagedChapterCache) finalize() {
	if staged == nil {
		return
	}
	if staged.hadPrevious {
		_, _ = staged.storage.RemoveRegular(staged.backup)
	}
	staged.hadPrevious = false
	staged.published = false
}

func (staged *stagedChapterCache) restorePrevious() {
	if !staged.hadPrevious {
		return
	}
	_ = staged.storage.Move(staged.backup, staged.relative, false)
	staged.hadPrevious = false
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
