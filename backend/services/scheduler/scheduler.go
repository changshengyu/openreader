package scheduler

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/bookcatalog"
	"openreader/backend/services/booksources"
)

const manualRefreshFetchConcurrency = 16

var ErrStaleBookSnapshot = errors.New("book changed while checking updates")

// CheckResult is the durable outcome of one update-check round. Cache paths are
// internal post-commit cleanup work and are never serialized to API clients.
type CheckResult struct {
	Checked              int
	Updated              int
	Failed               int
	NewChapters          int
	UpdatedBookIDs       []uint
	ReplacedBookIDs      []uint
	SupersededCachePaths []string
}

type fetchedBook struct {
	snapshot models.Book
	chapters []engine.RemoteChapter
	variable string
	err      error
}

type bookMutation struct {
	updated     bool
	replaced    bool
	legacyAdded int
	newChapters int
	cachePaths  []string
}

// Scheduler periodically checks remote books for new chapters.
type Scheduler struct {
	db       *gorm.DB
	sources  *booksources.Service
	interval time.Duration
	stopCh   chan struct{}
}

// New creates a scheduler with the given check interval.
func New(db *gorm.DB, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &Scheduler{
		db:       db,
		sources:  booksources.New(db),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the periodic update loop.
func (s *Scheduler) Start() {
	log.Printf("scheduler started with interval %v", s.interval)
	go s.loop()
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAllBooks()
		case <-s.stopCh:
			log.Println("scheduler stopped")
			return
		}
	}
}

// CheckNow triggers an immediate check of all remote books. Returns the number
// of chapters beyond each persisted book summary, not catalogue repair rows.
func (s *Scheduler) CheckNow() int {
	result, err := s.checkBooksDetailed("source_id > ? AND can_update = ?", 0, true)
	if err != nil {
		log.Printf("scheduler: failed to list updateable books")
		return 0
	}
	return result.NewChapters
}

// CheckNowForUser keeps the deployed internal signature for existing callers.
// New API code should use CheckNowForUserDetailed.
func (s *Scheduler) CheckNowForUser(userID uint) (int, []uint) {
	result, err := s.CheckNowForUserDetailed(userID)
	if err != nil {
		log.Printf("scheduler: failed to list updateable books for user %d", userID)
		return 0, nil
	}
	return result.NewChapters, result.UpdatedBookIDs
}

// CheckNowForUserDetailed checks only one authenticated user's remote books.
func (s *Scheduler) CheckNowForUserDetailed(userID uint) (CheckResult, error) {
	return s.checkBooksDetailed("user_id = ? AND source_id > ? AND can_update = ?", userID, 0, true)
}

func (s *Scheduler) checkAllBooks() (int, []uint) {
	result, err := s.checkBooksDetailed("source_id > ? AND can_update = ?", 0, true)
	if err != nil {
		log.Printf("scheduler: failed to list updateable books")
		return 0, nil
	}
	return result.NewChapters, result.UpdatedBookIDs
}

func (s *Scheduler) checkBooksDetailed(query any, args ...any) (CheckResult, error) {
	var books []models.Book
	if err := s.db.Where(query, args...).Order("id asc").Find(&books).Error; err != nil {
		return CheckResult{}, err
	}
	result := CheckResult{
		Checked:              len(books),
		UpdatedBookIDs:       make([]uint, 0),
		ReplacedBookIDs:      make([]uint, 0),
		SupersededCachePaths: make([]string, 0),
	}
	if len(books) == 0 {
		return result, nil
	}

	fetched := s.fetchBooks(books)
	for index := range fetched {
		item := fetched[index]
		if item.err != nil {
			result.Failed++
			log.Printf("scheduler: update check failed for book id=%d", item.snapshot.ID)
			continue
		}
		mutation, err := s.applyFetchedBook(item)
		if err != nil {
			result.Failed++
			log.Printf("scheduler: update commit failed for book id=%d", item.snapshot.ID)
			continue
		}
		if !mutation.updated {
			continue
		}
		result.UpdatedBookIDs = append(result.UpdatedBookIDs, item.snapshot.ID)
		result.Updated++
		result.NewChapters += mutation.newChapters
		result.SupersededCachePaths = append(result.SupersededCachePaths, mutation.cachePaths...)
		if mutation.replaced {
			result.ReplacedBookIDs = append(result.ReplacedBookIDs, item.snapshot.ID)
		}
	}

	if result.NewChapters > 0 {
		log.Printf("scheduler: found %d new chapters total", result.NewChapters)
	}
	return result, nil
}

func (s *Scheduler) fetchBooks(books []models.Book) []fetchedBook {
	results := make([]fetchedBook, len(books))
	jobs := make(chan int)
	workerCount := len(books)
	if workerCount > manualRefreshFetchConcurrency {
		workerCount = manualRefreshFetchConcurrency
	}
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for index := range jobs {
				results[index] = s.fetchBook(books[index])
			}
		}()
	}
	for index := range books {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return results
}

