package api

import (
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

type rssSourceRequest struct {
	Title             string `json:"title"`
	URL               string `json:"url"`
	Icon              string `json:"icon"`
	Group             string `json:"group"`
	CustomOrder       *int   `json:"customOrder"`
	SingleURL         *bool  `json:"singleUrl"`
	ArticleStyle      *int   `json:"articleStyle"`
	SortURL           string `json:"sortUrl"`
	RuleArticles      string `json:"ruleArticles"`
	RuleTitle         string `json:"ruleTitle"`
	RulePubDate       string `json:"rulePubDate"`
	RuleImage         string `json:"ruleImage"`
	RuleLink          string `json:"ruleLink"`
	RuleContent       string `json:"ruleContent"`
	EnableJS          *bool  `json:"enableJs"`
	Enabled           *bool  `json:"enabled"`
	UpstreamTitle     string `json:"sourceName"`
	UpstreamURL       string `json:"sourceUrl"`
	UpstreamIcon      string `json:"sourceIcon"`
	UpstreamGroup     string `json:"sourceGroup"`
	UpstreamIsEnabled *bool  `json:"isEnabled"`
}

func (s *Server) listRSSSources(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var sources []models.RSSSource
	if err := s.db.Where("user_id = ?", userID).Order("custom_order asc, updated_at desc").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list RSS sources"})
		return
	}
	c.JSON(http.StatusOK, sources)
}

func (s *Server) createRSSSource(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req rssSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	req.normalize()
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	customOrder := req.orderOrDefault(s, userID)
	source := models.RSSSource{
		UserID:       userID,
		Title:        strings.TrimSpace(req.Title),
		URL:          strings.TrimSpace(req.URL),
		Icon:         strings.TrimSpace(req.Icon),
		Group:        strings.TrimSpace(req.Group),
		CustomOrder:  customOrder,
		SingleURL:    req.singleURLOrDefault(),
		ArticleStyle: req.articleStyleOrDefault(),
		SortURL:      strings.TrimSpace(req.SortURL),
		RuleArticles: strings.TrimSpace(req.RuleArticles),
		RuleTitle:    strings.TrimSpace(req.RuleTitle),
		RulePubDate:  strings.TrimSpace(req.RulePubDate),
		RuleImage:    strings.TrimSpace(req.RuleImage),
		RuleLink:     strings.TrimSpace(req.RuleLink),
		RuleContent:  strings.TrimSpace(req.RuleContent),
		EnableJS:     req.enableJSOrDefault(),
		Enabled:      enabled,
	}
	if source.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	if source.Title == "" {
		source.Title = source.URL
	}
	if err := s.db.Create(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create RSS source"})
		return
	}
	s.broadcastRSSUpdate(userID, "source-create", gin.H{"sourceId": source.ID})
	c.JSON(http.StatusCreated, source)
}

func (s *Server) updateRSSSource(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	sourceID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var source models.RSSSource
	if err := s.db.Where("user_id = ? AND id = ?", userID, sourceID).First(&source).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "RSS source not found"})
		return
	}
	var req rssSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	req.normalize()
	source.Title = strings.TrimSpace(req.Title)
	source.URL = strings.TrimSpace(req.URL)
	source.Icon = strings.TrimSpace(req.Icon)
	source.Group = strings.TrimSpace(req.Group)
	if req.CustomOrder != nil {
		source.CustomOrder = *req.CustomOrder
	}
	if req.SingleURL != nil {
		source.SingleURL = *req.SingleURL
	}
	if req.ArticleStyle != nil {
		source.ArticleStyle = *req.ArticleStyle
	}
	source.SortURL = strings.TrimSpace(req.SortURL)
	source.RuleArticles = strings.TrimSpace(req.RuleArticles)
	source.RuleTitle = strings.TrimSpace(req.RuleTitle)
	source.RulePubDate = strings.TrimSpace(req.RulePubDate)
	source.RuleImage = strings.TrimSpace(req.RuleImage)
	source.RuleLink = strings.TrimSpace(req.RuleLink)
	source.RuleContent = strings.TrimSpace(req.RuleContent)
	if req.EnableJS != nil {
		source.EnableJS = *req.EnableJS
	}
	if req.Enabled != nil {
		source.Enabled = *req.Enabled
	}
	if source.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	if source.Title == "" {
		source.Title = source.URL
	}
	if err := s.db.Save(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update RSS source"})
		return
	}
	s.broadcastRSSUpdate(userID, "source-update", gin.H{"sourceId": source.ID})
	c.JSON(http.StatusOK, source)
}

