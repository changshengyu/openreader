package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/audioreader"
)

const (
	remoteReaderSessionIdleTTL = 30 * time.Minute
	remoteReaderSessionMaxTTL  = 4 * time.Hour
)

var (
	errRemoteReaderSessionMissing = errors.New("remote reader session missing")
	errRemoteReaderSessionExpired = errors.New("remote reader session expired")
)

// remoteReaderSession mirrors reader-dev's in-memory readingBook. It is not a
// GORM model deliberately: search/explore reading must not create shelf rows,
// catalogue rows, progress, cache files, backups, or websocket updates.
type remoteReaderSession struct {
	ID           string
	UserID       uint
	Source       models.BookSource
	Book         models.Book
	Chapters     []models.Chapter
	CreatedAt    time.Time
	ExpiresAt    time.Time
	MaxExpiresAt time.Time
}

type remoteReaderSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*remoteReaderSession
	now      func() time.Time
}

func newRemoteReaderSessionStore() *remoteReaderSessionStore {
	return &remoteReaderSessionStore{
		sessions: make(map[string]*remoteReaderSession),
		now:      time.Now,
	}
}

func (s *remoteReaderSessionStore) create(userID uint, source models.BookSource, book models.Book, chapters []models.Chapter) (remoteReaderSession, error) {
	identifier, err := randomRemoteReaderSessionID()
	if err != nil {
		return remoteReaderSession{}, err
	}
	now := s.now().UTC()
	session := remoteReaderSession{
		ID:           identifier,
		UserID:       userID,
		Source:       source,
		Book:         book,
		Chapters:     cloneRemoteReaderChapters(chapters),
		CreatedAt:    now,
		ExpiresAt:    now.Add(remoteReaderSessionIdleTTL),
		MaxExpiresAt: now.Add(remoteReaderSessionMaxTTL),
	}
	s.mu.Lock()
	s.purgeExpiredLocked(now)
	s.sessions[identifier] = &session
	s.mu.Unlock()
	return cloneRemoteReaderSession(session), nil
}

func (s *remoteReaderSessionStore) get(userID uint, identifier string) (remoteReaderSession, error) {
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[strings.TrimSpace(identifier)]
	if !ok || session.UserID != userID {
		s.purgeExpiredLocked(now)
		return remoteReaderSession{}, errRemoteReaderSessionMissing
	}
	if !now.Before(session.ExpiresAt) || !now.Before(session.MaxExpiresAt) {
		delete(s.sessions, session.ID)
		return remoteReaderSession{}, errRemoteReaderSessionExpired
	}
	session.ExpiresAt = minRemoteReaderExpiry(now.Add(remoteReaderSessionIdleTTL), session.MaxExpiresAt)
	return cloneRemoteReaderSession(*session), nil
}

func (s *remoteReaderSessionStore) updateVariables(userID uint, identifier, bookVariable string, chapterIndex int, chapterVariable string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[strings.TrimSpace(identifier)]
	if !ok || session.UserID != userID {
		return
	}
	session.Book.Variable = bookVariable
	for index := range session.Chapters {
		if session.Chapters[index].Index == chapterIndex {
			session.Chapters[index].Variable = chapterVariable
			return
		}
	}
}

func (s *remoteReaderSessionStore) purgeExpiredLocked(now time.Time) {
	for identifier, session := range s.sessions {
		if !now.Before(session.ExpiresAt) || !now.Before(session.MaxExpiresAt) {
			delete(s.sessions, identifier)
		}
	}
}

func randomRemoteReaderSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func cloneRemoteReaderSession(value remoteReaderSession) remoteReaderSession {
	value.Chapters = cloneRemoteReaderChapters(value.Chapters)
	return value
}

func cloneRemoteReaderChapters(value []models.Chapter) []models.Chapter {
	return append([]models.Chapter(nil), value...)
}

func minRemoteReaderExpiry(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

type remoteReaderSessionRequest struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	CoverURL  string `json:"coverUrl"`
	Intro     string `json:"intro"`
	Kind      string `json:"kind"`
	WordCount string `json:"wordCount"`
	BookURL   string `json:"bookUrl"`
	SourceID  uint   `json:"sourceId"`
	Variable  string `json:"variable"`
}

