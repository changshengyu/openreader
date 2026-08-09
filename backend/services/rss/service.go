package rss

import (
	"strings"

	"gorm.io/gorm"

	"openreader/backend/models"
)

const MaxImportSources = 5000

type Service struct {
	db *gorm.DB
}

type ImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) ImportSources(userID uint, candidates []models.RSSSource) (ImportResult, error) {
	result := ImportResult{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
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
				candidate.ID = existing.ID
				candidate.CreatedAt = existing.CreatedAt
				if err := tx.Save(&candidate).Error; err != nil {
					return err
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
	return result, err
}

func (s *Service) UpsertArticlePage(userID, sourceID uint, sortName string, articles []models.RSSArticle) ([]models.RSSArticle, int, error) {
	persisted := make([]models.RSSArticle, 0, len(articles))
	created := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
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
			for _, duplicate := range existingRows[1:] {
				existing.IsRead = existing.IsRead || duplicate.IsRead
				existing.Favorite = existing.Favorite || duplicate.Favorite
				duplicateIDs = append(duplicateIDs, duplicate.ID)
			}
			existing.Title = article.Title
			existing.Sort = article.Sort
			existing.GUID = article.GUID
			existing.Author = article.Author
			existing.Image = article.Image
			existing.Summary = article.Summary
			existing.Content = article.Content
			existing.PubDate = article.PubDate
			existing.PublishedAt = article.PublishedAt
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			if len(duplicateIDs) > 0 {
				if err := tx.Where("id IN ?", duplicateIDs).Delete(&models.RSSArticle{}).Error; err != nil {
					return err
				}
			}
			persisted = append(persisted, existing)
		}
		return nil
	})
	return persisted, created, err
}
