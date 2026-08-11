package sourcecandidates

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	MaxCandidatesPerBook = 200
	maxSourcesPerSearch  = 40
	searchConcurrency    = 4
	searchTimeout        = 12 * time.Second
)

type Service struct {
	db *gorm.DB
}

type SourceFailure struct {
	Source models.BookSource
	Err    error
}

type SearchBatch struct {
	Candidates []models.BookSourceCandidate
	Failures   []SourceFailure
	Offset     int
	NextOffset int
	HasMore    bool
	Total      int
	Searched   int
	Matched    int
	Failed     int
	Empty      int
}

type verifyOutcome struct {
	index      int
	source     models.BookSource
	candidates []models.BookSourceCandidate
	err        error
}

func New(database *gorm.DB) *Service {
	return &Service{db: database}
}

func (s *Service) Available(userID uint, book models.Book, source *models.BookSource) ([]models.BookSourceCandidate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("source candidate service is unavailable")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.SeedCurrent(tx, book, source)
	})
	if err != nil {
		return nil, err
	}
	return s.List(userID, book.ID)
}

func (s *Service) List(userID, bookID uint) ([]models.BookSourceCandidate, error) {
	rows := make([]models.BookSourceCandidate, 0)
	err := s.db.Where("user_id = ? AND book_id = ?", userID, bookID).
		Order("sort_order asc, id asc").
		Find(&rows).Error
	return rows, err
}

func (s *Service) SeedCurrent(tx *gorm.DB, book models.Book, source *models.BookSource) error {
	if tx == nil {
		return errors.New("source candidate transaction is unavailable")
	}
	candidate := CandidateFromBook(book, source)
	if candidate.BookURL == "" {
		return nil
	}
	return mergeCandidatesTx(tx, book, []models.BookSourceCandidate{candidate})
}

func (s *Service) Merge(userID uint, book models.Book, candidates []models.BookSourceCandidate) ([]models.BookSourceCandidate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("source candidate service is unavailable")
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return mergeCandidatesTx(tx, book, candidates)
	}); err != nil {
		return nil, err
	}
	return s.List(userID, book.ID)
}

func (s *Service) Replace(userID uint, book models.Book, candidates []models.BookSourceCandidate) ([]models.BookSourceCandidate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("source candidate service is unavailable")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing []models.BookSourceCandidate
		if err := tx.Where("user_id = ? AND book_id = ?", userID, book.ID).
			Order("sort_order asc, id asc").Find(&existing).Error; err != nil {
			return err
		}
		orderByURL := make(map[string]int, len(existing))
		maxOrder := 0
		for _, row := range existing {
			orderByURL[row.BookURL] = row.SortOrder
			if row.SortOrder > maxOrder {
				maxOrder = row.SortOrder
			}
		}
		rows := uniqueCandidates(book, candidates)
		for index := range rows {
			if order := orderByURL[rows[index].BookURL]; order > 0 {
				rows[index].SortOrder = order
				continue
			}
			maxOrder++
			rows[index].SortOrder = maxOrder
		}
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].SortOrder < rows[j].SortOrder
		})
		if len(rows) > MaxCandidatesPerBook {
			rows = boundedCandidates(rows, book)
		}
		if err := tx.Where("user_id = ? AND book_id = ?", userID, book.ID).
			Delete(&models.BookSourceCandidate{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(&rows, 100).Error
	})
	if err != nil {
		return nil, err
	}
	return s.List(userID, book.ID)
}

func (s *Service) Search(
	ctx context.Context,
	book models.Book,
	sources []models.BookSource,
	activeFailures map[uint]models.SourceFailure,
	offset int,
	limit int,
) SearchBatch {
	if offset < 0 {
		offset = 0
	}
	if offset > len(sources) {
		offset = len(sources)
	}
	if limit < 1 {
		limit = 10
	}
	batch := SearchBatch{Offset: offset, NextOffset: offset, Total: len(sources)}
	seen := make(map[string]bool)
	cursor := offset
	for cursor < len(sources) && batch.Searched < maxSourcesPerSearch && len(batch.Candidates) < limit {
		if ctx.Err() != nil {
			break
		}
		remainingBudget := maxSourcesPerSearch - batch.Searched
		end := cursor + searchConcurrency
		if end > len(sources) {
			end = len(sources)
		}
		if end > cursor+remainingBudget {
			end = cursor + remainingBudget
		}
		window := sources[cursor:end]
		toVerify := make([]models.BookSource, 0, len(window))
		for _, source := range window {
			batch.Searched++
			if _, suppressed := activeFailures[source.ID]; suppressed {
				batch.Failed++
				continue
			}
			toVerify = append(toVerify, source)
		}
		outcomes := verifySources(ctx, book, toVerify)
		for _, outcome := range outcomes {
			if outcome.err != nil {
				batch.Failed++
				batch.Failures = append(batch.Failures, SourceFailure{Source: outcome.source, Err: outcome.err})
				continue
			}
			if len(outcome.candidates) == 0 {
				batch.Empty++
				continue
			}
			batch.Matched++
			for _, candidate := range outcome.candidates {
				if seen[candidate.BookURL] {
					continue
				}
				seen[candidate.BookURL] = true
				batch.Candidates = append(batch.Candidates, candidate)
			}
		}
		cursor = end
		batch.NextOffset = cursor
		if ctx.Err() != nil {
			break
		}
	}
	batch.HasMore = batch.NextOffset < len(sources)
	return batch
}

