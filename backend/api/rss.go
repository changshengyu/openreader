package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

const maxRSSPaginationPages = 1000

func rssSourceRequestPolicy(source models.RSSSource) engine.SourceRequestPolicy {
	key := strings.TrimSpace(source.URL)
	if key == "" {
		key = fmt.Sprintf("rss-source:%d", source.ID)
	}
	return engine.SourceRequestPolicy{
		SourceKey:      key,
		ConcurrentRate: strings.TrimSpace(source.ConcurrentRate),
	}
}

type rssSourceRequest struct {
	Title             string `json:"title"`
	URL               string `json:"url"`
	Icon              string `json:"icon"`
	Group             string `json:"group"`
	Comment           string `json:"comment"`
	CustomOrder       *int   `json:"customOrder"`
	ConcurrentRate    string `json:"concurrentRate"`
	Header            any    `json:"header"`
	HeaderMap         any    `json:"headerMap"`
	LoginURL          string `json:"loginUrl"`
	LoginCheckJS      string `json:"loginCheckJs"`
	SingleURL         *bool  `json:"singleUrl"`
	ArticleStyle      *int   `json:"articleStyle"`
	SortURL           string `json:"sortUrl"`
	RuleArticles      string `json:"ruleArticles"`
	RuleNextPage      string `json:"ruleNextPage"`
	RuleTitle         string `json:"ruleTitle"`
	RulePubDate       string `json:"rulePubDate"`
	RuleDescription   string `json:"ruleDescription"`
	RuleImage         string `json:"ruleImage"`
	RuleLink          string `json:"ruleLink"`
	RuleContent       string `json:"ruleContent"`
	Style             string `json:"style"`
	EnableJS          *bool  `json:"enableJs"`
	LoadWithBaseURL   *bool  `json:"loadWithBaseUrl"`
	Enabled           *bool  `json:"enabled"`
	UpstreamTitle     string `json:"sourceName"`
	UpstreamURL       string `json:"sourceUrl"`
	UpstreamIcon      string `json:"sourceIcon"`
	UpstreamGroup     string `json:"sourceGroup"`
	UpstreamComment   string `json:"sourceComment"`
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
		UserID:          userID,
		Title:           strings.TrimSpace(req.Title),
		URL:             strings.TrimSpace(req.URL),
		Icon:            strings.TrimSpace(req.Icon),
		Group:           strings.TrimSpace(req.Group),
		Comment:         strings.TrimSpace(req.Comment),
		CustomOrder:     customOrder,
		ConcurrentRate:  strings.TrimSpace(req.ConcurrentRate),
		Header:          req.headerText(),
		LoginURL:        strings.TrimSpace(req.LoginURL),
		LoginCheckJS:    strings.TrimSpace(req.LoginCheckJS),
		SingleURL:       req.singleURLOrDefault(),
		ArticleStyle:    req.articleStyleOrDefault(),
		SortURL:         strings.TrimSpace(req.SortURL),
		RuleArticles:    strings.TrimSpace(req.RuleArticles),
		RuleNextPage:    strings.TrimSpace(req.RuleNextPage),
		RuleTitle:       strings.TrimSpace(req.RuleTitle),
		RulePubDate:     strings.TrimSpace(req.RulePubDate),
		RuleDescription: strings.TrimSpace(req.RuleDescription),
		RuleImage:       strings.TrimSpace(req.RuleImage),
		RuleLink:        strings.TrimSpace(req.RuleLink),
		RuleContent:     strings.TrimSpace(req.RuleContent),
		Style:           strings.TrimSpace(req.Style),
		EnableJS:        req.enableJSOrDefault(),
		LoadWithBaseURL: req.loadWithBaseURLOrDefault(),
		Enabled:         enabled,
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
	source.Comment = strings.TrimSpace(req.Comment)
	source.ConcurrentRate = strings.TrimSpace(req.ConcurrentRate)
	source.Header = req.headerText()
	source.LoginURL = strings.TrimSpace(req.LoginURL)
	source.LoginCheckJS = strings.TrimSpace(req.LoginCheckJS)
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
	source.RuleNextPage = strings.TrimSpace(req.RuleNextPage)
	source.RuleTitle = strings.TrimSpace(req.RuleTitle)
	source.RulePubDate = strings.TrimSpace(req.RulePubDate)
	source.RuleDescription = strings.TrimSpace(req.RuleDescription)
	source.RuleImage = strings.TrimSpace(req.RuleImage)
	source.RuleLink = strings.TrimSpace(req.RuleLink)
	source.RuleContent = strings.TrimSpace(req.RuleContent)
	source.Style = strings.TrimSpace(req.Style)
	if req.EnableJS != nil {
		source.EnableJS = *req.EnableJS
	}
	if req.LoadWithBaseURL != nil {
		source.LoadWithBaseURL = *req.LoadWithBaseURL
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
	if strings.TrimSpace(r.Comment) == "" {
		r.Comment = r.UpstreamComment
	}
	if normalizeRSSHeaderValue(r.Header) == "" && r.HeaderMap != nil {
		r.Header = r.HeaderMap
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

func (r rssSourceRequest) loadWithBaseURLOrDefault() bool {
	if r.LoadWithBaseURL != nil {
		return *r.LoadWithBaseURL
	}
	return true
}

func (r rssSourceRequest) headerText() string {
	return normalizeRSSHeaderValue(r.Header)
}

func normalizeRSSHeaderValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		bytes, err := json.Marshal(typed)
		if err == nil {
			return string(bytes)
		}
	case map[string]string:
		bytes, err := json.Marshal(typed)
		if err == nil {
			return string(bytes)
		}
	default:
		bytes, err := json.Marshal(typed)
		if err == nil && string(bytes) != "null" {
			return string(bytes)
		}
	}
	return ""
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
	requestedSortURL := strings.TrimSpace(c.Query("sortUrl"))
	articles, pageCount, err := fetchRSSArticlesContext(c.Request.Context(), source, requestedSortURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to fetch RSS source: " + err.Error()})
		return
	}
	imported := 0
	sortName := strings.TrimSpace(c.Query("sortName"))
	if sortName == "" {
		sortName = rssSourceSortName(source, requestedSortURL)
	}
	for _, article := range articles {
		article.UserID = userID
		article.SourceID = source.ID
		article.Sort = sortName
		var existing models.RSSArticle
		if article.Link != "" && s.db.Where("user_id = ? AND source_id = ? AND link = ?", userID, source.ID, article.Link).First(&existing).Error == nil {
			existing.Title = article.Title
			existing.Sort = article.Sort
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
		"pages":    pageCount,
	})
	c.JSON(http.StatusOK, gin.H{
		"imported": imported,
		"total":    len(articles),
		"pages":    pageCount,
		"sortUrl":  rssSourceFetchURL(source, requestedSortURL),
	})
}

func (s *Server) listRSSArticles(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	query := s.db.Where("user_id = ?", userID)
	if sourceID := strings.TrimSpace(c.Query("sourceId")); sourceID != "" {
		query = query.Where("source_id = ?", sourceID)
	}
	if sortName := strings.TrimSpace(c.Query("sort")); sortName != "" {
		query = query.Where("sort = ?", sortName)
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

func (s *Server) getRSSArticleContent(c *gin.Context) {
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
	var source models.RSSSource
	if err := s.db.Where("user_id = ? AND id = ?", userID, article.SourceID).First(&source).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "RSS source not found"})
		return
	}
	if strings.TrimSpace(source.RuleContent) != "" && strings.TrimSpace(article.Link) != "" &&
		(strings.TrimSpace(article.Content) == "" || c.Query("refresh") == "1") {
		request, err := engine.PrepareSourceRequest(article.Link, "", 1, "utf-8", rssSourceHeaders(source), rssSourceRequestPolicy(source))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to prepare RSS article: " + err.Error()})
			return
		}
		body, responseURL, err := engine.FetchSourceTextWithURLContext(c.Request.Context(), request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to fetch RSS article: " + err.Error()})
			return
		}
		content, err := engine.ExtractRSSRuleContent(body, responseURL, source.RuleContent)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse RSS article: " + err.Error()})
			return
		}
		if strings.TrimSpace(content) != "" {
			article.Content = content
			if err := s.db.Save(&article).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cache RSS article"})
				return
			}
		}
	}
	article.Summary = engine.SanitizeRSSHTML(article.Summary, article.Link)
	article.Content = engine.SanitizeRSSHTML(article.Content, article.Link)
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

