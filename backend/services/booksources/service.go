package booksources

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openreader/backend/models"
)

var (
	ErrSourceNotFound          = errors.New("book source not found")
	ErrSourceInUse             = errors.New("book source is in use")
	ErrNoDefault               = errors.New("default book sources are not configured")
	ErrNamespaceNotInitialized = errors.New("user book sources are not initialized")
	sourceNamespaceInitMu      sync.Mutex
)

type ImportResult struct {
	Imported int `json:"imported"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
	Detached int `json:"detached,omitempty"`
	Removed  int `json:"removed,omitempty"`
}

type RestoreUsersResult struct {
	Reset    int `json:"reset"`
	Imported int `json:"imported"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
}

const (
	managedCreateSessionKey       = "openreader:book-sources:managed-create"
	namespaceInitRetryLimit       = 10
	namespaceInitRetryBaseBackoff = 10 * time.Millisecond
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
	initialized, err := s.namespaceInitialized(userID)
	if err != nil || initialized {
		return err
	}

	// Two root-workspace requests can discover the same new user at once.
	// SQLite cannot upgrade both deferred read transactions to writers, so the
	// loser receives SQLITE_BUSY immediately even with busy_timeout configured.
	// Serialize only the rare initialization path, then recheck after waiting.
	sourceNamespaceInitMu.Lock()
	defer sourceNamespaceInitMu.Unlock()
	initialized, err = s.namespaceInitialized(userID)
	if err != nil || initialized {
		return err
	}

	for attempt := 0; attempt < namespaceInitRetryLimit; attempt++ {
		err = s.initializeNamespace(userID)
		if err == nil {
			return nil
		}
		if !isSQLiteWriteContention(err) || attempt == namespaceInitRetryLimit-1 {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * namespaceInitRetryBaseBackoff)
		initialized, checkErr := s.namespaceInitialized(userID)
		if checkErr == nil && initialized {
			return nil
		}
		if checkErr != nil && !isSQLiteWriteContention(checkErr) {
			return checkErr
		}
	}
	return err
}

func (s *Service) initializeNamespace(userID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
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

func isSQLiteWriteContention(err error) bool {
	var sqliteError sqlite3.Error
	if !errors.As(err, &sqliteError) {
		return false
	}
	return sqliteError.Code == sqlite3.ErrBusy || sqliteError.Code == sqlite3.ErrLocked
}

func (s *Service) ListActive(userID uint) ([]models.BookSource, error) {
	return s.ListActiveByIDs(userID, nil, false)
}

// ActiveCounts reports the visible source count for each user without
// initializing any namespace. An uninitialized user projects the current
// default count, matching the sources they would receive on first access while
// keeping later default changes observable until that first access.
func (s *Service) ActiveCounts(userIDs []uint) (map[uint]int64, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("book source service is unavailable")
	}
	userIDs = uniqueSourceUserIDs(userIDs)
	counts := make(map[uint]int64, len(userIDs))
	if len(userIDs) == 0 {
		return counts, nil
	}

	var initializedIDs []uint
	if err := s.db.Model(&models.BookSourceNamespace{}).
		Where("user_id IN ?", userIDs).
		Pluck("user_id", &initializedIDs).Error; err != nil {
		return nil, err
	}
	initialized := make(map[uint]bool, len(initializedIDs))
	for _, userID := range initializedIDs {
		initialized[userID] = true
	}

	type countRow struct {
		UserID uint
		Count  int64
	}
	var rows []countRow
	if err := s.db.Model(&models.UserBookSource{}).
		Select("user_id, COUNT(*) AS count").
		Where("user_id IN ? AND detached = ?", userIDs, false).
		Group("user_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.UserID] = row.Count
	}

	_, defaultCount, err := s.DefaultStatus()
	if err != nil {
		return nil, err
	}
	for _, userID := range userIDs {
		if !initialized[userID] {
			counts[userID] = int64(defaultCount)
		}
	}
	return counts, nil
}