func (s *Service) Verify(
	ctx context.Context,
	book models.Book,
	sources []models.BookSource,
	activeFailures map[uint]models.SourceFailure,
) ([]models.BookSourceCandidate, []SourceFailure) {
	filtered := make([]models.BookSource, 0, len(sources))
	failures := make([]SourceFailure, 0)
	for _, source := range sources {
		if failure, suppressed := activeFailures[source.ID]; suppressed {
			failures = append(failures, SourceFailure{Source: source, Err: errors.New(failure.Message)})
			continue
		}
		filtered = append(filtered, source)
	}
	outcomes := verifySources(ctx, book, filtered)
	rows := make([]models.BookSourceCandidate, 0)
	seen := make(map[string]bool)
	for _, outcome := range outcomes {
		if outcome.err != nil {
			failures = append(failures, SourceFailure{Source: outcome.source, Err: outcome.err})
			continue
		}
		for _, candidate := range outcome.candidates {
			if seen[candidate.BookURL] {
				continue
			}
			seen[candidate.BookURL] = true
			rows = append(rows, candidate)
		}
	}
	return rows, failures
}

func CandidateFromBook(book models.Book, source *models.BookSource) models.BookSourceCandidate {
	candidate := models.BookSourceCandidate{
		UserID:             book.UserID,
		BookID:             book.ID,
		SourceID:           book.SourceID,
		Title:              book.Title,
		Author:             book.Author,
		BookURL:            book.URL,
		CoverURL:           book.CoverURL,
		Intro:              book.Intro,
		Kind:               book.Kind,
		WordCount:          book.WordCount,
		LatestChapterTitle: book.LastChapter,
		Type:               book.Type,
	}
	if source != nil {
		candidate.SourceURL = source.BaseURL
		candidate.SourceName = source.Name
		candidate.SourceGroup = source.Group
		candidate.Type = source.SourceType
	} else if book.SourceID == 0 {
		candidate.SourceName = "本地书籍"
	}
	return sanitizeCandidate(candidate, book)
}

func candidateFromSearch(book models.Book, source models.BookSource, result engine.SearchResult, elapsed int64) models.BookSourceCandidate {
	return sanitizeCandidate(models.BookSourceCandidate{
		UserID:             book.UserID,
		BookID:             book.ID,
		SourceID:           source.ID,
		SourceURL:          source.BaseURL,
		SourceName:         source.Name,
		SourceGroup:        source.Group,
		Title:              result.Title,
		Author:             result.Author,
		BookURL:            result.BookURL,
		CoverURL:           result.CoverURL,
		Intro:              result.Intro,
		Kind:               result.Kind,
		WordCount:          result.WordCount,
		LatestChapterTitle: result.LatestChapter,
		Type:               source.SourceType,
		RespondTime:        elapsed,
	}, book)
}

func verifySources(ctx context.Context, book models.Book, sources []models.BookSource) []verifyOutcome {
	if len(sources) == 0 {
		return nil
	}
	channel := make(chan verifyOutcome, len(sources))
	gate := make(chan struct{}, searchConcurrency)
	var wg sync.WaitGroup
	for index, source := range sources {
		index, source := index, source
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ctx.Err(); err != nil {
				channel <- verifyOutcome{index: index, source: source, err: err}
				return
			}
			select {
			case gate <- struct{}{}:
			case <-ctx.Done():
				channel <- verifyOutcome{index: index, source: source, err: ctx.Err()}
				return
			}
			defer func() { <-gate }()
			requestCtx, cancel := context.WithTimeout(ctx, searchTimeout)
			started := time.Now()
			results, err := engine.SearchBooksContext(requestCtx, source, book.Title)
			cancel()
			outcome := verifyOutcome{index: index, source: source, err: err}
			if err == nil {
				elapsed := time.Since(started).Milliseconds()
				for _, result := range results {
					if !exactBookIdentity(book, result) || strings.TrimSpace(result.BookURL) == "" {
						continue
					}
					outcome.candidates = append(outcome.candidates, candidateFromSearch(book, source, result, elapsed))
				}
			}
			channel <- outcome
		}()
	}
	go func() {
		wg.Wait()
		close(channel)
	}()
	outcomes := make([]verifyOutcome, len(sources))
	for outcome := range channel {
		outcomes[outcome.index] = outcome
	}
	return outcomes
}

