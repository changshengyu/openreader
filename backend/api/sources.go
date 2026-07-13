package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

func (s *Server) listSources(c *gin.Context) {
	var sources []models.BookSource
	if err := s.db.Order("custom_order asc, id asc").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}
	s.attachBookSourceUsage(sources)
	c.JSON(http.StatusOK, sources)
}

type bookSourcePayload struct {
	Name              string                   `json:"name"`
	BaseURL           string                   `json:"baseUrl"`
	SearchURL         string                   `json:"searchUrl"`
	Charset           string                   `json:"charset"`
	ConcurrentRate    string                   `json:"concurrentRate"`
	LoginURL          string                   `json:"loginUrl"`
	LoginCheckJS      string                   `json:"loginCheckJs"`
	CustomOrder       int                      `json:"customOrder"`
	LastUpdateTime    int64                    `json:"lastUpdateTime"`
	Weight            int                      `json:"weight"`
	RespondTime       *int64                   `json:"respondTime"`
	Rules             string                   `json:"rules"`
	Enabled           *bool                    `json:"enabled"`
	EnabledExplore    *bool                    `json:"enabledExplore"`
	Group             string                   `json:"group"`
	BookSourceName    string                   `json:"bookSourceName"`
	BookSourceURL     string                   `json:"bookSourceUrl"`
	BookURLPattern    string                   `json:"bookUrlPattern"`
	RuleURLPattern    string                   `json:"ruleBookUrlPattern"`
	BookSourceType    int                      `json:"bookSourceType"`
	BookSourceComment string                   `json:"bookSourceComment"`
	BookSourceGroup   string                   `json:"bookSourceGroup"`
	ExploreURL        string                   `json:"exploreUrl"`
	Header            string                   `json:"header"`
	HeaderMap         json.RawMessage          `json:"headerMap"`
	RuleSearch        legacySourceSearchRule   `json:"ruleSearch"`
	RuleExplore       legacySourceSearchRule   `json:"ruleExplore"`
	RuleBookInfo      legacySourceBookInfoRule `json:"ruleBookInfo"`
	RuleTOC           legacySourceTOCRule      `json:"ruleToc"`
	RuleContent       legacySourceContentRule  `json:"ruleContent"`
}

type legacySourceSearchRule struct {
	BookList    string `json:"bookList"`
	Name        string `json:"name"`
	Author      string `json:"author"`
	CoverURL    string `json:"coverUrl"`
	Intro       string `json:"intro"`
	Kind        string `json:"kind"`
	WordCount   string `json:"wordCount"`
	LastChapter string `json:"lastChapter"`
	UpdateTime  string `json:"updateTime"`
	BookURL     string `json:"bookUrl"`
}

type legacySourceTOCRule struct {
	PreUpdateJS string `json:"preUpdateJs,omitempty"`
	ChapterList string `json:"chapterList"`
	ChapterName string `json:"chapterName"`
	ChapterURL  string `json:"chapterUrl"`
	IsVolume    string `json:"isVolume,omitempty"`
	IsVIP       string `json:"isVip,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
	NextTOCURL  string `json:"nextTocUrl,omitempty"`
}

type legacySourceBookInfoRule struct {
	Init        string `json:"init,omitempty"`
	Name        string `json:"name,omitempty"`
	Author      string `json:"author,omitempty"`
	CoverURL    string `json:"coverUrl,omitempty"`
	Intro       string `json:"intro,omitempty"`
	Kind        string `json:"kind,omitempty"`
	LastChapter string `json:"lastChapter,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
	WordCount   string `json:"wordCount,omitempty"`
	TOCURL      string `json:"tocUrl,omitempty"`
	CanRename   string `json:"canReName,omitempty"`
}

type legacySourceContentRule struct {
	Content        string `json:"content"`
	NextContentURL string `json:"nextContentUrl,omitempty"`
	WebJS          string `json:"webJs,omitempty"`
	SourceRegex    string `json:"sourceRegex,omitempty"`
	ReplaceRegex   string `json:"replaceRegex,omitempty"`
	ImageStyle     string `json:"imageStyle,omitempty"`
}

