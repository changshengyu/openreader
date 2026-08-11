package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/sourcecandidates"
)

type bookSourceCandidateResponse struct {
	models.BookSourceCandidate
	CoverResourceURL *string `json:"coverResourceUrl,omitempty"`
	Current          bool    `json:"current"`
}

func (s *Server) listBookSourceCandidates(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(c.Query("mode")))
	if mode == "" {
		mode = "available"
	}
	switch mode {
	case "available":
		s.listAvailableBookSourceCandidates(c, userID, book)
	case "refresh":
		s.refreshBookSourceCandidates(c, userID, book)
	case "search":
		s.searchBookSourceCandidates(c, userID, book)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source candidate mode"})
	}
}

func (s *Server) listAvailableBookSourceCandidates(c *gin.Context, userID uint, book models.Book) {
	rows, err := s.sourceCandidates.Available(userID, book, s.currentCandidateSource(userID, book))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source candidates"})
		return
	}
	c.JSON(http.StatusOK, s.projectBookSourceCandidates(userID, book, rows))
}

func (s *Server) searchBookSourceCandidates(c *gin.Context, userID uint, book models.Book) {
	group := strings.TrimSpace(c.Query("group"))
	limit := parseBoundedInt(c.Query("limit"), 10, 1, 40)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 10000)
	sources, err := s.bookSources.ListActiveByIDs(userID, nil, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sources"})
		return
	}
	if group != "" {
		filtered := make([]models.BookSource, 0, len(sources))
		for _, source := range sources {
			if source.Group == group {
				filtered = append(filtered, source)
			}
		}
		sources = filtered
	}
	activeFailures, err := s.activeSourceFailures(userID, sources)
	if err != nil {
		activeFailures = nil
	}
	batch := s.sourceCandidates.Search(c.Request.Context(), book, sources, activeFailures, offset, limit)
	for _, failure := range batch.Failures {
		s.recordSourceFailure(userID, failure.Source, failure.Err)
	}
	if c.Request.Context().Err() != nil {
		return
	}
	if _, err := s.sourceCandidates.Merge(userID, book, batch.Candidates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save source candidates"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"list":       s.projectBookSourceCandidates(userID, book, batch.Candidates),
		"offset":     batch.Offset,
		"nextOffset": batch.NextOffset,
		"hasMore":    batch.HasMore,
		"total":      batch.Total,
		"searched":   batch.Searched,
		"matched":    batch.Matched,
		"failed":     batch.Failed,
		"empty":      batch.Empty,
	})
}

func (s *Server) refreshBookSourceCandidates(c *gin.Context, userID uint, book models.Book) {
	currentSource := s.currentCandidateSource(userID, book)
	rows, err := s.sourceCandidates.Available(userID, book, currentSource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source candidates"})
		return
	}
	sourceIDs := candidateSourceIDs(rows)
	sources := make([]models.BookSource, 0)
	if len(sourceIDs) > 0 {
		sources, err = s.bookSources.ListActiveByIDs(userID, sourceIDs, true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sources"})
			return
		}
	}
	sources = orderCandidateSources(rows, sources)
	activeFailures, err := s.activeSourceFailures(userID, sources)
	if err != nil {
		activeFailures = nil
	}
	verified, failures := s.sourceCandidates.Verify(c.Request.Context(), book, sources, activeFailures)
	for _, failure := range failures {
		s.recordSourceFailure(userID, failure.Source, failure.Err)
	}
	if c.Request.Context().Err() != nil {
		return
	}
	replacement := append([]models.BookSourceCandidate{
		sourcecandidates.CandidateFromBook(book, currentSource),
	}, verified...)
	replacement, err = s.sourceCandidates.Replace(userID, book, replacement)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save source candidates"})
		return
	}
	c.JSON(http.StatusOK, s.projectBookSourceCandidates(userID, book, replacement))
}

func (s *Server) currentCandidateSource(userID uint, book models.Book) *models.BookSource {
	if book.SourceID == 0 {
		return nil
	}
	source, err := s.bookSources.FindExistingForBook(userID, book.SourceID)
	if err != nil {
		return nil
	}
	return &source
}

func (s *Server) projectBookSourceCandidates(userID uint, book models.Book, rows []models.BookSourceCandidate) []bookSourceCandidateResponse {
	result := make([]bookSourceCandidateResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, bookSourceCandidateResponse{
			BookSourceCandidate: row,
			CoverResourceURL:    s.projectCoverResource(userID, row.SourceID, row.CoverURL),
			Current:             row.SourceID == book.SourceID && row.BookURL == book.URL,
		})
	}
	return result
}

func candidateSourceIDs(rows []models.BookSourceCandidate) []uint {
	ids := make([]uint, 0, len(rows))
	seen := make(map[uint]bool)
	for _, row := range rows {
		if row.SourceID == 0 || seen[row.SourceID] {
			continue
		}
		seen[row.SourceID] = true
		ids = append(ids, row.SourceID)
	}
	return ids
}

func orderCandidateSources(rows []models.BookSourceCandidate, sources []models.BookSource) []models.BookSource {
	byID := make(map[uint]models.BookSource, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}
	ordered := make([]models.BookSource, 0, len(sources))
	seen := make(map[uint]bool)
	for _, row := range rows {
		source, exists := byID[row.SourceID]
		if !exists || seen[source.ID] {
			continue
		}
		seen[source.ID] = true
		ordered = append(ordered, source)
	}
	return ordered
}