func (s *Service) ListExistingActive(userID uint) ([]models.BookSource, error) {
	initialized, err := s.namespaceInitialized(userID)
	if err != nil {
		return nil, err
	}
	if !initialized {
		return nil, ErrNamespaceNotInitialized
	}
	sources := make([]models.BookSource, 0)
	if err := activeSourceQuery(s.db, userID).
		Order("book_sources.custom_order asc, book_sources.id asc").
		Find(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}

// FindExistingActiveByIdentity resolves a source already visible in the user's
// active namespace without initializing that namespace. URL is authoritative
// when present; name is the compatibility fallback used by older archives.
func (s *Service) FindExistingActiveByIdentity(userID uint, sourceURL, sourceName string) (models.BookSource, error) {
	if s == nil || s.db == nil {
		return models.BookSource{}, errors.New("book source service is unavailable")
	}
	find := func(column, value string) (models.BookSource, error) {
		var source models.BookSource
		err := activeSourceQuery(s.db, userID).
			Where("book_sources."+column+" = ?", value).
			Order("book_sources.custom_order asc, book_sources.id asc").
			First(&source).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.BookSource{}, ErrSourceNotFound
		}
		return source, err
	}
	if sourceURL = strings.TrimSpace(sourceURL); sourceURL != "" && !strings.EqualFold(sourceURL, "loc_book") {
		if source, err := find("base_url", sourceURL); err == nil {
			return source, nil
		} else if !errors.Is(err, ErrSourceNotFound) {
			return models.BookSource{}, err
		}
	}
	if sourceName = strings.TrimSpace(sourceName); sourceName != "" {
		return find("name", sourceName)
	}
	return models.BookSource{}, ErrSourceNotFound
}

func (s *Service) ListActiveByIDs(userID uint, sourceIDs []uint, enabledOnly bool) ([]models.BookSource, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return nil, err
	}
	sources := make([]models.BookSource, 0)
	query := activeSourceQuery(s.db, userID)
	if len(sourceIDs) > 0 {
		query = query.Where("book_sources.id IN ?", sourceIDs)
	}
	if enabledOnly {
		query = query.Where("book_sources.enabled = ?", true)
	}
	if err := query.
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
	return s.FindExistingForBook(userID, sourceID)
}

// FindExistingForBook accepts active or detached associations because an
// existing user book may continue reading through its detached source
// snapshot. It deliberately has no namespace-initialization side effect.
func (s *Service) FindExistingForBook(userID, sourceID uint) (models.BookSource, error) {
	if s == nil || s.db == nil {
		return models.BookSource{}, errors.New("book source service is unavailable")
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
		if err := tx.Set(managedCreateSessionKey, true).Create(&source).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.UserBookSource{
			UserID: userID, SourceID: source.ID,
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
			if err := tx.Set(managedCreateSessionKey, true).Create(&next).Error; err != nil {
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

func (s *Service) ClearActive(userID uint) (int, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return 0, err
	}
	affected := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.SourceFailure{}).Error; err != nil {
			return err
		}
		var associations []models.UserBookSource
		if err := tx.Where("user_id = ? AND detached = ?", userID, false).
			Order("source_id asc").
			Find(&associations).Error; err != nil {
			return err
		}
		if len(associations) == 0 {
			return nil
		}
		sourceIDs := make([]uint, 0, len(associations))
		for _, association := range associations {
			sourceIDs = append(sourceIDs, association.SourceID)
		}
		usage, err := sourceUsageCounts(tx, userID, sourceIDs)
		if err != nil {
			return err
		}
		for _, association := range associations {
			affected++
			if usage[association.SourceID] > 0 {
				if err := tx.Model(&models.UserBookSource{}).
					Where("user_id = ? AND source_id = ?", userID, association.SourceID).
					Update("detached", true).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Delete(&association).Error; err != nil {
				return err
			}
			if err := removeUnreferencedSource(tx, association.SourceID); err != nil {
				return err
			}
		}
		return nil
	})
	return affected, err
}

func (s *Service) BatchSetEnabled(userID uint, sourceIDs []uint, enabled bool) (int, error) {
	return s.batchUpdate(userID, sourceIDs, func(source *models.BookSource) {
		source.Enabled = enabled
	})
}

func (s *Service) BatchSetGroup(userID uint, sourceIDs []uint, group string) (int, error) {
	group = strings.TrimSpace(group)
	return s.batchUpdate(userID, sourceIDs, func(source *models.BookSource) {
		source.Group = group
	})
}

func (s *Service) BatchDelete(userID uint, sourceIDs []uint) (int, int, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return 0, 0, err
	}
	affected := 0
	skippedUsed := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		scoped := New(tx)
		sources, err := scoped.ListActiveByIDs(userID, sourceIDs, false)
		if err != nil {
			return err
		}
		for _, source := range sources {
			err := scoped.Delete(userID, source.ID)
			if errors.Is(err, ErrSourceInUse) {
				skippedUsed++
				continue
			}
			if err != nil {
				return err
			}
			affected++
		}
		return nil
	})
	return affected, skippedUsed, err
}