type exportedBookSource struct {
	BookSourceName    string                   `json:"bookSourceName"`
	BookSourceGroup   string                   `json:"bookSourceGroup,omitempty"`
	BookSourceURL     string                   `json:"bookSourceUrl"`
	BookSourceType    int                      `json:"bookSourceType"`
	BookURLPattern    string                   `json:"bookUrlPattern,omitempty"`
	BookSourceComment string                   `json:"bookSourceComment,omitempty"`
	Enabled           bool                     `json:"enabled"`
	EnabledExplore    bool                     `json:"enabledExplore"`
	SearchURL         string                   `json:"searchUrl,omitempty"`
	ExploreURL        string                   `json:"exploreUrl,omitempty"`
	Header            string                   `json:"header,omitempty"`
	RuleSearch        legacySourceSearchRule   `json:"ruleSearch"`
	RuleExplore       legacySourceSearchRule   `json:"ruleExplore"`
	RuleBookInfo      legacySourceBookInfoRule `json:"ruleBookInfo"`
	RuleTOC           legacySourceTOCRule      `json:"ruleToc"`
	RuleContent       legacySourceContentRule  `json:"ruleContent"`
	Charset           string                   `json:"charset,omitempty"`
	ConcurrentRate    string                   `json:"concurrentRate,omitempty"`
	LoginURL          string                   `json:"loginUrl,omitempty"`
	LoginCheckJS      string                   `json:"loginCheckJs,omitempty"`
	CustomOrder       int                      `json:"customOrder"`
	LastUpdateTime    int64                    `json:"lastUpdateTime"`
	Weight            int                      `json:"weight"`
	RespondTime       int64                    `json:"respondTime"`
	Rules             string                   `json:"rules,omitempty"`
}

func (p bookSourcePayload) toModel() models.BookSource {
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	enabledExplore := true
	if p.EnabledExplore != nil {
		enabledExplore = *p.EnabledExplore
	}
	respondTime := int64(180000)
	if p.RespondTime != nil {
		respondTime = *p.RespondTime
	}
	rules := strings.TrimSpace(p.Rules)
	if rules == "" {
		rules = p.compatRules()
	}
	charset := strings.TrimSpace(p.Charset)
	if charset == "" && (strings.TrimSpace(p.BookSourceName) != "" || strings.TrimSpace(p.BookSourceURL) != "") {
		charset = "auto"
	}
	return models.BookSource{
		Name:           firstNonBlank(p.Name, p.BookSourceName),
		BaseURL:        firstNonBlank(p.BaseURL, p.BookSourceURL),
		BookURLPattern: firstNonBlank(p.BookURLPattern, p.RuleURLPattern),
		SourceType:     p.BookSourceType,
		Comment:        strings.TrimSpace(p.BookSourceComment),
		SearchURL:      normalizeUpstreamURLTemplate(p.SearchURL),
		Charset:        charset,
		ConcurrentRate: strings.TrimSpace(p.ConcurrentRate),
		Header:         p.rawHeader(),
		LoginURL:       strings.TrimSpace(p.LoginURL),
		LoginCheckJS:   strings.TrimSpace(p.LoginCheckJS),
		CustomOrder:    p.CustomOrder,
		LastUpdateTime: p.LastUpdateTime,
		Weight:         p.Weight,
		RespondTime:    respondTime,
		Rules:          rules,
		Enabled:        enabled,
		EnabledExplore: &enabledExplore,
		Group:          firstNonBlank(p.Group, p.BookSourceGroup),
	}
}

