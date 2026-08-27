package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/audioreader"
	"openreader/backend/services/booksources"
	"openreader/backend/services/remotereader"
)

const remoteReaderSessionPayloadBytes int64 = 64 << 10

var errRemoteReaderPayloadTooLarge = errors.New("remote reader payload too large")

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
	if err := decodeRemoteReaderSessionRequest(c, &req); err != nil {
		if errors.Is(err, errRemoteReaderPayloadTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "remote reader payload too large"})
			return
		}
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

	source, err := s.bookSources.FindActive(userID, req.SourceID)
	if errors.Is(err, booksources.ErrSourceNotFound) || err == nil && !source.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}

	ctx := c.Request.Context()
	remoteInfo, remoteChapters, variable, err := engine.FetchBookInfoAndTOCWithVariablesContext(ctx, req.BookURL, source, variable, req.Title, nil)
	if err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		s.recordSourceFailure(userID, source, err)
		writeSourceError(c, http.StatusBadGateway, "failed to fetch chapters", err, "book_info")
		return
	}
	if ctx.Err() != nil {
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
	session, err := s.remoteReaders.Create(userID, source, book, chapters)
	if err != nil {
		if errors.Is(err, remotereader.ErrTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "remote reader session too large"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create remote reader session"})
		return
	}
	s.writeRemoteReaderSession(c, http.StatusCreated, session)
}

func decodeRemoteReaderSessionRequest(c *gin.Context, target *remoteReaderSessionRequest) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, remoteReaderSessionPayloadBytes)
	if c.Request.ContentLength > remoteReaderSessionPayloadBytes {
		return errRemoteReaderPayloadTooLarge
	}
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRemoteReaderPayloadTooLarge
		}
		return err
	}
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return errRemoteReaderPayloadTooLarge
	}
	if err == nil {
		return errors.New("remote reader payload contains multiple JSON values")
	}
	return err
}

func (s *Server) getRemoteReaderSession(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	session, ok := s.lookupRemoteReaderSession(c, userID)
	if !ok {
		return
	}
	s.writeRemoteReaderSession(c, http.StatusOK, session)
}

func (s *Server) remoteReaderSessionChapterContent(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil || index < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter index"})
		return
	}
	session, ok := s.lookupRemoteReaderSession(c, userID)
	if !ok {
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
		if errors.Is(err, context.Canceled) {
			return
		}
		s.recordSourceFailure(userID, session.Source, err)
		writeSourceError(c, http.StatusBadGateway, "failed to load chapter content", err, "content")
		return
	}
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
	if err := s.remoteReaders.UpdateVariables(userID, session.ID, variableState.BookVariable, chapter.Index, variableState.ChapterVariable); err != nil {
		s.writeRemoteReaderSessionStoreError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, response)
}

func (s *Server) lookupRemoteReaderSession(c *gin.Context, userID uint) (remotereader.Session, bool) {
	session, err := s.remoteReaders.Get(userID, c.Param("id"))
	switch {
	case err == nil:
		return session, true
	case errors.Is(err, remotereader.ErrExpired):
		c.JSON(http.StatusGone, gin.H{"error": "remote reader session expired"})
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "remote reader session not found"})
	}
	return remotereader.Session{}, false
}

func (s *Server) writeRemoteReaderSessionStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, remotereader.ErrTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "remote reader session too large"})
	case errors.Is(err, remotereader.ErrExpired):
		c.JSON(http.StatusGone, gin.H{"error": "remote reader session expired"})
	case errors.Is(err, remotereader.ErrMissing):
		c.JSON(http.StatusNotFound, gin.H{"error": "remote reader session not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update remote reader session"})
	}
}

func (s *Server) writeRemoteReaderSession(c *gin.Context, status int, session remotereader.Session) {
	book := struct {
		models.Book
		CoverResourceURL *string `json:"coverResourceUrl,omitempty"`
	}{
		Book:             session.Book,
		CoverResourceURL: s.projectCoverResource(session.UserID, session.Source.ID, session.Book.CoverURL),
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{
		"id":        session.ID,
		"expiresAt": session.ExpiresAt.UTC().Format(time.RFC3339),
		"book":      book,
		"chapters":  session.Chapters,
	})
}
