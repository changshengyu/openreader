package engine

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"openreader/backend/models"
)

// SearchResult represents a single book found through remote search.
type SearchResult struct {
	Title         string `json:"title"`
	Author        string `json:"author"`
	CoverURL      string `json:"coverUrl"`
	Intro         string `json:"intro"`
	LatestChapter string `json:"latestChapter"`
	BookURL       string `json:"bookUrl"`
	SourceID      uint   `json:"sourceId"`
	SourceName    string `json:"sourceName"`
}

// ExploreResult represents one page of source discovery results.
type ExploreResult struct {
	Items   []SearchResult `json:"items"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
	NextURL string         `json:"nextUrl,omitempty"`
}

// RemoteChapter represents a chapter parsed from a remote book source.
type RemoteChapter struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Index int    `json:"index"`
}

// SearchBooks performs a remote search against a single book source.
func SearchBooks(source models.BookSource, keyword string) ([]SearchResult, error) {
	rule, err := source.ParsedRules()
	if err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	if rule.SearchURL == "" {
		return nil, fmt.Errorf("source %q has no search URL", source.Name)
	}

	searchURL := strings.ReplaceAll(rule.SearchURL, "{keyword}", url.QueryEscape(keyword))
	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}

	doc, err := FetchDocument(searchURL, charset)
	if err != nil {
		return nil, fmt.Errorf("fetch search page: %w", err)
	}

	return parseSearchResults(doc, rule, source), nil
}

func ExploreBooks(source models.BookSource) ([]SearchResult, error) {
	result, err := ExploreBooksPage(source, 1)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func ExploreBooksPage(source models.BookSource, page int) (ExploreResult, error) {
	return ExploreBooksPageWithURL(source, "", page)
}

func ExploreBooksPageWithURL(source models.BookSource, exploreURLOverride string, page int) (ExploreResult, error) {
	if page < 1 {
		page = 1
	}
	rule, err := source.ParsedRules()
	if err != nil {
		return ExploreResult{}, fmt.Errorf("parse rules: %w", err)
	}
	activeExploreURL := strings.TrimSpace(exploreURLOverride)
	if activeExploreURL == "" {
		activeExploreURL = strings.TrimSpace(rule.ExploreURL)
	}
	if activeExploreURL == "" {
		return ExploreResult{}, fmt.Errorf("source %q has no explore URL", source.Name)
	}
	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	baseURL := source.BaseURL
	if baseURL == "" {
		baseURL = source.SearchURL
	}
	if baseURL != "" {
		activeExploreURL = resolveURL(baseURL, activeExploreURL)
	}
	exploreURL := strings.ReplaceAll(activeExploreURL, "{page}", fmt.Sprintf("%d", page))
	doc, err := FetchDocument(exploreURL, charset)
	if err != nil {
		return ExploreResult{}, fmt.Errorf("fetch explore page: %w", err)
	}
	items := parseBookResults(doc, rule, source, exploreURL)
	nextURL := ""
	if rule.PaginationRule != "" {
		nextURL = resolveURL(exploreURL, firstMatch(doc.Selection, rule.PaginationRule))
	}
	hasMore := strings.Contains(activeExploreURL, "{page}") && len(items) > 0
	if nextURL != "" {
		hasMore = true
	}
	return ExploreResult{
		Items:   items,
		Page:    page,
		HasMore: hasMore,
		NextURL: nextURL,
	}, nil
}

func parseSearchResults(doc *goquery.Document, rule models.BookSourceRule, source models.BookSource) []SearchResult {
	baseURL := source.BaseURL
	if baseURL == "" {
		baseURL = source.SearchURL
	}
	return parseBookResults(doc, rule, source, baseURL)
}

func parseBookResults(doc *goquery.Document, rule models.BookSourceRule, source models.BookSource, baseURL string) []SearchResult {
	items := findItems(doc, rule.BookListRule)

	results := make([]SearchResult, 0, len(items))
	for _, sel := range items {
		result := SearchResult{
			SourceID:   source.ID,
			SourceName: source.Name,
		}
		result.Title = firstMatch(sel, rule.BookNameRule)
		result.Author = firstMatch(sel, rule.BookAuthorRule)
		result.CoverURL = resolveURL(baseURL, firstMatch(sel, rule.BookCoverRule))
		result.Intro = firstMatch(sel, rule.BookIntroRule)
		result.LatestChapter = firstMatch(sel, rule.LatestChapterRule)
		result.BookURL = resolveURL(baseURL, firstMatch(sel, rule.BookURLRule))

		if result.Title == "" || result.BookURL == "" {
			continue
		}
		results = append(results, result)
	}
	return results
}

// ParseTOC fetches and parses a book's table of contents.
func ParseTOC(bookURL string, source models.BookSource) ([]RemoteChapter, error) {
	rule, err := source.ParsedRules()
	if err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}

	tocURL := bookURL
	if rule.TOCURLRule != "" {
		tocURL = resolveURL(bookURL, rule.TOCURLRule)
	}

	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}

	doc, err := FetchDocument(tocURL, charset)
	if err != nil {
		return nil, fmt.Errorf("fetch toc page: %w", err)
	}

	chapters := parseChapterList(doc, rule, tocURL)
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapters found on toc page")
	}
	return chapters, nil
}

func parseChapterList(doc *goquery.Document, rule models.BookSourceRule, baseURL string) []RemoteChapter {
	items := findItems(doc, rule.ChapterListRule)
	chapters := make([]RemoteChapter, 0, len(items))
	for i, sel := range items {
		title := firstMatch(sel, rule.ChapterNameRule)
		chapterURL := resolveURL(baseURL, firstMatch(sel, rule.ChapterURLRule))
		if title == "" || chapterURL == "" {
			continue
		}
		chapters = append(chapters, RemoteChapter{
			Title: title,
			URL:   chapterURL,
			Index: i,
		})
	}
	return chapters
}

// FetchChapterContent fetches and parses a single chapter's content.
func FetchChapterContent(chapterURL string, source models.BookSource) (string, error) {
	rule, err := source.ParsedRules()
	if err != nil {
		return "", fmt.Errorf("parse rules: %w", err)
	}

	contentURL := chapterURL
	if rule.ContentURLRule != "" {
		contentURL = resolveURL(chapterURL, rule.ContentURLRule)
	}

	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}

	doc, err := FetchDocument(contentURL, charset)
	if err != nil {
		return "", fmt.Errorf("fetch content page: %w", err)
	}

	text := ""
	if rule.ContentRule != "" {
		values := Extract(doc.Selection, rule.ContentRule)
		text = strings.Join(values, "\n")
	} else {
		values := Extract(doc.Selection, "body|text")
		text = strings.Join(values, "\n")
	}

	text = ApplyTextReplacements(text, rule.TextReplaceRules)
	return text, nil
}

func findItems(doc *goquery.Document, rule string) []*goquery.Selection {
	if rule == "" {
		return []*goquery.Selection{doc.Selection}
	}
	parts := strings.SplitN(rule, "|", 2)
	selector := strings.TrimSpace(parts[0])
	if selector == "" {
		return []*goquery.Selection{doc.Selection}
	}
	items := make([]*goquery.Selection, 0)
	doc.Find(selector).Each(func(_ int, sel *goquery.Selection) {
		items = append(items, sel)
	})
	if len(items) == 0 {
		return []*goquery.Selection{doc.Selection}
	}
	return items
}

func firstMatch(sel *goquery.Selection, rule string) string {
	if rule == "" {
		return ""
	}
	values := Extract(sel, rule)
	if len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}

func ApplyTextReplacements(text string, rules []models.TextReplaceRule) string {
	for _, r := range rules {
		if r.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			text = strings.ReplaceAll(text, r.Pattern, r.Replacement)
			continue
		}
		text = re.ReplaceAllString(text, r.Replacement)
	}
	return text
}

func resolveURL(base, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "javascript:") {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return href
	}
	resolved, err := baseURL.Parse(href)
	if err != nil {
		return href
	}
	return resolved.String()
}