func (p bookSourcePayload) compatRules() string {
	rule := models.BookSourceRule{
		SearchURL:                 normalizeUpstreamURLTemplate(p.SearchURL),
		ExploreURL:                normalizeUpstreamURLTemplate(p.ExploreURL),
		BookListRule:              normalizeUpstreamSelectorRule(p.RuleSearch.BookList),
		BookNameRule:              normalizeUpstreamSelectorRule(p.RuleSearch.Name),
		BookAuthorRule:            normalizeUpstreamSelectorRule(p.RuleSearch.Author),
		BookCoverRule:             normalizeUpstreamSelectorRule(p.RuleSearch.CoverURL),
		BookIntroRule:             normalizeUpstreamSelectorRule(p.RuleSearch.Intro),
		BookKindRule:              normalizeUpstreamSelectorRule(p.RuleSearch.Kind),
		BookWordCountRule:         normalizeUpstreamSelectorRule(p.RuleSearch.WordCount),
		LatestChapterRule:         normalizeUpstreamSelectorRule(p.RuleSearch.LastChapter),
		BookUpdateTimeRule:        normalizeUpstreamSelectorRule(p.RuleSearch.UpdateTime),
		BookURLRule:               normalizeUpstreamSelectorRule(p.RuleSearch.BookURL),
		ExploreBookListRule:       normalizeUpstreamSelectorRule(p.RuleExplore.BookList),
		ExploreBookNameRule:       normalizeUpstreamSelectorRule(p.RuleExplore.Name),
		ExploreBookAuthorRule:     normalizeUpstreamSelectorRule(p.RuleExplore.Author),
		ExploreBookCoverRule:      normalizeUpstreamSelectorRule(p.RuleExplore.CoverURL),
		ExploreBookIntroRule:      normalizeUpstreamSelectorRule(p.RuleExplore.Intro),
		ExploreBookKindRule:       normalizeUpstreamSelectorRule(p.RuleExplore.Kind),
		ExploreBookWordCountRule:  normalizeUpstreamSelectorRule(p.RuleExplore.WordCount),
		ExploreLatestChapterRule:  normalizeUpstreamSelectorRule(p.RuleExplore.LastChapter),
		ExploreBookUpdateTimeRule: normalizeUpstreamSelectorRule(p.RuleExplore.UpdateTime),
		ExploreBookURLRule:        normalizeUpstreamSelectorRule(p.RuleExplore.BookURL),
		BookInfoInitRule:          normalizeUpstreamSelectorRule(p.RuleBookInfo.Init),
		BookInfoNameRule:          normalizeUpstreamSelectorRule(p.RuleBookInfo.Name),
		BookInfoAuthorRule:        normalizeUpstreamSelectorRule(p.RuleBookInfo.Author),
		BookInfoCoverRule:         normalizeUpstreamSelectorRule(p.RuleBookInfo.CoverURL),
		BookInfoIntroRule:         normalizeUpstreamSelectorRule(p.RuleBookInfo.Intro),
		BookInfoKindRule:          normalizeUpstreamSelectorRule(p.RuleBookInfo.Kind),
		BookInfoLatestChapterRule: normalizeUpstreamSelectorRule(p.RuleBookInfo.LastChapter),
		BookInfoUpdateTimeRule:    normalizeUpstreamSelectorRule(p.RuleBookInfo.UpdateTime),
		BookInfoWordCountRule:     normalizeUpstreamSelectorRule(p.RuleBookInfo.WordCount),
		BookInfoCanRenameRule:     normalizeUpstreamSelectorRule(p.RuleBookInfo.CanRename),
		TOCURLRule:                normalizeUpstreamSelectorRule(p.RuleBookInfo.TOCURL),
		ChapterPreUpdateJSRule:    strings.TrimSpace(p.RuleTOC.PreUpdateJS),
		ChapterListRule:           normalizeUpstreamSelectorRule(p.RuleTOC.ChapterList),
		ChapterNameRule:           normalizeUpstreamSelectorRule(p.RuleTOC.ChapterName),
		ChapterURLRule:            normalizeUpstreamSelectorRule(p.RuleTOC.ChapterURL),
		ChapterIsVolumeRule:       normalizeUpstreamSelectorRule(p.RuleTOC.IsVolume),
		ChapterIsVIPRule:          normalizeUpstreamSelectorRule(p.RuleTOC.IsVIP),
		ChapterUpdateTimeRule:     normalizeUpstreamSelectorRule(p.RuleTOC.UpdateTime),
		NextTOCURLRule:            normalizeUpstreamSelectorRule(p.RuleTOC.NextTOCURL),
		ContentRule:               normalizeUpstreamSelectorRule(p.RuleContent.Content),
		NextContentURLRule:        normalizeUpstreamSelectorRule(p.RuleContent.NextContentURL),
		ContentWebJSRule:          strings.TrimSpace(p.RuleContent.WebJS),
		ContentSourceRegex:        strings.TrimSpace(p.RuleContent.SourceRegex),
		ContentReplaceRegex:       strings.TrimSpace(p.RuleContent.ReplaceRegex),
		ContentImageStyle:         strings.TrimSpace(p.RuleContent.ImageStyle),
		Headers:                   p.compatHeaders(),
	}
	if isEmptyCompatRule(rule) {
		return ""
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return ""
	}
	return string(data)
}