func fetchRSSArticles(source models.RSSSource, requestedSortURL ...string) ([]models.RSSArticle, error) {
	articles, _, err := fetchRSSArticlesContext(context.Background(), source, requestedSortURL...)
	return articles, err
}

func fetchRSSArticlesContext(ctx context.Context, source models.RSSSource, requestedSortURL ...string) ([]models.RSSArticle, int, error) {
	overrideURL := ""
	if len(requestedSortURL) > 0 {
		overrideURL = requestedSortURL[0]
	}
	fetchURL := rssSourceFetchURL(source, overrideURL)
	headers := rssSourceHeaders(source)
	if strings.TrimSpace(source.RuleArticles) != "" {
		return fetchRSSRuleArticles(ctx, source, fetchURL, headers)
	}
	request, err := engine.PrepareSourceRequest(fetchURL, "", 1, "utf-8", headers, rssSourceRequestPolicy(source))
	if err != nil {
		return nil, 0, err
	}
	text, responseURL, err := engine.FetchSourceTextWithURLContext(ctx, request)
	if err != nil {
		return nil, 0, err
	}
	var parsed parsedRSS
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, 0, err
	}
	articles := make([]models.RSSArticle, 0)
	for _, item := range parsed.Channel.Items {
		link := strings.TrimSpace(item.Link)
		if link != "" {
			link = resolveRSSFetchURL(responseURL, link)
		}
		articles = append(articles, models.RSSArticle{
			Title:       strings.TrimSpace(item.Title),
			Link:        link,
			Author:      firstNonEmpty(item.Creator, item.Author),
			Image:       rssItemImage(item.Enclosure.URL, item.Enclosure.Type, item.MediaThumbnail, item.MediaContent),
			Summary:     engine.SanitizeRSSHTML(item.Description, link),
			Content:     engine.SanitizeRSSHTML(item.Encoded, link),
			PublishedAt: parseRSSDate(item.PubDate),
		})
	}
	for _, entry := range parsed.Entries {
		link := ""
		if len(entry.Link) > 0 {
			link = entry.Link[0].Href
		}
		if strings.TrimSpace(link) != "" {
			link = resolveRSSFetchURL(responseURL, link)
		}
		articles = append(articles, models.RSSArticle{
			Title:       strings.TrimSpace(entry.Title),
			Link:        strings.TrimSpace(link),
			Author:      strings.TrimSpace(entry.Author.Name),
			Image:       atomEntryImage(entry.Link, entry.MediaThumbnail, entry.MediaContent),
			Summary:     engine.SanitizeRSSHTML(entry.Summary, link),
			Content:     engine.SanitizeRSSHTML(entry.Content, link),
			PublishedAt: parseRSSDate(firstNonEmpty(entry.Published, entry.Updated)),
		})
	}
	filtered := articles[:0]
	for _, article := range articles {
		if article.Title != "" {
			filtered = append(filtered, article)
		}
	}
	return filtered, 1, nil
}