func (s *Service) Import(userID uint, sources []models.BookSource) (ImportResult, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		scoped := New(tx)
		active := make([]models.BookSource, 0)
		if err := activeSourceQuery(tx, userID).
			Order("book_sources.custom_order asc, book_sources.id asc").
			Find(&active).Error; err != nil {
			return err
		}
		byIdentity := make(map[string]models.BookSource, len(active))
		for _, source := range active {
			byIdentity[sourceIdentity(source)] = source
		}
		seen := make(map[string]bool, len(sources))
		for _, source := range sources {
			source = normalizeSource(source)
			if source.Name == "" {
				result.Skipped++
				continue
			}
			identity := sourceIdentity(source)
			if identity == "" || seen[identity] {
				result.Skipped++
				continue
			}
			seen[identity] = true
			if existing, ok := byIdentity[identity]; ok {
				source.ID = existing.ID
				updated, err := scoped.Update(userID, existing.ID, source)
				if err != nil {
					return err
				}
				byIdentity[identity] = updated
				result.Updated++
				continue
			}
			created, err := scoped.Create(userID, source)
			if err != nil {
				return err
			}
			byIdentity[identity] = created
			result.Imported++
		}
		return nil
	})
	return result, err
}

// ReplaceActive reconciles an archive source list into exactly one user's
// active namespace. Missing sources still used by that user's books remain as
// detached snapshots; unused associations are removed. The caller may provide
// a transaction-scoped DB, in which case this savepoint remains governed by
// the caller's outer restore transaction.
func (s *Service) ReplaceActive(userID uint, sources []models.BookSource) (ImportResult, error) {
	if s == nil || s.db == nil {
		return ImportResult{}, errors.New("book source service is unavailable")
	}
	result := ImportResult{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.BookSourceNamespace{UserID: userID}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.SourceFailure{}).Error; err != nil {
			return err
		}

		var associations []models.UserBookSource
		if err := tx.Where("user_id = ?", userID).
			Order("detached asc, source_id asc").
			Find(&associations).Error; err != nil {
			return err
		}
		sourceIDs := make([]uint, 0, len(associations))
		associationByID := make(map[uint]models.UserBookSource, len(associations))
		for _, association := range associations {
			sourceIDs = append(sourceIDs, association.SourceID)
			associationByID[association.SourceID] = association
		}
		var current []models.BookSource
		if len(sourceIDs) > 0 {
			if err := tx.Where("id IN ?", sourceIDs).Find(&current).Error; err != nil {
				return err
			}
		}
		sourceByID := make(map[uint]models.BookSource, len(current))
		for _, source := range current {
			sourceByID[source.ID] = source
		}
		identityCandidates := make(map[string][]uint, len(current))
		for _, association := range associations {
			source, exists := sourceByID[association.SourceID]
			if !exists {
				continue
			}
			identity := sourceIdentity(source)
			if identity != "" {
				identityCandidates[identity] = append(identityCandidates[identity], source.ID)
			}
		}

		consumed := make(map[uint]bool, len(sources))
		seen := make(map[string]bool, len(sources))
		scoped := New(tx)
		for _, source := range sources {
			source = normalizeSource(source)
			identity := sourceIdentity(source)
			if source.Name == "" || identity == "" || seen[identity] {
				result.Skipped++
				continue
			}
			seen[identity] = true

			var existingID uint
			for _, candidate := range identityCandidates[identity] {
				if !consumed[candidate] {
					existingID = candidate
					break
				}
			}
			if existingID == 0 {
				if _, err := scoped.Create(userID, source); err != nil {
					return err
				}
				result.Imported++
				continue
			}

			association := associationByID[existingID]
			if association.Detached {
				if err := tx.Model(&models.UserBookSource{}).
					Where("user_id = ? AND source_id = ?", userID, existingID).
					Update("detached", false).Error; err != nil {
					return err
				}
			}
			if _, err := scoped.Update(userID, existingID, source); err != nil {
				return err
			}
			consumed[existingID] = true
			result.Updated++
		}

		for _, association := range associations {
			if consumed[association.SourceID] {
				continue
			}
			usage, err := sourceUsageCounts(tx, userID, []uint{association.SourceID})
			if err != nil {
				return err
			}
			if usage[association.SourceID] > 0 {
				if !association.Detached {
					if err := tx.Model(&models.UserBookSource{}).
						Where("user_id = ? AND source_id = ?", userID, association.SourceID).
						Update("detached", true).Error; err != nil {
						return err
					}
					result.Skipped++
					result.Detached++
				}
				continue
			}
			if err := tx.Delete(&association).Error; err != nil {
				return err
			}
			if err := removeUnreferencedSource(tx, association.SourceID); err != nil {
				return err
			}
			result.Removed++
		}
		return nil
	})
	return result, err
}

