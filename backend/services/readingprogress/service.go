package readingprogress

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

var (
	ErrBookNotFound          = errors.New("book not found")
	ErrInvalidProgress       = errors.New("invalid progress")
	ErrChapterNotFound       = errors.New("chapter not found")
	ErrChapterIdentity       = errors.New("chapter identity mismatch")
	ErrProgressMirrorOutside = errors.New("progress mirror outside WebDAV root")
)

type MirrorStatus string

const (
	MirrorSkipped MirrorStatus = "skipped"
	MirrorWritten MirrorStatus = "written"
	MirrorFailed  MirrorStatus = "failed"
)

type Input struct {
	UserID          uint
	BookID          uint
	ChapterID       uint
	ChapterIndex    int
	Offset          int
	Percent         float64
	ChapterPercent  float64
	Mode            string
	BaseUpdatedAt   string
	ClientUpdatedAt string
}

type Result struct {
	Progress     models.ReadingProgress
	Book         models.Book
	Conflict     bool
	MirrorStatus MirrorStatus
}

type Service struct {
	db      *gorm.DB
	dataDir string
}

func New(database *gorm.DB, dataDir string) *Service {
	return &Service{db: database, dataDir: dataDir}
}

func (s *Service) Get(userID, bookID uint) (models.ReadingProgress, bool, error) {
	var book models.Book
	if err := s.db.Select("id").Where("user_id = ? AND id = ?", userID, bookID).First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.ReadingProgress{}, false, ErrBookNotFound
		}
		return models.ReadingProgress{}, false, err
	}

	var progress models.ReadingProgress
	if err := s.db.Where("user_id = ? AND book_id = ?", userID, bookID).First(&progress).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.ReadingProgress{}, false, nil
		}
		return models.ReadingProgress{}, false, err
	}
	return progress, true, nil
}

func (s *Service) Save(input Input) (Result, error) {
	if input.UserID == 0 || input.BookID == 0 || input.ChapterIndex < 0 || input.Offset < 0 {
		return Result{}, ErrInvalidProgress
	}

	var book models.Book
	if err := s.db.Where("user_id = ? AND id = ?", input.UserID, input.BookID).First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Result{}, ErrBookNotFound
		}
		return Result{}, err
	}

	var chapter models.Chapter
	if err := s.db.Where("book_id = ? AND `index` = ?", book.ID, input.ChapterIndex).First(&chapter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Result{}, ErrChapterNotFound
		}
		return Result{}, err
	}
	if input.ChapterID != 0 && input.ChapterID != chapter.ID {
		return Result{}, ErrChapterIdentity
	}

	candidate := models.ReadingProgress{
		UserID:         input.UserID,
		BookID:         input.BookID,
		ChapterID:      chapter.ID,
		ChapterIndex:   chapter.Index,
		Offset:         input.Offset,
		Percent:        ClampPercent(input.Percent),
		ChapterPercent: ClampPercent(input.ChapterPercent),
		ChapterTitle:   chapter.Title,
		Mode:           input.Mode,
		UpdatedAt:      time.Now().UTC(),
	}
	progress, conflict, err := s.saveCAS(candidate, input.BaseUpdatedAt, input.ClientUpdatedAt)
	if err != nil {
		return Result{}, err
	}
	result := Result{Progress: progress, Book: book, Conflict: conflict, MirrorStatus: MirrorSkipped}
	if conflict {
		return result, nil
	}
	configured, err := s.mirrorProgress(book, progress)
	if err != nil {
		result.MirrorStatus = MirrorFailed
	} else if configured {
		result.MirrorStatus = MirrorWritten
	}
	return result, nil
}

func (s *Service) saveCAS(candidate models.ReadingProgress, baseUpdatedAt, clientUpdatedAt string) (models.ReadingProgress, bool, error) {
	var existing models.ReadingProgress
	err := s.db.Where("user_id = ? AND book_id = ?", candidate.UserID, candidate.BookID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		createErr := s.db.Create(&candidate).Error
		if createErr == nil {
			return candidate, false, nil
		}
		winner, loadErr := s.load(candidate.UserID, candidate.BookID)
		if loadErr == nil {
			return winner, true, nil
		}
		return models.ReadingProgress{}, false, createErr
	}
	if err != nil {
		return models.ReadingProgress{}, false, err
	}
	if IsStaleUpdate(existing.UpdatedAt, baseUpdatedAt, clientUpdatedAt) {
		return existing, true, nil
	}

	candidate.ID = existing.ID
	updates := map[string]any{
		"chapter_id":      candidate.ChapterID,
		"chapter_index":   candidate.ChapterIndex,
		"offset":          candidate.Offset,
		"percent":         candidate.Percent,
		"chapter_percent": candidate.ChapterPercent,
		"chapter_title":   candidate.ChapterTitle,
		"mode":            candidate.Mode,
		"updated_at":      candidate.UpdatedAt,
	}
	write := s.db.Model(&models.ReadingProgress{}).
		Where("id = ? AND user_id = ? AND book_id = ? AND updated_at = ?", existing.ID, candidate.UserID, candidate.BookID, existing.UpdatedAt).
		Updates(updates)
	if write.Error != nil {
		return models.ReadingProgress{}, false, write.Error
	}
	if write.RowsAffected == 1 {
		return candidate, false, nil
	}
	winner, err := s.load(candidate.UserID, candidate.BookID)
	if err != nil {
		return models.ReadingProgress{}, false, err
	}
	return winner, true, nil
}

