package engine

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"openreader/backend/services/webdavfs"
)

func ChapterCachePath(bookURL, chapterURL string) string {
	sum := md5.Sum([]byte(bookURL + "\n" + chapterURL))
	encoded := hex.EncodeToString(sum[:])
	return filepath.Join(encoded[:2], encoded[2:])
}

func WriteChapterCache(cacheDir, bookURL, chapterURL, content string) (string, error) {
	return WriteChapterCacheContext(context.Background(), cacheDir, bookURL, chapterURL, content)
}

func WriteChapterCacheContext(ctx context.Context, cacheDir, bookURL, chapterURL, content string) (string, error) {
	relativePath := ChapterCachePath(bookURL, chapterURL)
	storage, err := webdavfs.New(cacheDir)
	if err != nil {
		return "", err
	}
	if err := storage.EnsureRoot(); err != nil {
		return "", err
	}
	if err := storage.Mkdir(filepath.ToSlash(filepath.Dir(relativePath))); err != nil {
		return "", err
	}
	if err := storage.Put(ctx, filepath.ToSlash(relativePath), strings.NewReader(content), int64(len(content))); err != nil {
		return "", err
	}
	return relativePath, nil
}

func ReadChapterCache(cacheDir, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) || filepath.VolumeName(relativePath) != "" {
		return "", webdavfs.ErrUnsafePath
	}
	storage, err := webdavfs.New(cacheDir)
	if err != nil {
		return "", err
	}
	file, _, err := storage.Open(filepath.ToSlash(relativePath))
	if err != nil {
		if errors.Is(err, webdavfs.ErrNotFound) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