func (r *rssSourceRequest) normalize() {
	if strings.TrimSpace(r.Title) == "" {
		r.Title = r.UpstreamTitle
	}
	if strings.TrimSpace(r.URL) == "" {
		r.URL = r.UpstreamURL
	}
	if strings.TrimSpace(r.Icon) == "" {
		r.Icon = r.UpstreamIcon
	}
	if strings.TrimSpace(r.Group) == "" {
		r.Group = r.UpstreamGroup
	}
	if r.Enabled == nil && r.UpstreamIsEnabled != nil {
		r.Enabled = r.UpstreamIsEnabled
	}
}

func (r rssSourceRequest) orderOrDefault(s *Server, userID uint) int {
	if r.CustomOrder != nil && *r.CustomOrder > 0 {
		return *r.CustomOrder
	}
	var maxOrder int
	_ = s.db.Model(&models.RSSSource{}).Where("user_id = ?", userID).Select("COALESCE(MAX(custom_order), 0)").Scan(&maxOrder).Error
	return maxOrder + 1
}

func (r rssSourceRequest) singleURLOrDefault() bool {
	if r.SingleURL != nil {
		return *r.SingleURL
	}
	return true
}

func (r rssSourceRequest) articleStyleOrDefault() int {
	if r.ArticleStyle != nil {
		return *r.ArticleStyle
	}
	return 0
}

func (r rssSourceRequest) enableJSOrDefault() bool {
	if r.EnableJS != nil {
		return *r.EnableJS
	}
	return true
}

func (s *Server) deleteRSSSource(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	sourceID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := s.db.Where("user_id = ? AND source_id = ?", userID, sourceID).Delete(&models.RSSArticle{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete RSS articles"})
		return
	}
	result := s.db.Where("user_id = ? AND id = ?", userID, sourceID).Delete(&models.RSSSource{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete RSS source"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "RSS source not found"})
		return
	}
	s.broadcastRSSUpdate(userID, "source-delete", gin.H{"sourceId": sourceID})
	c.Status(http.StatusNoContent)
}

func (s *Server) refreshRSSSource(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	sourceID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var source models.RSSSource
	if err := s.db.Where("user_id = ? AND id = ?", userID, sourceID).First(&source).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "RSS source not found"})
		return
	}
	articles, err := fetchRSSArticles(source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to fetch RSS source: " + err.Error()})
		return
	}
	imported := 0
	for _, article := range articles {
		article.UserID = userID
		article.SourceID = source.ID
		var existing models.RSSArticle
		if article.Link != "" && s.db.Where("user_id = ? AND source_id = ? AND link = ?", userID, source.ID, article.Link).First(&existing).Error == nil {
			existing.Title = article.Title
			existing.Author = article.Author
			existing.Image = article.Image
			existing.Summary = article.Summary
			existing.Content = article.Content
			existing.PublishedAt = article.PublishedAt
			_ = s.db.Save(&existing).Error
			continue
		}
		if err := s.db.Create(&article).Error; err == nil {
			imported++
		}
	}
	s.broadcastRSSUpdate(userID, "source-refresh", gin.H{
		"sourceId": source.ID,
		"imported": imported,
		"total":    len(articles),
	})
	c.JSON(http.StatusOK, gin.H{"imported": imported, "total": len(articles)})
}