func (s *Service) DefaultStatus() (bool, int, error) {
	if s == nil || s.db == nil {
		return false, 0, errors.New("book source service is unavailable")
	}
	var namespaceCount int64
	if err := s.db.Model(&models.BookSourceNamespace{}).
		Where("user_id = ?", 0).
		Count(&namespaceCount).Error; err != nil {
		return false, 0, err
	}
	if namespaceCount == 0 {
		return false, 0, nil
	}
	var sourceCount int64
	if err := s.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND detached = ?", 0, false).
		Count(&sourceCount).Error; err != nil {
		return false, 0, err
	}
	return true, int(sourceCount), nil
}

func (s *Service) SaveDefaultFromUser(userID uint) (int, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return 0, err
	}
	return s.saveDefaultFromUser(userID)
}

func (s *Service) SaveDefaultFromExistingUser(userID uint) (int, error) {
	initialized, err := s.namespaceInitialized(userID)
	if err != nil {
		return 0, err
	}
	if !initialized {
		return 0, ErrNamespaceNotInitialized
	}
	return s.saveDefaultFromUser(userID)
}

func (s *Service) saveDefaultFromUser(userID uint) (int, error) {
	count := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var sourceIDs []uint
		if err := tx.Model(&models.UserBookSource{}).
			Where("user_id = ? AND detached = ?", userID, false).
			Order("source_id asc").
			Pluck("source_id", &sourceIDs).Error; err != nil {
			return err
		}
		var oldSourceIDs []uint
		if err := tx.Model(&models.UserBookSource{}).
			Where("user_id = ?", 0).
			Pluck("source_id", &oldSourceIDs).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", 0).Delete(&models.UserBookSource{}).Error; err != nil {
			return err
		}
		if len(sourceIDs) > 0 {
			associations := make([]models.UserBookSource, 0, len(sourceIDs))
			for _, sourceID := range sourceIDs {
				associations = append(associations, models.UserBookSource{UserID: 0, SourceID: sourceID})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&associations).Error; err != nil {
				return err
			}
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.BookSourceNamespace{UserID: 0}).Error; err != nil {
			return err
		}
		for _, sourceID := range oldSourceIDs {
			if err := removeUnreferencedSource(tx, sourceID); err != nil {
				return err
			}
		}
		count = len(sourceIDs)
		return nil
	})
	return count, err
}