func fetchRSSRuleArticles(ctx context.Context, source models.RSSSource, fetchURL string, headers map[string]string) ([]models.RSSArticle, int, error) {
	rules := engine.RSSRuleSet{
		Articles:    source.RuleArticles,
		Title:       source.RuleTitle,
		PubDate:     source.RulePubDate,
		Description: source.RuleDescription,
		Image:       source.RuleImage,
		Link:        source.RuleLink,
	}
	currentTemplate := fetchURL
	pageMode := strings.EqualFold(strings.TrimSpace(source.RuleNextPage), "PAGE")
	visitedRequests := make(map[string]bool)
	articleKeys := make(map[string]bool)
	articles := make([]models.RSSArticle, 0)
	pageCount := 0

	for page := 1; pageCount < maxRSSPaginationPages; page++ {
		request, err := engine.PrepareSourceRequest(currentTemplate, "", page, "utf-8", headers, rssSourceRequestPolicy(source))
		if err != nil {
			return nil, pageCount, err
		}
		requestKey := engine.SourceRequestKey(request)
		if visitedRequests[requestKey] {
			break
		}
		visitedRequests[requestKey] = true

		text, responseURL, err := engine.FetchSourceTextWithURLContext(ctx, request)
		if err != nil {
			return nil, pageCount, err
		}
		responseRequest := request
		responseRequest.URL = responseURL
		responseRequestKey := engine.SourceRequestKey(responseRequest)
		if responseRequestKey != requestKey && visitedRequests[responseRequestKey] {
			break
		}
		visitedRequests[responseRequestKey] = true
		pageCount++
		result, err := engine.ParseRSSRulePage(text, responseURL, rules, source.RuleNextPage)
		if err != nil {
			return nil, pageCount, err
		}
		for _, row := range result.Articles {
			key := strings.TrimSpace(row.Link)
			if key == "" {
				key = strings.TrimSpace(row.Title) + "\n" + strings.TrimSpace(row.PubDate)
			}
			if key == "" || articleKeys[key] {
				continue
			}
			articleKeys[key] = true
			summaryBaseURL := row.Link
			if request, prepareErr := engine.PrepareSourceRequest(row.Link, "", 1, "utf-8", headers, rssSourceRequestPolicy(source)); prepareErr == nil {
				summaryBaseURL = request.URL
			}
			articles = append(articles, models.RSSArticle{
				Title:       row.Title,
				Link:        row.Link,
				Image:       row.Image,
				Summary:     engine.SanitizeRSSHTML(row.Description, summaryBaseURL),
				PublishedAt: parseRSSDate(row.PubDate),
			})
		}
		if result.NextURL == "" {
			break
		}
		if pageCount >= maxRSSPaginationPages {
			return nil, pageCount, fmt.Errorf("RSS pagination exceeds %d pages", maxRSSPaginationPages)
		}
		if !pageMode {
			currentTemplate = result.NextURL
		}
	}
	return articles, pageCount, nil
}

