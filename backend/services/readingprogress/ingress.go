package readingprogress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openreader/backend/models"
)

const progressRecognitionBytes = 16 * 1024

type uploadGate struct {
	token chan struct{}
	refs  int
}

// UploadWebDAV coordinates the already-rooted file publication with its optional
// progress side effect. A sync failure AFTER publication never means file failure.
// notify runs after SQL commit, while the caller's ingress gate remains held.
func (s *Service) UploadWebDAV(ctx context.Context, userID uint, relative string, body io.Reader, publish func(io.Reader) error, notify func(Result)) (syncFailed bool, err error) {
	if !isProgressUpload(relative) {
		return false, publish(body)
	}
	release, err := s.acquireUpload(ctx, userID)
	if err != nil {
		return false, err
	}
	defer release()
	var capture recognitionBuffer
	if err := publish(io.TeeReader(body, &capture)); err != nil {
		return false, err
	}
	payload, valid := decodeUploadedProgress(capture.data)
	if !valid {
		return false, nil
	}
	result, applied, err := s.applyUploadedProgress(ctx, userID, payload)
	if err != nil {
		return true, nil
	}
	if applied && notify != nil {
		notify(result)
	}
	return false, nil
}

func isProgressUpload(relative string) bool {
	name, found := strings.CutPrefix(relative, "bookProgress/")
	if !found {
		name, found = strings.CutPrefix(relative, "legado/bookProgress/")
	}
	return found && name != "" && !strings.Contains(name, "/") && strings.HasSuffix(name, ".json")
}

func (s *Service) acquireUpload(ctx context.Context, userID uint) (func(), error) {
	s.uploadMu.Lock()
	if s.uploads == nil {
		s.uploads = make(map[uint]*uploadGate)
	}
	gate := s.uploads[userID]
	if gate == nil {
		gate = &uploadGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		s.uploads[userID] = gate
	}
	gate.refs++
	s.uploadMu.Unlock()
	unref := func() {
		s.uploadMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(s.uploads, userID)
		}
		s.uploadMu.Unlock()
	}
	select {
	case <-ctx.Done():
		unref()
		return nil, ctx.Err()
	case <-gate.token:
		if err := ctx.Err(); err != nil {
			gate.token <- struct{}{}
			unref()
			return nil, err
		}
		return func() { gate.token <- struct{}{}; unref() }, nil
	}
}

type recognitionBuffer struct{ data []byte }

func (b *recognitionBuffer) Write(p []byte) (int, error) {
	remaining := progressRecognitionBytes + 1 - len(b.data)
	if remaining > len(p) {
		remaining = len(p)
	}
	if remaining > 0 {
		b.data = append(b.data, p[:remaining]...)
	}
	return len(p), nil
}

type uploadedProgress struct {
	Name    string `json:"name"`
	Author  string `json:"author"`
	BookURL string `json:"bookUrl"`
	Index   *int   `json:"durChapterIndex"`
	Offset  int    `json:"durChapterPos"`
	Time    int64  `json:"durChapterTime"`
}

func decodeUploadedProgress(data []byte) (uploadedProgress, bool) {
	var payload uploadedProgress
	trimmed := bytes.TrimSpace(data)
	if len(data) > progressRecognitionBytes || !utf8.Valid(data) || len(trimmed) == 0 || trimmed[0] != '{' {
		return payload, false
	}
	if json.Unmarshal(data, &payload) != nil || payload.Index == nil || *payload.Index < 0 || payload.Offset < 0 || payload.Time <= 0 || payload.Time > time.Now().UnixMilli() {
		return payload, false
	}
	return payload, true
}

// applyUploadedProgress never calls the outgoing mirror: the exact uploaded file
// is authoritative for raw DAV clients and must not be rewritten as an echo.
func (s *Service) applyUploadedProgress(ctx context.Context, userID uint, payload uploadedProgress) (Result, bool, error) {
	var result Result
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		book, found, err := uploadedBook(tx, userID, payload)
		if err != nil || !found {
			return err
		}
		var chapter models.Chapter
		if err := tx.Where("book_id = ? AND `index` = ?", book.ID, *payload.Index).First(&chapter).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var existing models.ReadingProgress
		err = tx.Where("user_id = ? AND book_id = ?", userID, book.ID).First(&existing).Error
		foundProgress := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if foundProgress && (payload.Time <= existing.UpdatedAt.UnixMilli() ||
			(existing.ChapterID == chapter.ID && existing.ChapterIndex == chapter.Index && existing.Offset == payload.Offset && existing.ChapterTitle == chapter.Title)) {
			return nil
		}
		var count int64
		if err := tx.Model(&models.Chapter{}).Where("book_id = ?", book.ID).Count(&count).Error; err != nil {
			return err
		}
		mode := existing.Mode
		if mode == "" {
			mode = "scroll"
		}
		candidate := models.ReadingProgress{
			UserID: userID, BookID: book.ID, ChapterID: chapter.ID,
			ChapterIndex: chapter.Index, ChapterTitle: chapter.Title, Offset: payload.Offset,
			Percent: ClampPercent(float64(chapter.Index) / float64(count)),
			Mode:    mode, UpdatedAt: time.Now().UTC(),
		}
		if foundProgress {
			candidate.ID = existing.ID
			write := tx.Model(&models.ReadingProgress{}).
				Where("id = ? AND user_id = ? AND book_id = ? AND updated_at = ?", existing.ID, userID, book.ID, existing.UpdatedAt).
				Updates(map[string]any{
					"chapter_id": candidate.ChapterID, "chapter_index": candidate.ChapterIndex,
					"chapter_title": candidate.ChapterTitle, "offset": candidate.Offset,
					"percent": candidate.Percent, "chapter_percent": 0,
					"mode": candidate.Mode, "updated_at": candidate.UpdatedAt,
				})
			if write.Error != nil {
				return write.Error
			}
			if write.RowsAffected != 1 {
				return nil
			}
		} else {
			write := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate)
			if write.Error != nil {
				return write.Error
			}
			if write.RowsAffected != 1 {
				return nil
			}
		}
		// Check again after ORM write callbacks too. A removed/replaced catalog or
		// changed owner must roll back the candidate rather than create an orphan.
		var currentCount int64
		if err := tx.Model(&models.Chapter{}).
			Where("id = ? AND book_id = ? AND `index` = ? AND title = ?", chapter.ID, book.ID, chapter.Index, chapter.Title).
			Where("EXISTS (SELECT 1 FROM books WHERE books.id = ? AND books.user_id = ?)", book.ID, userID).
			Count(&currentCount).Error; err != nil {
			return err
		}
		if currentCount != 1 {
			return ErrChapterIdentity
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		result = Result{Progress: candidate, Book: book, MirrorStatus: MirrorSkipped}
		applied = true
		return nil
	})
	return result, applied && err == nil, err
}

func uploadedBook(tx *gorm.DB, userID uint, payload uploadedProgress) (models.Book, bool, error) {
	var books []models.Book
	if strings.TrimSpace(payload.BookURL) != "" {
		if err := tx.Where("user_id = ? AND url = ?", userID, payload.BookURL).Limit(2).Find(&books).Error; err != nil {
			return models.Book{}, false, err
		}
		if len(books) != 0 {
			return books[0], len(books) == 1, nil
		}
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Author) == "" {
		return models.Book{}, false, nil
	}
	if err := tx.Where("user_id = ? AND title = ? AND author = ?", userID, payload.Name, payload.Author).Limit(2).Find(&books).Error; err != nil {
		return models.Book{}, false, err
	}
	if len(books) == 1 {
		return books[0], true, nil
	}
	return models.Book{}, false, nil
}
