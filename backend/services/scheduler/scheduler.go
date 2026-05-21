package scheduler

import (
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

// Scheduler periodically checks remote books for new chapters.
type Scheduler struct {
	db       *gorm.DB
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

// CheckNow triggers an immediate check of all remote books. Returns the number of new chapters found.
func (s *Scheduler) CheckNow() int {
	return s.checkAllBooks()
}

func (s *Scheduler) checkAllBooks() int {
	var books []models.Book
	if err := s.db.Where("source_id > ? AND can_update = ?", 0, true).Find(&books).Error; err != nil {
		log.Printf("scheduler: failed to list books: %v", err)
		return 0
	}

	totalNew := 0
	for _, book := range books {
		count, err := s.checkBook(book)
		if err != nil {
			log.Printf("scheduler: check book %q (id=%d): %v", book.Title, book.ID, err)
			continue
		}
		totalNew += count
	}

	if totalNew > 0 {
		log.Printf("scheduler: found %d new chapters total", totalNew)
	}
	return totalNew
}

func (s *Scheduler) checkBook(book models.Book) (int, error) {
	var source models.BookSource
	if err := s.db.First(&source, book.SourceID).Error; err != nil {
		return 0, err
	}

	remoteChapters, err := engine.ParseTOC(book.URL, source)
	if err != nil {
		return 0, err
	}

	var existingCount int64
	if err := s.db.Model(&models.Chapter{}).Where("book_id = ?", book.ID).Count(&existingCount).Error; err != nil {
		return 0, err
	}

	if len(remoteChapters) <= int(existingCount) {
		return 0, nil
	}

	newChapters := remoteChapters[existingCount:]
	for _, ch := range newChapters {
		chapter := models.Chapter{
			BookID: book.ID,
			Index:  ch.Index,
			Title:  strings.TrimSpace(ch.Title),
			URL:    ch.URL,
		}
		if err := s.db.Create(&chapter).Error; err != nil {
			return 0, err
		}
	}

	book.LastChapter = newChapters[len(newChapters)-1].Title
	book.ChapterCount = len(remoteChapters)
	if err := s.db.Save(&book).Error; err != nil {
		return 0, err
	}

	return len(newChapters), nil
}
