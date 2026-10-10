package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/importstage"
	"openreader/backend/services/localbook"
)

func (s *Server) listTXTTocRules(c *gin.Context) {
	c.JSON(http.StatusOK, engine.DefaultTXTTocRules())
}

func (s *Server) previewTXTImport(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	payload, err := s.parseLocalImportMultipart(c, true)
	defer func() {
		if payload != nil && payload.stage != nil {
			payload.stage.Close()
		}
	}()
	if payload != nil && payload.form != nil {
		defer func() {
			_ = payload.form.RemoveAll()
		}()
	}
	if err != nil {
		writeLocalImportRequestError(c, err)
		return
	}
	fileName, ext, data, importToken, err := s.readLocalImportPayload(c.Request.Context(), payload, userID, true)
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
	if err := payload.stage.SavePrepared(prepared); err != nil {
		if writeLocalImportStageCancellation(c, err, importToken) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stage parsed import", "importToken": importToken})
		return
	}
	preview.ImportToken = importToken
	c.JSON(http.StatusOK, preview)
}

func (s *Server) importTXT(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	payload, err := s.parseLocalImportMultipart(c, false)
	defer func() {
		if payload != nil && payload.stage != nil {
			payload.stage.Close()
		}
	}()
	if payload != nil && payload.form != nil {
		defer func() {
			_ = payload.form.RemoveAll()
		}()
	}
	if err != nil {
		writeLocalImportRequestError(c, err)
		return
	}
	fileName, ext, data, importToken, err := s.readLocalImportPayload(c.Request.Context(), payload, userID, false)
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
		book, err = payload.stage.ImportBook(importer, request)
	} else {
		book, err = importer.Import(request)
	}
	if err != nil {
		if writeLocalImportStageCancellation(c, err, importToken) {
			return
		}
		if errors.Is(err, errInvalidLocalImportToken) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
		_ = payload.stage.Consume()
	}

	c.JSON(http.StatusCreated, s.broadcastBookShelfUpdate(userID, book))
}

func writeLocalImportError(c *gin.Context, err error) {
	if writeLocalImportStageCancellation(c, err, "") {
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, errLocalImportTooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func isLocalImportStageCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
func writeLocalImportStageCancellation(c *gin.Context, err error, token string) bool {
	if !isLocalImportStageCancellation(err) {
		return false
	}
	response := gin.H{"error": "local import stage canceled"}
	if validLocalImportToken(token) {
		response["importToken"] = token
	}
	c.JSON(http.StatusInternalServerError, response)
	return true
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
	ctx context.Context,
	userID uint,
	fileName string,
	extension string,
	data []byte,
	override localBookImportItem,
) (localbook.PreviewResult, string, error) {
	stage, err := s.localImportStages().Create(ctx, userID, fileName, extension, data)
	if err != nil {
		if isLocalImportStageCancellation(err) {
			return localbook.PreviewResult{}, "", err
		}
		return localbook.PreviewResult{}, "", errors.New("failed to stage import")
	}
	defer stage.Close()
	importToken := stage.Token
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
	if err := stage.SavePrepared(prepared); err != nil {
		if isLocalImportStageCancellation(err) {
			return localbook.PreviewResult{}, importToken, err
		}
		return localbook.PreviewResult{}, importToken, importstage.ErrPreparedWrite
	}
	preview.ImportToken = importToken
	return preview, importToken, nil
}

// reparseStagedStorageImport keeps the immutable preview snapshot authoritative
// when a user changes the TOC rule. In particular, it must not fall back to a
// mutable LocalStore/WebDAV path after the preview has already succeeded.
func (s *Server) reparseStagedStorageImport(ctx context.Context, userID uint, importToken string, override localBookImportItem) (localbook.PreviewResult, string, error) {
	stage, err := s.localImportStages().Open(ctx, userID, importToken)
	if err != nil {
		return localbook.PreviewResult{}, "", err
	}
	defer stage.Close()
	metadata, data := stage.Metadata, stage.Data
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
	if err := stage.SavePrepared(prepared); err != nil {
		if isLocalImportStageCancellation(err) {
			return localbook.PreviewResult{}, importToken, err
		}
		return localbook.PreviewResult{}, importToken, importstage.ErrPreparedWrite
	}
	preview.ImportToken = importToken
	return preview, importToken, nil
}

func (s *Server) importStagedStorageBook(ctx context.Context, userID uint, userName string, importToken string, override localBookImportItem, categoryID *uint, importer localbook.Importer) (models.Book, error) {
	stage, err := s.localImportStages().Open(ctx, userID, importToken)
	if err != nil {
		return models.Book{}, err
	}
	defer stage.Close()
	request := localbook.ImportRequest{
		UserID:     userID,
		UserName:   userName,
		FileName:   stage.Metadata.FileName,
		Extension:  stage.Metadata.Extension,
		Data:       stage.Data,
		Title:      override.Title,
		Author:     override.Author,
		CategoryID: categoryID,
		TOCRule:    override.TOCRule,
	}
	book, err := stage.ImportBook(importer, request)
	if err == nil {
		_ = stage.Consume()
	}
	return book, err
}
