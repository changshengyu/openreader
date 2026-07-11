package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"slices"
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

func (s *Server) previewTXTImport(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	fileName, ext, data, importToken, err := s.readLocalImportPayload(c, userID, true)
	if err != nil {
		writeLocalImportError(c, err)
		return
	}
	preview, err := localbook.NewImporter(s.cfg, s.db).Preview(localbook.ImportRequest{
		FileName:  fileName,
		Extension: ext,
		Data:      data,
		Title:     c.PostForm("title"),
		Author:    c.PostForm("author"),
		TOCRule:   c.PostForm("tocRule"),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "importToken": importToken})
		return
	}
	preview.ImportToken = importToken
	c.JSON(http.StatusOK, preview)
}

func (s *Server) importTXT(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	fileName, ext, data, importToken, err := s.readLocalImportPayload(c, userID, false)
	if err != nil {
		writeLocalImportError(c, err)
		return
	}
	if ext != ".txt" && ext != ".text" && ext != ".md" && ext != ".epub" && ext != ".pdf" && ext != ".umd" && ext != ".cbz" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only txt/text/md/epub/pdf/umd/cbz files are supported"})
		return
	}

	categoryIDs := parseOptionalCategoryIDs(c.PostFormArray("categoryIds"))
	categoryID := parseOptionalCategoryID(c.PostForm("categoryId"))
	if len(categoryIDs) > 0 {
		if !s.validateCategoryIDs(c, userID, categoryIDs) {
			return
		}
		categoryID = &categoryIDs[0]
	} else if !s.validateCategory(c, userID, categoryID) {
		return
	} else if categoryID != nil {
		categoryIDs = []uint{*categoryID}
	}
	userName, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}

	importer := localbook.NewImporter(s.cfg, s.db)
	book, err := importer.Import(localbook.ImportRequest{
		UserID:     userID,
		UserName:   userName,
		FileName:   fileName,
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
	if len(categoryIDs) > 0 {
		_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
	}
	if importToken != "" {
		s.removeStagedLocalImport(userID, importToken)
	}

	c.JSON(http.StatusCreated, s.broadcastBookShelfUpdate(userID, book))
}

func writeLocalImportError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, errLocalImportTooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func (s *Server) readLocalImportPayload(c *gin.Context, userID uint, createStage bool) (string, string, []byte, string, error) {
	importToken := strings.TrimSpace(c.PostForm("importToken"))
	if importToken != "" {
		metadata, data, err := s.loadStagedLocalImport(userID, importToken)
		if err != nil {
			return "", "", nil, "", err
		}
		return metadata.FileName, metadata.Extension, data, importToken, nil
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return "", "", nil, "", errors.New("file or importToken is required")
	}
	if fileHeader.Size > s.maxLocalImportBytes() {
		return "", "", nil, "", errLocalImportTooLarge
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	file, err := fileHeader.Open()
	if err != nil {
		return "", "", nil, "", errors.New("failed to open file")
	}
	defer file.Close()
	data, err := s.readBoundedLocalImport(file)
	if err != nil {
		if errors.Is(err, errLocalImportTooLarge) {
			return "", "", nil, "", err
		}
		return "", "", nil, "", errors.New("failed to read file")
	}
	if !createStage {
		return fileHeader.Filename, ext, data, "", nil
	}
	importToken, err = s.stageLocalImport(userID, fileHeader.Filename, ext, data)
	if err != nil {
		return "", "", nil, "", errors.New("failed to stage import")
	}
	return fileHeader.Filename, ext, data, importToken, nil
}

func parseOptionalCategoryIDs(values []string) []uint {
	result := make([]uint, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if id := parseOptionalCategoryID(part); id != nil && !slices.Contains(result, *id) {
				result = append(result, *id)
			}
		}
	}
	return result
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

type localBookImportItem struct {
	Path        string `json:"path"`
	ImportToken string `json:"importToken"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	TOCRule     string `json:"tocRule"`
}

type localBookImportRequest struct {
	Paths       []string              `json:"paths"`
	Items       []localBookImportItem `json:"items"`
	CategoryID  *uint                 `json:"categoryId"`
	CategoryIDs []uint                `json:"categoryIds"`
}

func (request localBookImportRequest) requestedPaths() []string {
	if len(request.Items) == 0 {
		return request.Paths
	}
	paths := make([]string, 0, len(request.Items))
	for _, item := range request.Items {
		if path := strings.TrimSpace(item.Path); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func (request localBookImportRequest) itemByPath() map[string]localBookImportItem {
	items := make(map[string]localBookImportItem, len(request.Items))
	for _, item := range request.Items {
		if path := cleanRelativePath(item.Path); path != "" {
			items[path] = item
		}
	}
	return items
}

// previewStagedStorageImport fixes the source bytes at preview time for files
// coming from LocalStore or WebDAV. Confirm/import can then safely reparse the
// user's edited TOC rule without depending on a mutable mounted file.
func (s *Server) previewStagedStorageImport(userID uint, file localStoreImportFile, override localBookImportItem) (localbook.PreviewResult, string, error) {
	data, err := s.readBoundedLocalImportFile(file.filePath)
	if err != nil {
		return localbook.PreviewResult{}, "", err
	}
	importToken, err := s.stageLocalImport(userID, filepath.Base(file.filePath), file.extension, data)
	if err != nil {
		return localbook.PreviewResult{}, "", errors.New("failed to stage import")
	}
	preview, err := localbook.NewImporter(s.cfg, s.db).Preview(localbook.ImportRequest{
		FileName:  filepath.Base(file.filePath),
		Extension: file.extension,
		Data:      data,
		Title:     override.Title,
		Author:    override.Author,
		TOCRule:   override.TOCRule,
	})
	if err != nil {
		return localbook.PreviewResult{}, importToken, err
	}
	preview.ImportToken = importToken
	return preview, importToken, nil
}

// reparseStagedStorageImport keeps the immutable preview snapshot authoritative
// when a user changes the TOC rule. In particular, it must not fall back to a
// mutable LocalStore/WebDAV path after the preview has already succeeded.
func (s *Server) reparseStagedStorageImport(userID uint, importToken string, override localBookImportItem) (localbook.PreviewResult, string, error) {
	metadata, data, err := s.loadStagedLocalImport(userID, importToken)
	if err != nil {
		return localbook.PreviewResult{}, "", err
	}
	preview, err := localbook.NewImporter(s.cfg, s.db).Preview(localbook.ImportRequest{
		FileName:  metadata.FileName,
		Extension: metadata.Extension,
		Data:      data,
		Title:     override.Title,
		Author:    override.Author,
		TOCRule:   override.TOCRule,
	})
	if err != nil {
		return localbook.PreviewResult{}, importToken, err
	}
	preview.ImportToken = importToken
	return preview, importToken, nil
}

func (s *Server) stagedStorageImportRequest(userID uint, userName string, importToken string, override localBookImportItem, categoryID *uint) (localbook.ImportRequest, error) {
	metadata, data, err := s.loadStagedLocalImport(userID, importToken)
	if err != nil {
		return localbook.ImportRequest{}, err
	}
	return localbook.ImportRequest{
		UserID:     userID,
		UserName:   userName,
		FileName:   metadata.FileName,
		Extension:  metadata.Extension,
		Data:       data,
		Title:      override.Title,
		Author:     override.Author,
		CategoryID: categoryID,
		TOCRule:    override.TOCRule,
	}, nil
}