func (s *Service) RestoreDefault(userID uint) (ImportResult, error) {
	configured, count, err := s.DefaultStatus()
	if err != nil {
		return ImportResult{}, err
	}
	if !configured {
		return ImportResult{}, ErrNoDefault
	}
	if err := s.EnsureNamespace(userID); err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var restoreErr error
		result, restoreErr = restoreDefaultForUser(tx, userID, count)
		return restoreErr
	})
	return result, err
}

func (s *Service) RestoreDefaultForUsers(userIDs []uint) (RestoreUsersResult, error) {
	userIDs = uniqueSourceUserIDs(userIDs)
	result := RestoreUsersResult{}
	if len(userIDs) == 0 {
		return result, nil
	}
	configured, count, err := s.DefaultStatus()
	if err != nil {
		return result, err
	}
	if !configured {
		return result, ErrNoDefault
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		scoped := New(tx)
		for _, userID := range userIDs {
			if err := scoped.EnsureNamespace(userID); err != nil {
				return err
			}
			userResult, err := restoreDefaultForUser(tx, userID, count)
			if err != nil {
				return err
			}
			result.Reset++
			result.Imported += userResult.Imported
			result.Updated += userResult.Updated
			result.Skipped += userResult.Skipped
		}
		return nil
	})
	return result, err
}