func (s *Server) listRSSArticles(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	query := s.db.Where("user_id = ?", userID)
	if sourceID := strings.TrimSpace(c.Query("sourceId")); sourceID != "" {
		query = query.Where("source_id = ?", sourceID)
	}
	if strings.TrimSpace(c.Query("unread")) == "true" {
		query = query.Where("is_read = ?", false)
	}
	if strings.TrimSpace(c.Query("favorite")) == "true" {
		query = query.Where("favorite = ?", true)
	}
	page := parseBoundedInt(c.Query("page"), 0, 0, 100000)
	limit := parseBoundedInt(c.Query("limit"), 0, 0, 100)
	var articles []models.RSSArticle
	if page > 0 || limit > 0 {
		if page <= 0 {
			page = 1
		}
		if limit <= 0 {
			limit = 50
		}
		if limit > 100 {
			limit = 100
		}
		offset := (page - 1) * limit
		if err := query.Order("published_at desc, updated_at desc").Limit(limit + 1).Offset(offset).Find(&articles).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list RSS articles"})
			return
		}
		hasMore := len(articles) > limit
		if hasMore {
			articles = articles[:limit]
		}
		c.JSON(http.StatusOK, gin.H{
			"items":   articles,
			"page":    page,
			"limit":   limit,
			"hasMore": hasMore,
		})
		return
	}
	if err := query.Order("published_at desc, updated_at desc").Limit(200).Find(&articles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list RSS articles"})
		return
	}
	c.JSON(http.StatusOK, articles)
}

type rssArticleStateRequest struct {
	IsRead   *bool `json:"isRead"`
	Favorite *bool `json:"favorite"`
}

func (s *Server) updateRSSArticleState(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	articleID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var article models.RSSArticle
	if err := s.db.Where("user_id = ? AND id = ?", userID, articleID).First(&article).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "RSS article not found"})
		return
	}
	var req rssArticleStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid RSS article payload"})
		return
	}
	if req.IsRead != nil {
		article.IsRead = *req.IsRead
	}
	if req.Favorite != nil {
		article.Favorite = *req.Favorite
	}
	if err := s.db.Save(&article).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update RSS article"})
		return
	}
	s.broadcastRSSUpdate(userID, "article-update", gin.H{
		"sourceId": article.SourceID,
		"article":  article,
	})
	c.JSON(http.StatusOK, article)
}

func (s *Server) broadcastRSSUpdate(userID uint, kind string, payload gin.H) {
	if s.hub == nil {
		return
	}
	if payload == nil {
		payload = gin.H{}
	}
	payload["kind"] = kind
	_ = s.hub.Broadcast(userID, nil, gin.H{
		"type":    "rss_update",
		"payload": payload,
	})
}

type parsedRSS struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Creator     string `xml:"creator"`
			Author      string `xml:"author"`
			PubDate     string `xml:"pubDate"`
			Encoded     string `xml:"encoded"`
			Enclosure   struct {
				URL  string `xml:"url,attr"`
				Type string `xml:"type,attr"`
			} `xml:"enclosure"`
			MediaThumbnail []struct {
				URL string `xml:"url,attr"`
			} `xml:"thumbnail"`
			MediaContent []struct {
				URL    string `xml:"url,attr"`
				Type   string `xml:"type,attr"`
				Medium string `xml:"medium,attr"`
			} `xml:"content"`
		} `xml:"item"`
	} `xml:"channel"`
	Entries []struct {
		Title string `xml:"title"`
		Link  []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
			Type string `xml:"type,attr"`
		} `xml:"link"`
		Summary string `xml:"summary"`
		Content string `xml:"content"`
		Author  struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Published      string `xml:"published"`
		Updated        string `xml:"updated"`
		MediaThumbnail []struct {
			URL string `xml:"url,attr"`
		} `xml:"thumbnail"`
		MediaContent []struct {
			URL    string `xml:"url,attr"`
			Type   string `xml:"type,attr"`
			Medium string `xml:"medium,attr"`
		} `xml:"content"`
	} `xml:"entry"`
}

