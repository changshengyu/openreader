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
	Variable      string `json:"variable,omitempty"`
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
	Variable string `json:"variable,omitempty"`
}

// SourceRuleVariableState is the bounded persistent parser state associated
// with one shelf book and one chapter. It is deliberately not an API response
// schema; API handlers persist it only after a successful parser operation.
type SourceRuleVariableState struct {
	BookVariable    string
	ChapterVariable string
	BookName        string
	ChapterTitle    string
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

// ensureSourceScriptEntryPointsSupported fails closed for the two source-level
// JavaScript entry points that reader-dev invokes around every remote request.
// Running them in the Go server would expose cookies, cache and network access
// outside the bounded source-request model. They must therefore fail before a
// request is prepared instead of silently dropping authentication/header logic.
func ensureSourceScriptEntryPointsSupported(source models.BookSource) error {
	header := strings.ToLower(strings.TrimSpace(source.Header))
	if strings.HasPrefix(header, "@js:") || strings.HasPrefix(header, "<js>") {
		return fmt.Errorf("%w: dynamic source header JavaScript is disabled", ErrUnsupportedSourceRule)
	}
	if strings.TrimSpace(source.LoginCheckJS) != "" {
		return fmt.Errorf("%w: login-check JavaScript is disabled", ErrUnsupportedSourceRule)
	}
	return nil
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
	if err := ensureSourceScriptEntryPointsSupported(source); err != nil {
		return SearchPageResult{}, err
	}
	searchURLTemplate := resolveSourceURLTemplate(source.BaseURL, rule.SearchURL)

	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	runtime := newSourceRuleRuntime()

	if strings.Contains(searchURLTemplate, "{page}") {
		request, err := prepareSourceRequest(searchURLTemplate, keyword, page, charset, rule.Headers, bookSourceRequestPolicy(source))
		if err != nil {
			return SearchPageResult{}, err
		}
		document, request, err := fetchSourceRuleDocumentContext(ctx, request)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("fetch search page: %w", err)
		}
		items, err := parseBookResultsFromSourceDocumentWithRuntime(document, rule, source, request, runtime)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("parse search page: %w", err)
		}
		nextURL, err := searchNextURLFromSourceDocumentWithRuntime(document, rule, request.URL, runtime)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("parse search pagination: %w", err)
		}
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
		document, fetchedRequest, err := fetchSourceRuleDocumentContext(ctx, request)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("fetch search page: %w", err)
		}
		request = fetchedRequest
		items, err := parseBookResultsFromSourceDocumentWithRuntime(document, rule, source, request, runtime)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("parse search page: %w", err)
		}
		nextURL, err := searchNextURLFromSourceDocumentWithRuntime(document, rule, request.URL, runtime)
		if err != nil {
			return SearchPageResult{}, fmt.Errorf("parse search pagination: %w", err)
		}
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

func searchNextURLFromSourceDocument(document *sourceRuleDocument, rule models.BookSourceRule, searchURL string) (string, error) {
	return searchNextURLFromSourceDocumentWithRuntime(document, rule, searchURL, newSourceRuleRuntime())
}

func searchNextURLFromSourceDocumentWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, searchURL string, runtime *sourceRuleRuntime) (string, error) {
	if strings.TrimSpace(rule.PaginationRule) == "" {
		return "", nil
	}
	if !sourceRuleNeedsEvaluator(rule.PaginationRule) {
		return searchNextURL(document.document, rule, searchURL), nil
	}
	value, err := sourceRuleString(document.RootWithRuntime(runtime), rule.PaginationRule)
	if err != nil {
		return "", err
	}
	return resolveSourceURLTemplate(searchURL, value), nil
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
	if err := ensureSourceScriptEntryPointsSupported(source); err != nil {
		return ExploreResult{}, err
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
	document, request, err := fetchSourceRuleDocumentContext(context.Background(), request)
	if err != nil {
		return ExploreResult{}, fmt.Errorf("fetch explore page: %w", err)
	}
	exploreRule := effectiveExploreRule(rule)
	runtime := newSourceRuleRuntime()
	items, err := parseBookResultsFromSourceDocumentWithRuntime(document, exploreRule, source, request, runtime)
	if err != nil {
		return ExploreResult{}, fmt.Errorf("parse explore page: %w", err)
	}
	nextURL, err := searchNextURLFromSourceDocumentWithRuntime(document, exploreRule, request.URL, runtime)
	if err != nil {
		return ExploreResult{}, fmt.Errorf("parse explore pagination: %w", err)
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

func parseBookResultsFromSourceDocument(document *sourceRuleDocument, rule models.BookSourceRule, source models.BookSource, request sourceRequest) ([]SearchResult, error) {
	return parseBookResultsFromSourceDocumentWithRuntime(document, rule, source, request, newSourceRuleRuntime())
}

func parseBookResultsFromSourceDocumentWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, source models.BookSource, request sourceRequest, runtime *sourceRuleRuntime) ([]SearchResult, error) {
	if !bookResultRuleNeedsEvaluator(rule) {
		return parseBookResults(document.document, rule, source, request)
	}
	return parseBookResultsWithEvaluatorWithRuntime(document, rule, source, request, runtime)
}

func bookResultRuleNeedsEvaluator(rule models.BookSourceRule) bool {
	for _, value := range []string{
		rule.BookListRule,
		rule.BookNameRule,
		rule.BookAuthorRule,
		rule.BookCoverRule,
		rule.BookIntroRule,
		rule.BookKindRule,
		rule.BookWordCountRule,
		rule.LatestChapterRule,
		rule.BookUpdateTimeRule,
		rule.BookURLRule,
		rule.BookInfoInitRule,
		rule.BookInfoNameRule,
		rule.BookInfoAuthorRule,
		rule.BookInfoCoverRule,
		rule.BookInfoIntroRule,
		rule.BookInfoKindRule,
		rule.BookInfoLatestChapterRule,
		rule.BookInfoUpdateTimeRule,
		rule.BookInfoWordCountRule,
	} {
		if sourceRuleNeedsEvaluator(value) {
			return true
		}
	}
	return false
}

func parseBookResultsWithEvaluator(document *sourceRuleDocument, rule models.BookSourceRule, source models.BookSource, request sourceRequest) ([]SearchResult, error) {
	return parseBookResultsWithEvaluatorWithRuntime(document, rule, source, request, newSourceRuleRuntime())
}

func parseBookResultsWithEvaluatorWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, source models.BookSource, request sourceRequest, runtime *sourceRuleRuntime) ([]SearchResult, error) {
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
			result, ok, err := parseDirectBookResultWithEvaluatorWithRuntime(document, rule, source, request, runtime)
			if err != nil {
				return nil, err
			}
			if ok {
				return []SearchResult{result}, nil
			}
			return []SearchResult{}, nil
		}
	}

	listRule, reverse := sourceListRule(rule.BookListRule)
	items, err := sourceRuleElements(document.RootWithRuntime(runtime), listRule)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 && pattern == "" {
		result, ok, err := parseDirectBookResultWithEvaluatorWithRuntime(document, rule, source, request, runtime)
		if err != nil {
			return nil, err
		}
		if ok {
			return []SearchResult{result}, nil
		}
	}

	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		item = item.withRuntime(runtime.asSearchBookRuntime())
		title, err := sourceRuleString(item, rule.BookNameRule)
		if err != nil {
			return nil, err
		}
		if title == "" {
			continue
		}
		item.runtime.setBookName(title)
		author, err := sourceRuleString(item, rule.BookAuthorRule)
		if err != nil {
			return nil, err
		}
		coverURL, err := sourceRuleString(item, rule.BookCoverRule)
		if err != nil {
			return nil, err
		}
		intro, err := sourceRuleString(item, rule.BookIntroRule)
		if err != nil {
			return nil, err
		}
		kinds, err := sourceRuleStrings(item, rule.BookKindRule)
		if err != nil {
			return nil, err
		}
		wordCount, err := sourceRuleString(item, rule.BookWordCountRule)
		if err != nil {
			return nil, err
		}
		latestChapter, err := sourceRuleString(item, rule.LatestChapterRule)
		if err != nil {
			return nil, err
		}
		updateTime, err := sourceRuleString(item, rule.BookUpdateTimeRule)
		if err != nil {
			return nil, err
		}
		bookURL, err := sourceRuleString(item, rule.BookURLRule)
		if err != nil {
			return nil, err
		}
		bookURL = resolveSourceURLTemplate(baseURL, bookURL)
		if bookURL == "" {
			bookURL = baseURL
		}
		results = append(results, SearchResult{
			Title:         title,
			Author:        author,
			CoverURL:      resolveURL(baseURL, coverURL),
			Intro:         intro,
			Kind:          strings.Join(kinds, ","),
			WordCount:     formatSourceWordCount(wordCount),
			LatestChapter: latestChapter,
			UpdateTime:    updateTime,
			BookURL:       bookURL,
			SourceID:      source.ID,
			SourceName:    source.Name,
			OriginOrder:   source.CustomOrder,
			Type:          source.SourceType,
			Variable:      item.runtime.persistentBookVariables(),
		})
	}
	if reverse {
		reverseSearchResults(results)
	}
	return results, nil
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
		if result.BookURL == "" {
			result.BookURL = baseURL
		}

		if result.Title == "" {
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

func parseDirectBookResultWithEvaluator(document *sourceRuleDocument, rule models.BookSourceRule, source models.BookSource, request sourceRequest) (SearchResult, bool, error) {
	return parseDirectBookResultWithEvaluatorWithRuntime(document, rule, source, request, newSourceRuleRuntime())
}

func parseDirectBookResultWithEvaluatorWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, source models.BookSource, request sourceRequest, runtime *sourceRuleRuntime) (SearchResult, bool, error) {
	runtime = runtime.asSearchBookRuntime()
	info, err := parseRemoteBookInfoWithEvaluatorWithRuntime(document, rule, request.URL, runtime)
	if err != nil {
		return SearchResult{}, false, err
	}
	if strings.TrimSpace(info.Title) == "" {
		return SearchResult{}, false, nil
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
		Variable:      runtime.persistentBookVariables(),
	}, true, nil
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
	if err := ensureSourceScriptEntryPointsSupported(source); err != nil {
		return nil, err
	}

	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	return parseTOCWithRule(bookURL, source.BaseURL, rule, charset, bookSourceRequestPolicy(source), nil, nil, newSourceRuleRuntime())
}