func isEmptyCompatRule(rule models.BookSourceRule) bool {
	return rule.SearchURL == "" &&
		rule.ExploreURL == "" &&
		rule.BookListRule == "" &&
		rule.BookNameRule == "" &&
		rule.BookAuthorRule == "" &&
		rule.BookCoverRule == "" &&
		rule.BookIntroRule == "" &&
		rule.BookKindRule == "" &&
		rule.BookWordCountRule == "" &&
		rule.LatestChapterRule == "" &&
		rule.BookUpdateTimeRule == "" &&
		rule.BookURLRule == "" &&
		rule.ExploreBookListRule == "" &&
		rule.ExploreBookNameRule == "" &&
		rule.ExploreBookAuthorRule == "" &&
		rule.ExploreBookCoverRule == "" &&
		rule.ExploreBookIntroRule == "" &&
		rule.ExploreBookKindRule == "" &&
		rule.ExploreBookWordCountRule == "" &&
		rule.ExploreLatestChapterRule == "" &&
		rule.ExploreBookUpdateTimeRule == "" &&
		rule.ExploreBookURLRule == "" &&
		rule.BookInfoInitRule == "" &&
		rule.BookInfoNameRule == "" &&
		rule.BookInfoAuthorRule == "" &&
		rule.BookInfoCoverRule == "" &&
		rule.BookInfoIntroRule == "" &&
		rule.BookInfoKindRule == "" &&
		rule.BookInfoLatestChapterRule == "" &&
		rule.BookInfoUpdateTimeRule == "" &&
		rule.BookInfoWordCountRule == "" &&
		rule.BookInfoCanRenameRule == "" &&
		rule.ChapterPreUpdateJSRule == "" &&
		rule.ChapterListRule == "" &&
		rule.ChapterNameRule == "" &&
		rule.ChapterURLRule == "" &&
		rule.ChapterIsVolumeRule == "" &&
		rule.ChapterIsVIPRule == "" &&
		rule.ChapterUpdateTimeRule == "" &&
		rule.NextTOCURLRule == "" &&
		rule.ContentRule == "" &&
		rule.NextContentURLRule == "" &&
		rule.ContentWebJSRule == "" &&
		rule.ContentSourceRegex == "" &&
		rule.ContentReplaceRegex == "" &&
		rule.ContentImageStyle == "" &&
		len(rule.Headers) == 0
}

func (p bookSourcePayload) compatHeaders() map[string]string {
	if header := strings.TrimSpace(p.Header); header != "" {
		if headers := decodeHeaderMap([]byte(header)); len(headers) > 0 {
			return headers
		}
	}
	if len(p.HeaderMap) > 0 {
		if headers := decodeHeaderMap(p.HeaderMap); len(headers) > 0 {
			return headers
		}
		var headerText string
		if err := json.Unmarshal(p.HeaderMap, &headerText); err == nil {
			return decodeHeaderMap([]byte(headerText))
		}
	}
	return nil
}

