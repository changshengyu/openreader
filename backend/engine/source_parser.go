package engine

import (
	"context"
	"fmt"
	stdhtml "html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"

	"openreader/backend/models"
)

const maxSourcePaginationPages = 1000

var sourceFalsePattern = regexp.MustCompile(`(?i)^\s*(false|no|not|0)\s*$`)

// SearchResult represents a single book found through remote search.
type SearchResult struct {
	Title         string `json:"title"`
	Author        string `json:"author"`
	CoverURL      string `json:"coverUrl"`
	Intro         string `json:"intro"`
	Kind          string `json:"kind"`
	WordCount     string `json:"wordCount"`
	LatestChapter string `json:"latestChapter"`
	UpdateTime    string `json:"updateTime"`
	BookURL       string `json:"bookUrl"`
	SourceID      uint   `json:"sourceId"`
	SourceName    string `json:"sourceName"`
	OriginOrder   int    `json:"originOrder"`
	Type          int    `json:"type"`
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
	CanRename     bool   `json:"canRename"`
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
	Title    string `json:"title"`
	URL      string `json:"url"`
	Index    int    `json:"index"`
	IsVolume bool   `json:"isVolume"`
	Tag      string `json:"tag"`
}

func bookSourceRequestPolicy(source models.BookSource) SourceRequestPolicy {
	key := strings.TrimSpace(source.BaseURL)
	if key == "" {
		key = fmt.Sprintf("book-source:%d", source.ID)
	}
	return SourceRequestPolicy{
		SourceKey:      key,
		ConcurrentRate: strings.TrimSpace(source.ConcurrentRate),
	}
}

func fetchSourceDocumentContext(ctx context.Context, request sourceRequest) (*goquery.Document, sourceRequest, error) {
	document, responseURL, err := FetchSourceDocumentWithURLContext(ctx, request)
	if responseURL != "" {
		request.URL = responseURL
	}
	return document, request, err
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
	searchURLTemplate := resolveSourceURLTemplate(source.BaseURL, rule.SearchURL)

	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}

	if strings.Contains(searchURLTemplate, "{page}") {
		request, err := prepareSourceRequest(searchURLTemplate, keyword, page, charset, rule.Headers, bookSourceRequestPolicy(source))
		if err != nil {
			return SearchPageResult{}, err
		}
		doc, request, err := fetchSourceDocumentContext(ctx, request)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("fetch search page: %w", err)
		}
		items, err := parseBookResults(doc, rule, source, request)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("parse search page: %w", err)
		}
		nextURL := searchNextURL(doc, rule, request.URL)
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

	request, err := prepareSourceRequest(searchURLTemplate, keyword, 1, charset, rule.Headers, bookSourceRequestPolicy(source))
	if err != nil {
		return SearchPageResult{}, err
	}
	for currentPage := 1; currentPage <= page; currentPage++ {
		doc, fetchedRequest, err := fetchSourceDocumentContext(ctx, request)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("fetch search page: %w", err)
		}
		request = fetchedRequest
		items, err := parseBookResults(doc, rule, source, request)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("parse search page: %w", err)
		}
		nextURL := searchNextURL(doc, rule, request.URL)
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
		request, err = prepareSourceRequest(nextURL, keyword, currentPage+1, charset, rule.Headers, bookSourceRequestPolicy(source))
		if err != nil {
			return SearchPageResult{}, err
		}
	}

	return SearchPageResult{Items: []SearchResult{}, Page: page}, nil
}

func searchNextURL(doc *goquery.Document, rule models.BookSourceRule, searchURL string) string {
	if strings.TrimSpace(rule.PaginationRule) == "" {
		return ""
	}
	return resolveSourceURLTemplate(searchURL, firstMatch(doc.Selection, rule.PaginationRule))
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
		activeExploreURL = resolveSourceURLTemplate(baseURL, activeExploreURL)
	}
	request, err := prepareSourceRequest(activeExploreURL, "", page, charset, rule.Headers, bookSourceRequestPolicy(source))
	if err != nil {
		return ExploreResult{}, err
	}
	doc, request, err := fetchSourceDocumentContext(context.Background(), request)
	if err != nil {
		return ExploreResult{}, fmt.Errorf("fetch explore page: %w", err)
	}
	exploreRule := effectiveExploreRule(rule)
	items, err := parseBookResults(doc, exploreRule, source, request)
	if err != nil {
		return ExploreResult{}, fmt.Errorf("parse explore page: %w", err)
	}
	nextURL := ""
	if exploreRule.PaginationRule != "" {
		nextURL = resolveSourceURLTemplate(request.URL, firstMatch(doc.Selection, exploreRule.PaginationRule))
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
	exploreRule.BookKindRule = rule.ExploreBookKindRule
	exploreRule.BookWordCountRule = rule.ExploreBookWordCountRule
	exploreRule.LatestChapterRule = rule.ExploreLatestChapterRule
	exploreRule.BookUpdateTimeRule = rule.ExploreBookUpdateTimeRule
	exploreRule.BookURLRule = rule.ExploreBookURLRule
	exploreRule.PaginationRule = rule.ExplorePaginationRule
	return exploreRule
}