func (s *Server) createRemoteReaderSession(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req remoteReaderSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid remote reader payload"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.BookURL = strings.TrimSpace(req.BookURL)
	if req.Title == "" || req.BookURL == "" || req.SourceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title, bookUrl, and sourceId are required"})
		return
	}
	variable, err := engine.NormalizeSourceRuleVariables(req.Variable)
	if err != nil {
		writeSourceError(c, http.StatusBadRequest, "book source variables are invalid", err, "book_info")
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, req.SourceID).Error; err != nil || !source.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}

	remoteInfo, remoteChapters, variable, err := engine.FetchBookInfoAndTOCWithVariables(req.BookURL, source, variable, req.Title)
	if err != nil {
		s.recordSourceFailure(userID, source, err)
		writeSourceError(c, http.StatusBadGateway, "failed to fetch chapters", err, "book_info")
		return
	}
	if len(remoteChapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	book := models.Book{
		UserID:       userID,
		SourceID:     source.ID,
		Type:         source.SourceType,
		Title:        firstNonBlankCanRename(remoteInfo.Title, req.Title, remoteInfo.CanRename),
		Author:       firstNonBlankCanRename(remoteInfo.Author, req.Author, remoteInfo.CanRename),
		CoverURL:     firstNonBlank(remoteInfo.CoverURL, req.CoverURL),
		Intro:        firstNonBlank(remoteInfo.Intro, req.Intro),
		Kind:         firstNonBlank(remoteInfo.Kind, req.Kind),
		WordCount:    firstNonBlank(remoteInfo.WordCount, req.WordCount),
		URL:          req.BookURL,
		Variable:     variable,
		LastChapter:  remoteChapters[len(remoteChapters)-1].Title,
		ChapterCount: len(remoteChapters),
	}
	chapters := make([]models.Chapter, 0, len(remoteChapters))
	for _, row := range remoteChapters {
		chapters = append(chapters, models.Chapter{
			Index:    row.Index,
			Title:    row.Title,
			URL:      row.URL,
			IsVolume: row.IsVolume,
			Tag:      row.Tag,
			Variable: row.Variable,
		})
	}
	session, err := s.remoteReaders.create(userID, source, book, chapters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create remote reader session"})
		return
	}
	writeRemoteReaderSession(c, http.StatusCreated, session)
}

func (s *Server) getRemoteReaderSession(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	session, ok := s.lookupRemoteReaderSession(c, userID)
	if !ok {
		return
	}
	writeRemoteReaderSession(c, http.StatusOK, session)
}

func (s *Server) remoteReaderSessionChapterContent(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	session, ok := s.lookupRemoteReaderSession(c, userID)
	if !ok {
		return
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil || index < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter index"})
		return
	}
	chapterPosition := -1
	for position, chapter := range session.Chapters {
		if chapter.Index == index {
			chapterPosition = position
			break
		}
	}
	if chapterPosition < 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "chapter not found"})
		return
	}
	chapter := session.Chapters[chapterPosition]
	nextChapterURL := ""
	if session.Source.SourceType != 1 && chapterPosition+1 < len(session.Chapters) {
		nextChapterURL = session.Chapters[chapterPosition+1].URL
	}
	content, variableState, err := engine.FetchChapterContentContextWithState(c.Request.Context(), chapter.URL, nextChapterURL, session.Source, engine.SourceRuleVariableState{
		BookVariable:    session.Book.Variable,
		ChapterVariable: chapter.Variable,
		BookName:        session.Book.Title,
		ChapterTitle:    chapter.Title,
	})
	if err != nil {
		s.recordSourceFailure(userID, session.Source, err)
		if errors.Is(err, context.Canceled) {
			return
		}
		writeSourceError(c, http.StatusBadGateway, "failed to load chapter content", err, "content")
		return
	}
	s.remoteReaders.updateVariables(userID, session.ID, variableState.BookVariable, chapter.Index, variableState.ChapterVariable)
	if session.Book.Type != 1 {
		content = s.applyUserReplaceRules(session.Book, content)
	}
	response := gin.H{"chapter": chapter, "content": content, "format": "text"}
	if session.Book.Type == 1 {
		prepared, prepareErr := audioreader.PrepareDirectOrLocal(s.audioReader, session.Book, &chapter, content)
		if prepareErr != nil {
			writeAudioChapterPrepareError(c, prepareErr)
			return
		}
		response["content"] = prepared.ResourceURL
		response["format"] = "audio"
		response["resourceUrl"] = prepared.ResourceURL
		response["resourceExpiresAt"] = prepared.ExpiresAt.UTC().Format(time.RFC3339)
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, response)
}

func (s *Server) lookupRemoteReaderSession(c *gin.Context, userID uint) (remoteReaderSession, bool) {
	session, err := s.remoteReaders.get(userID, c.Param("id"))
	switch {
	case err == nil:
		return session, true
	case errors.Is(err, errRemoteReaderSessionExpired):
		c.JSON(http.StatusGone, gin.H{"error": "remote reader session expired"})
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "remote reader session not found"})
	}
	return remoteReaderSession{}, false
}

func writeRemoteReaderSession(c *gin.Context, status int, session remoteReaderSession) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{
		"id":        session.ID,
		"expiresAt": session.ExpiresAt.UTC().Format(time.RFC3339),
		"book":      session.Book,
		"chapters":  session.Chapters,
	})
}
