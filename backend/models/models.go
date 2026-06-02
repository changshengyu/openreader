package models

import (
	"encoding/json"
	"time"
)

type User struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Username       string    `json:"username" gorm:"size:80;not null;uniqueIndex"`
	PasswordHash   string    `json:"-" gorm:"size:120;not null"`
	Role           string    `json:"role" gorm:"size:20;default:user"`
	BookLimit      int       `json:"bookLimit" gorm:"default:0"`
	SourceLimit    int       `json:"sourceLimit" gorm:"default:0"`
	CanEditSources bool      `json:"canEditSources" gorm:"default:true"`
	CanAccessStore bool      `json:"canAccessStore" gorm:"default:true"`
	LastActiveAt   time.Time `json:"lastActiveAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type UserSetting struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"userId" gorm:"not null;uniqueIndex:idx_user_setting"`
	Key       string    `json:"key" gorm:"size:80;not null;uniqueIndex:idx_user_setting"`
	Value     string    `json:"value" gorm:"type:text;not null"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BookSource struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:120;not null"`
	BaseURL   string    `json:"baseUrl" gorm:"size:500"`
	SearchURL string    `json:"searchUrl" gorm:"size:500"`
	Charset   string    `json:"charset" gorm:"size:40;default:utf-8"`
	Rules     string    `json:"rules" gorm:"type:text"`
	Enabled   bool      `json:"enabled"`
	Group     string    `json:"group" gorm:"size:80"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ParsedRules deserializes the Rules JSON into a BookSourceRule.
func (s BookSource) ParsedRules() (BookSourceRule, error) {
	if s.Rules == "" {
		return BookSourceRule{}, nil
	}
	var rule BookSourceRule
	if err := json.Unmarshal([]byte(s.Rules), &rule); err != nil {
		return BookSourceRule{}, err
	}
	return rule, nil
}

// SetRules serializes rule and stores it in the Rules field.
func (s *BookSource) SetRules(rule BookSourceRule) error {
	data, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	s.Rules = string(data)
	return nil
}

// BookSourceRule defines the reader3-compatible book source rule structure.
type BookSourceRule struct {
	// Search: URL template with {keyword} placeholder.
	SearchURL  string `json:"searchUrl,omitempty"`
	ExploreURL string `json:"exploreUrl,omitempty"`

	// Search result list: CSS selector for the container of each result item.
	BookListRule string `json:"bookListRule,omitempty"`
	// Per-item field selectors (relative to each result item).
	BookNameRule      string `json:"bookNameRule,omitempty"`
	BookAuthorRule    string `json:"bookAuthorRule,omitempty"`
	BookCoverRule     string `json:"bookCoverRule,omitempty"`
	BookIntroRule     string `json:"bookIntroRule,omitempty"`
	LatestChapterRule string `json:"latestChapterRule,omitempty"`
	BookURLRule       string `json:"bookUrlRule,omitempty"`

	// TOC/directory page URL template (typically derived from book URL).
	TOCURLRule string `json:"tocUrlRule,omitempty"`

	// Chapter list selectors.
	ChapterListRule string `json:"chapterListRule,omitempty"`
	ChapterNameRule string `json:"chapterNameRule,omitempty"`
	ChapterURLRule  string `json:"chapterUrlRule,omitempty"`

	// Content page: URL template and content selector.
	ContentURLRule string `json:"contentUrlRule,omitempty"`
	ContentRule    string `json:"contentRule,omitempty"`

	// HTTP headers for requests made with this source.
	Headers map[string]string `json:"headers,omitempty"`

	// Pagination: selector for "next page" link in search results.
	PaginationRule string `json:"paginationRule,omitempty"`

	// Text replacement rules applied to fetched content.
	TextReplaceRules []TextReplaceRule `json:"textReplaceRules,omitempty"`
}

// TextReplaceRule defines a regex-based text replacement for content filtering.
type TextReplaceRule struct {
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
}

type ReplaceRule struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"userId" gorm:"not null;index"`
	Name        string    `json:"name" gorm:"size:120;not null"`
	Pattern     string    `json:"pattern" gorm:"type:text;not null"`
	Replacement string    `json:"replacement" gorm:"type:text"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type RSSSource struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"userId" gorm:"not null;index"`
	Title     string    `json:"title" gorm:"size:160;not null"`
	URL       string    `json:"url" gorm:"size:800;not null"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RSSArticle struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"userId" gorm:"not null;index"`
	SourceID    uint      `json:"sourceId" gorm:"not null;index"`
	Title       string    `json:"title" gorm:"size:240;not null"`
	Link        string    `json:"link" gorm:"size:800;index"`
	Author      string    `json:"author" gorm:"size:160"`
	Summary     string    `json:"summary" gorm:"type:text"`
	Content     string    `json:"content" gorm:"type:text"`
	IsRead      bool      `json:"isRead"`
	Favorite    bool      `json:"favorite"`
	PublishedAt time.Time `json:"publishedAt"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Book struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	UserID         uint      `json:"userId" gorm:"index"`
	SourceID       uint      `json:"sourceId" gorm:"index"`
	CategoryID     *uint     `json:"categoryId,omitempty" gorm:"index"`
	Title          string    `json:"title" gorm:"size:240;not null"`
	Author         string    `json:"author" gorm:"size:160"`
	CoverURL       string    `json:"coverUrl" gorm:"size:600"`
	CustomCoverURL string    `json:"customCoverUrl" gorm:"size:600"`
	Intro          string    `json:"intro" gorm:"type:text"`
	URL            string    `json:"url" gorm:"size:800;index"`
	LibraryPath    string    `json:"libraryPath" gorm:"size:600"`
	OriginalFile   string    `json:"originalFile" gorm:"size:600"`
	TOCFile        string    `json:"tocFile" gorm:"size:600"`
	SourceFile     string    `json:"sourceFile" gorm:"size:600"`
	LastChapter    string    `json:"lastChapter" gorm:"size:240"`
	ChapterCount   int       `json:"chapterCount"`
	CanUpdate      bool      `json:"canUpdate" gorm:"default:true"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Category struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"userId" gorm:"not null;uniqueIndex:idx_user_category"`
	Name      string    `json:"name" gorm:"size:80;not null;uniqueIndex:idx_user_category"`
	Color     string    `json:"color" gorm:"size:24"`
	Show      bool      `json:"show" gorm:"default:true"`
	SortOrder int       `json:"sortOrder" gorm:"default:0"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Chapter struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	BookID    uint      `json:"bookId" gorm:"not null;uniqueIndex:idx_book_chapter"`
	Index     int       `json:"index" gorm:"not null;uniqueIndex:idx_book_chapter"`
	Title     string    `json:"title" gorm:"size:240;not null"`
	URL       string    `json:"url" gorm:"size:800"`
	CachePath string    `json:"cachePath" gorm:"size:500"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ReadingProgress struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	UserID         uint      `json:"userId" gorm:"not null;uniqueIndex:idx_user_book_progress"`
	BookID         uint      `json:"bookId" gorm:"not null;uniqueIndex:idx_user_book_progress"`
	ChapterID      uint      `json:"chapterId"`
	ChapterIndex   int       `json:"chapterIndex"`
	Offset         int       `json:"offset"`
	Percent        float64   `json:"percent"`
	ChapterPercent float64   `json:"chapterPercent"`
	ChapterTitle   string    `json:"chapterTitle" gorm:"size:240"`
	Mode           string    `json:"mode" gorm:"size:20"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Bookmark struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	UserID       uint      `json:"userId" gorm:"not null;index"`
	BookID       uint      `json:"bookId" gorm:"not null;index"`
	ChapterID    uint      `json:"chapterId"`
	ChapterIndex int       `json:"chapterIndex"`
	Offset       int       `json:"offset"`
	Percent      float64   `json:"percent"`
	Title        string    `json:"title" gorm:"size:160"`
	Excerpt      string    `json:"excerpt" gorm:"size:500"`
	Note         string    `json:"note" gorm:"size:500"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