func fetchRSSArticles(source models.RSSSource) ([]models.RSSArticle, error) {
	text, err := engine.FetchText(source.URL, "utf-8")
	if err != nil {
		return nil, err
	}
	var parsed parsedRSS
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, err
	}
	articles := make([]models.RSSArticle, 0)
	for _, item := range parsed.Channel.Items {
		articles = append(articles, models.RSSArticle{
			Title:       strings.TrimSpace(item.Title),
			Link:        strings.TrimSpace(item.Link),
			Author:      firstNonEmpty(item.Creator, item.Author),
			Image:       rssItemImage(item.Enclosure.URL, item.Enclosure.Type, item.MediaThumbnail, item.MediaContent),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Encoded),
			PublishedAt: parseRSSDate(item.PubDate),
		})
	}
	for _, entry := range parsed.Entries {
		link := ""
		if len(entry.Link) > 0 {
			link = entry.Link[0].Href
		}
		articles = append(articles, models.RSSArticle{
			Title:       strings.TrimSpace(entry.Title),
			Link:        strings.TrimSpace(link),
			Author:      strings.TrimSpace(entry.Author.Name),
			Image:       atomEntryImage(entry.Link, entry.MediaThumbnail, entry.MediaContent),
			Summary:     strings.TrimSpace(entry.Summary),
			Content:     strings.TrimSpace(entry.Content),
			PublishedAt: parseRSSDate(firstNonEmpty(entry.Published, entry.Updated)),
		})
	}
	filtered := articles[:0]
	for _, article := range articles {
		if article.Title != "" {
			filtered = append(filtered, article)
		}
	}
	return filtered, nil
}

func rssItemImage(enclosureURL string, enclosureType string, thumbnails []struct {
	URL string `xml:"url,attr"`
}, contents []struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Medium string `xml:"medium,attr"`
}) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(enclosureType)), "image/") {
		if url := strings.TrimSpace(enclosureURL); url != "" {
			return url
		}
	}
	for _, thumb := range thumbnails {
		if url := strings.TrimSpace(thumb.URL); url != "" {
			return url
		}
	}
	for _, content := range contents {
		if isRSSImageMedia(content.URL, content.Type, content.Medium) {
			return strings.TrimSpace(content.URL)
		}
	}
	if url := strings.TrimSpace(enclosureURL); looksLikeImageURL(url) {
		return url
	}
	return ""
}

func atomEntryImage(links []struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}, thumbnails []struct {
	URL string `xml:"url,attr"`
}, contents []struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Medium string `xml:"medium,attr"`
}) string {
	for _, link := range links {
		rel := strings.ToLower(strings.TrimSpace(link.Rel))
		if rel == "enclosure" || rel == "image" {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(link.Type)), "image/") || looksLikeImageURL(link.Href) {
				return strings.TrimSpace(link.Href)
			}
		}
	}
	for _, thumb := range thumbnails {
		if url := strings.TrimSpace(thumb.URL); url != "" {
			return url
		}
	}
	for _, content := range contents {
		if isRSSImageMedia(content.URL, content.Type, content.Medium) {
			return strings.TrimSpace(content.URL)
		}
	}
	return ""
}

func isRSSImageMedia(url string, mediaType string, medium string) bool {
	if strings.TrimSpace(url) == "" {
		return false
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	medium = strings.ToLower(strings.TrimSpace(medium))
	return strings.HasPrefix(mediaType, "image/") || medium == "image" || looksLikeImageURL(url)
}

func looksLikeImageURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasSuffix(value, ".jpg") ||
		strings.HasSuffix(value, ".jpeg") ||
		strings.HasSuffix(value, ".png") ||
		strings.HasSuffix(value, ".gif") ||
		strings.HasSuffix(value, ".webp") ||
		strings.Contains(value, ".jpg?") ||
		strings.Contains(value, ".jpeg?") ||
		strings.Contains(value, ".png?") ||
		strings.Contains(value, ".gif?") ||
		strings.Contains(value, ".webp?")
}

func parseRSSDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 02 Jan 2006 15:04:05 -0700"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