func parseSearchResults(doc *goquery.Document, rule models.BookSourceRule, source models.BookSource) []SearchResult {
	baseURL := source.BaseURL
	if baseURL == "" {
		baseURL = source.SearchURL
	}
	items, _ := parseBookResults(doc, rule, source, sourceRequest{URL: baseURL, Descriptor: baseURL})
	return items
}

func parseBookResults(doc *goquery.Document, rule models.BookSourceRule, source models.BookSource, request sourceRequest) ([]SearchResult, error) {
	baseURL := request.URL
	pattern := strings.TrimSpace(source.BookURLPattern)
	if pattern != "" {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid book URL pattern: %w", err)
		}
		match := compiled.FindStringIndex(baseURL)
		matched := len(match) == 2 && match[0] == 0 && match[1] == len(baseURL)
		if matched {
			if result, ok := parseDirectBookResult(doc, rule, source, request); ok {
				return []SearchResult{result}, nil
			}
			return []SearchResult{}, nil
		}
	}
	listRule, reverse := sourceListRule(rule.BookListRule)
	items := findItems(doc, listRule)
	if len(items) == 0 && pattern == "" {
		if result, ok := parseDirectBookResult(doc, rule, source, request); ok {
			return []SearchResult{result}, nil
		}
	}

	results := make([]SearchResult, 0, len(items))
	for _, sel := range items {
		result := SearchResult{
			SourceID:    source.ID,
			SourceName:  source.Name,
			OriginOrder: source.CustomOrder,
			Type:        source.SourceType,
		}
		result.Title = firstMatch(sel, rule.BookNameRule)
		result.Author = firstMatch(sel, rule.BookAuthorRule)
		result.CoverURL = resolveURL(baseURL, firstMatch(sel, rule.BookCoverRule))
		result.Intro = firstMatch(sel, rule.BookIntroRule)
		result.Kind = strings.Join(Extract(sel, rule.BookKindRule), ",")
		result.WordCount = formatSourceWordCount(firstMatch(sel, rule.BookWordCountRule))
		result.LatestChapter = firstMatch(sel, rule.LatestChapterRule)
		result.UpdateTime = firstMatch(sel, rule.BookUpdateTimeRule)
		result.BookURL = resolveSourceURLTemplate(baseURL, firstMatch(sel, rule.BookURLRule))

		if result.Title == "" || result.BookURL == "" {
			continue
		}
		results = append(results, result)
	}
	if reverse {
		reverseSearchResults(results)
	}
	return results, nil
}

func parseDirectBookResult(doc *goquery.Document, rule models.BookSourceRule, source models.BookSource, request sourceRequest) (SearchResult, bool) {
	info := parseRemoteBookInfo(doc, rule, request.URL)
	if strings.TrimSpace(info.Title) == "" {
		return SearchResult{}, false
	}
	bookURL := request.Descriptor
	if bookURL == "" {
		bookURL = request.URL
	}
	return SearchResult{
		Title:         info.Title,
		Author:        info.Author,
		CoverURL:      info.CoverURL,
		Intro:         info.Intro,
		Kind:          info.Kind,
		WordCount:     formatSourceWordCount(info.WordCount),
		LatestChapter: info.LatestChapter,
		UpdateTime:    info.UpdateTime,
		BookURL:       bookURL,
		SourceID:      source.ID,
		SourceName:    source.Name,
		OriginOrder:   source.CustomOrder,
		Type:          source.SourceType,
	}, true
}