func (s *Service) load(userID, bookID uint) (models.ReadingProgress, error) {
	var progress models.ReadingProgress
	err := s.db.Where("user_id = ? AND book_id = ?", userID, bookID).First(&progress).Error
	return progress, err
}

func IsStaleUpdate(serverUpdatedAt time.Time, baseUpdatedAt, clientUpdatedAt string) bool {
	if baseUpdatedAt == "" || serverUpdatedAt.IsZero() {
		return serverNewerThanClient(serverUpdatedAt, clientUpdatedAt)
	}
	base, err := time.Parse(time.RFC3339Nano, baseUpdatedAt)
	if err != nil {
		return serverNewerThanClient(serverUpdatedAt, clientUpdatedAt)
	}
	return serverUpdatedAt.After(base)
}

func serverNewerThanClient(serverUpdatedAt time.Time, clientUpdatedAt string) bool {
	if clientUpdatedAt == "" || serverUpdatedAt.IsZero() {
		return false
	}
	clientTime, err := time.Parse(time.RFC3339Nano, clientUpdatedAt)
	if err != nil {
		return false
	}
	return serverUpdatedAt.After(clientTime)
}

func ClampPercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 1 {
		return 1
	}
	return percent
}

type mirrorPayload struct {
	Name            string `json:"name"`
	Author          string `json:"author"`
	BookURL         string `json:"bookUrl"`
	DurChapterIndex int    `json:"durChapterIndex"`
	DurChapterPos   int    `json:"durChapterPos"`
	DurChapterTime  int64  `json:"durChapterTime"`
	DurChapterTitle string `json:"durChapterTitle"`
}

func (s *Service) mirrorProgress(book models.Book, progress models.ReadingProgress) (bool, error) {
	directory, configured, err := s.progressMirrorDirectory(book.UserID)
	if err != nil || !configured {
		return false, err
	}
	base := engine.SafeBookFolderName(book.Title, book.Author)
	if strings.TrimSpace(book.Title) == "" && strings.TrimSpace(book.Author) == "" {
		base = fmt.Sprintf("book-%d", book.ID)
	}
	target := filepath.Join(directory, base+".json")
	if !pathInside(directory, target) {
		return true, ErrProgressMirrorOutside
	}

	temporary, err := os.CreateTemp(directory, ".openreader-progress-")
	if err != nil {
		return true, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	payload := mirrorPayload{
		Name:            book.Title,
		Author:          book.Author,
		BookURL:         book.URL,
		DurChapterIndex: progress.ChapterIndex,
		DurChapterPos:   progress.Offset,
		DurChapterTime:  progress.UpdatedAt.UnixMilli(),
		DurChapterTitle: progress.ChapterTitle,
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		_ = temporary.Close()
		return true, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return true, err
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return true, err
	}
	if err := temporary.Close(); err != nil {
		return true, err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) progressMirrorDirectory(userID uint) (string, bool, error) {
	var user models.User
	if err := s.db.Select("id", "username", "role", "can_access_store", "can_access_webdav").First(&user, userID).Error; err != nil {
		return "", false, err
	}
	if !webDAVAllowed(user) {
		return "", false, nil
	}
	root := filepath.Join(s.dataDir, "webdav")
	if user.Role != "admin" {
		root = filepath.Join(root, "users", engine.SafeFilename(user.Username))
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	rootInfo, err := os.Stat(rootResolved)
	if err != nil || !rootInfo.IsDir() {
		if err == nil {
			err = ErrProgressMirrorOutside
		}
		return "", false, err
	}

	for _, relative := range []string{"bookProgress", filepath.Join("legado", "bookProgress")} {
		candidate := filepath.Join(root, relative)
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", false, ErrProgressMirrorOutside
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", false, err
		}
		if !pathInside(rootResolved, resolved) {
			return "", false, ErrProgressMirrorOutside
		}
		return resolved, true, nil
	}
	return "", false, nil
}

func webDAVAllowed(user models.User) bool {
	if user.CanAccessWebDAV == nil {
		return user.CanAccessStore
	}
	return *user.CanAccessWebDAV
}

func pathInside(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	return targetAbs == rootAbs || strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator))
}