func (s *Scheduler) fetchBook(book models.Book) fetchedBook {
	result := fetchedBook{snapshot: book}
	source, err := s.sources.FindForBook(book.UserID, book.SourceID)
	if err != nil {
		result.err = err
		return result
	}
	result.chapters, result.variable, result.err = engine.ParseTOCWithVariables(book.URL, source, book.Variable, book.Title)
	if result.err == nil && len(result.chapters) == 0 {
		result.err = fmt.Errorf("source returned no chapters")
	}
	return result
}

// checkBook is retained for focused scheduler contracts. A detailed round uses
// the same fetch/apply path; the returned count preserves the historical repair
// meaning for prefix appends while replacement-only changes return zero.
func (s *Scheduler) checkBook(book models.Book) (int, error) {
	fetched := s.fetchBook(book)
	if fetched.err != nil {
		return 0, fetched.err
	}
	mutation, err := s.applyFetchedBook(fetched)
	if err != nil {
		return 0, err
	}
	return mutation.legacyAdded, nil
}

func (s *Scheduler) applyFetchedBook(fetched fetchedBook) (bookMutation, error) {
	next := make([]models.Chapter, 0, len(fetched.chapters))
	for index, remote := range fetched.chapters {
		title := strings.TrimSpace(remote.Title)
		if title == "" {
			title = fmt.Sprintf("第%d章", index+1)
		}
		next = append(next, models.Chapter{
			BookID:   fetched.snapshot.ID,
			Index:    remote.Index,
			Title:    title,
			URL:      strings.TrimSpace(remote.URL),
			IsVolume: remote.IsVolume,
			Tag:      remote.Tag,
			Variable: remote.Variable,
		})
	}
	if len(next) == 0 {
		return bookMutation{}, fmt.Errorf("source returned no chapters")
	}

	mutation := bookMutation{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var current models.Book
		if err := tx.Where("id = ? AND user_id = ?", fetched.snapshot.ID, fetched.snapshot.UserID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStaleBookSnapshot
			}
			return err
		}
		if !sameBookSnapshot(current, fetched.snapshot) {
			return ErrStaleBookSnapshot
		}

		var previous []models.Chapter
		if err := tx.Where("book_id = ?", current.ID).Order("`index` asc").Find(&previous).Error; err != nil {
			return err
		}
		equal := cataloguesEqual(previous, next)
		prefix := !equal && catalogueIsExactPrefix(previous, next)
		if prefix {
			for index := len(previous); index < len(next); index++ {
				chapter := next[index]
				if err := tx.Create(&chapter).Error; err != nil {
					return err
				}
			}
			mutation.legacyAdded = len(next) - len(previous)
		} else if !equal {
			var err error
			mutation.cachePaths, _, err = bookcatalog.ReplaceChapterRows(tx, current.UserID, current.ID, next)
			if err != nil {
				return err
			}
			mutation.replaced = true
			if len(next) > len(previous) {
				mutation.legacyAdded = len(next) - len(previous)
			}
		}

		lastChapter := next[len(next)-1].Title
		updates := make(map[string]any)
		if current.LastChapter != lastChapter {
			updates["last_chapter"] = lastChapter
		}
		if current.ChapterCount != len(next) {
			updates["chapter_count"] = len(next)
		}
		if current.Variable != fetched.variable {
			updates["variable"] = fetched.variable
		}
		if len(next) > current.ChapterCount {
			mutation.newChapters = len(next) - current.ChapterCount
			updates["last_check_time"] = time.Now().UnixMilli()
		}
		mutation.updated = !equal || len(updates) > 0
		if len(updates) == 0 {
			return nil
		}
		write := tx.Model(&models.Book{}).
			Where("id = ? AND user_id = ? AND source_id = ? AND url = ? AND can_update = ?", current.ID, current.UserID, current.SourceID, current.URL, true).
			Updates(updates)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return ErrStaleBookSnapshot
		}
		return nil
	})
	if err != nil {
		return bookMutation{}, err
	}
	return mutation, nil
}

func sameBookSnapshot(current, snapshot models.Book) bool {
	return current.ID == snapshot.ID &&
		current.UserID == snapshot.UserID &&
		current.SourceID == snapshot.SourceID &&
		current.URL == snapshot.URL &&
		current.CanUpdate == snapshot.CanUpdate &&
		current.Variable == snapshot.Variable &&
		current.UpdatedAt.Equal(snapshot.UpdatedAt)
}

func cataloguesEqual(previous, next []models.Chapter) bool {
	return len(previous) == len(next) && catalogueIsExactPrefix(previous, next)
}

func catalogueIsExactPrefix(previous, next []models.Chapter) bool {
	if len(previous) > len(next) {
		return false
	}
	for index := range previous {
		if !sameChapterIdentity(previous[index], next[index]) {
			return false
		}
	}
	return true
}

func sameChapterIdentity(left, right models.Chapter) bool {
	return left.Index == right.Index &&
		left.Title == right.Title &&
		left.URL == right.URL &&
		left.IsVolume == right.IsVolume &&
		left.Tag == right.Tag &&
		left.Variable == right.Variable
}
