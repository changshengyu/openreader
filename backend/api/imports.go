package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/localbook"
)

func (s *Server) listTXTTocRules(c *gin.Context) {
	c.JSON(http.StatusOK, engine.DefaultTXTTocRules())
}

func (s *Server) previewTXTImport(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	payload, err := s.parseLocalImportMultipart(c, true)
	if payload != nil && payload.form != nil {
		defer func() {
			_ = payload.form.RemoveAll()
		}()
	}
	if err != nil {
		writeLocalImportRequestError(c, err)
		return
	}
	fileName, ext, data, importToken, err := s.readLocalImportPayload(payload, userID, true)
	if err != nil {
		writeLocalImportError(c, err)
		return
	}
	request := localbook.ImportRequest{
		FileName:  fileName,
		Extension: ext,
		Data:      data,
		Title:     payload.title,
		Author:    payload.author,
		TOCRule:   payload.tocRule,
	}
	preview, prepared, err := localbook.NewImporter(s.cfg, s.db).Prepare(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "importToken": importToken})
		return
	}
	if err := s.saveStagedPreparedImport(userID, importToken, prepared); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stage parsed import", "importToken": importToken})
		return
	}
	preview.ImportToken = importToken
	c.JSON(http.StatusOK, preview)
}

func (s *Server) importTXT(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	payload, err := s.parseLocalImportMultipart(c, false)
	if payload != nil && payload.form != nil {
		defer func() {
			_ = payload.form.RemoveAll()
		}()
	}
	if err != nil {
		writeLocalImportRequestError(c, err)
		return
	}
	fileName, ext, data, importToken, err := s.readLocalImportPayload(payload, userID, false)
	if err != nil {
		writeLocalImportError(c, err)
		return
	}
	if ext != ".txt" && ext != ".text" && ext != ".md" && ext != ".epub" && ext != ".pdf" && ext != ".umd" && ext != ".cbz" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only txt/text/md/epub/pdf/umd/cbz files are supported"})
		return
	}

	categoryIDs := payload.categoryIDs
	categoryID := payload.categoryID
	if categoryID != nil && !s.validateCategory(c, userID, categoryID) {
		return
	}
	if len(categoryIDs) > 0 {
		if !s.validateCategoryIDs(c, userID, categoryIDs) {
			return
		}
		categoryID = &categoryIDs[0]
	} else if categoryID != nil {
		categoryIDs = []uint{*categoryID}
	}
	userName, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}

	importer := localbook.NewImporter(s.cfg, s.db)
	request := localbook.ImportRequest{
		UserID:     userID,
		UserName:   userName,
		FileName:   fileName,
		Extension:  ext,
		Data:       data,
		Title:      payload.title,
		Author:     payload.author,
		CategoryID: categoryID,
		TOCRule:    payload.tocRule,
	}
	var book models.Book
	if importToken != "" {
		book, err = s.importStagedLocalBook(userID, importToken, importer, request)
	} else {
		book, err = importer.Import(request)
	}
	if err != nil {
		if errors.Is(err, localbook.ErrUnsupportedFormat) ||
			errors.Is(err, localbook.ErrParseFailed) {
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

func (s *Server) previewStagedStorageImportData(
	userID uint,
	fileName string,
	extension string,
	data []byte,
	override localBookImportItem,
) (localbook.PreviewResult, string, error) {
	importToken, err := s.stageLocalImport(userID, fileName, extension, data)
	if err != nil {
		return localbook.PreviewResult{}, "", errors.New("failed to stage import")
	}
	request := localbook.ImportRequest{
		FileName:  fileName,
		Extension: extension,
		Data:      data,
		Title:     override.Title,
		Author:    override.Author,
		TOCRule:   override.TOCRule,
	}
	preview, prepared, err := localbook.NewImporter(s.cfg, s.db).Prepare(request)
	if err != nil {
		return localbook.PreviewResult{}, importToken, err
	}
	if err := s.saveStagedPreparedImport(userID, importToken, prepared); err != nil {
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
	request := localbook.ImportRequest{
		FileName:  metadata.FileName,
		Extension: metadata.Extension,
		Data:      data,
		Title:     override.Title,
		Author:    override.Author,
		TOCRule:   override.TOCRule,
	}
	preview, prepared, err := localbook.NewImporter(s.cfg, s.db).Prepare(request)
	if err != nil {
		return localbook.PreviewResult{}, importToken, err
	}
	if err := s.saveStagedPreparedImport(userID, importToken, prepared); err != nil {
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

func (s *Server) importStagedLocalBook(userID uint, importToken string, importer localbook.Importer, request localbook.ImportRequest) (models.Book, error) {
	if prepared, ok := s.loadStagedPreparedImport(userID, importToken, request); ok {
		return importer.ImportPrepared(request, prepared)
	}
	_, prepared, err := importer.Prepare(request)
	if err != nil {
		return models.Book{}, err
	}
	if err := s.saveStagedPreparedImport(userID, importToken, prepared); err != nil {
		return models.Book{}, err
	}
	return importer.ImportPrepared(request, prepared)
}