func restoreDefaultForUser(tx *gorm.DB, userID uint, count int) (ImportResult, error) {
	result := ImportResult{}
	if err := tx.Where("user_id = ?", userID).Delete(&models.SourceFailure{}).Error; err != nil {
		return result, err
	}
	defaults := make([]models.BookSource, 0, count)
	if err := activeSourceQuery(tx, 0).
		Order("book_sources.custom_order asc, book_sources.id asc").
		Find(&defaults).Error; err != nil {
		return result, err
	}
	var associations []models.UserBookSource
	if err := tx.Where("user_id = ?", userID).Order("source_id asc").Find(&associations).Error; err != nil {
		return result, err
	}
	sourceIDs := make([]uint, 0, len(associations))
	for _, association := range associations {
		sourceIDs = append(sourceIDs, association.SourceID)
	}
	userSources := make([]models.BookSource, 0, len(sourceIDs))
	if len(sourceIDs) > 0 {
		if err := tx.Where("id IN ?", sourceIDs).Find(&userSources).Error; err != nil {
			return result, err
		}
	}
	sourceByID := make(map[uint]models.BookSource, len(userSources))
	associationByID := make(map[uint]models.UserBookSource, len(associations))
	identityToID := make(map[string]uint, len(userSources))
	for _, source := range userSources {
		sourceByID[source.ID] = source
		if _, exists := identityToID[sourceIdentity(source)]; !exists {
			identityToID[sourceIdentity(source)] = source.ID
		}
	}
	for _, association := range associations {
		associationByID[association.SourceID] = association
	}

	desiredIDs := make(map[uint]bool, len(defaults))
	consumedIDs := make(map[uint]bool, len(defaults))
	for _, defaultSource := range defaults {
		desiredIDs[defaultSource.ID] = true
		if association, ok := associationByID[defaultSource.ID]; ok {
			consumedIDs[defaultSource.ID] = true
			if association.Detached {
				if err := tx.Model(&models.UserBookSource{}).
					Where("user_id = ? AND source_id = ?", userID, defaultSource.ID).
					Update("detached", false).Error; err != nil {
					return result, err
				}
				result.Updated++
			}
			continue
		}

		oldID := identityToID[sourceIdentity(defaultSource)]
		if oldID > 0 && !consumedIDs[oldID] {
			oldSource := sourceByID[oldID]
			if err := tx.Model(&models.Book{}).
				Where("user_id = ? AND source_id = ?", userID, oldID).
				Update("source_id", defaultSource.ID).Error; err != nil {
				return result, err
			}
			if sourceVariableSemanticsChanged(oldSource, defaultSource) {
				if err := clearPersistentVariables(tx, userID, defaultSource.ID); err != nil {
					return result, err
				}
			}
			if err := tx.Where("user_id = ? AND source_id = ?", userID, oldID).
				Delete(&models.UserBookSource{}).Error; err != nil {
				return result, err
			}
			consumedIDs[oldID] = true
			if err := removeUnreferencedSource(tx, oldID); err != nil {
				return result, err
			}
			result.Updated++
		} else {
			result.Imported++
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "source_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"detached":   false,
				"updated_at": time.Now(),
			}),
		}).Create(&models.UserBookSource{UserID: userID, SourceID: defaultSource.ID}).Error; err != nil {
			return result, err
		}
		consumedIDs[defaultSource.ID] = true
	}

	for _, association := range associations {
		if desiredIDs[association.SourceID] || consumedIDs[association.SourceID] {
			continue
		}
		usage, err := sourceUsageCounts(tx, userID, []uint{association.SourceID})
		if err != nil {
			return result, err
		}
		if usage[association.SourceID] > 0 {
			if err := tx.Model(&models.UserBookSource{}).
				Where("user_id = ? AND source_id = ?", userID, association.SourceID).
				Update("detached", true).Error; err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		if err := tx.Delete(&association).Error; err != nil {
			return result, err
		}
		if err := removeUnreferencedSource(tx, association.SourceID); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Service) RemoveUserNamespaces(userIDs []uint) error {
	userIDs = uniqueSourceUserIDs(userIDs)
	if len(userIDs) == 0 {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var sourceIDs []uint
		if err := tx.Model(&models.UserBookSource{}).
			Where("user_id IN ?", userIDs).
			Distinct().
			Pluck("source_id", &sourceIDs).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", userIDs).Delete(&models.UserBookSource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", userIDs).Delete(&models.BookSourceNamespace{}).Error; err != nil {
			return err
		}
		for _, sourceID := range sourceIDs {
			if err := removeUnreferencedSource(tx, sourceID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) batchUpdate(userID uint, sourceIDs []uint, mutate func(*models.BookSource)) (int, error) {
	if err := s.EnsureNamespace(userID); err != nil {
		return 0, err
	}
	affected := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		scoped := New(tx)
		sources, err := scoped.ListActiveByIDs(userID, sourceIDs, false)
		if err != nil {
			return err
		}
		for _, source := range sources {
			mutate(&source)
			if _, err := scoped.Update(userID, source.ID, source); err != nil {
				return err
			}
			affected++
		}
		return nil
	})
	return affected, err
}

func (s *Service) namespaceInitialized(userID uint) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("book source service is unavailable")
	}
	var count int64
	if err := s.db.Model(&models.BookSourceNamespace{}).
		Where("user_id = ?", userID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func uniqueSourceUserIDs(userIDs []uint) []uint {
	unique := make([]uint, 0, len(userIDs))
	seen := make(map[uint]bool, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 || seen[userID] {
			continue
		}
		seen[userID] = true
		unique = append(unique, userID)
	}
	return unique
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

func sourceIdentity(source models.BookSource) string {
	if value := strings.TrimSpace(source.BaseURL); value != "" {
		return "url:" + value
	}
	if value := strings.TrimSpace(source.Name); value != "" {
		return "name:" + value
	}
	return ""
}

func normalizeSource(source models.BookSource) models.BookSource {
	source.ID = 0
	source.Name = strings.TrimSpace(source.Name)
	source.BaseURL = strings.TrimSpace(source.BaseURL)
	source.SearchURL = strings.TrimSpace(source.SearchURL)
	source.BookURLPattern = strings.TrimSpace(source.BookURLPattern)
	source.Comment = strings.TrimSpace(source.Comment)
	source.Charset = strings.TrimSpace(source.Charset)
	source.ConcurrentRate = strings.TrimSpace(source.ConcurrentRate)
	source.Header = strings.TrimSpace(source.Header)
	source.LoginURL = strings.TrimSpace(source.LoginURL)
	source.LoginCheckJS = strings.TrimSpace(source.LoginCheckJS)
	source.Rules = strings.TrimSpace(source.Rules)
	source.Group = strings.TrimSpace(source.Group)
	source.CreatedAt = time.Time{}
	source.UpdatedAt = time.Time{}
	source.UsedBookCount = 0
	if source.Charset == "" {
		source.Charset = "utf-8"
	}
	return source
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