func FetchBookInfoAndTOC(bookURL string, source models.BookSource) (RemoteBookInfo, []RemoteChapter, error) {
	info, chapters, _, err := FetchBookInfoAndTOCWithVariables(bookURL, source, "", "")
	return info, chapters, err
}

// FetchBookInfoAndTOCWithVariables carries the reader-dev Book.variable map
// across detail and catalogue parsing. The returned map is already bounded and
// normalized for one durable book row; each returned chapter owns its own map.
func FetchBookInfoAndTOCWithVariables(bookURL string, source models.BookSource, bookVariable, bookName string) (RemoteBookInfo, []RemoteChapter, string, error) {
	rule, err := source.ParsedRules()
	if err != nil {
		return RemoteBookInfo{}, nil, "", fmt.Errorf("parse rules: %w", err)
	}
	if err := ensureSourceScriptEntryPointsSupported(source); err != nil {
		return RemoteBookInfo{}, nil, "", err
	}
	// Validate persisted input before opening a remote request. A corrupt SQLite
	// row or a forged add-to-shelf payload must fail as a local rule error, not
	// trigger a network fetch or poison the remote-source failure cache.
	runtime, err := newSourceRuleRuntimeWithBookVariables(bookVariable, bookName)
	if err != nil {
		return RemoteBookInfo{}, nil, "", err
	}
	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	policy := bookSourceRequestPolicy(source)
	bookRequest, err := prepareResolvedSourceRequest(source.BaseURL, bookURL, "", 1, charset, rule.Headers, policy)
	if err != nil {
		return RemoteBookInfo{}, nil, "", fmt.Errorf("prepare book info request: %w", err)
	}
	bookDocument, bookRequest, err := fetchSourceRuleDocumentContext(context.Background(), bookRequest)
	if err != nil {
		return RemoteBookInfo{}, nil, "", fmt.Errorf("fetch book info page: %w", err)
	}
	info, err := parseRemoteBookInfoFromSourceDocumentWithRuntime(bookDocument, rule, bookRequest.URL, runtime)
	if err != nil {
		return RemoteBookInfo{}, nil, "", fmt.Errorf("parse book info page: %w", err)
	}
	chapters, err := parseTOCWithRule(bookURL, source.BaseURL, rule, charset, policy, bookDocument, &bookRequest, runtime)
	if err != nil {
		return RemoteBookInfo{}, nil, "", err
	}
	return info, chapters, runtime.persistentBookVariables(), nil
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

func parseRemoteBookInfoFromSourceDocument(document *sourceRuleDocument, rule models.BookSourceRule, baseURL string) (RemoteBookInfo, error) {
	return parseRemoteBookInfoFromSourceDocumentWithRuntime(document, rule, baseURL, newSourceRuleRuntime())
}

func parseRemoteBookInfoFromSourceDocumentWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, baseURL string, runtime *sourceRuleRuntime) (RemoteBookInfo, error) {
	if !bookInfoRuleNeedsEvaluator(rule) {
		return parseRemoteBookInfo(document.document, rule, baseURL), nil
	}
	return parseRemoteBookInfoWithEvaluatorWithRuntime(document, rule, baseURL, runtime)
}

func bookInfoRuleNeedsEvaluator(rule models.BookSourceRule) bool {
	for _, value := range []string{
		rule.BookInfoInitRule,
		rule.BookInfoNameRule,
		rule.BookInfoAuthorRule,
		rule.BookInfoCoverRule,
		rule.BookInfoIntroRule,
		rule.BookInfoKindRule,
		rule.BookInfoLatestChapterRule,
		rule.BookInfoUpdateTimeRule,
		rule.BookInfoWordCountRule,
		rule.BookInfoCanRenameRule,
	} {
		if sourceRuleNeedsEvaluator(value) {
			return true
		}
	}
	return false
}