func (p bookSourcePayload) rawHeader() string {
	if header := strings.TrimSpace(p.Header); header != "" {
		return header
	}
	if len(p.HeaderMap) == 0 {
		return ""
	}
	var headerText string
	if err := json.Unmarshal(p.HeaderMap, &headerText); err == nil {
		return strings.TrimSpace(headerText)
	}
	var compact any
	if err := json.Unmarshal(p.HeaderMap, &compact); err != nil {
		return ""
	}
	data, err := json.Marshal(compact)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeHeaderMap(data []byte) map[string]string {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	headers := make(map[string]string, len(raw))
	for key, value := range raw {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		headers[name] = strings.TrimSpace(strings.Trim(fmt.Sprint(value), `"`))
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeUpstreamSelectorRule(value string) string {
	rule := strings.TrimSpace(value)
	if rule == "" || strings.HasPrefix(rule, "/") || strings.HasPrefix(strings.ToLower(rule), "@js:") {
		return rule
	}
	at := strings.LastIndex(rule, "@")
	if at <= 0 || at == len(rule)-1 {
		return rule
	}
	selector := strings.TrimSpace(rule[:at])
	operation := strings.TrimSpace(rule[at+1:])
	if selector == "" || operation == "" || strings.ContainsAny(operation, " /|@[](){}") {
		return rule
	}
	switch strings.ToLower(operation) {
	case "text", "html":
		return selector + "|" + strings.ToLower(operation)
	default:
		return selector + "|attr:" + operation
	}
}

func normalizeUpstreamURLTemplate(value string) string {
	template := strings.TrimSpace(value)
	template = strings.ReplaceAll(template, "{{key}}", "{keyword}")
	template = strings.ReplaceAll(template, "{{keyword}}", "{keyword}")
	template = strings.ReplaceAll(template, "{{page}}", "{page}")
	return template
}

func exportUpstreamURLTemplate(value string) string {
	template := normalizeUpstreamURLTemplate(value)
	template = strings.ReplaceAll(template, "{keyword}", "{{key}}")
	template = strings.ReplaceAll(template, "{page}", "{{page}}")
	return template
}

func exportUpstreamSelectorRule(value string) string {
	rule := strings.TrimSpace(value)
	parts := strings.SplitN(rule, "|", 2)
	if len(parts) != 2 {
		return rule
	}
	selector := strings.TrimSpace(parts[0])
	operation := strings.TrimSpace(parts[1])
	if selector == "" {
		return rule
	}
	switch {
	case operation == "text" || operation == "html":
		return selector + "@" + operation
	case strings.HasPrefix(operation, "attr:"):
		attribute := strings.TrimSpace(strings.TrimPrefix(operation, "attr:"))
		if attribute != "" {
			return selector + "@" + attribute
		}
	}
	return rule
}

func (s *Server) createSource(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

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

	if err := s.db.Select("Name", "BaseURL", "SearchURL", "BookURLPattern", "SourceType", "Comment", "Charset", "ConcurrentRate", "Header", "LoginURL", "LoginCheckJS", "CustomOrder", "LastUpdateTime", "Weight", "RespondTime", "Rules", "Enabled", "EnabledExplore", "Group").Create(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create source"})
		return
	}
	s.broadcastSourcesUpdate("create")
	c.JSON(http.StatusCreated, source)
}

func (s *Server) updateSource(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

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
	source.BookURLPattern = strings.TrimSpace(req.BookURLPattern)
	source.SourceType = req.SourceType
	source.Comment = strings.TrimSpace(req.Comment)
	source.Charset = strings.TrimSpace(req.Charset)
	if source.Charset == "" {
		source.Charset = "utf-8"
	}
	source.Rules = strings.TrimSpace(req.Rules)
	source.ConcurrentRate = strings.TrimSpace(req.ConcurrentRate)
	source.Header = strings.TrimSpace(req.Header)
	source.LoginURL = strings.TrimSpace(req.LoginURL)
	source.LoginCheckJS = strings.TrimSpace(req.LoginCheckJS)
	source.CustomOrder = req.CustomOrder
	source.LastUpdateTime = req.LastUpdateTime
	source.Weight = req.Weight
	source.RespondTime = req.RespondTime
	source.Group = strings.TrimSpace(req.Group)
	source.Enabled = req.Enabled
	if req.EnabledExplore != nil {
		source.EnabledExplore = req.EnabledExplore
	}

	if err := s.db.Save(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update source"})
		return
	}
	s.clearSourceFailureIDs([]uint{source.ID})
	s.broadcastSourcesUpdate("update")
	c.JSON(http.StatusOK, source)
}

func (s *Server) deleteSource(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	if count := s.bookSourceUsageCount(uint(id)); count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "source is used by bookshelf books", "usedBookCount": count})
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
	s.clearSourceFailureIDs([]uint{uint(id)})
	s.broadcastSourcesUpdate("delete")
	c.Status(http.StatusNoContent)
}

func (s *Server) clearSources(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

	result := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.BookSource{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear sources"})
		return
	}
	s.clearAllSourceFailures()
	s.broadcastSourcesUpdate("clear")
	c.JSON(http.StatusOK, gin.H{"affected": result.RowsAffected})
}

func (s *Server) attachBookSourceUsage(sources []models.BookSource) {
	if len(sources) == 0 {
		return
	}
	counts := s.bookSourceUsageCounts(nil)
	for i := range sources {
		sources[i].UsedBookCount = counts[sources[i].ID]
	}
}

func (s *Server) bookSourceUsageCount(sourceID uint) int {
	return s.bookSourceUsageCounts([]uint{sourceID})[sourceID]
}

func (s *Server) bookSourceUsageCounts(sourceIDs []uint) map[uint]int {
	type sourceUsage struct {
		SourceID uint
		Count    int
	}
	query := s.db.Model(&models.Book{}).Select("source_id, COUNT(*) AS count").Where("source_id > 0").Group("source_id")
	if len(sourceIDs) > 0 {
		query = query.Where("source_id IN ?", sourceIDs)
	}
	var rows []sourceUsage
	if err := query.Scan(&rows).Error; err != nil {
		return map[uint]int{}
	}
	counts := make(map[uint]int, len(rows))
	for _, row := range rows {
		counts[row.SourceID] = row.Count
	}
	return counts
}

func (s *Server) defaultSourcesStatus(c *gin.Context) {
	sources, err := s.loadDefaultBookSources()
	if errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusOK, gin.H{"configured": false, "count": 0})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"configured": false, "count": 0, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configured": true, "count": len(sources)})
}

func (s *Server) saveDefaultSources(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

	var sources []models.BookSource
	if err := s.db.Order("custom_order asc, id asc").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}
	if len(sources) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no sources to save as default"})
		return
	}
	for i := range sources {
		sources[i].ID = 0
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode default sources"})
		return
	}
	path := s.defaultBookSourcesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare default sources"})
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save default sources"})
		return
	}
	s.broadcastSourcesUpdate("save-default")
	c.JSON(http.StatusOK, gin.H{"count": len(sources)})
}

