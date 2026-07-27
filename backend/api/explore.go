package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/services/booksources"
)

type exploreSourceResponse struct {
	ID            uint             `json:"id"`
	Name          string           `json:"name"`
	Group         string           `json:"group"`
	ExploreGroups [][]exploreEntry `json:"exploreGroups"`
}

type exploreEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *Server) listExploreSources(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	sources, err := s.bookSources.ListActiveByIDs(userID, nil, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}
	out := make([]exploreSourceResponse, 0)
	for _, source := range sources {
		if !source.IsExploreEnabled() {
			continue
		}
		rule, err := source.ParsedRules()
		if err == nil && strings.TrimSpace(rule.ExploreURL) != "" {
			out = append(out, exploreSourceResponse{
				ID:            source.ID,
				Name:          source.Name,
				Group:         source.Group,
				ExploreGroups: parseExploreGroups(rule.ExploreURL),
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) exploreBooks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	sourceID, ok := parseUintParam(c, "sourceId")
	if !ok {
		return
	}
	source, err := s.bookSources.FindActive(userID, uint(sourceID))
	if errors.Is(err, booksources.ErrSourceNotFound) || err == nil && (!source.Enabled || !source.IsExploreEnabled()) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}
	page := 1
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
			return
		}
		page = parsed
	}
	exploreURL := strings.TrimSpace(c.Query("url"))
	if exploreURL == "" {
		exploreURL = strings.TrimSpace(c.Query("exploreUrl"))
	}
	results, err := engine.ExploreBooksPageWithURL(source, exploreURL, page)
	if err != nil {
		s.recordSourceFailure(userID, source, err)
		writeSourceError(c, http.StatusBadRequest, "failed to explore source", err, "explore")
		return
	}
	results.Items = s.projectSearchResultCovers(userID, results.Items)
	c.JSON(http.StatusOK, results)
}

type exploreURLItem struct {
	Title string                 `json:"title"`
	Name  string                 `json:"name"`
	URL   string                 `json:"url"`
	Style map[string]interface{} `json:"style"`
}

func parseExploreGroups(raw string) [][]exploreEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if groups := parseExploreJSONGroups(raw); len(groups) > 0 {
		return groups
	}
	if groups := parseExploreLineGroups(raw); len(groups) > 0 {
		return groups
	}
	return [][]exploreEntry{{{Name: "默认", URL: raw}}}
}

func parseExploreJSONGroups(raw string) [][]exploreEntry {
	var items []exploreURLItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	groups := make([][]exploreEntry, 0)
	row := make([]exploreEntry, 0)
	rowWidth := 0.0
	for _, item := range items {
		name := strings.TrimSpace(item.Title)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		entryURL := strings.TrimSpace(item.URL)
		if name == "" || entryURL == "" {
			continue
		}
		row = append(row, exploreEntry{Name: name, URL: entryURL})
		rowWidth += exploreItemWidth(item.Style)
		if rowWidth >= 0.999 {
			groups = append(groups, row)
			row = make([]exploreEntry, 0)
			rowWidth = 0
		}
	}
	if len(row) > 0 {
		groups = append(groups, row)
	}
	return groups
}

func exploreItemWidth(style map[string]interface{}) float64 {
	if style == nil {
		return 0.25
	}
	for _, key := range []string{"layout_flexBasisPercent", "layout_flexGrow"} {
		if value, ok := style[key]; ok {
			switch typed := value.(type) {
			case float64:
				if typed > 0 {
					return typed
				}
			case string:
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil && parsed > 0 {
					return parsed
				}
			}
		}
	}
	return 0.25
}

func parseExploreLineGroups(raw string) [][]exploreEntry {
	groups := make([][]exploreEntry, 0)
	row := make([]exploreEntry, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(row) > 0 {
				groups = append(groups, row)
				row = make([]exploreEntry, 0)
			}
			continue
		}
		name, entryURL, ok := strings.Cut(line, "::")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		entryURL = strings.TrimSpace(entryURL)
		if name == "" || entryURL == "" {
			continue
		}
		row = append(row, exploreEntry{Name: name, URL: entryURL})
	}
	if len(row) > 0 {
		groups = append(groups, row)
	}
	return groups
}