func exactBookIdentity(book models.Book, result engine.SearchResult) bool {
	return strings.TrimSpace(result.Title) == strings.TrimSpace(book.Title) &&
		strings.TrimSpace(result.Author) == strings.TrimSpace(book.Author)
}

func mergeCandidatesTx(tx *gorm.DB, book models.Book, candidates []models.BookSourceCandidate) error {
	var maxOrder int
	if err := tx.Model(&models.BookSourceCandidate{}).
		Where("user_id = ? AND book_id = ?", book.UserID, book.ID).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder).Error; err != nil {
		return err
	}
	for _, candidate := range uniqueCandidates(book, candidates) {
		var existing models.BookSourceCandidate
		err := tx.Where("user_id = ? AND book_id = ? AND book_url = ?", book.UserID, book.ID, candidate.BookURL).
			First(&existing).Error
		if err == nil {
			candidate.ID = existing.ID
			candidate.SortOrder = existing.SortOrder
			candidate.CreatedAt = existing.CreatedAt
			if err := tx.Save(&candidate).Error; err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		maxOrder++
		candidate.SortOrder = maxOrder
		if err := tx.Create(&candidate).Error; err != nil {
			return err
		}
	}
	return pruneCandidatesTx(tx, book)
}

func pruneCandidatesTx(tx *gorm.DB, book models.Book) error {
	var rows []models.BookSourceCandidate
	if err := tx.Where("user_id = ? AND book_id = ?", book.UserID, book.ID).
		Order("sort_order asc, id asc").Find(&rows).Error; err != nil {
		return err
	}
	removeCount := len(rows) - MaxCandidatesPerBook
	if removeCount <= 0 {
		return nil
	}
	ids := make([]uint, 0, removeCount)
	for _, row := range rows {
		if row.SourceID == book.SourceID && row.BookURL == book.URL {
			continue
		}
		ids = append(ids, row.ID)
		if len(ids) == removeCount {
			break
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return tx.Where("id IN ?", ids).Delete(&models.BookSourceCandidate{}).Error
}

func uniqueCandidates(book models.Book, candidates []models.BookSourceCandidate) []models.BookSourceCandidate {
	rows := make([]models.BookSourceCandidate, 0, len(candidates))
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		candidate = sanitizeCandidate(candidate, book)
		if candidate.BookURL == "" || seen[candidate.BookURL] {
			continue
		}
		if candidate.BookURL == book.URL && candidate.SourceID != book.SourceID {
			continue
		}
		seen[candidate.BookURL] = true
		rows = append(rows, candidate)
	}
	return rows
}

func boundedCandidates(rows []models.BookSourceCandidate, book models.Book) []models.BookSourceCandidate {
	removeCount := len(rows) - MaxCandidatesPerBook
	kept := make([]models.BookSourceCandidate, 0, MaxCandidatesPerBook)
	for _, row := range rows {
		if removeCount > 0 && !(row.SourceID == book.SourceID && row.BookURL == book.URL) {
			removeCount--
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

func sanitizeCandidate(candidate models.BookSourceCandidate, book models.Book) models.BookSourceCandidate {
	candidate.UserID = book.UserID
	candidate.BookID = book.ID
	candidate.SourceURL = boundedString(candidate.SourceURL, 500)
	candidate.SourceName = boundedString(candidate.SourceName, 120)
	candidate.SourceGroup = boundedString(candidate.SourceGroup, 80)
	candidate.Title = boundedString(candidate.Title, 240)
	candidate.Author = boundedString(candidate.Author, 160)
	candidate.BookURL = boundedString(candidate.BookURL, 800)
	candidate.CoverURL = boundedString(candidate.CoverURL, 600)
	candidate.Intro = boundedString(candidate.Intro, 4000)
	candidate.Kind = boundedString(candidate.Kind, 400)
	candidate.WordCount = boundedString(candidate.WordCount, 120)
	candidate.LatestChapterTitle = boundedString(candidate.LatestChapterTitle, 240)
	return candidate
}

func boundedString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