func (s *Server) restoreDefaultSources(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

	sources, err := s.loadDefaultBookSources()
	if errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusNotFound, gin.H{"error": "default sources are not configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "default sources are invalid"})
		return
	}
	if len(sources) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "default sources are empty"})
		return
	}

	var result gin.H
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.BookSource{}).Error; err != nil {
			return err
		}
		result = importBookSourcesWithDB(tx, sources)
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore default sources"})
		return
	}
	s.clearAllSourceFailures()
	s.broadcastSourcesUpdate("restore-default")
	c.JSON(http.StatusOK, result)
}

type batchSourcesRequest struct {
	Action    string `json:"action" binding:"required"`
	SourceIDs []uint `json:"sourceIds" binding:"required"`
	Group     string `json:"group"`
}

func (s *Server) batchSources(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

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
	skippedUsed := 0
	deletedIDs := make([]uint, 0)
	switch req.Action {
	case "enable":
		result = s.db.Model(&models.BookSource{}).Where("id IN ?", req.SourceIDs).Update("enabled", true)
	case "disable":
		result = s.db.Model(&models.BookSource{}).Where("id IN ?", req.SourceIDs).Update("enabled", false)
	case "delete":
		usageCounts := s.bookSourceUsageCounts(req.SourceIDs)
		deletableIDs := make([]uint, 0, len(req.SourceIDs))
		for _, sourceID := range req.SourceIDs {
			if usageCounts[sourceID] > 0 {
				skippedUsed++
				continue
			}
			deletableIDs = append(deletableIDs, sourceID)
		}
		if len(deletableIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"affected": 0, "skippedUsed": skippedUsed})
			return
		}
		result = s.db.Where("id IN ?", deletableIDs).Delete(&models.BookSource{})
		deletedIDs = deletableIDs
	case "group":
		result = s.db.Model(&models.BookSource{}).Where("id IN ?", req.SourceIDs).Update("group", strings.TrimSpace(req.Group))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported batch action"})
		return
	}
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update sources"})
		return
	}
	if len(deletedIDs) > 0 {
		s.clearSourceFailureIDs(deletedIDs)
	}

	s.broadcastSourcesUpdate("batch-" + req.Action)
	c.JSON(http.StatusOK, gin.H{"affected": result.RowsAffected, "skippedUsed": skippedUsed})
}

func (s *Server) importSources(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

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
	s.clearAllSourceFailures()
	s.broadcastSourcesUpdate("import")
	c.JSON(http.StatusOK, result)
}

func (s *Server) exportSources(c *gin.Context) {
	var sources []models.BookSource
	sourceIDs, ok := parseSourceIDsQuery(c)
	if !ok {
		return
	}
	query := s.db.Order("custom_order asc, id asc")
	if len(sourceIDs) > 0 {
		query = query.Where("id IN ?", sourceIDs)
	}
	if err := query.Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=bookSources.json")
	c.JSON(http.StatusOK, exportBookSources(sources))
}

