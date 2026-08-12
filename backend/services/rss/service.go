package rss

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"openreader/backend/models"
)

const (
	MaxImportSources       = 5000
	rssWriteRetryLimit     = 10
	rssWriteRetryBaseDelay = 5 * time.Millisecond
)

var (
	ErrSourceNotFound    = errors.New("RSS source not found")
	ErrSourceURLConflict = errors.New("RSS source URL already exists")
	ErrArticleNotFound   = errors.New("RSS article not found")
	rssSourceWriteLocks  = sourceUserLockRegistry{locks: make(map[uint]*sourceUserLock)}
)

type sourceUserLock struct {
	mutex sync.Mutex
	refs  int
}

type sourceUserLockRegistry struct {
	mutex sync.Mutex
	locks map[uint]*sourceUserLock
}

type Service struct {
	db *gorm.DB
}

type SourceInput struct {
	Source          models.RSSSource
	CustomOrder     *int
	SingleURL       *bool
	ArticleStyle    *int
	EnableJS        *bool
	LoadWithBaseURL *bool
	Enabled         *bool
}

type ImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

func lockSourceUserWrite(userID uint) func() {
	rssSourceWriteLocks.mutex.Lock()
	entry := rssSourceWriteLocks.locks[userID]
	if entry == nil {
		entry = &sourceUserLock{}
		rssSourceWriteLocks.locks[userID] = entry
	}
	entry.refs++
	rssSourceWriteLocks.mutex.Unlock()

	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		rssSourceWriteLocks.mutex.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(rssSourceWriteLocks.locks, userID)
		}
		rssSourceWriteLocks.mutex.Unlock()
	}
}

func (s *Service) CreateOrReplaceSource(userID uint, input SourceInput) (models.RSSSource, bool, error) {
	unlock := lockSourceUserWrite(userID)
	defer unlock()

	var source models.RSSSource
	created := false
	missing := false
	err := retryRSSSQLiteWrite(func() error {
		source = models.RSSSource{}
		created = false
		missing = false
		return s.db.Transaction(func(tx *gorm.DB) error {
			input.Source.UserID = userID
			input.Source.Title = strings.TrimSpace(input.Source.Title)
			input.Source.URL = strings.TrimSpace(input.Source.URL)

			query := tx.Where("user_id = ? AND url = ?", userID, input.Source.URL).
				Order("id asc").Limit(1).Find(&source)
			if query.Error != nil {
				return query.Error
			}
			if query.RowsAffected > 0 {
				write := tx.Model(&models.RSSSource{}).
					Where("user_id = ? AND id = ?", userID, source.ID).
					Updates(sourceInputColumns(input))
				if write.Error != nil {
					return write.Error
				}
				if write.RowsAffected == 0 {
					missing = true
					return nil
				}
				if err := tx.Where("user_id = ? AND id = ?", userID, source.ID).First(&source).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						missing = true
						return nil
					}
					return err
				}
				return nil
			}

			order, err := nextSourceOrder(tx, userID, input.CustomOrder)
			if err != nil {
				return err
			}
			source = input.Source
			source.ID = 0
			source.UserID = userID
			source.CustomOrder = order
			if err := tx.Create(&source).Error; err != nil {
				return err
			}
			created = true
			return nil
		})
	})
	if err != nil {
		return models.RSSSource{}, false, err
	}
	if missing {
		return models.RSSSource{}, false, ErrSourceNotFound
	}
	return source, created, nil
}

func (s *Service) UpdateSource(userID, sourceID uint, input SourceInput) (models.RSSSource, error) {
	unlock := lockSourceUserWrite(userID)
	defer unlock()

	var source models.RSSSource
	missing := false
	conflict := false
	err := retryRSSSQLiteWrite(func() error {
		source = models.RSSSource{}
		missing = false
		conflict = false
		return s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("user_id = ? AND id = ?", userID, sourceID).First(&source).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					missing = true
					return nil
				}
				return err
			}

			var collisionCount int64
			if err := tx.Model(&models.RSSSource{}).
				Where("user_id = ? AND url = ? AND id <> ?", userID, strings.TrimSpace(input.Source.URL), sourceID).
				Count(&collisionCount).Error; err != nil {
				return err
			}
			if collisionCount > 0 {
				conflict = true
				return nil
			}

			write := tx.Model(&models.RSSSource{}).
				Where("user_id = ? AND id = ?", userID, sourceID).
				Updates(sourceInputColumns(input))
			if write.Error != nil {
				return write.Error
			}
			if write.RowsAffected == 0 {
				missing = true
				return nil
			}
			if err := tx.Where("user_id = ? AND id = ?", userID, sourceID).First(&source).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					missing = true
					return nil
				}
				return err
			}
			return nil
		})
	})
	if err != nil {
		return models.RSSSource{}, err
	}
	if conflict {
		return models.RSSSource{}, ErrSourceURLConflict
	}
	if missing {
		return models.RSSSource{}, ErrSourceNotFound
	}
	return source, nil
}

