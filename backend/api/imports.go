package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/services/localbook"
)

func (s *Server) listTXTTocRules(c *gin.Context) {
	c.JSON(http.StatusOK, engine.DefaultTXTTocRules())
}

func (s *Server) importTXT(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".txt" && ext != ".text" && ext != ".md" && ext != ".epub" && ext != ".pdf" && ext != ".umd" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only txt/text/md/epub/pdf/umd files are supported"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open file"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file"})
		return
	}

	categoryID := parseOptionalCategoryID(c.PostForm("categoryId"))
	if !s.validateCategory(c, userID, categoryID) {
		return
	}
	userName, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}

	importer := localbook.NewImporter(s.cfg, s.db)
	book, err := importer.Import(localbook.ImportRequest{
		UserID:     userID,
		UserName:   userName,
		FileName:   fileHeader.Filename,
		Extension:  ext,
		Data:       data,
		Title:      c.PostForm("title"),
		Author:     c.PostForm("author"),
		CategoryID: categoryID,
		TOCRule:    c.PostForm("tocRule"),
	})
	if err != nil {
		if errors.Is(err, localbook.ErrUnsupportedFormat) ||
			errors.Is(err, localbook.ErrParseFailed) ||
			errors.Is(err, localbook.ErrNoReadableChapters) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import book"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": book})
	c.JSON(http.StatusCreated, book)
}

func parseOptionalCategoryID(value string) *uint {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return nil
	}
	result := uint(parsed)
	return &result
}