func formatSourceWordCount(value string) string {
	value = strings.TrimSpace(value)
	words, err := strconv.Atoi(value)
	if err != nil {
		return value
	}
	if words <= 0 {
		return ""
	}
	if words > 10000 {
		formatted := strconv.FormatFloat(float64(words)/10000, 'f', 1, 64)
		formatted = strings.TrimSuffix(formatted, ".0")
		return formatted + "万字"
	}
	return strconv.Itoa(words) + "字"
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
	return parseTOCWithRule(bookURL, source.BaseURL, rule, charset, bookSourceRequestPolicy(source), nil, nil)
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
	policy := bookSourceRequestPolicy(source)
	bookRequest, err := prepareResolvedSourceRequest(source.BaseURL, bookURL, "", 1, charset, rule.Headers, policy)
	if err != nil {
		return RemoteBookInfo{}, nil, fmt.Errorf("prepare book info request: %w", err)
	}
	bookDoc, bookRequest, err := fetchSourceDocumentContext(context.Background(), bookRequest)
	if err != nil {
		return RemoteBookInfo{}, nil, fmt.Errorf("fetch book info page: %w", err)
	}
	info := parseRemoteBookInfo(bookDoc, rule, bookRequest.URL)
	chapters, err := parseTOCWithRule(bookURL, source.BaseURL, rule, charset, policy, bookDoc, &bookRequest)
	if err != nil {
		return RemoteBookInfo{}, nil, err
	}
	return info, chapters, nil
}

func parseRemoteBookInfo(doc *goquery.Document, rule models.BookSourceRule, baseURL string) RemoteBookInfo {
	scope := bookInfoScope(doc, rule.BookInfoInitRule)
	return RemoteBookInfo{
		Title:         firstMatch(scope, rule.BookInfoNameRule),
		Author:        firstMatch(scope, rule.BookInfoAuthorRule),
		CoverURL:      resolveURL(baseURL, firstMatch(scope, rule.BookInfoCoverRule)),
		Intro:         firstMatch(scope, rule.BookInfoIntroRule),
		Kind:          firstMatch(scope, rule.BookInfoKindRule),
		LatestChapter: firstMatch(scope, rule.BookInfoLatestChapterRule),
		UpdateTime:    firstMatch(scope, rule.BookInfoUpdateTimeRule),
		WordCount:     formatSourceWordCount(firstMatch(scope, rule.BookInfoWordCountRule)),
		CanRename:     strings.TrimSpace(rule.BookInfoCanRenameRule) != "",
	}
}

func bookInfoScope(doc *goquery.Document, initRule string) *goquery.Selection {
	initRule = strings.TrimSpace(initRule)
	if initRule == "" || strings.HasPrefix(initRule, "@") {
		return doc.Selection
	}
	parts := strings.SplitN(initRule, "|", 2)
	selector := strings.TrimSpace(parts[0])
	if selector == "" {
		return doc.Selection
	}
	scope := doc.Find(selector).First()
	if scope.Length() == 0 {
		return doc.Selection
	}
	return scope
}

