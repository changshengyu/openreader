package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/webdavfs"
)

// localRefreshStage keeps regenerated local-book artifacts off the active
// reader paths until the replacement catalogue has committed. Every refresh
// receives a new content generation, so a failed promotion can never make a
// replacement chapter row read a previous generation's cache by accident.
type localRefreshStage struct {
	stageDir        string
	stageInfo       os.FileInfo
	stagedContent   string
	finalContent    string
	finalContentRel string
	cachePathPrefix string
	archiveRoot     string
	archive         *localBookArchive
	storage         *webdavfs.Service
	usesArchive     bool
	promotions      []localRefreshPromotion
}

type localRefreshPromotion struct {
	stagedPath   string
	finalPath    string
	relativePath string
}

// localRefreshStageTestHook is deliberately package-private and used only by
// the API contract test to force an inactive staging failure. API tests are
// not parallel, so it cannot affect a concurrent production request.
var localRefreshStageTestHook func(string) error

func (s *Server) stageLocalRefresh(book models.Book, archive *localBookArchive, parsed []engine.TXTChapter, bookURL string) (*localRefreshStage, []models.Chapter, error) {
	return s.stageLocalRefreshContext(context.Background(), book, archive, parsed, bookURL)
}

func (s *Server) stageLocalRefreshContext(ctx context.Context, book models.Book, archive *localBookArchive, parsed []engine.TXTChapter, bookURL string) (*localRefreshStage, []models.Chapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	usesArchive := archive != nil && strings.TrimSpace(archive.root) != ""
	if usesArchive && !archive.current() {
		return nil, nil, fmt.Errorf("unsafe local refresh archive root")
	}
	stageParent := s.cfg.CacheDir
	if usesArchive {
		stageParent = archive.root
		if _, _, err := archive.storage.Resolve("content"); err != nil {
			return nil, nil, err
		}
	}
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	stageDir, err := os.MkdirTemp(stageParent, ".refresh-")
	if err != nil {
		return nil, nil, err
	}
	generation := strings.TrimPrefix(filepath.Base(stageDir), ".refresh-")
	if generation == "" {
		_ = os.RemoveAll(stageDir)
		return nil, nil, fmt.Errorf("create local refresh generation")
	}
	stageInfo, err := os.Lstat(stageDir)
	if err != nil || !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 {
		_ = os.RemoveAll(stageDir)
		return nil, nil, fmt.Errorf("verify local refresh generation")
	}

	stage := &localRefreshStage{
		stageDir:      stageDir,
		stageInfo:     stageInfo,
		stagedContent: filepath.Join(stageDir, "content"),
		usesArchive:   usesArchive,
	}
	if usesArchive {
		stage.archiveRoot = archive.root
		stage.archive = archive
		stage.storage = archive.storage
		stage.finalContentRel = filepath.Join("content", generation)
		stage.finalContent, _, err = archive.storage.Resolve(filepath.ToSlash(stage.finalContentRel))
		if err != nil {
			stage.cleanup()
			return nil, nil, err
		}
		stage.cachePathPrefix = filepath.Join("content", generation)
	} else {
		stage.finalContent = filepath.Join(s.cfg.CacheDir, "local-refresh", fmt.Sprintf("book-%d", book.ID), generation)
		stage.cachePathPrefix = filepath.Join("local-refresh", fmt.Sprintf("book-%d", book.ID), generation)
	}

	chapters := make([]models.Chapter, 0, len(parsed))
	for index, parsedChapter := range parsed {
		if err := ctx.Err(); err != nil {
			stage.cleanup()
			return nil, nil, err
		}
		if stage.archive != nil && !stage.archive.current() {
			stage.cleanup()
			return nil, nil, fmt.Errorf("unsafe local refresh archive root")
		}
		title := strings.TrimSpace(parsedChapter.Title)
		if title == "" {
			title = fmt.Sprintf("第 %d 章", index+1)
		}
		chapterURL := fmt.Sprintf("%s/chapter_%d", bookURL, index)
		cachePath, err := engine.WriteChapterCacheContext(ctx, stage.stagedContent, bookURL, chapterURL, parsedChapter.Content)
		if err != nil {
			stage.cleanup()
			return nil, nil, err
		}
		if stage.archive != nil && !stage.archive.current() {
			stage.cleanup()
			return nil, nil, fmt.Errorf("unsafe local refresh archive root")
		}
		chapters = append(chapters, models.Chapter{
			BookID:              book.ID,
			Index:               index,
			Title:               title,
			URL:                 chapterURL,
			CachePath:           filepath.Join(stage.cachePathPrefix, cachePath),
			ResourcePath:        parsedChapter.ResourcePath,
			ResourceFragment:    parsedChapter.ResourceFragment,
			ResourceEndFragment: parsedChapter.ResourceEndFragment,
		})
	}
	if localRefreshStageTestHook != nil {
		if err := localRefreshStageTestHook(stageDir); err != nil {
			stage.cleanup()
			return nil, nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		stage.cleanup()
		return nil, nil, err
	}
	return stage, chapters, nil
}

func (stage *localRefreshStage) stageArchiveMetadata(archive engine.ArchivedBook, chapters []engine.ArchivedChapter, source engine.ArchivedBookSource) error {
	return stage.stageArchiveMetadataContext(context.Background(), archive, chapters, source)
}

func (stage *localRefreshStage) stageArchiveMetadataContext(ctx context.Context, archive engine.ArchivedBook, chapters []engine.ArchivedChapter, source engine.ArchivedBookSource) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !stage.usesArchive {
		return nil
	}
	if stage.archive == nil || !stage.archive.current() {
		return fmt.Errorf("unsafe local refresh archive root")
	}
	if _, _, err := stage.storage.Resolve("content"); err != nil {
		return fmt.Errorf("unsafe local refresh content path")
	}
	if strings.TrimSpace(archive.TOCFile) != "" {
		if err := stage.stageJSONFileContext(ctx, archive.Directory, archive.TOCFile, chapters); err != nil {
			return err
		}
	}
	if strings.TrimSpace(archive.SourceFile) != "" {
		if err := stage.stageJSONFileContext(ctx, archive.Directory, archive.SourceFile, []engine.ArchivedBookSource{source}); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (stage *localRefreshStage) stageJSONFile(archiveDirectory, storedPath string, value any) error {
	return stage.stageJSONFileContext(context.Background(), archiveDirectory, storedPath, value)
}

func (stage *localRefreshStage) stageJSONFileContext(ctx context.Context, archiveDirectory, storedPath string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if stage.storage == nil || filepath.IsAbs(storedPath) || filepath.IsAbs(archiveDirectory) {
		return fmt.Errorf("unsafe local refresh metadata path")
	}
	relativePath, err := filepath.Rel(filepath.Clean(archiveDirectory), filepath.Clean(storedPath))
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("local refresh metadata path is outside archive")
	}
	finalPath, _, err := stage.storage.Resolve(filepath.ToSlash(relativePath))
	if err != nil {
		return fmt.Errorf("unsafe local refresh metadata path")
	}
	stagedPath := filepath.Join(stage.stageDir, "metadata", relativePath)
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(stagedPath, data, 0o644); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stage.promotions = append(stage.promotions, localRefreshPromotion{stagedPath: stagedPath, finalPath: finalPath, relativePath: relativePath})
	return nil
}

func (stage *localRefreshStage) promote() error {
	if stage.archive != nil && !stage.archive.current() {
		return fmt.Errorf("unsafe local refresh archive root")
	}
	if stage.storage != nil {
		if err := stage.storage.Mkdir("content"); err != nil {
			return err
		}
		finalContent, _, err := stage.storage.Resolve(filepath.ToSlash(stage.finalContentRel))
		if err != nil || filepath.Clean(finalContent) != filepath.Clean(stage.finalContent) {
			return fmt.Errorf("unsafe local refresh content path")
		}
		for index := range stage.promotions {
			promotion := &stage.promotions[index]
			parent := filepath.Dir(promotion.relativePath)
			if parent != "." {
				if err := stage.storage.Mkdir(filepath.ToSlash(parent)); err != nil {
					return err
				}
			}
			finalPath, _, err := stage.storage.Resolve(filepath.ToSlash(promotion.relativePath))
			if err != nil || filepath.Clean(finalPath) != filepath.Clean(promotion.finalPath) {
				return fmt.Errorf("unsafe local refresh metadata path")
			}
		}
	} else if err := os.MkdirAll(filepath.Dir(stage.finalContent), 0o755); err != nil {
		return err
	}
	if err := os.Rename(stage.stagedContent, stage.finalContent); err != nil {
		return err
	}
	for _, promotion := range stage.promotions {
		if stage.storage == nil {
			if err := os.MkdirAll(filepath.Dir(promotion.finalPath), 0o755); err != nil {
				return err
			}
		}
		if err := os.Rename(promotion.stagedPath, promotion.finalPath); err != nil {
			return err
		}
	}
	return nil
}

func (stage *localRefreshStage) cleanup() {
	if stage == nil || stage.stageDir == "" || stage.stageInfo == nil {
		return
	}
	current, err := os.Lstat(stage.stageDir)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(stage.stageInfo, current) {
		return
	}
	detached := stage.stageDir + ".cleanup"
	if err := os.Rename(stage.stageDir, detached); err != nil {
		return
	}
	detachedInfo, err := os.Lstat(detached)
	if err != nil || !detachedInfo.IsDir() || detachedInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(stage.stageInfo, detachedInfo) {
		if _, targetErr := os.Lstat(stage.stageDir); os.IsNotExist(targetErr) {
			_ = os.Rename(detached, stage.stageDir)
		}
		return
	}
	_ = os.RemoveAll(detached)
}

func (s *Server) pruneSupersededLocalDerivedContent(book models.Book, archive *localBookArchive, supersededCachePaths []string) {
	if archive == nil || !archive.current() || strings.TrimSpace(archive.root) == "" || len(supersededCachePaths) == 0 {
		return
	}
	var current []models.Chapter
	if err := s.db.Select("cache_path").Where("book_id = ? AND cache_path <> ''", book.ID).Find(&current).Error; err != nil {
		return
	}
	active := make(map[string]struct{}, len(current))
	for _, chapter := range current {
		active[chapter.CachePath] = struct{}{}
	}
	contentRoot := filepath.Join(archive.root, "content")
	for _, cachePath := range supersededCachePaths {
		if _, retained := active[cachePath]; retained || filepath.IsAbs(cachePath) {
			continue
		}
		candidate := filepath.Join(archive.root, cachePath)
		if _, ok := relativePathInside(contentRoot, candidate); !ok {
			continue
		}
		relative, ok := relativePathInside(archive.root, candidate)
		if !ok {
			continue
		}
		_, _ = archive.storage.RemoveRegular(filepath.ToSlash(relative))
	}
}