var rssSortURLSeparator = regexp.MustCompile(`(?:&&|\r?\n)+`)

func rssSourceFetchURL(source models.RSSSource, requestedURL ...string) string {
	baseURL := strings.TrimSpace(source.URL)
	if len(requestedURL) > 0 && strings.TrimSpace(requestedURL[0]) != "" {
		return resolveRSSFetchURL(baseURL, requestedURL[0])
	}
	sortRule := strings.TrimSpace(source.SortURL)
	if sortRule == "" || strings.HasPrefix(sortRule, "@js:") || strings.HasPrefix(sortRule, "<js>") {
		return baseURL
	}
	first := strings.TrimSpace(rssSortURLSeparator.Split(sortRule, 2)[0])
	if index := strings.Index(first, "::"); index >= 0 {
		first = strings.TrimSpace(first[index+2:])
	}
	if first == "" {
		return baseURL
	}
	return resolveRSSFetchURL(baseURL, first)
}

func rssSourceSortName(source models.RSSSource, requestedURL string) string {
	requestedURL = strings.TrimSpace(requestedURL)
	options := rssSourceSortOptions(source)
	if requestedURL == "" {
		return options[0].Name
	}
	resolvedRequestedURL := resolveRSSFetchURL(source.URL, requestedURL)
	for _, option := range options {
		if resolveRSSFetchURL(source.URL, option.URL) == resolvedRequestedURL {
			return option.Name
		}
	}
	return ""
}

type rssSortOption struct {
	Name string
	URL  string
}

func rssSourceSortOptions(source models.RSSSource) []rssSortOption {
	baseURL := strings.TrimSpace(source.URL)
	sortRule := strings.TrimSpace(source.SortURL)
	if sortRule == "" || strings.HasPrefix(sortRule, "@js:") || strings.HasPrefix(sortRule, "<js>") {
		return []rssSortOption{{Name: "", URL: baseURL}}
	}
	rows := rssSortURLSeparator.Split(sortRule, -1)
	options := make([]rssSortOption, 0, len(rows))
	for _, row := range rows {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		name := ""
		value := row
		if index := strings.Index(row, "::"); index >= 0 {
			name = strings.TrimSpace(row[:index])
			value = strings.TrimSpace(row[index+2:])
		}
		if value != "" {
			options = append(options, rssSortOption{Name: name, URL: value})
		}
	}
	if len(options) == 0 {
		return []rssSortOption{{Name: "", URL: baseURL}}
	}
	return options
}

func resolveRSSFetchURL(baseURL string, value string) string {
	resolved := engine.ResolveSourceURLTemplate(baseURL, strings.TrimSpace(value))
	if resolved == "" {
		return baseURL
	}
	return resolved
}

func rssSourceHeaders(source models.RSSSource) map[string]string {
	raw := strings.TrimSpace(source.Header)
	if raw == "" {
		return nil
	}
	var object map[string]any
	if json.Unmarshal([]byte(raw), &object) == nil {
		headers := make(map[string]string, len(object))
		for name, value := range object {
			if strings.TrimSpace(name) != "" && value != nil {
				headers[name] = fmt.Sprint(value)
			}
		}
		return headers
	}
	headers := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		name, value, found := strings.Cut(line, ":")
		if found && strings.TrimSpace(name) != "" {
			headers[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	return headers
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
