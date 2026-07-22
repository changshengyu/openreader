// Package sourcecompat owns the reader-dev compatible book-source export shape.
// API downloads and logical backups must use the same encoder so the two paths
// cannot silently drift apart.
package sourcecompat

import (
	"encoding/json"
	"strings"

	"openreader/backend/models"
)

type SearchRule struct {
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

type TOCRule struct {
	PreUpdateJS string `json:"preUpdateJs,omitempty"`
	ChapterList string `json:"chapterList"`
	ChapterName string `json:"chapterName"`
	ChapterURL  string `json:"chapterUrl"`
	IsVolume    string `json:"isVolume,omitempty"`
	IsVIP       string `json:"isVip,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
	NextTOCURL  string `json:"nextTocUrl,omitempty"`
}

type BookInfoRule struct {
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

type ContentRule struct {
	Content        string `json:"content"`
	NextContentURL string `json:"nextContentUrl,omitempty"`
	WebJS          string `json:"webJs,omitempty"`
	SourceRegex    string `json:"sourceRegex,omitempty"`
	ReplaceRegex   string `json:"replaceRegex,omitempty"`
	ImageStyle     string `json:"imageStyle,omitempty"`
}

type BookSource struct {
	BookSourceName    string       `json:"bookSourceName"`
	BookSourceGroup   string       `json:"bookSourceGroup,omitempty"`
	BookSourceURL     string       `json:"bookSourceUrl"`
	BookSourceType    int          `json:"bookSourceType"`
	BookURLPattern    string       `json:"bookUrlPattern,omitempty"`
	BookSourceComment string       `json:"bookSourceComment,omitempty"`
	Enabled           bool         `json:"enabled"`
	EnabledExplore    bool         `json:"enabledExplore"`
	SearchURL         string       `json:"searchUrl,omitempty"`
	ExploreURL        string       `json:"exploreUrl,omitempty"`
	Header            string       `json:"header,omitempty"`
	RuleSearch        SearchRule   `json:"ruleSearch"`
	RuleExplore       SearchRule   `json:"ruleExplore"`
	RuleBookInfo      BookInfoRule `json:"ruleBookInfo"`
	RuleTOC           TOCRule      `json:"ruleToc"`
	RuleContent       ContentRule  `json:"ruleContent"`
	Charset           string       `json:"charset,omitempty"`
	ConcurrentRate    string       `json:"concurrentRate,omitempty"`
	LoginURL          string       `json:"loginUrl,omitempty"`
	LoginCheckJS      string       `json:"loginCheckJs,omitempty"`
	CustomOrder       int          `json:"customOrder"`
	LastUpdateTime    int64        `json:"lastUpdateTime"`
	Weight            int          `json:"weight"`
	RespondTime       int64        `json:"respondTime"`
	Rules             string       `json:"rules,omitempty"`
}

func Export(sources []models.BookSource) []BookSource {
	exported := make([]BookSource, 0, len(sources))
	for _, source := range sources {
		rule, err := source.ParsedRules()
		if err != nil {
			rule = models.BookSourceRule{}
		}
		searchRule := SearchRule{
			BookList:    exportSelector(rule.BookListRule),
			Name:        exportSelector(rule.BookNameRule),
			Author:      exportSelector(rule.BookAuthorRule),
			CoverURL:    exportSelector(rule.BookCoverRule),
			Intro:       exportSelector(rule.BookIntroRule),
			Kind:        exportSelector(rule.BookKindRule),
			WordCount:   exportSelector(rule.BookWordCountRule),
			LastChapter: exportSelector(rule.LatestChapterRule),
			UpdateTime:  exportSelector(rule.BookUpdateTimeRule),
			BookURL:     exportSelector(rule.BookURLRule),
		}
		exploreRule := SearchRule{
			BookList:    exportSelector(rule.ExploreBookListRule),
			Name:        exportSelector(rule.ExploreBookNameRule),
			Author:      exportSelector(rule.ExploreBookAuthorRule),
			CoverURL:    exportSelector(rule.ExploreBookCoverRule),
			Intro:       exportSelector(rule.ExploreBookIntroRule),
			Kind:        exportSelector(rule.ExploreBookKindRule),
			WordCount:   exportSelector(rule.ExploreBookWordCountRule),
			LastChapter: exportSelector(rule.ExploreLatestChapterRule),
			UpdateTime:  exportSelector(rule.ExploreBookUpdateTimeRule),
			BookURL:     exportSelector(rule.ExploreBookURLRule),
		}
		header := strings.TrimSpace(source.Header)
		if header == "" && len(rule.Headers) > 0 {
			if data, marshalErr := json.Marshal(rule.Headers); marshalErr == nil {
				header = string(data)
			}
		}
		exported = append(exported, BookSource{
			BookSourceName:    source.Name,
			BookSourceGroup:   source.Group,
			BookSourceURL:     source.BaseURL,
			BookSourceType:    source.SourceType,
			BookURLPattern:    source.BookURLPattern,
			BookSourceComment: source.Comment,
			Enabled:           source.Enabled,
			EnabledExplore:    source.IsExploreEnabled(),
			SearchURL:         exportURL(firstNonBlank(rule.SearchURL, source.SearchURL)),
			ExploreURL:        exportURL(rule.ExploreURL),
			Header:            header,
			RuleSearch:        searchRule,
			RuleExplore:       exploreRule,
			RuleBookInfo: BookInfoRule{
				Init:        exportSelector(rule.BookInfoInitRule),
				Name:        exportSelector(rule.BookInfoNameRule),
				Author:      exportSelector(rule.BookInfoAuthorRule),
				CoverURL:    exportSelector(rule.BookInfoCoverRule),
				Intro:       exportSelector(rule.BookInfoIntroRule),
				Kind:        exportSelector(rule.BookInfoKindRule),
				LastChapter: exportSelector(rule.BookInfoLatestChapterRule),
				UpdateTime:  exportSelector(rule.BookInfoUpdateTimeRule),
				WordCount:   exportSelector(rule.BookInfoWordCountRule),
				TOCURL:      exportSelector(rule.TOCURLRule),
				CanRename:   exportSelector(rule.BookInfoCanRenameRule),
			},
			RuleTOC: TOCRule{
				PreUpdateJS: rule.ChapterPreUpdateJSRule,
				ChapterList: exportSelector(rule.ChapterListRule),
				ChapterName: exportSelector(rule.ChapterNameRule),
				ChapterURL:  exportSelector(rule.ChapterURLRule),
				IsVolume:    exportSelector(rule.ChapterIsVolumeRule),
				IsVIP:       exportSelector(rule.ChapterIsVIPRule),
				UpdateTime:  exportSelector(rule.ChapterUpdateTimeRule),
				NextTOCURL:  exportSelector(rule.NextTOCURLRule),
			},
			RuleContent: ContentRule{
				Content:        exportSelector(rule.ContentRule),
				NextContentURL: exportSelector(rule.NextContentURLRule),
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

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func exportURL(value string) string {
	template := strings.TrimSpace(value)
	template = strings.ReplaceAll(template, "{{key}}", "{keyword}")
	template = strings.ReplaceAll(template, "{{keyword}}", "{keyword}")
	template = strings.ReplaceAll(template, "{{page}}", "{page}")
	template = strings.ReplaceAll(template, "{keyword}", "{{key}}")
	template = strings.ReplaceAll(template, "{page}", "{{page}}")
	return template
}

func exportSelector(value string) string {
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