func exportBookSources(sources []models.BookSource) []exportedBookSource {
	exported := make([]exportedBookSource, 0, len(sources))
	for _, source := range sources {
		rule, err := source.ParsedRules()
		if err != nil {
			rule = models.BookSourceRule{}
		}
		searchRule := legacySourceSearchRule{
			BookList:    exportUpstreamSelectorRule(rule.BookListRule),
			Name:        exportUpstreamSelectorRule(rule.BookNameRule),
			Author:      exportUpstreamSelectorRule(rule.BookAuthorRule),
			CoverURL:    exportUpstreamSelectorRule(rule.BookCoverRule),
			Intro:       exportUpstreamSelectorRule(rule.BookIntroRule),
			Kind:        exportUpstreamSelectorRule(rule.BookKindRule),
			WordCount:   exportUpstreamSelectorRule(rule.BookWordCountRule),
			LastChapter: exportUpstreamSelectorRule(rule.LatestChapterRule),
			UpdateTime:  exportUpstreamSelectorRule(rule.BookUpdateTimeRule),
			BookURL:     exportUpstreamSelectorRule(rule.BookURLRule),
		}
		exploreRule := legacySourceSearchRule{
			BookList:    exportUpstreamSelectorRule(rule.ExploreBookListRule),
			Name:        exportUpstreamSelectorRule(rule.ExploreBookNameRule),
			Author:      exportUpstreamSelectorRule(rule.ExploreBookAuthorRule),
			CoverURL:    exportUpstreamSelectorRule(rule.ExploreBookCoverRule),
			Intro:       exportUpstreamSelectorRule(rule.ExploreBookIntroRule),
			Kind:        exportUpstreamSelectorRule(rule.ExploreBookKindRule),
			WordCount:   exportUpstreamSelectorRule(rule.ExploreBookWordCountRule),
			LastChapter: exportUpstreamSelectorRule(rule.ExploreLatestChapterRule),
			UpdateTime:  exportUpstreamSelectorRule(rule.ExploreBookUpdateTimeRule),
			BookURL:     exportUpstreamSelectorRule(rule.ExploreBookURLRule),
		}
		header := strings.TrimSpace(source.Header)
		if header == "" && len(rule.Headers) > 0 {
			if data, marshalErr := json.Marshal(rule.Headers); marshalErr == nil {
				header = string(data)
			}
		}
		exported = append(exported, exportedBookSource{
			BookSourceName:    source.Name,
			BookSourceGroup:   source.Group,
			BookSourceURL:     source.BaseURL,
			BookSourceType:    source.SourceType,
			BookURLPattern:    source.BookURLPattern,
			BookSourceComment: source.Comment,
			Enabled:           source.Enabled,
			EnabledExplore:    source.IsExploreEnabled(),
			SearchURL:         exportUpstreamURLTemplate(firstNonBlank(rule.SearchURL, source.SearchURL)),
			ExploreURL:        exportUpstreamURLTemplate(rule.ExploreURL),
			Header:            header,
			RuleSearch:        searchRule,
			RuleExplore:       exploreRule,
			RuleBookInfo: legacySourceBookInfoRule{
				Init:        exportUpstreamSelectorRule(rule.BookInfoInitRule),
				Name:        exportUpstreamSelectorRule(rule.BookInfoNameRule),
				Author:      exportUpstreamSelectorRule(rule.BookInfoAuthorRule),
				CoverURL:    exportUpstreamSelectorRule(rule.BookInfoCoverRule),
				Intro:       exportUpstreamSelectorRule(rule.BookInfoIntroRule),
				Kind:        exportUpstreamSelectorRule(rule.BookInfoKindRule),
				LastChapter: exportUpstreamSelectorRule(rule.BookInfoLatestChapterRule),
				UpdateTime:  exportUpstreamSelectorRule(rule.BookInfoUpdateTimeRule),
				WordCount:   exportUpstreamSelectorRule(rule.BookInfoWordCountRule),
				TOCURL:      exportUpstreamSelectorRule(rule.TOCURLRule),
				CanRename:   exportUpstreamSelectorRule(rule.BookInfoCanRenameRule),
			},
			RuleTOC: legacySourceTOCRule{
				PreUpdateJS: rule.ChapterPreUpdateJSRule,
				ChapterList: exportUpstreamSelectorRule(rule.ChapterListRule),
				ChapterName: exportUpstreamSelectorRule(rule.ChapterNameRule),
				ChapterURL:  exportUpstreamSelectorRule(rule.ChapterURLRule),
				IsVolume:    exportUpstreamSelectorRule(rule.ChapterIsVolumeRule),
				IsVIP:       exportUpstreamSelectorRule(rule.ChapterIsVIPRule),
				UpdateTime:  exportUpstreamSelectorRule(rule.ChapterUpdateTimeRule),
				NextTOCURL:  exportUpstreamSelectorRule(rule.NextTOCURLRule),
			},
			RuleContent: legacySourceContentRule{
				Content:        exportUpstreamSelectorRule(rule.ContentRule),
				NextContentURL: exportUpstreamSelectorRule(rule.NextContentURLRule),
				WebJS:          rule.ContentWebJSRule,
				SourceRegex:    rule.ContentSourceRegex,
				ReplaceRegex:   rule.ContentReplaceRegex,
				ImageStyle:     rule.ContentImageStyle,
			},
			Charset:        source.Charset,
			ConcurrentRate: source.ConcurrentRate,
			LoginURL:       source.LoginURL,
			LoginCheckJS:   source.LoginCheckJS,
			CustomOrder:    source.CustomOrder,
			LastUpdateTime: source.LastUpdateTime,
			Weight:         source.Weight,
			RespondTime:    source.RespondTime,
			Rules:          source.Rules,
		})
	}
	return exported
}

