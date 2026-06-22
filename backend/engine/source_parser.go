package engine

import (
	"context"
	"fmt"
	stdhtml "html"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"

	"openreader/backend/models"
)

const maxSourcePaginationPages = 1000

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

type SearchPageResult struct {
	Items   []SearchResult `json:"items"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
	NextURL string         `json:"nextUrl,omitempty"`
}

type RemoteBookInfo struct {
	Title         string `json:"title"`
	Author        string `json:"author"`
	CoverURL      string `json:"coverUrl"`
	Intro         string `json:"intro"`
	Kind          string `json:"kind"`
	LatestChapter string `json:"latestChapter"`
	UpdateTime    string `json:"updateTime"`
	WordCount     string `json:"wordCount"`
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
	return SearchBooksContext(context.Background(), source, keyword)
}

func SearchBooksContext(ctx context.Context, source models.BookSource, keyword string) ([]SearchResult, error) {
	result, err := SearchBooksPageContext(ctx, source, keyword, 1)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func SearchBooksPage(source models.BookSource, keyword string, page int) (SearchPageResult, error) {
	return SearchBooksPageContext(context.Background(), source, keyword, page)
}

func SearchBooksPageContext(ctx context.Context, source models.BookSource, keyword string, page int) (SearchPageResult, error) {
	if page < 1 {
		page = 1
	}
	rule, err := source.ParsedRules()
	if err != nil {
		return SearchPageResult{}, fmt.Errorf("parse rules: %w", err)
	}
	if rule.SearchURL == "" {
		return SearchPageResult{}, fmt.Errorf("source %q has no search URL", source.Name)
	}

	searchURLTemplate := strings.ReplaceAll(rule.SearchURL, "{keyword}", url.QueryEscape(keyword))
	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}

	if strings.Contains(searchURLTemplate, "{page}") {
		searchURL := strings.ReplaceAll(searchURLTemplate, "{page}", fmt.Sprintf("%d", page))
		doc, err := FetchDocumentWithHeadersContext(ctx, searchURL, charset, rule.Headers)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("fetch search page: %w", err)
		}
		items := parseBookResults(doc, rule, source, searchURL)
		nextURL := searchNextURL(doc, rule, searchURL)
		return SearchPageResult{
			Items:   items,
			Page:    page,
			HasMore: len(items) > 0 || nextURL != "",
			NextURL: nextURL,
		}, nil
	}

	if page > 1 && strings.TrimSpace(rule.PaginationRule) == "" {
		return SearchPageResult{Items: []SearchResult{}, Page: page}, nil
	}

	searchURL := searchURLTemplate
	for currentPage := 1; currentPage <= page; currentPage++ {
		doc, err := FetchDocumentWithHeadersContext(ctx, searchURL, charset, rule.Headers)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("fetch search page: %w", err)
		}
		items := parseBookResults(doc, rule, source, searchURL)
		nextURL := searchNextURL(doc, rule, searchURL)
		if currentPage == page {
			return SearchPageResult{
				Items:   items,
				Page:    page,
				HasMore: nextURL != "",
				NextURL: nextURL,
			}, nil
		}
		if nextURL == "" {
			return SearchPageResult{Items: []SearchResult{}, Page: page}, nil
		}
		searchURL = nextURL
	}

	return SearchPageResult{Items: []SearchResult{}, Page: page}, nil
}

func searchNextURL(doc *goquery.Document, rule models.BookSourceRule, searchURL string) string {
	if strings.TrimSpace(rule.PaginationRule) == "" {
		return ""
	}
	return resolveURL(searchURL, firstMatch(doc.Selection, rule.PaginationRule))
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
	doc, err := FetchDocumentWithHeaders(exploreURL, charset, rule.Headers)
	if err != nil {
		return ExploreResult{}, fmt.Errorf("fetch explore page: %w", err)
	}
	exploreRule := effectiveExploreRule(rule)
	items := parseBookResults(doc, exploreRule, source, exploreURL)
	nextURL := ""
	if exploreRule.PaginationRule != "" {
		nextURL = resolveURL(exploreURL, firstMatch(doc.Selection, exploreRule.PaginationRule))
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

func effectiveExploreRule(rule models.BookSourceRule) models.BookSourceRule {
	if strings.TrimSpace(rule.ExploreBookListRule) == "" {
		return rule
	}
	exploreRule := rule
	exploreRule.BookListRule = rule.ExploreBookListRule
	exploreRule.BookNameRule = rule.ExploreBookNameRule
	exploreRule.BookAuthorRule = rule.ExploreBookAuthorRule
	exploreRule.BookCoverRule = rule.ExploreBookCoverRule
	exploreRule.BookIntroRule = rule.ExploreBookIntroRule
	exploreRule.LatestChapterRule = rule.ExploreLatestChapterRule
	exploreRule.BookURLRule = rule.ExploreBookURLRule
	exploreRule.PaginationRule = rule.ExplorePaginationRule
	return exploreRule
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

	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	return parseTOCWithRule(bookURL, rule, charset, nil)
}

func FetchBookInfoAndTOC(bookURL string, source models.BookSource) (RemoteBookInfo, []RemoteChapter, error) {
	rule, err := source.ParsedRules()
	if err != nil {
		return RemoteBookInfo{}, nil, fmt.Errorf("parse rules: %w", err)
	}
	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	bookDoc, err := FetchDocumentWithHeaders(bookURL, charset, rule.Headers)
	if err != nil {
		return RemoteBookInfo{}, nil, fmt.Errorf("fetch book info page: %w", err)
	}
	info := parseRemoteBookInfo(bookDoc, rule, bookURL)
	chapters, err := parseTOCWithRule(bookURL, rule, charset, bookDoc)
	if err != nil {
		return RemoteBookInfo{}, nil, err
	}
	return info, chapters, nil
}

func parseRemoteBookInfo(doc *goquery.Document, rule models.BookSourceRule, baseURL string) RemoteBookInfo {
	return RemoteBookInfo{
		Title:         firstMatch(doc.Selection, rule.BookInfoNameRule),
		Author:        firstMatch(doc.Selection, rule.BookInfoAuthorRule),
		CoverURL:      resolveURL(baseURL, firstMatch(doc.Selection, rule.BookInfoCoverRule)),
		Intro:         firstMatch(doc.Selection, rule.BookInfoIntroRule),
		Kind:          firstMatch(doc.Selection, rule.BookInfoKindRule),
		LatestChapter: firstMatch(doc.Selection, rule.BookInfoLatestChapterRule),
		UpdateTime:    firstMatch(doc.Selection, rule.BookInfoUpdateTimeRule),
		WordCount:     firstMatch(doc.Selection, rule.BookInfoWordCountRule),
	}
}

func parseTOCWithRule(bookURL string, rule models.BookSourceRule, charset string, bookDoc *goquery.Document) ([]RemoteChapter, error) {
	var err error
	tocURL := bookURL
	var doc *goquery.Document
	tocURLRule := strings.TrimSpace(rule.TOCURLRule)
	switch {
	case tocURLRule == "":
		if bookDoc != nil {
			doc = bookDoc
		} else {
			doc, err = FetchDocumentWithHeaders(bookURL, charset, rule.Headers)
		}
	case isDirectTOCURLRule(tocURLRule):
		tocURL = resolveURL(bookURL, tocURLRule)
		if tocURL == bookURL && bookDoc != nil {
			doc = bookDoc
		} else {
			doc, err = FetchDocumentWithHeaders(tocURL, charset, rule.Headers)
		}
	default:
		if bookDoc == nil {
			bookDoc, err = FetchDocumentWithHeaders(bookURL, charset, rule.Headers)
		}
		if err == nil {
			parsedTOCURL := firstMatch(bookDoc.Selection, tocURLRule)
			if parsedTOCURL == "" {
				doc = bookDoc
			} else {
				tocURL = resolveURL(bookURL, parsedTOCURL)
				if tocURL == bookURL {
					doc = bookDoc
				} else {
					doc, err = FetchDocumentWithHeaders(tocURL, charset, rule.Headers)
				}
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("fetch toc page: %w", err)
	}

	type tocPage struct {
		url string
		doc *goquery.Document
	}
	queue := []tocPage{{url: tocURL, doc: doc}}
	visited := map[string]bool{tocURL: true}
	chapters := make([]RemoteChapter, 0)
	chapterKeys := make(map[string]bool)
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		for _, chapter := range parseChapterList(page.doc, rule, page.url) {
			key := chapter.URL
			if key == "" {
				key = chapter.Title
			}
			if chapterKeys[key] {
				continue
			}
			chapterKeys[key] = true
			chapter.Index = len(chapters)
			chapters = append(chapters, chapter)
		}
		for _, nextURL := range extractResolvedURLs(page.doc.Selection, rule.NextTOCURLRule, page.url) {
			if visited[nextURL] {
				continue
			}
			if len(visited) >= maxSourcePaginationPages {
				return nil, fmt.Errorf("toc pagination exceeds %d pages", maxSourcePaginationPages)
			}
			visited[nextURL] = true
			nextDoc, fetchErr := FetchDocumentWithHeaders(nextURL, charset, rule.Headers)
			if fetchErr != nil {
				return nil, fmt.Errorf("fetch toc page: %w", fetchErr)
			}
			queue = append(queue, tocPage{url: nextURL, doc: nextDoc})
		}
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapters found on toc page")
	}
	return chapters, nil
}

func isDirectTOCURLRule(rule string) bool {
	value := strings.TrimSpace(rule)
	if value == "" || strings.Contains(value, "|") {
		return false
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(value, "//") ||
		strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "./") ||
		strings.HasPrefix(value, "../")
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

func extractResolvedURLs(selection *goquery.Selection, rule string, baseURL string) []string {
	if strings.TrimSpace(rule) == "" {
		return nil
	}
	values := Extract(selection, rule)
	urls := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		resolved := resolveURL(baseURL, value)
		if resolved == "" || seen[resolved] {
			continue
		}
		seen[resolved] = true
		urls = append(urls, resolved)
	}
	return urls
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

	doc, err := FetchDocumentWithHeaders(contentURL, charset, rule.Headers)
	if err != nil {
		return "", fmt.Errorf("fetch content page: %w", err)
	}

	type contentPage struct {
		url string
		doc *goquery.Document
	}
	queue := []contentPage{{url: contentURL, doc: doc}}
	visited := map[string]bool{contentURL: true}
	parts := make([]string, 0)
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		if text := extractChapterContent(page.doc, rule.ContentRule, page.url); text != "" {
			parts = append(parts, text)
		}
		for _, nextURL := range extractResolvedURLs(page.doc.Selection, rule.NextContentURLRule, page.url) {
			if visited[nextURL] {
				continue
			}
			if len(visited) >= maxSourcePaginationPages {
				return "", fmt.Errorf("content pagination exceeds %d pages", maxSourcePaginationPages)
			}
			visited[nextURL] = true
			nextDoc, fetchErr := FetchDocumentWithHeaders(nextURL, charset, rule.Headers)
			if fetchErr != nil {
				return "", fmt.Errorf("fetch content page: %w", fetchErr)
			}
			queue = append(queue, contentPage{url: nextURL, doc: nextDoc})
		}
	}

	text := strings.Join(parts, "\n")
	text = ApplyTextReplacements(text, rule.TextReplaceRules)
	return text, nil
}

func extractChapterContent(doc *goquery.Document, contentRule string, baseURL string) string {
	if contentRule == "" {
		return strings.Join(Extract(doc.Selection, "body|text"), "\n")
	}
	values := Extract(doc.Selection, contentRule)
	if ruleOperation(contentRule) == "html" {
		for index := range values {
			values[index] = normalizeChapterHTML(values[index], baseURL)
		}
	}
	return strings.Join(values, "\n")
}

func ruleOperation(rule string) string {
	parts := strings.Split(rule, "|")
	if len(parts) < 2 {
		return "text"
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func normalizeChapterHTML(fragment string, baseURL string) string {
	doc, err := html.Parse(strings.NewReader("<html><body>" + fragment + "</body></html>"))
	if err != nil {
		return strings.TrimSpace(fragment)
	}
	lines := make([]string, 0)
	var text strings.Builder
	flushText := func() {
		value := strings.Join(strings.Fields(text.String()), " ")
		text.Reset()
		if value != "" {
			lines = append(lines, value)
		}
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			if value := strings.TrimSpace(node.Data); value != "" {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(value)
			}
			return
		}
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			if tag == "script" || tag == "style" || tag == "noscript" {
				return
			}
			if tag == "img" {
				flushText()
				src := firstHTMLAttr(node, "src", "data-src", "data-original", "data-url")
				src = resolveURL(baseURL, src)
				if isSafeChapterImageURL(src) {
					alt := strings.TrimSpace(firstHTMLAttr(node, "alt", "title"))
					lines = append(lines, `<img src="`+stdhtml.EscapeString(src)+`" alt="`+stdhtml.EscapeString(alt)+`">`)
				}
				return
			}
			if isChapterBlockTag(tag) {
				flushText()
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			if isChapterBlockTag(tag) {
				flushText()
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	flushText()
	return strings.Join(lines, "\n")
}

func firstHTMLAttr(node *html.Node, names ...string) string {
	for _, name := range names {
		for _, attr := range node.Attr {
			if strings.EqualFold(attr.Key, name) && strings.TrimSpace(attr.Val) != "" {
				return strings.TrimSpace(attr.Val)
			}
		}
	}
	return ""
}

func isChapterBlockTag(tag string) bool {
	switch tag {
	case "address", "article", "aside", "blockquote", "br", "div", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "li", "main", "nav", "ol", "p", "pre", "section", "table", "tr", "ul":
		return true
	default:
		return false
	}
}

func isSafeChapterImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
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
		if r.IsRegex != nil && !*r.IsRegex {
			text = strings.Replace(text, r.Pattern, r.Replacement, 1)
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
