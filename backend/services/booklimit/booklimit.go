// Package booklimit enforces reader-dev's per-user bookshelf cap
// (reader.app.userBookLimit, BookController.kt#saveBook: 超过用户书籍数上限).
// Contract: docs/compat/user-book-limit-fixed-baseline-second-audit-p2-contract.md.
//
// Only a path that is about to create a new shelf row checks the cap. Books
// already on the shelf keep being updated and backup restores are not
// counted, so a shelf that is already over the cap is never changed by it.
// Since 2026-11-13 there is no cap unless the instance or the user sets one.
package booklimit

import (
	"errors"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

// ErrReached means one more book would exceed the user's cap. Handlers answer
// it with 403 {"error":"book limit reached"}.
var ErrReached = errors.New("book limit reached")

// Effective is the cap for one user: their own positive BookLimit, set by an
// administrator, otherwise the instance value. 0 means no limit.
func Effective(userBookLimit int, cfg config.Config) int {
	if userBookLimit > 0 {
		return userBookLimit
	}
	return cfg.EffectiveUserBookLimit()
}

// Check returns ErrReached when the user's shelf already holds their cap.
// Callers run it inside the transaction that inserts the book, so the count
// and the insert happen under the same write lock; a check on the outer
// handle is only an early refusal before expensive work.
func Check(db *gorm.DB, userID uint, cfg config.Config) error {
	var user models.User
	if err := db.Model(&models.User{}).Select("book_limit").Where("id = ?", userID).Take(&user).Error; err != nil {
		return err
	}
	limit := Effective(user.BookLimit, cfg)
	if limit <= 0 {
		return nil
	}
	var count int64
	if err := db.Model(&models.Book{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(limit) {
		return ErrReached
	}
	return nil
}