func parseTOCWithRule(bookURL, sourceBaseURL string, rule models.BookSourceRule, charset string, policy SourceRequestPolicy, bookDoc *goquery.Document, preparedBookRequest *sourceRequest) ([]RemoteChapter, error) {
	bookRequest := sourceRequest{}
	if preparedBookRequest != nil {
		bookRequest = *preparedBookRequest
	} else {
		var err error
		bookRequest, err = prepareResolvedSourceRequest(sourceBaseURL, bookURL, "", 1, charset, rule.Headers, policy)
		if err != nil {
			return nil, fmt.Errorf("prepare book page request: %w", err)
		}
	}
	fetchDocument := func(request sourceRequest) (*goquery.Document, sourceRequest, error) {
		return fetchSourceDocumentContext(context.Background(), request)
	}
	ensureBookDocument := func() (*goquery.Document, error) {
		if bookDoc != nil {
			return bookDoc, nil
		}
		var err error
		bookDoc, bookRequest, err = fetchDocument(bookRequest)
		return bookDoc, err
	}

	tocRequest := bookRequest
	var doc *goquery.Document
	tocURLRule := strings.TrimSpace(rule.TOCURLRule)
	switch {
	case tocURLRule == "":
		var err error
		doc, err = ensureBookDocument()
		if err != nil {
			return nil, fmt.Errorf("fetch toc page: %w", err)
		}
	case isDirectTOCURLRule(tocURLRule):
		var err error
		tocRequest, err = prepareResolvedSourceRequest(bookRequest.URL, tocURLRule, "", 1, charset, rule.Headers, policy)
		if err != nil {
			return nil, fmt.Errorf("prepare toc page request: %w", err)
		}
		if sourceRequestKey(tocRequest) == sourceRequestKey(bookRequest) {
			doc, err = ensureBookDocument()
			tocRequest = bookRequest
		} else {
			doc, tocRequest, err = fetchDocument(tocRequest)
		}
		if err != nil {
			return nil, fmt.Errorf("fetch toc page: %w", err)
		}
	default:
		var err error
		bookDoc, err = ensureBookDocument()
		if err != nil {
			return nil, fmt.Errorf("fetch toc page: %w", err)
		}
		parsedTOCURL := firstMatch(bookDoc.Selection, tocURLRule)
		if parsedTOCURL == "" {
			doc = bookDoc
		} else {
			tocRequest, err = prepareResolvedSourceRequest(bookRequest.URL, parsedTOCURL, "", 1, charset, rule.Headers, policy)
			if err != nil {
				return nil, fmt.Errorf("prepare toc page request: %w", err)
			}
			if sourceRequestKey(tocRequest) == sourceRequestKey(bookRequest) {
				doc = bookDoc
				tocRequest = bookRequest
			} else {
				doc, tocRequest, err = fetchDocument(tocRequest)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("fetch toc page: %w", err)
		}
	}

	type tocPage struct {
		request sourceRequest
		doc     *goquery.Document
	}
	queue := []tocPage{{request: tocRequest, doc: doc}}
	visited := map[string]bool{sourceRequestKey(tocRequest): true}
	pageCount := 1
	chapterListRule, reverse := sourceListRule(rule.ChapterListRule)
	chapters := make([]RemoteChapter, 0)
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		chapters = append(chapters, parseChapterList(page.doc, rule, chapterListRule, page.request.URL)...)
		for _, nextURL := range extractResolvedURLs(page.doc.Selection, rule.NextTOCURLRule, page.request.URL) {
			nextRequest, prepareErr := prepareSourceRequest(nextURL, "", 1, charset, rule.Headers, policy)
			if prepareErr != nil {
				return nil, fmt.Errorf("prepare toc page request: %w", prepareErr)
			}
			requestKey := sourceRequestKey(nextRequest)
			if visited[requestKey] {
				continue
			}
			if pageCount >= maxSourcePaginationPages {
				return nil, fmt.Errorf("toc pagination exceeds %d pages", maxSourcePaginationPages)
			}
			nextDoc, fetchedNextRequest, fetchErr := fetchDocument(nextRequest)
			if fetchErr != nil {
				return nil, fmt.Errorf("fetch toc page: %w", fetchErr)
			}
			fetchedRequestKey := sourceRequestKey(fetchedNextRequest)
			alreadyVisited := visited[fetchedRequestKey]
			visited[requestKey] = true
			if alreadyVisited {
				continue
			}
			visited[fetchedRequestKey] = true
			pageCount++
			queue = append(queue, tocPage{request: fetchedNextRequest, doc: nextDoc})
		}
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapters found on toc page")
	}
	return normalizeChapterOrder(chapters, reverse), nil
}

