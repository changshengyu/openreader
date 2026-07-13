package api

import (
	"openreader/backend/models"

	"gorm.io/gorm"
)

// sourceVariableSemanticsChanged identifies the source fields that can alter
// how an existing Book.variable or Chapter.variable is interpreted. Keeping a
// value after any of these changes could replay an old source token into a new
// origin, so variables are deliberately cleared rather than translated.
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

func clearPersistentVariablesForSource(tx *gorm.DB, sourceID uint) error {
	if sourceID == 0 {
		return nil
	}
	if err := tx.Model(&models.Book{}).Where("source_id = ?", sourceID).Update("variable", "").Error; err != nil {
		return err
	}
	bookIDs := tx.Model(&models.Book{}).Select("id").Where("source_id = ?", sourceID)
	return tx.Model(&models.Chapter{}).Where("book_id IN (?)", bookIDs).Update("variable", "").Error
}

func clearAllPersistentSourceVariables(tx *gorm.DB) error {
	if err := tx.Model(&models.Book{}).Where("source_id > 0").Update("variable", "").Error; err != nil {
		return err
	}
	bookIDs := tx.Model(&models.Book{}).Select("id").Where("source_id > 0")
	return tx.Model(&models.Chapter{}).Where("book_id IN (?)", bookIDs).Update("variable", "").Error
}
