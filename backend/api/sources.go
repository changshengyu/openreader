package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func (s *Server) listSources(c *gin.Context) {
	var sources []models.BookSource
	if err := s.db.Order("created_at desc").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}
	c.JSON(http.StatusOK, sources)
}

type bookSourcePayload struct {
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	SearchURL string `json:"searchUrl"`
	Charset   string `json:"charset"`
	Rules     string `json:"rules"`
	Enabled   *bool  `json:"enabled"`
	Group     string `json:"group"`
}

func (p bookSourcePayload) toModel() models.BookSource {
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	return models.BookSource{
		Name:      strings.TrimSpace(p.Name),
		BaseURL:   strings.TrimSpace(p.BaseURL),
		SearchURL: strings.TrimSpace(p.SearchURL),
		Charset:   strings.TrimSpace(p.Charset),
		Rules:     strings.TrimSpace(p.Rules),
		Enabled:   enabled,
		Group:     strings.TrimSpace(p.Group),
	}
}

func (s *Server) createSource(c *gin.Context) {
	var req bookSourcePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source payload"})
		return
	}
	source := req.toModel()
	if source.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source name is required"})
		return
	}
	if source.Charset == "" {
		source.Charset = "utf-8"
	}

	if err := s.db.Select("Name", "BaseURL", "SearchURL", "Charset", "Rules", "Enabled", "Group").Create(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create source"})
		return
	}
	c.JSON(http.StatusCreated, source)
}

func (s *Server) updateSource(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}

	var req models.BookSource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source payload"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source name is required"})
		return
	}
	source.Name = req.Name
	source.BaseURL = strings.TrimSpace(req.BaseURL)
	source.SearchURL = strings.TrimSpace(req.SearchURL)
	source.Charset = strings.TrimSpace(req.Charset)
	if source.Charset == "" {
		source.Charset = "utf-8"
	}
	source.Rules = strings.TrimSpace(req.Rules)
	source.Group = strings.TrimSpace(req.Group)
	source.Enabled = req.Enabled

	if err := s.db.Save(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update source"})
		return
	}
	c.JSON(http.StatusOK, source)
}

func (s *Server) deleteSource(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	result := s.db.Delete(&models.BookSource{}, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete source"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) clearSources(c *gin.Context) {
	result := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.BookSource{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear sources"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"affected": result.RowsAffected})
}

type batchSourcesRequest struct {
	Action    string `json:"action" binding:"required"`
	SourceIDs []uint `json:"sourceIds" binding:"required"`
}

func (s *Server) batchSources(c *gin.Context) {
	var req batchSourcesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action and sourceIds are required"})
		return
	}
	if len(req.SourceIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourceIds is required"})
		return
	}
	if len(req.SourceIDs) > 300 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many sources"})
		return
	}

	var result *gorm.DB
	switch req.Action {
	case "enable":
		result = s.db.Model(&models.BookSource{}).Where("id IN ?", req.SourceIDs).Update("enabled", true)
	case "disable":
		result = s.db.Model(&models.BookSource{}).Where("id IN ?", req.SourceIDs).Update("enabled", false)
	case "delete":
		result = s.db.Where("id IN ?", req.SourceIDs).Delete(&models.BookSource{})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported batch action"})
		return
	}
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update sources"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"affected": result.RowsAffected})
}

func (s *Server) importSources(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
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

	sources, err := decodeBookSources(data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON format"})
		return
	}

	result := s.importBookSources(sources)
	c.JSON(http.StatusOK, result)
}

func (s *Server) exportSources(c *gin.Context) {
	var sources []models.BookSource
	if err := s.db.Order("id asc").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=bookSources.json")
	c.JSON(http.StatusOK, sources)
}

type remoteSourceRequest struct {
	URL string `json:"url" binding:"required"`
}

func (s *Server) importRemoteSource(c *gin.Context) {
	var req remoteSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	sources, err := fetchRemoteBookSources(req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result := s.importBookSources(sources)
	c.JSON(http.StatusOK, result)
}

func (s *Server) previewRemoteSource(c *gin.Context) {
	var req remoteSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	sources, err := fetchRemoteBookSources(req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		if name := strings.TrimSpace(source.Name); name != "" {
			names = append(names, name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": len(sources), "names": names})
}

func fetchRemoteBookSources(rawURL string) ([]models.BookSource, error) {
	text, err := engine.FetchText(rawURL, "utf-8")
	if err != nil {
		return nil, errors.New("failed to fetch remote source URL")
	}
	sources, err := decodeBookSources([]byte(text))
	if err != nil {
		return nil, errors.New("invalid remote JSON format")
	}
	return sources, nil
}

func (s *Server) importBookSources(sources []models.BookSource) gin.H {
	imported := 0
	updated := 0
	skipped := 0
	seen := make(map[string]bool)
	for _, source := range sources {
		source.ID = 0
		source.Name = strings.TrimSpace(source.Name)
		if source.Name == "" || seen[source.Name] {
			skipped++
			continue
		}
		seen[source.Name] = true
		source.BaseURL = strings.TrimSpace(source.BaseURL)
		source.SearchURL = strings.TrimSpace(source.SearchURL)
		source.Rules = strings.TrimSpace(source.Rules)
		source.Group = strings.TrimSpace(source.Group)
		source.Charset = strings.TrimSpace(source.Charset)
		if source.Charset == "" {
			source.Charset = "utf-8"
		}

		var existing models.BookSource
		if err := s.db.Where("name = ?", source.Name).First(&existing).Error; err == nil {
			existing.BaseURL = source.BaseURL
			existing.SearchURL = source.SearchURL
			existing.Charset = source.Charset
			existing.Rules = source.Rules
			existing.Enabled = source.Enabled
			existing.Group = source.Group
			if err := s.db.Save(&existing).Error; err == nil {
				updated++
				continue
			}
			skipped++
			continue
		}

		if err := s.db.Create(&source).Error; err != nil {
			skipped++
			continue
		}
		imported++
	}
	return gin.H{"imported": imported, "updated": updated, "skipped": skipped}
}

func decodeBookSources(data []byte) ([]models.BookSource, error) {
	var payloads []bookSourcePayload
	if err := json.Unmarshal(data, &payloads); err == nil {
		return bookSourcePayloadsToModels(payloads), nil
	}

	var wrapper struct {
		BookSources []bookSourcePayload `json:"bookSources"`
		Sources     []bookSourcePayload `json:"sources"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil {
		if len(wrapper.BookSources) > 0 {
			return bookSourcePayloadsToModels(wrapper.BookSources), nil
		}
		if len(wrapper.Sources) > 0 {
			return bookSourcePayloadsToModels(wrapper.Sources), nil
		}
	}

	var payload bookSourcePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	source := payload.toModel()
	if source.Name == "" {
		return nil, errors.New("no source entries")
	}
	return []models.BookSource{source}, nil
}

func bookSourcePayloadsToModels(payloads []bookSourcePayload) []models.BookSource {
	sources := make([]models.BookSource, 0, len(payloads))
	for _, payload := range payloads {
		sources = append(sources, payload.toModel())
	}
	return sources
}

// getSource returns a single book source by ID.
func (s *Server) getSource(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var source models.BookSource
	err := s.db.First(&source, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}
	c.JSON(http.StatusOK, source)
}