func isDirectTOCURLRule(rule string) bool {
	value, _ := splitSourceURLOption(rule)
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

func parseChapterList(doc *goquery.Document, rule models.BookSourceRule, listRule string, baseURL string) []RemoteChapter {
	items := findItems(doc, listRule)
	chapters := make([]RemoteChapter, 0, len(items))
	for i, sel := range items {
		title := firstMatch(sel, rule.ChapterNameRule)
		isVolume := sourceRuleBool(firstMatch(sel, rule.ChapterIsVolumeRule))
		chapterURL := resolveSourceURLTemplate(baseURL, firstMatch(sel, rule.ChapterURLRule))
		if chapterURL == "" {
			if isVolume {
				chapterURL = title + strconv.Itoa(i)
			} else {
				chapterURL = baseURL
			}
		}
		if title == "" || chapterURL == "" {
			continue
		}
		if sourceRuleBool(firstMatch(sel, rule.ChapterIsVIPRule)) {
			title = "🔒" + title
		}
		chapters = append(chapters, RemoteChapter{
			Title:    title,
			URL:      chapterURL,
			Index:    i,
			IsVolume: isVolume,
			Tag:      firstMatch(sel, rule.ChapterUpdateTimeRule),
		})
	}
	return chapters
}

func sourceRuleBool(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "null" {
		return false
	}
	return !sourceFalsePattern.MatchString(value)
}

func sourceListRule(rule string) (selector string, reverse bool) {
	selector = strings.TrimSpace(rule)
	if selector == "" {
		return "", false
	}
	switch selector[0] {
	case '-':
		return strings.TrimSpace(selector[1:]), true
	case '+':
		return strings.TrimSpace(selector[1:]), false
	default:
		return selector, false
	}
}

func reverseSearchResults(results []SearchResult) {
	for left, right := 0, len(results)-1; left < right; left, right = left+1, right-1 {
		results[left], results[right] = results[right], results[left]
	}
}

func normalizeChapterOrder(chapters []RemoteChapter, reverse bool) []RemoteChapter {
	ordered := make([]RemoteChapter, 0, len(chapters))
	seen := make(map[string]bool, len(chapters))
	if reverse {
		for _, chapter := range chapters {
			key := chapter.URL
			if key == "" {
				key = chapter.Title
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			ordered = append(ordered, chapter)
		}
		for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
			ordered[left], ordered[right] = ordered[right], ordered[left]
		}
	} else {
		for index := len(chapters) - 1; index >= 0; index-- {
			chapter := chapters[index]
			key := chapter.URL
			if key == "" {
				key = chapter.Title
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			ordered = append(ordered, chapter)
		}
		for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
			ordered[left], ordered[right] = ordered[right], ordered[left]
		}
	}
	for index := range ordered {
		ordered[index].Index = index
	}
	return ordered
}

func extractResolvedURLs(selection *goquery.Selection, rule string, baseURL string) []string {
	if strings.TrimSpace(rule) == "" {
		return nil
	}
	values := Extract(selection, rule)
	urls := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		resolved := resolveSourceURLTemplate(baseURL, value)
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
	if source.SourceType == 1 && strings.TrimSpace(rule.ContentRule) == "" {
		if resolved := resolveAudioContentURL(source.BaseURL, chapterURL); resolved != "" {
			return resolved, nil
		}
		return strings.TrimSpace(chapterURL), nil
	}

	policy := bookSourceRequestPolicy(source)
	chapterRequest, err := prepareResolvedSourceRequest(source.BaseURL, chapterURL, "", 1, source.Charset, rule.Headers, policy)
	if err != nil {
		return "", fmt.Errorf("prepare content page request: %w", err)
	}
	contentRequest := chapterRequest
	if rule.ContentURLRule != "" {
		contentRequest, err = prepareResolvedSourceRequest(chapterRequest.URL, rule.ContentURLRule, "", 1, source.Charset, rule.Headers, policy)
		if err != nil {
			return "", fmt.Errorf("prepare content page request: %w", err)
		}
	}

	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	if contentRequest.Charset == "" {
		contentRequest.Charset = charset
	}

	fetchDocument := func(request sourceRequest) (*goquery.Document, sourceRequest, error) {
		return fetchSourceDocumentContext(context.Background(), request)
	}
	doc, contentRequest, err := fetchDocument(contentRequest)
	if err != nil {
		return "", fmt.Errorf("fetch content page: %w", err)
	}

	type contentPage struct {
		request sourceRequest
		doc     *goquery.Document
	}
	queue := []contentPage{{request: contentRequest, doc: doc}}
	visited := map[string]bool{sourceRequestKey(contentRequest): true}
	pageCount := 1
	parts := make([]string, 0)
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		if text := extractChapterContent(page.doc, rule, page.request.URL, source.SourceType); text != "" {
			parts = append(parts, text)
		}
		for _, nextURL := range extractResolvedURLs(page.doc.Selection, rule.NextContentURLRule, page.request.URL) {
			nextRequest, prepareErr := prepareSourceRequest(nextURL, "", 1, charset, rule.Headers, policy)
			if prepareErr != nil {
				return "", fmt.Errorf("prepare content page request: %w", prepareErr)
			}
			requestKey := sourceRequestKey(nextRequest)
			if visited[requestKey] {
				continue
			}
			if pageCount >= maxSourcePaginationPages {
				return "", fmt.Errorf("content pagination exceeds %d pages", maxSourcePaginationPages)
			}
			nextDoc, fetchedNextRequest, fetchErr := fetchDocument(nextRequest)
			if fetchErr != nil {
				return "", fmt.Errorf("fetch content page: %w", fetchErr)
			}
			fetchedRequestKey := sourceRequestKey(fetchedNextRequest)
			alreadyVisited := visited[fetchedRequestKey]
			visited[requestKey] = true
			if alreadyVisited {
				continue
			}
			visited[fetchedRequestKey] = true
			pageCount++
			queue = append(queue, contentPage{request: fetchedNextRequest, doc: nextDoc})
		}
	}

	text := strings.Join(parts, "\n")
	text = applyContentReplaceRegex(text, rule.ContentReplaceRegex)
	text = ApplyTextReplacements(text, rule.TextReplaceRules)
	return text, nil
}