func (s *Service) ImportSources(userID uint, candidates []models.RSSSource) (ImportResult, error) {
	unlock := lockSourceUserWrite(userID)
	defer unlock()

	result := ImportResult{}
	err := retryRSSSQLiteWrite(func() error {
		result = ImportResult{}
		return s.db.Transaction(func(tx *gorm.DB) error {
			var maxOrder int
			if err := tx.Model(&models.RSSSource{}).
				Where("user_id = ?", userID).
				Select("COALESCE(MAX(custom_order), 0)").
				Scan(&maxOrder).Error; err != nil {
				return err
			}
			for _, candidate := range candidates {
				candidate.UserID = userID
				candidate.Title = strings.TrimSpace(candidate.Title)
				candidate.URL = strings.TrimSpace(candidate.URL)
				if candidate.Title == "" || candidate.URL == "" {
					result.Skipped++
					continue
				}

				var existing models.RSSSource
				query := tx.Where("user_id = ? AND url = ?", userID, candidate.URL).
					Order("id asc").Limit(1).Find(&existing)
				if query.Error != nil {
					return query.Error
				}
				if query.RowsAffected > 0 {
					if candidate.CustomOrder <= 0 {
						candidate.CustomOrder = existing.CustomOrder
					}
					write := tx.Model(&models.RSSSource{}).
						Where("user_id = ? AND id = ?", userID, existing.ID).
						Updates(sourceColumns(candidate))
					if write.Error != nil {
						return write.Error
					}
					if write.RowsAffected == 0 {
						return ErrSourceNotFound
					}
					result.Updated++
					continue
				}

				if candidate.CustomOrder <= 0 {
					maxOrder++
					candidate.CustomOrder = maxOrder
				} else if candidate.CustomOrder > maxOrder {
					maxOrder = candidate.CustomOrder
				}
				candidate.ID = 0
				if err := tx.Create(&candidate).Error; err != nil {
					return err
				}
				result.Created++
			}
			return nil
		})
	})
	return result, err
}

func (s *Service) DeleteSource(userID, sourceID uint) error {
	unlock := lockSourceUserWrite(userID)
	defer unlock()

	err := retryRSSSQLiteWrite(func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			var source models.RSSSource
			if err := tx.Where("user_id = ? AND id = ?", userID, sourceID).First(&source).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id = ? AND source_id = ?", userID, sourceID).
				Delete(&models.RSSArticle{}).Error; err != nil {
				return err
			}
			return tx.Where("user_id = ? AND id = ?", userID, sourceID).Delete(&models.RSSSource{}).Error
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSourceNotFound
	}
	return err
}