func parseSourceIDsQuery(c *gin.Context) ([]uint, bool) {
	raw := strings.TrimSpace(c.Query("sourceIds"))
	if raw == "" {
		return nil, true
	}

	parts := strings.Split(raw, ",")
	if len(parts) > 300 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many sources"})
		return nil, false
	}

	sourceIDs := make([]uint, 0, len(parts))
	seen := make(map[uint]bool, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		id, err := strconv.ParseUint(value, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sourceIds"})
			return nil, false
		}
		sourceID := uint(id)
		if seen[sourceID] {
			continue
		}
		seen[sourceID] = true
		sourceIDs = append(sourceIDs, sourceID)
	}
	if len(sourceIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourceIds is required"})
		return nil, false
	}
	return sourceIDs, true
}

type remoteSourceRequest struct {
	URL string `json:"url" binding:"required"`
}

func (s *Server) importRemoteSource(c *gin.Context) {
	if !s.requireSourceEdit(c) {
		return
	}

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
	s.clearAllSourceFailures()
	s.broadcastSourcesUpdate("remote-import")
	c.JSON(http.StatusOK, result)
}

func (s *Server) broadcastSourcesUpdate(kind string) {
	if s.hub == nil {
		return
	}
	_ = s.hub.BroadcastAll(nil, gin.H{
		"type":    "sources_update",
		"payload": gin.H{"kind": kind},
	})
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
	c.JSON(http.StatusOK, gin.H{"count": len(sources), "names": names, "sources": sources})
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
	return importBookSourcesWithDB(s.db, sources)
}

func importBookSourcesWithDB(db *gorm.DB, sources []models.BookSource) gin.H {
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
		source.BookURLPattern = strings.TrimSpace(source.BookURLPattern)
		source.Comment = strings.TrimSpace(source.Comment)
		source.Rules = strings.TrimSpace(source.Rules)
		source.Group = strings.TrimSpace(source.Group)
		source.Charset = strings.TrimSpace(source.Charset)
		source.ConcurrentRate = strings.TrimSpace(source.ConcurrentRate)
		source.Header = strings.TrimSpace(source.Header)
		source.LoginURL = strings.TrimSpace(source.LoginURL)
		source.LoginCheckJS = strings.TrimSpace(source.LoginCheckJS)
		if source.Charset == "" {
			source.Charset = "utf-8"
		}

		var existing models.BookSource
		if err := db.Where("name = ?", source.Name).First(&existing).Error; err == nil {
			existing.BaseURL = source.BaseURL
			existing.SearchURL = source.SearchURL
			existing.BookURLPattern = source.BookURLPattern
			existing.SourceType = source.SourceType
			existing.Comment = source.Comment
			existing.Charset = source.Charset
			existing.ConcurrentRate = source.ConcurrentRate
			existing.Header = source.Header
			existing.LoginURL = source.LoginURL
			existing.LoginCheckJS = source.LoginCheckJS
			existing.CustomOrder = source.CustomOrder
			existing.LastUpdateTime = source.LastUpdateTime
			existing.Weight = source.Weight
			existing.RespondTime = source.RespondTime
			existing.Rules = source.Rules
			existing.Enabled = source.Enabled
			existing.EnabledExplore = source.EnabledExplore
			existing.Group = source.Group
			if err := db.Save(&existing).Error; err == nil {
				updated++
				continue
			}
			skipped++
			continue
		}

		if err := db.Select("Name", "BaseURL", "SearchURL", "BookURLPattern", "SourceType", "Comment", "Charset", "ConcurrentRate", "Header", "LoginURL", "LoginCheckJS", "CustomOrder", "LastUpdateTime", "Weight", "RespondTime", "Rules", "Enabled", "EnabledExplore", "Group").Create(&source).Error; err != nil {
			skipped++
			continue
		}
		imported++
	}
	return gin.H{"imported": imported, "updated": updated, "skipped": skipped}
}

func (s *Server) defaultBookSourcesPath() string {
	return filepath.Join(s.cfg.DataDir, "defaultBookSources.json")
}

func (s *Server) loadDefaultBookSources() ([]models.BookSource, error) {
	data, err := os.ReadFile(s.defaultBookSourcesPath())
	if err != nil {
		return nil, err
	}
	return decodeBookSources(data)
}

func (s *Server) requireSourceEdit(c *gin.Context) bool {
	userID, ok := middleware.UserID(c)
	if !ok {
		unauthorized(c, "missing user")
		return false
	}

	var user models.User
	err := s.db.Select("can_edit_sources").First(&user, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		unauthorized(c, "user not found")
		return false
	}
	if err != nil {
		internalError(c, "failed to load user")
		return false
	}
	if !user.CanEditSources {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "source editing is disabled for this user"))
		return false
	}
	return true
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
