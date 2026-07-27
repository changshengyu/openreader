package booksources

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openreader/backend/models"
)

var (
	ErrSourceNotFound = errors.New("book source not found")
	ErrSourceInUse    = errors.New("book source is in use")
)

type sourceInUseError struct {
	count int
}

func (e sourceInUseError) Error() string {
	return ErrSourceInUse.Error()
}

func (e sourceInUseError) Unwrap() error {
	return ErrSourceInUse
}

func SourceUsage(err error) int {
	var target sourceInUseError
	if errors.As(err, &target) {
		return target.count
	}
	return 0
}

type Service struct {
	db *gorm.DB
}

func New(database *gorm.DB) *Service {
	return &Service{db: database}
}

func (s *Service) EnsureNamespace(userID uint) error {
	if s == nil || s.db == nil {
		return errors.New("book source service is unavailable")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var initialized int64
		if err := tx.Model(&models.BookSourceNamespace{}).
			Where("user_id = ?", userID).
			Count(&initialized).Error; err != nil {
			return err
		}
		if initialized > 0 {
			return nil
		}

		if userID != 0 {
			var defaults []models.UserBookSource
			if err := tx.Where("user_id = ? AND detached = ?", 0, false).
				Order("source_id asc").
				Find(&defaults).Error; err != nil {
				return err
			}
			associations := make([]models.UserBookSource, 0, len(defaults))
			for _, item := range defaults {
				associations = append(associations, models.UserBookSource{
					UserID:   userID,
					SourceID: item.SourceID,
				})
			}
			if len(associations) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
					CreateInBatches(&associations, 500).Error; err != nil {
					return err
				}
			}
		}

		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.BookSourceNamespace{UserID: userID}).Error
	})
}

func (s *Service) ListActive(userID uint) ([]models.BookSource, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return nil, err
	}
	var sources []models.BookSource
	if err := activeSourceQuery(s.db, userID).
		Order("book_sources.custom_order asc, book_sources.id asc").
		Find(&sources).Error; err != nil {
		return nil, err
	}
	if len(sources) == 0 || userID == 0 {
		return sources, nil
	}
	counts, err := sourceUsageCounts(s.db, userID, nil)
	if err != nil {
		return nil, err
	}
	for index := range sources {
		sources[index].UsedBookCount = counts[sources[index].ID]
	}
	return sources, nil
}

func (s *Service) FindActive(userID, sourceID uint) (models.BookSource, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return models.BookSource{}, err
	}
	var source models.BookSource
	err := activeSourceQuery(s.db, userID).
		Where("book_sources.id = ?", sourceID).
		First(&source).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.BookSource{}, ErrSourceNotFound
	}
	return source, err
}

func (s *Service) FindForBook(userID, sourceID uint) (models.BookSource, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return models.BookSource{}, err
	}
	var source models.BookSource
	err := s.db.Model(&models.BookSource{}).
		Joins("JOIN user_book_sources ON user_book_sources.source_id = book_sources.id").
		Where("user_book_sources.user_id = ? AND book_sources.id = ?", userID, sourceID).
		First(&source).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.BookSource{}, ErrSourceNotFound
	}
	return source, err
}

func (s *Service) Create(userID uint, source models.BookSource) (models.BookSource, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return models.BookSource{}, err
	}
	source.ID = 0
	source.UsedBookCount = 0
	source.CreatedAt = time.Time{}
	source.UpdatedAt = time.Time{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		return tx.Create(&models.UserBookSource{
			UserID:   userID,
			SourceID: source.ID,
		}).Error
	})
	return source, err
}

func (s *Service) Update(userID, sourceID uint, next models.BookSource) (models.BookSource, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return models.BookSource{}, err
	}
	var updated models.BookSource
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var association models.UserBookSource
		err := tx.Where("user_id = ? AND source_id = ? AND detached = ?", userID, sourceID, false).
			First(&association).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSourceNotFound
		}
		if err != nil {
			return err
		}

		var previous models.BookSource
		if err := tx.First(&previous, sourceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSourceNotFound
			}
			return err
		}

		next.ID = previous.ID
		next.CreatedAt = previous.CreatedAt
		next.UpdatedAt = previous.UpdatedAt
		next.UsedBookCount = 0
		shared, err := sourceSnapshotIsShared(tx, userID, sourceID)
		if err != nil {
			return err
		}
		if shared {
			next.ID = 0
			next.CreatedAt = time.Time{}
			next.UpdatedAt = time.Time{}
			if err := tx.Create(&next).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.UserBookSource{}).
				Where("user_id = ? AND source_id = ?", userID, sourceID).
				Update("source_id", next.ID).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.Book{}).
				Where("user_id = ? AND source_id = ?", userID, sourceID).
				Update("source_id", next.ID).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.SourceFailure{}).
				Where("user_id = ? AND source_id = ?", userID, sourceID).
				Updates(map[string]any{"source_id": next.ID, "source_url": next.BaseURL}).Error; err != nil {
				return err
			}
		} else if err := tx.Save(&next).Error; err != nil {
			return err
		}

		if sourceVariableSemanticsChanged(previous, next) {
			if err := clearPersistentVariables(tx, userID, next.ID); err != nil {
				return err
			}
		}
		updated = next
		return nil
	})
	return updated, err
}