func applyContentReplaceRegex(text string, rule string) string {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return text
	}
	parts := strings.Split(rule, "##")
	if len(parts) < 2 {
		return text
	}
	result := text
	selector := strings.TrimSpace(parts[0])
	if selector != "" {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(text))
		if err != nil {
			return text
		}
		result = strings.Join(Extract(doc.Selection, selector), "\n")
	}
	pattern := parts[1]
	replacement := ""
	if len(parts) > 2 {
		replacement = parts[2]
	}
	if pattern == "" {
		return result
	}
	replaceFirst := len(parts) > 3
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		if replaceFirst {
			return strings.Replace(result, pattern, replacement, 1)
		}
		return strings.ReplaceAll(result, pattern, replacement)
	}
	if replaceFirst {
		loc := compiled.FindStringIndex(result)
		if loc == nil {
			return ""
		}
		return result[:loc[0]] + compiled.ReplaceAllString(result[loc[0]:loc[1]], replacement) + result[loc[1]:]
	}
	return compiled.ReplaceAllString(result, replacement)
}

func extractChapterContent(doc *goquery.Document, rule models.BookSourceRule, baseURL string, sourceType int) string {
	contentRule := rule.ContentRule
	if contentRule == "" {
		return strings.Join(Extract(doc.Selection, "body|text"), "\n")
	}
	values := Extract(doc.Selection, contentRule)
	if sourceType == 1 {
		for index := range values {
			values[index] = resolveAudioContentURL(baseURL, values[index])
		}
		return strings.Join(nonBlankStrings(values), "\n")
	}
	if ruleOperation(contentRule) == "html" {
		for index := range values {
			values[index] = normalizeChapterHTMLWithImageStyle(values[index], baseURL, rule.ContentImageStyle)
		}
	}
	return strings.Join(values, "\n")
}

func resolveAudioContentURL(baseURL string, value string) string {
	urlPart, _ := splitSourceURLOption(value)
	urlPart = strings.TrimSpace(urlPart)
	if urlPart == "" {
		return ""
	}
	return resolveURL(baseURL, urlPart)
}

func nonBlankStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}

func ruleOperation(rule string) string {
	parts := strings.Split(rule, "|")
	if len(parts) < 2 {
		return "text"
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func normalizeChapterHTML(fragment string, baseURL string) string {
	return normalizeChapterHTMLWithImageStyle(fragment, baseURL, "")
}

func normalizeChapterHTMLWithImageStyle(fragment string, baseURL string, imageStyle string) string {
	doc, err := html.Parse(strings.NewReader("<html><body>" + fragment + "</body></html>"))
	if err != nil {
		return strings.TrimSpace(fragment)
	}
	normalizedImageStyle := normalizeContentImageStyle(imageStyle)
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
					imageTag := `<img src="` + stdhtml.EscapeString(src) + `" alt="` + stdhtml.EscapeString(alt) + `"`
					if normalizedImageStyle != "" {
						imageTag += ` data-image-style="` + stdhtml.EscapeString(normalizedImageStyle) + `"`
					}
					imageTag += `>`
					lines = append(lines, imageTag)
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

func normalizeContentImageStyle(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "FULL") {
		return "FULL"
	}
	return ""
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
		return nil
	}
	parts := strings.SplitN(rule, "|", 2)
	selector := strings.TrimSpace(parts[0])
	if selector == "" {
		return nil
	}
	items := make([]*goquery.Selection, 0)
	doc.Find(selector).Each(func(_ int, sel *goquery.Selection) {
		items = append(items, sel)
	})
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
