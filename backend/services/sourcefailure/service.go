package sourcefailure

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openreader/backend/models"
)

const TTL = 10 * time.Minute

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func New(database *gorm.DB) *Service {
	return &Service{db: database, now: time.Now}
}

func (s *Service) Record(userID uint, source models.BookSource, cause error) {
	if userID == 0 || source.ID == 0 || cause == nil || errors.Is(cause, context.Canceled) {
		return
	}
	now := s.now().UTC()
	failure := models.SourceFailure{
		UserID:    userID,
		SourceID:  source.ID,
		SourceURL: sourceURL(source),
		Message:   clientMessage(cause),
		FailedAt:  now,
		ExpiresAt: now.Add(TTL),
	}
	_ = s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "source_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"source_url", "message", "failed_at", "expires_at"}),
	}).Create(&failure).Error
}

func (s *Service) Active(userID uint, sources []models.BookSource) (map[uint]models.SourceFailure, error) {
	active := make(map[uint]models.SourceFailure)
	if userID == 0 || len(sources) == 0 {
		return active, nil
	}
	now := s.now().UTC()
	_ = s.db.Where("user_id = ? AND expires_at <= ?", userID, now).Delete(&models.SourceFailure{}).Error

	var rows []models.SourceFailure
	if err := s.db.Where("user_id = ? AND expires_at > ?", userID, now).Order("failed_at desc, id desc").Find(&rows).Error; err != nil {
		return active, err
	}
	byID := make(map[uint]models.BookSource, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}
	staleIDs := make([]uint, 0)
	for _, row := range rows {
		source, ok := byID[row.SourceID]
		if !ok || row.SourceURL != sourceURL(source) {
			staleIDs = append(staleIDs, row.ID)
			continue
		}
		active[row.SourceID] = row
	}
	if len(staleIDs) > 0 {
		_ = s.db.Where("user_id = ? AND id IN ?", userID, staleIDs).Delete(&models.SourceFailure{}).Error
	}
	return active, nil
}

func (s *Service) ClearSourceIDs(sourceIDs []uint) {
	if len(sourceIDs) == 0 {
		return
	}
	_ = s.db.Where("source_id IN ?", sourceIDs).Delete(&models.SourceFailure{}).Error
}

func (s *Service) ClearAll() {
	_ = s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.SourceFailure{}).Error
}

func sourceURL(source models.BookSource) string {
	if value := strings.TrimSpace(source.BaseURL); value != "" {
		return value
	}
	if value := strings.TrimSpace(source.SearchURL); value != "" {
		return value
	}
	rule, err := source.ParsedRules()
	if err == nil {
		return strings.TrimSpace(rule.SearchURL)
	}
	return ""
}

func clientMessage(cause error) string {
	if errors.Is(cause, context.DeadlineExceeded) {
		return "请求超时"
	}
	return "请求书源失败"
}