func parseRemoteBookInfoWithEvaluator(document *sourceRuleDocument, rule models.BookSourceRule, baseURL string) (RemoteBookInfo, error) {
	return parseRemoteBookInfoWithEvaluatorWithRuntime(document, rule, baseURL, newSourceRuleRuntime())
}

func parseRemoteBookInfoWithEvaluatorWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, baseURL string, runtime *sourceRuleRuntime) (RemoteBookInfo, error) {
	scope := document.RootWithRuntime(runtime)
	if strings.TrimSpace(rule.BookInfoInitRule) != "" {
		items, err := sourceRuleElements(scope, rule.BookInfoInitRule)
		if err != nil {
			return RemoteBookInfo{}, err
		}
		if len(items) > 0 {
			scope = items[0]
		}
	}
	name, err := sourceRuleString(scope, rule.BookInfoNameRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	runtime.setBookName(name)
	author, err := sourceRuleString(scope, rule.BookInfoAuthorRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	coverURL, err := sourceRuleString(scope, rule.BookInfoCoverRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	intro, err := sourceRuleString(scope, rule.BookInfoIntroRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	kinds, err := sourceRuleStrings(scope, rule.BookInfoKindRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	latestChapter, err := sourceRuleString(scope, rule.BookInfoLatestChapterRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	updateTime, err := sourceRuleString(scope, rule.BookInfoUpdateTimeRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	wordCount, err := sourceRuleString(scope, rule.BookInfoWordCountRule)
	if err != nil {
		return RemoteBookInfo{}, err
	}
	canRename := false
	if strings.TrimSpace(rule.BookInfoCanRenameRule) != "" {
		canRenameValue, err := sourceRuleString(scope, rule.BookInfoCanRenameRule)
		if err != nil {
			return RemoteBookInfo{}, err
		}
		canRename = sourceRuleBool(canRenameValue)
	}
	return RemoteBookInfo{
		Title:         name,
		Author:        author,
		CoverURL:      resolveURL(baseURL, coverURL),
		Intro:         intro,
		Kind:          strings.Join(kinds, ","),
		LatestChapter: latestChapter,
		UpdateTime:    updateTime,
		WordCount:     formatSourceWordCount(wordCount),
		CanRename:     canRename,
	}, nil
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

func parseTOCWithRule(bookURL, sourceBaseURL string, rule models.BookSourceRule, charset string, policy SourceRequestPolicy, bookDocument *sourceRuleDocument, preparedBookRequest *sourceRequest, runtime *sourceRuleRuntime) ([]RemoteChapter, error) {
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
	fetchDocument := func(request sourceRequest) (*sourceRuleDocument, sourceRequest, error) {
		return fetchSourceRuleDocumentContext(context.Background(), request)
	}
	ensureBookDocument := func() (*sourceRuleDocument, error) {
		if bookDocument != nil {
			return bookDocument, nil
		}
		var err error
		bookDocument, bookRequest, err = fetchDocument(bookRequest)
		return bookDocument, err
	}

	tocRequest := bookRequest
	var document *sourceRuleDocument
	tocURLRule := strings.TrimSpace(rule.TOCURLRule)
	switch {
	case tocURLRule == "":
		var err error
		document, err = ensureBookDocument()
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
			document, err = ensureBookDocument()
			tocRequest = bookRequest
		} else {
			document, tocRequest, err = fetchDocument(tocRequest)
		}
		if err != nil {
			return nil, fmt.Errorf("fetch toc page: %w", err)
		}
	default:
		var err error
		bookDocument, err = ensureBookDocument()
		if err != nil {
			return nil, fmt.Errorf("fetch toc page: %w", err)
		}
		parsedTOCURL, parseErr := sourceRuleStringFromDocumentWithRuntime(bookDocument, tocURLRule, runtime)
		if parseErr != nil {
			return nil, fmt.Errorf("parse toc URL rule: %w", parseErr)
		}
		if parsedTOCURL == "" {
			document = bookDocument
		} else {
			tocRequest, err = prepareResolvedSourceRequest(bookRequest.URL, parsedTOCURL, "", 1, charset, rule.Headers, policy)
			if err != nil {
				return nil, fmt.Errorf("prepare toc page request: %w", err)
			}
			if sourceRequestKey(tocRequest) == sourceRequestKey(bookRequest) {
				document = bookDocument
				tocRequest = bookRequest
			} else {
				document, tocRequest, err = fetchDocument(tocRequest)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("fetch toc page: %w", err)
		}
	}

	visited := map[string]bool{sourceRequestKey(tocRequest): true}
	pageCount := 1
	chapterListRule, reverse := sourceListRule(rule.ChapterListRule)
	chapters := make([]RemoteChapter, 0)
	parsePage := func(pageDocument *sourceRuleDocument, pageRequest sourceRequest, includeNext bool) ([]string, error) {
		pageChapters, parseErr := parseChapterListFromSourceDocumentWithRuntime(pageDocument, rule, chapterListRule, pageRequest.URL, runtime)
		if parseErr != nil {
			return nil, fmt.Errorf("parse toc chapters: %w", parseErr)
		}
		chapters = append(chapters, pageChapters...)
		if !includeNext {
			return nil, nil
		}
		nextURLs, parseErr := extractResolvedURLsFromSourceDocumentWithRuntime(pageDocument, rule.NextTOCURLRule, pageRequest.URL, runtime)
		if parseErr != nil {
			return nil, fmt.Errorf("parse toc pagination: %w", parseErr)
		}
		return nextURLs, nil
	}
	fetchNextPage := func(nextURL string) (*sourceRuleDocument, sourceRequest, bool, error) {
		nextRequest, prepareErr := prepareSourceRequest(nextURL, "", 1, charset, rule.Headers, policy)
		if prepareErr != nil {
			return nil, sourceRequest{}, false, fmt.Errorf("prepare toc page request: %w", prepareErr)
		}
		requestKey := sourceRequestKey(nextRequest)
		if visited[requestKey] {
			return nil, sourceRequest{}, false, nil
		}
		if pageCount >= maxSourcePaginationPages {
			return nil, sourceRequest{}, false, fmt.Errorf("toc pagination exceeds %d pages", maxSourcePaginationPages)
		}
		nextDocument, fetchedNextRequest, fetchErr := fetchDocument(nextRequest)
		if fetchErr != nil {
			return nil, sourceRequest{}, false, fmt.Errorf("fetch toc page: %w", fetchErr)
		}
		fetchedRequestKey := sourceRequestKey(fetchedNextRequest)
		alreadyVisited := visited[fetchedRequestKey]
		visited[requestKey] = true
		if alreadyVisited {
			return nil, sourceRequest{}, false, nil
		}
		visited[fetchedRequestKey] = true
		pageCount++
		return nextDocument, fetchedNextRequest, true, nil
	}

	nextURLs, parseErr := parsePage(document, tocRequest, true)
	if parseErr != nil {
		return nil, parseErr
	}
	if len(nextURLs) == 1 {
		nextURL := nextURLs[0]
		for nextURL != "" {
			nextDocument, nextRequest, fetched, fetchErr := fetchNextPage(nextURL)
			if fetchErr != nil {
				return nil, fetchErr
			}
			if !fetched {
				break
			}
			pageNextURLs, pageErr := parsePage(nextDocument, nextRequest, true)
			if pageErr != nil {
				return nil, pageErr
			}
			nextURL = ""
			if len(pageNextURLs) > 0 {
				nextURL = pageNextURLs[0]
			}
		}
	} else {
		for _, nextURL := range nextURLs {
			nextDocument, nextRequest, fetched, fetchErr := fetchNextPage(nextURL)
			if fetchErr != nil {
				return nil, fetchErr
			}
			if !fetched {
				continue
			}
			if _, pageErr := parsePage(nextDocument, nextRequest, false); pageErr != nil {
				return nil, pageErr
			}
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
	if strings.HasPrefix(value, "//") {
		host := strings.SplitN(strings.TrimPrefix(value, "//"), "/", 2)[0]
		return strings.Contains(host, ".") || strings.Contains(host, ":") || strings.EqualFold(host, "localhost")
	}
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
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

func parseChapterListFromSourceDocument(document *sourceRuleDocument, rule models.BookSourceRule, listRule string, baseURL string) ([]RemoteChapter, error) {
	return parseChapterListFromSourceDocumentWithRuntime(document, rule, listRule, baseURL, newSourceRuleRuntime())
}

func parseChapterListFromSourceDocumentWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, listRule string, baseURL string, runtime *sourceRuleRuntime) ([]RemoteChapter, error) {
	if !chapterRuleNeedsEvaluator(rule, listRule) {
		return parseChapterList(document.document, rule, listRule, baseURL), nil
	}
	items, err := sourceRuleElements(document.RootWithRuntime(runtime), listRule)
	if err != nil {
		return nil, err
	}
	chapters := make([]RemoteChapter, 0, len(items))
	for index, item := range items {
		chapterRuntime, err := item.runtime.withChapterVariables("", "")
		if err != nil {
			return nil, err
		}
		item = item.withRuntime(chapterRuntime)
		title, err := sourceRuleString(item, rule.ChapterNameRule)
		if err != nil {
			return nil, err
		}
		chapterRuntime.setChapterTitle(title)
		isVolumeValue, err := sourceRuleString(item, rule.ChapterIsVolumeRule)
		if err != nil {
			return nil, err
		}
		isVolume := sourceRuleBool(isVolumeValue)
		chapterURL, err := sourceRuleString(item, rule.ChapterURLRule)
		if err != nil {
			return nil, err
		}
		chapterURL = resolveSourceURLTemplate(baseURL, chapterURL)
		if chapterURL == "" {
			if isVolume {
				chapterURL = title + strconv.Itoa(index)
			} else {
				chapterURL = baseURL
			}
		}
		if title == "" || chapterURL == "" {
			continue
		}
		isVIPValue, err := sourceRuleString(item, rule.ChapterIsVIPRule)
		if err != nil {
			return nil, err
		}
		if sourceRuleBool(isVIPValue) {
			title = "🔒" + title
		}
		updateTime, err := sourceRuleString(item, rule.ChapterUpdateTimeRule)
		if err != nil {
			return nil, err
		}
		chapters = append(chapters, RemoteChapter{
			Title:    title,
			URL:      chapterURL,
			Index:    index,
			IsVolume: isVolume,
			Tag:      updateTime,
			Variable: chapterRuntime.persistentChapterVariables(),
		})
	}
	return chapters, nil
}

func chapterRuleNeedsEvaluator(rule models.BookSourceRule, listRule string) bool {
	for _, value := range []string{
		listRule,
		rule.ChapterNameRule,
		rule.ChapterURLRule,
		rule.ChapterIsVolumeRule,
		rule.ChapterIsVIPRule,
		rule.ChapterUpdateTimeRule,
	} {
		if sourceRuleNeedsEvaluator(value) {
			return true
		}
	}
	return false
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

func sourceRuleStringFromDocument(document *sourceRuleDocument, rule string) (string, error) {
	return sourceRuleStringFromDocumentWithRuntime(document, rule, newSourceRuleRuntime())
}

func sourceRuleStringFromDocumentWithRuntime(document *sourceRuleDocument, rule string, runtime *sourceRuleRuntime) (string, error) {
	if strings.TrimSpace(rule) == "" {
		return "", nil
	}
	if !sourceRuleNeedsEvaluator(rule) {
		return firstMatch(document.document.Selection, rule), nil
	}
	return sourceRuleString(document.RootWithRuntime(runtime), rule)
}

func sourceRuleStringsFromDocument(document *sourceRuleDocument, rule string) ([]string, error) {
	return sourceRuleStringsFromDocumentWithRuntime(document, rule, newSourceRuleRuntime())
}

func sourceRuleStringsFromDocumentWithRuntime(document *sourceRuleDocument, rule string, runtime *sourceRuleRuntime) ([]string, error) {
	if strings.TrimSpace(rule) == "" {
		return nil, nil
	}
	if !sourceRuleNeedsEvaluator(rule) {
		return Extract(document.document.Selection, rule), nil
	}
	return sourceRuleStrings(document.RootWithRuntime(runtime), rule)
}

func extractResolvedURLsFromSourceDocument(document *sourceRuleDocument, rule string, baseURL string) ([]string, error) {
	return extractResolvedURLsFromSourceDocumentWithRuntime(document, rule, baseURL, newSourceRuleRuntime())
}

func extractResolvedURLsFromSourceDocumentWithRuntime(document *sourceRuleDocument, rule string, baseURL string, runtime *sourceRuleRuntime) ([]string, error) {
	values, err := sourceRuleStringsFromDocumentWithRuntime(document, rule, runtime)
	if err != nil {
		return nil, err
	}
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
	return urls, nil
}

// FetchChapterContent fetches and parses a single chapter's content.
func FetchChapterContent(chapterURL string, source models.BookSource) (string, error) {
	return FetchChapterContentContext(context.Background(), chapterURL, source)
}

// FetchChapterContentContext is the cancellable counterpart used by bounded
// cache jobs. A client disconnect can therefore stop source pagination before
// another remote chapter request is scheduled.
func FetchChapterContentContext(ctx context.Context, chapterURL string, source models.BookSource) (string, error) {
	return FetchChapterContentContextWithNextChapter(ctx, chapterURL, "", source)
}

// FetchChapterContentContextWithNextChapter adds the catalog context used by
// reader-dev to prevent a single next-content link from crossing into the
// following chapter. Empty nextChapterURL preserves the legacy public API for
// callers that do not have a catalog row available.
func FetchChapterContentContextWithNextChapter(ctx context.Context, chapterURL, nextChapterURL string, source models.BookSource) (string, error) {
	return fetchChapterContentContextWithNextChapterRuntime(ctx, chapterURL, nextChapterURL, source, newSourceRuleRuntime())
}

// FetchChapterContentContextWithState applies the persisted Book.variable and
// BookChapter.variable maps to one content operation. Callers must persist the
// returned state atomically with any derived chapter cache metadata.
func FetchChapterContentContextWithState(ctx context.Context, chapterURL, nextChapterURL string, source models.BookSource, state SourceRuleVariableState) (string, SourceRuleVariableState, error) {
	runtime, err := newSourceRuleRuntimeWithBookVariables(state.BookVariable, state.BookName)
	if err != nil {
		return "", state, err
	}
	runtime, err = runtime.withChapterVariables(state.ChapterVariable, state.ChapterTitle)
	if err != nil {
		return "", state, err
	}
	content, err := fetchChapterContentContextWithNextChapterRuntime(ctx, chapterURL, nextChapterURL, source, runtime)
	if err != nil {
		return "", state, err
	}
	state.BookVariable = runtime.persistentBookVariables()
	state.ChapterVariable = runtime.persistentChapterVariables()
	return content, state, nil
}

func fetchChapterContentContextWithNextChapterRuntime(ctx context.Context, chapterURL, nextChapterURL string, source models.BookSource, runtime *sourceRuleRuntime) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
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
	if strings.TrimSpace(rule.ContentRule) == "" {
		return "", fmt.Errorf("%w: content rule is empty for a text source", ErrInvalidSourceRule)
	}
	if err := ensureSourceScriptEntryPointsSupported(source); err != nil {
		return "", err
	}

	policy := bookSourceRequestPolicy(source)
	chapterRequest, err := prepareResolvedSourceRequest(source.BaseURL, chapterURL, "", 1, source.Charset, rule.Headers, policy)
	if err != nil {
		return "", fmt.Errorf("prepare content page request: %w", err)
	}
	charset := source.Charset
	if charset == "" {
		charset = "utf-8"
	}
	fetchDocument := func(request sourceRequest) (*sourceRuleDocument, sourceRequest, error) {
		if request.Charset == "" {
			request.Charset = charset
		}
		return fetchSourceRuleDocumentContext(ctx, request)
	}

	contentRequest := chapterRequest
	var document *sourceRuleDocument
	contentURLRule := strings.TrimSpace(rule.ContentURLRule)
	switch {
	case contentURLRule == "":
		document, contentRequest, err = fetchDocument(chapterRequest)
	case isDirectTOCURLRule(contentURLRule):
		contentRequest, err = prepareResolvedSourceRequest(chapterRequest.URL, contentURLRule, "", 1, charset, rule.Headers, policy)
		if err == nil {
			document, contentRequest, err = fetchDocument(contentRequest)
		}
	default:
		chapterDocument, fetchedChapterRequest, fetchErr := fetchDocument(chapterRequest)
		if fetchErr != nil {
			return "", fmt.Errorf("fetch content page: %w", fetchErr)
		}
		contentURL, parseErr := sourceRuleStringFromDocumentWithRuntime(chapterDocument, contentURLRule, runtime)
		if parseErr != nil {
			return "", fmt.Errorf("parse content URL rule: %w", parseErr)
		}
		if contentURL == "" {
			document = chapterDocument
			contentRequest = fetchedChapterRequest
			break
		}
		contentRequest, err = prepareResolvedSourceRequest(fetchedChapterRequest.URL, contentURL, "", 1, charset, rule.Headers, policy)
		if err == nil && sourceRequestKey(contentRequest) == sourceRequestKey(fetchedChapterRequest) {
			document = chapterDocument
			contentRequest = fetchedChapterRequest
		} else if err == nil {
			document, contentRequest, err = fetchDocument(contentRequest)
		}
	}
	if err != nil {
		return "", fmt.Errorf("fetch content page: %w", err)
	}

	visited := map[string]bool{sourceRequestKey(contentRequest): true}
	pageCount := 1
	parts := make([]string, 0)
	parsePage := func(pageDocument *sourceRuleDocument, pageRequest sourceRequest, includeNext bool, pageRuntime *sourceRuleRuntime) ([]string, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, parseErr := extractChapterContentFromSourceDocumentWithRuntime(pageDocument, rule, pageRequest.URL, source.SourceType, pageRuntime)
		if parseErr != nil {
			return nil, fmt.Errorf("parse content page: %w", parseErr)
		}
		if text != "" {
			parts = append(parts, text)
		}
		if !includeNext {
			return nil, nil
		}
		nextURLs, parseErr := extractResolvedURLsFromSourceDocumentWithRuntime(pageDocument, rule.NextContentURLRule, pageRequest.URL, pageRuntime)
		if parseErr != nil {
			return nil, fmt.Errorf("parse content pagination: %w", parseErr)
		}
		return nextURLs, nil
	}
	fetchNextPage := func(nextURL string) (*sourceRuleDocument, sourceRequest, bool, error) {
		if err := ctx.Err(); err != nil {
			return nil, sourceRequest{}, false, err
		}
		nextRequest, prepareErr := prepareSourceRequest(nextURL, "", 1, charset, rule.Headers, policy)
		if prepareErr != nil {
			return nil, sourceRequest{}, false, fmt.Errorf("prepare content page request: %w", prepareErr)
		}
		requestKey := sourceRequestKey(nextRequest)
		if visited[requestKey] {
			return nil, sourceRequest{}, false, nil
		}
		if pageCount >= maxSourcePaginationPages {
			return nil, sourceRequest{}, false, fmt.Errorf("content pagination exceeds %d pages", maxSourcePaginationPages)
		}
		nextDocument, fetchedNextRequest, fetchErr := fetchDocument(nextRequest)
		if fetchErr != nil {
			return nil, sourceRequest{}, false, fmt.Errorf("fetch content page: %w", fetchErr)
		}
		fetchedRequestKey := sourceRequestKey(fetchedNextRequest)
		alreadyVisited := visited[fetchedRequestKey]
		visited[requestKey] = true
		if alreadyVisited {
			return nil, sourceRequest{}, false, nil
		}
		visited[fetchedRequestKey] = true
		pageCount++
		return nextDocument, fetchedNextRequest, true, nil
	}

	nextURLs, parseErr := parsePage(document, contentRequest, true, runtime)
	if parseErr != nil {
		return "", parseErr
	}
	if len(nextURLs) == 1 {
		nextURL := nextURLs[0]
		for nextURL != "" {
			if contentNextURLIsCatalogChapter(contentRequest.URL, nextURL, nextChapterURL, charset, rule.Headers, policy) {
				break
			}
			nextDocument, nextRequest, fetched, fetchErr := fetchNextPage(nextURL)
			if fetchErr != nil {
				return "", fetchErr
			}
			if !fetched {
				break
			}
			pageNextURLs, pageErr := parsePage(nextDocument, nextRequest, true, runtime)
			if pageErr != nil {
				return "", pageErr
			}
			nextURL = ""
			if len(pageNextURLs) > 0 {
				nextURL = pageNextURLs[0]
			}
		}
	} else {
		for _, nextURL := range nextURLs {
			nextDocument, nextRequest, fetched, fetchErr := fetchNextPage(nextURL)
			if fetchErr != nil {
				return "", fetchErr
			}
			if !fetched {
				continue
			}
			if _, pageErr := parsePage(nextDocument, nextRequest, false, runtime.clone()); pageErr != nil {
				return "", pageErr
			}
		}
	}

	text := strings.Join(parts, "\n")
	text = applyContentReplaceRegex(text, rule.ContentReplaceRegex)
	text = ApplyTextReplacements(text, rule.TextReplaceRules)
	return text, nil
}

func contentNextURLIsCatalogChapter(baseURL, nextURL, nextChapterURL, charset string, headers map[string]string, policy SourceRequestPolicy) bool {
	if strings.TrimSpace(nextURL) == "" || strings.TrimSpace(nextChapterURL) == "" {
		return false
	}
	nextRequest, nextErr := prepareResolvedSourceRequest(baseURL, nextURL, "", 1, charset, headers, policy)
	nextChapterRequest, chapterErr := prepareResolvedSourceRequest(baseURL, nextChapterURL, "", 1, charset, headers, policy)
	if nextErr != nil || chapterErr != nil {
		return false
	}
	return sourceRequestKey(nextRequest) == sourceRequestKey(nextChapterRequest)
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
		return ""
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

func extractChapterContentFromSourceDocument(document *sourceRuleDocument, rule models.BookSourceRule, baseURL string, sourceType int) (string, error) {
	return extractChapterContentFromSourceDocumentWithRuntime(document, rule, baseURL, sourceType, newSourceRuleRuntime())
}

func extractChapterContentFromSourceDocumentWithRuntime(document *sourceRuleDocument, rule models.BookSourceRule, baseURL string, sourceType int, runtime *sourceRuleRuntime) (string, error) {
	if !contentRuleNeedsEvaluator(rule) {
		return extractChapterContent(document.document, rule, baseURL, sourceType), nil
	}
	contentRule := strings.TrimSpace(rule.ContentRule)
	if contentRule == "" {
		return "", nil
	}
	values, err := sourceRuleStringsFromDocumentWithRuntime(document, contentRule, runtime)
	if err != nil {
		return "", err
	}
	if sourceType == 1 {
		for index := range values {
			values[index] = resolveAudioContentURL(baseURL, values[index])
		}
		return strings.Join(nonBlankStrings(values), "\n"), nil
	}
	if sourceRuleCSSOperation(contentRule) == "html" {
		for index := range values {
			values[index] = normalizeChapterHTMLWithImageStyle(values[index], baseURL, rule.ContentImageStyle)
		}
	}
	return strings.Join(values, "\n"), nil
}

func contentRuleNeedsEvaluator(rule models.BookSourceRule) bool {
	for _, value := range []string{
		rule.ContentURLRule,
		rule.ContentRule,
		rule.NextContentURLRule,
	} {
		if sourceRuleNeedsEvaluator(value) {
			return true
		}
	}
	return false
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