func (s *Service) UpsertArticlePage(userID, sourceID uint, sortName string, articles []models.RSSArticle) ([]models.RSSArticle, int, error) {
	unlock := lockSourceUserWrite(userID)
	defer unlock()

	persisted := make([]models.RSSArticle, 0, len(articles))
	created := 0
	err := retryRSSSQLiteWrite(func() error {
		persisted = make([]models.RSSArticle, 0, len(articles))
		created = 0
		return s.db.Transaction(func(tx *gorm.DB) error {
			var source models.RSSSource
			if err := tx.Where("user_id = ? AND id = ?", userID, sourceID).First(&source).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrSourceNotFound
				}
				return err
			}
			hasDetailContentRule := strings.TrimSpace(source.RuleContent) != ""

			for _, article := range articles {
				article.UserID = userID
				article.SourceID = sourceID
				article.Sort = sortName

				existingQuery := tx.Where(
					"user_id = ? AND source_id = ? AND sort = ?",
					userID,
					sourceID,
					article.Sort,
				)
				switch {
				case article.Link != "":
					existingQuery = existingQuery.Where("link = ?", article.Link)
				case article.GUID != "":
					existingQuery = existingQuery.Where(
						"link = '' AND (guid = ? OR (guid = '' AND title = ? AND author = ? AND pub_date = ?))",
						article.GUID,
						article.Title,
						article.Author,
						article.PubDate,
					)
				default:
					existingQuery = existingQuery.Where(
						"link = '' AND guid = '' AND title = ? AND author = ? AND pub_date = ?",
						article.Title,
						article.Author,
						article.PubDate,
					)
				}

				var existingRows []models.RSSArticle
				if err := existingQuery.Order("id asc").Find(&existingRows).Error; err != nil {
					return err
				}
				if len(existingRows) == 0 {
					if err := tx.Create(&article).Error; err != nil {
						return err
					}
					persisted = append(persisted, article)
					created++
					continue
				}

				existing := existingRows[0]
				duplicateIDs := make([]uint, 0, len(existingRows)-1)
				duplicateRead := false
				duplicateFavorite := false
				for _, duplicate := range existingRows[1:] {
					duplicateRead = duplicateRead || duplicate.IsRead
					duplicateFavorite = duplicateFavorite || duplicate.Favorite
					duplicateIDs = append(duplicateIDs, duplicate.ID)
				}

				columns := map[string]any{
					"sort":         article.Sort,
					"title":        article.Title,
					"link":         article.Link,
					"guid":         article.GUID,
					"author":       article.Author,
					"image":        article.Image,
					"summary":      article.Summary,
					"pub_date":     article.PubDate,
					"published_at": article.PublishedAt,
				}
				if !hasDetailContentRule || strings.TrimSpace(existing.Content) == "" {
					columns["content"] = article.Content
				}
				write := tx.Model(&models.RSSArticle{}).
					Where("user_id = ? AND source_id = ? AND id = ?", userID, sourceID, existing.ID).
					Updates(columns)
				if write.Error != nil {
					return write.Error
				}
				if write.RowsAffected == 0 {
					return ErrArticleNotFound
				}

				stateColumns := make(map[string]any, 2)
				if duplicateRead {
					stateColumns["is_read"] = true
				}
				if duplicateFavorite {
					stateColumns["favorite"] = true
				}
				if len(stateColumns) > 0 {
					write = tx.Model(&models.RSSArticle{}).
						Where("user_id = ? AND source_id = ? AND id = ?", userID, sourceID, existing.ID).
						Updates(stateColumns)
					if write.Error != nil {
						return write.Error
					}
					if write.RowsAffected == 0 {
						return ErrArticleNotFound
					}
				}
				if len(duplicateIDs) > 0 {
					if err := tx.Where("user_id = ? AND source_id = ? AND id IN ?", userID, sourceID, duplicateIDs).
						Delete(&models.RSSArticle{}).Error; err != nil {
						return err
					}
				}
				if err := tx.Where("user_id = ? AND source_id = ? AND id = ?", userID, sourceID, existing.ID).
					First(&existing).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return ErrArticleNotFound
					}
					return err
				}
				persisted = append(persisted, existing)
			}
			return nil
		})
	})
	return persisted, created, err
}

func (s *Service) UpdateArticleState(userID, articleID uint, isRead, favorite *bool) (models.RSSArticle, error) {
	columns := make(map[string]any, 2)
	if isRead != nil {
		columns["is_read"] = *isRead
	}
	if favorite != nil {
		columns["favorite"] = *favorite
	}

	write := s.db.Model(&models.RSSArticle{}).
		Where("user_id = ? AND id = ?", userID, articleID).
		Updates(columns)
	if write.Error != nil {
		return models.RSSArticle{}, write.Error
	}
	var article models.RSSArticle
	if err := s.db.Where("user_id = ? AND id = ?", userID, articleID).First(&article).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.RSSArticle{}, ErrArticleNotFound
		}
		return models.RSSArticle{}, err
	}
	return article, nil
}