func (s *Service) Delete(userID, sourceID uint) error {
	if err := s.EnsureNamespace(userID); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var association models.UserBookSource
		err := tx.Where("user_id = ? AND source_id = ? AND detached = ?", userID, sourceID, false).
			First(&association).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSourceNotFound
		}
		if err != nil {
			return err
		}
		usage, err := sourceUsageCounts(tx, userID, []uint{sourceID})
		if err != nil {
			return err
		}
		if usage[sourceID] > 0 {
			return sourceInUseError{count: usage[sourceID]}
		}
		if err := tx.Delete(&association).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND source_id = ?", userID, sourceID).
			Delete(&models.SourceFailure{}).Error; err != nil {
			return err
		}
		return removeUnreferencedSource(tx, sourceID)
	})
}

func activeSourceQuery(database *gorm.DB, userID uint) *gorm.DB {
	return database.Model(&models.BookSource{}).
		Joins("JOIN user_book_sources ON user_book_sources.source_id = book_sources.id").
		Where("user_book_sources.user_id = ? AND user_book_sources.detached = ?", userID, false)
}

func sourceUsageCounts(database *gorm.DB, userID uint, sourceIDs []uint) (map[uint]int, error) {
	type sourceUsage struct {
		SourceID uint
		Count    int
	}
	query := database.Model(&models.Book{}).
		Select("source_id, COUNT(*) AS count").
		Where("user_id = ? AND source_id > 0", userID).
		Group("source_id")
	if len(sourceIDs) > 0 {
		query = query.Where("source_id IN ?", sourceIDs)
	}
	var rows []sourceUsage
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[uint]int, len(rows))
	for _, row := range rows {
		counts[row.SourceID] = row.Count
	}
	return counts, nil
}

func sourceSnapshotIsShared(tx *gorm.DB, userID, sourceID uint) (bool, error) {
	var associations int64
	if err := tx.Model(&models.UserBookSource{}).
		Where("source_id = ? AND user_id <> ?", sourceID, userID).
		Count(&associations).Error; err != nil {
		return false, err
	}
	if associations > 0 {
		return true, nil
	}
	var foreignBooks int64
	if err := tx.Model(&models.Book{}).
		Where("source_id = ? AND user_id <> ?", sourceID, userID).
		Count(&foreignBooks).Error; err != nil {
		return false, err
	}
	return foreignBooks > 0, nil
}

func clearPersistentVariables(tx *gorm.DB, userID, sourceID uint) error {
	if sourceID == 0 {
		return nil
	}
	if err := tx.Model(&models.Book{}).
		Where("user_id = ? AND source_id = ?", userID, sourceID).
		Update("variable", "").Error; err != nil {
		return err
	}
	bookIDs := tx.Model(&models.Book{}).
		Select("id").
		Where("user_id = ? AND source_id = ?", userID, sourceID)
	return tx.Model(&models.Chapter{}).
		Where("book_id IN (?)", bookIDs).
		Update("variable", "").Error
}

func sourceVariableSemanticsChanged(before, after models.BookSource) bool {
	return before.BaseURL != after.BaseURL ||
		before.SearchURL != after.SearchURL ||
		before.BookURLPattern != after.BookURLPattern ||
		before.SourceType != after.SourceType ||
		before.Charset != after.Charset ||
		before.Header != after.Header ||
		before.LoginURL != after.LoginURL ||
		before.LoginCheckJS != after.LoginCheckJS ||
		before.Rules != after.Rules
}

func removeUnreferencedSource(tx *gorm.DB, sourceID uint) error {
	for _, model := range []any{&models.UserBookSource{}, &models.Book{}, &models.SourceFailure{}} {
		var count int64
		if err := tx.Model(model).Where("source_id = ?", sourceID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
	}
	return tx.Delete(&models.BookSource{}, sourceID).Error
}
