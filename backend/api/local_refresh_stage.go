package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"openreader/backend/engine"
	"openreader/backend/models"
)

// localRefreshStage keeps regenerated local-book artifacts off the active
// reader paths until the replacement catalogue has committed. Every refresh
// receives a new content generation, so a failed promotion can never make a
// replacement chapter row read a previous generation's cache by accident.
type localRefreshStage struct {
	stageDir        string
	stagedContent   string
	finalContent    string
	cachePathPrefix string
	archiveRoot     string
	usesArchive     bool
	promotions      []localRefreshPromotion
}

type localRefreshPromotion struct {
	stagedPath string
	finalPath  string
}

// localRefreshStageTestHook is deliberately package-private and used only by
// the API contract test to force an inactive staging failure. API tests are
// not parallel, so it cannot affect a concurrent production request.
var localRefreshStageTestHook func(string) error

func (s *Server) ownedLocalRefreshArchiveRoot(userID uint, book models.Book) (string, bool) {
	if strings.TrimSpace(book.LibraryPath) == "" {
		return "", false
	}
	var user models.User
	if err := s.db.Select("username").First(&user, userID).Error; err != nil {
		return "", false
	}
	return s.privateImportedBookDirectory(user.Username, book.LibraryPath)
}

func (s *Server) stageLocalRefresh(book models.Book, archiveRoot string, parsed []engine.TXTChapter, bookURL string) (*localRefreshStage, []models.Chapter, error) {
	usesArchive := strings.TrimSpace(archiveRoot) != ""
	stageParent := s.cfg.CacheDir
	if usesArchive {
		stageParent = archiveRoot
	}
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
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

	stage := &localRefreshStage{
		stageDir:      stageDir,
		stagedContent: filepath.Join(stageDir, "content"),
		archiveRoot:   archiveRoot,
		usesArchive:   usesArchive,
	}
	if usesArchive {
		stage.finalContent = filepath.Join(archiveRoot, "content", generation)
		stage.cachePathPrefix = filepath.Join("content", generation)
	} else {
		stage.finalContent = filepath.Join(s.cfg.CacheDir, "local-refresh", fmt.Sprintf("book-%d", book.ID), generation)
		stage.cachePathPrefix = filepath.Join("local-refresh", fmt.Sprintf("book-%d", book.ID), generation)
	}

	chapters := make([]models.Chapter, 0, len(parsed))
	for index, parsedChapter := range parsed {
		title := strings.TrimSpace(parsedChapter.Title)
		if title == "" {
			title = fmt.Sprintf("第 %d 章", index+1)
		}
		chapterURL := fmt.Sprintf("%s/chapter_%d", bookURL, index)
		cachePath, err := engine.WriteChapterCache(stage.stagedContent, bookURL, chapterURL, parsedChapter.Content)
		if err != nil {
			stage.cleanup()
			return nil, nil, err
		}
		chapters = append(chapters, models.Chapter{
			BookID:       book.ID,
			Index:        index,
			Title:        title,
			URL:          chapterURL,
			CachePath:    filepath.Join(stage.cachePathPrefix, cachePath),
			ResourcePath: parsedChapter.ResourcePath,
		})
	}
	if localRefreshStageTestHook != nil {
		if err := localRefreshStageTestHook(stageDir); err != nil {
			stage.cleanup()
			return nil, nil, err
		}
	}
	return stage, chapters, nil
}

func (stage *localRefreshStage) stageArchiveMetadata(libraryDir string, archive engine.ArchivedBook, chapters []engine.ArchivedChapter, source engine.ArchivedBookSource) error {
	if !stage.usesArchive {
		return nil
	}
	if strings.TrimSpace(archive.TOCFile) != "" {
		if err := stage.stageJSONFile(libraryDir, archive.TOCFile, chapters); err != nil {
			return err
		}
	}
	if strings.TrimSpace(archive.SourceFile) != "" {
		if err := stage.stageJSONFile(libraryDir, archive.SourceFile, []engine.ArchivedBookSource{source}); err != nil {
			return err
		}
	}
	return nil
}

func (stage *localRefreshStage) stageJSONFile(libraryDir, storedPath string, value any) error {
	if filepath.IsAbs(storedPath) {
		return fmt.Errorf("unsafe local refresh metadata path")
	}
	finalPath := filepath.Join(libraryDir, storedPath)
	relativePath, ok := relativePathInside(stage.archiveRoot, finalPath)
	if !ok {
		return fmt.Errorf("local refresh metadata path is outside archive")
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
	stage.promotions = append(stage.promotions, localRefreshPromotion{stagedPath: stagedPath, finalPath: finalPath})
	return nil
}

func (stage *localRefreshStage) promote() error {
	if err := os.MkdirAll(filepath.Dir(stage.finalContent), 0o755); err != nil {
		return err
	}
	if err := os.Rename(stage.stagedContent, stage.finalContent); err != nil {
		return err
	}
	for _, promotion := range stage.promotions {
		if err := os.MkdirAll(filepath.Dir(promotion.finalPath), 0o755); err != nil {
			return err
		}
		if err := os.Rename(promotion.stagedPath, promotion.finalPath); err != nil {
			return err
		}
	}
	return nil
}

func (stage *localRefreshStage) cleanup() {
	if stage != nil && stage.stageDir != "" {
		_ = os.RemoveAll(stage.stageDir)
	}
}

func (s *Server) pruneSupersededLocalDerivedContent(book models.Book, archiveRoot string, supersededCachePaths []string) {
	if strings.TrimSpace(archiveRoot) == "" || len(supersededCachePaths) == 0 {
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
	contentRoot := filepath.Join(archiveRoot, "content")
	for _, cachePath := range supersededCachePaths {
		if _, retained := active[cachePath]; retained || filepath.IsAbs(cachePath) {
			continue
		}
		candidate := filepath.Join(archiveRoot, cachePath)
		if _, ok := relativePathInside(contentRoot, candidate); !ok {
			continue
		}
		_ = os.Remove(candidate)
	}
}