func (s *Service) CommitArticleContent(userID, sourceID, articleID uint, content *string) (models.RSSArticle, error) {
	unlock := lockSourceUserWrite(userID)
	defer unlock()

	var article models.RSSArticle
	sourceMissing := false
	articleMissing := false
	err := retryRSSSQLiteWrite(func() error {
		article = models.RSSArticle{}
		sourceMissing = false
		articleMissing = false
		return s.db.Transaction(func(tx *gorm.DB) error {
			var source models.RSSSource
			if err := tx.Where("user_id = ? AND id = ?", userID, sourceID).First(&source).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					sourceMissing = true
					return nil
				}
				return err
			}
			if err := tx.Where("user_id = ? AND source_id = ? AND id = ?", userID, sourceID, articleID).
				First(&article).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					articleMissing = true
					return nil
				}
				return err
			}
			if content != nil {
				write := tx.Model(&models.RSSArticle{}).
					Where("user_id = ? AND source_id = ? AND id = ?", userID, sourceID, articleID).
					Update("content", *content)
				if write.Error != nil {
					return write.Error
				}
				if write.RowsAffected == 0 {
					articleMissing = true
					return nil
				}
				if err := tx.Where("user_id = ? AND source_id = ? AND id = ?", userID, sourceID, articleID).
					First(&article).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						articleMissing = true
						return nil
					}
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return models.RSSArticle{}, err
	}
	if sourceMissing {
		return models.RSSArticle{}, ErrSourceNotFound
	}
	if articleMissing {
		return models.RSSArticle{}, ErrArticleNotFound
	}
	return article, nil
}

func nextSourceOrder(tx *gorm.DB, userID uint, requested *int) (int, error) {
	if requested != nil && *requested > 0 {
		return *requested, nil
	}
	var maxOrder int
	if err := tx.Model(&models.RSSSource{}).
		Where("user_id = ?", userID).
		Select("COALESCE(MAX(custom_order), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

func sourceInputColumns(input SourceInput) map[string]any {
	columns := sourceColumns(input.Source)
	delete(columns, "custom_order")
	delete(columns, "single_url")
	delete(columns, "article_style")
	delete(columns, "enable_js")
	delete(columns, "load_with_base_url")
	delete(columns, "enabled")
	if input.CustomOrder != nil {
		columns["custom_order"] = *input.CustomOrder
	}
	if input.SingleURL != nil {
		columns["single_url"] = *input.SingleURL
	}
	if input.ArticleStyle != nil {
		columns["article_style"] = *input.ArticleStyle
	}
	if input.EnableJS != nil {
		columns["enable_js"] = *input.EnableJS
	}
	if input.LoadWithBaseURL != nil {
		columns["load_with_base_url"] = *input.LoadWithBaseURL
	}
	if input.Enabled != nil {
		columns["enabled"] = *input.Enabled
	}
	return columns
}

func sourceColumns(source models.RSSSource) map[string]any {
	return map[string]any{
		"title":              strings.TrimSpace(source.Title),
		"url":                strings.TrimSpace(source.URL),
		"icon":               source.Icon,
		"group":              source.Group,
		"comment":            source.Comment,
		"custom_order":       source.CustomOrder,
		"concurrent_rate":    source.ConcurrentRate,
		"header":             source.Header,
		"login_url":          source.LoginURL,
		"login_check_js":     source.LoginCheckJS,
		"single_url":         source.SingleURL,
		"article_style":      source.ArticleStyle,
		"sort_url":           source.SortURL,
		"rule_articles":      source.RuleArticles,
		"rule_next_page":     source.RuleNextPage,
		"rule_title":         source.RuleTitle,
		"rule_pub_date":      source.RulePubDate,
		"rule_description":   source.RuleDescription,
		"rule_image":         source.RuleImage,
		"rule_link":          source.RuleLink,
		"rule_content":       source.RuleContent,
		"style":              source.Style,
		"enable_js":          source.EnableJS,
		"load_with_base_url": source.LoadWithBaseURL,
		"enabled":            source.Enabled,
	}
}

func retryRSSSQLiteWrite(operation func() error) error {
	var err error
	for attempt := 0; attempt < rssWriteRetryLimit; attempt++ {
		err = operation()
		if err == nil || !isRSSSQLiteWriteContention(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * rssWriteRetryBaseDelay)
	}
	return err
}

func isRSSSQLiteWriteContention(err error) bool {
	var sqliteError sqlite3.Error
	if !errors.As(err, &sqliteError) {
		return false
	}
	return sqliteError.Code == sqlite3.ErrBusy || sqliteError.Code == sqlite3.ErrLocked
}
