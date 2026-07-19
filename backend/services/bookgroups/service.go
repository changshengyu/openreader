package bookgroups

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openreader/backend/models"
)

var (
	ErrInvalidBuiltIn = errors.New("invalid built-in book group")
	ErrInvalidOrder   = errors.New("invalid book group order")
)

type BuiltInDefinition struct {
	Key         string
	DefaultName string
	SortOrder   int
	GroupID     int
}

var BuiltIns = []BuiltInDefinition{
	{Key: "all", DefaultName: "全部", SortOrder: -10, GroupID: -1},
	{Key: "local", DefaultName: "本地", SortOrder: -9, GroupID: -2},
	{Key: "audio", DefaultName: "音频", SortOrder: -8, GroupID: -3},
	{Key: "ungrouped", DefaultName: "未分组", SortOrder: -7, GroupID: -4},
}

type Row struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Semantic    string `json:"semantic"`
	CategoryID  *uint  `json:"categoryId"`
	Name        string `json:"name"`
	DefaultName string `json:"defaultName"`
	Show        bool   `json:"show"`
	SortOrder   int    `json:"sortOrder"`
	Assignable  bool   `json:"assignable"`
	Deletable   bool   `json:"deletable"`
}

type RestoreRow struct {
	GroupID    int    `json:"groupId"`
	GroupName  string `json:"groupName"`
	Order      int    `json:"order"`
	Show       bool   `json:"show"`
	CategoryID *uint  `json:"categoryId,omitempty"`
	Key        string `json:"key,omitempty"`
}

type BackupRow struct {
	GroupID    int    `json:"groupId"`
	GroupName  string `json:"groupName"`
	Order      int    `json:"order"`
	Show       bool   `json:"show"`
	CategoryID *uint  `json:"categoryId,omitempty"`
	Key        string `json:"key,omitempty"`
}

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) List(userID uint) ([]Row, error) {
	var rows []Row
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		rows, err = list(tx, userID)
		return err
	})
	return rows, err
}

func (s *Service) UpdateBuiltIn(userID uint, key string, name *string, show *bool) (Row, error) {
	key = strings.TrimSpace(key)
	if _, ok := builtInDefinition(key); !ok {
		return Row{}, ErrInvalidBuiltIn
	}
	var updated Row
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureBuiltIns(tx, userID); err != nil {
			return err
		}
		changes := map[string]any{}
		if name != nil {
			normalized := strings.TrimSpace(*name)
			if normalized == "" {
				return ErrInvalidBuiltIn
			}
			changes["name"] = normalized
		}
		if show != nil {
			changes["show"] = *show
		}
		if len(changes) == 0 {
			return ErrInvalidBuiltIn
		}
		if err := tx.Model(&models.BookGroupPreference{}).
			Where("user_id = ? AND `key` = ?", userID, key).
			Updates(changes).Error; err != nil {
			return err
		}
		rows, err := list(tx, userID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.Key == builtInToken(key) {
				updated = row
				return nil
			}
		}
		return ErrInvalidBuiltIn
	})
	return updated, err
}

func (s *Service) Reorder(userID uint, keys []string) ([]Row, error) {
	var reordered []Row
	err := s.db.Transaction(func(tx *gorm.DB) error {
		current, err := list(tx, userID)
		if err != nil {
			return err
		}
		if len(keys) != len(current) {
			return ErrInvalidOrder
		}
		expected := make(map[string]Row, len(current))
		for _, row := range current {
			expected[row.Key] = row
		}
		seen := make(map[string]struct{}, len(keys))
		for index, key := range keys {
			key = strings.TrimSpace(key)
			row, ok := expected[key]
			if !ok {
				return ErrInvalidOrder
			}
			if _, duplicate := seen[key]; duplicate {
				return ErrInvalidOrder
			}
			seen[key] = struct{}{}
			order := (index + 1) * 10
			if row.Kind == "builtin" {
				result := tx.Model(&models.BookGroupPreference{}).
					Where("user_id = ? AND `key` = ?", userID, row.Semantic).
					Update("sort_order", order)
				if result.Error != nil || result.RowsAffected != 1 {
					return ErrInvalidOrder
				}
				continue
			}
			if row.CategoryID == nil {
				return ErrInvalidOrder
			}
			result := tx.Model(&models.Category{}).
				Where("user_id = ? AND id = ?", userID, *row.CategoryID).
				Update("sort_order", order)
			if result.Error != nil || result.RowsAffected != 1 {
				return ErrInvalidOrder
			}
		}
		reordered, err = list(tx, userID)
		return err
	})
	return reordered, err
}

func (s *Service) NextSortOrder(userID uint) (int, error) {
	maxOrder := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureBuiltIns(tx, userID); err != nil {
			return err
		}
		var builtInMax, categoryMax int
		if err := tx.Model(&models.BookGroupPreference{}).
			Where("user_id = ?", userID).
			Select("COALESCE(MAX(sort_order), 0)").
			Scan(&builtInMax).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Category{}).
			Where("user_id = ?", userID).
			Select("COALESCE(MAX(sort_order), 0)").
			Scan(&categoryMax).Error; err != nil {
			return err
		}
		if builtInMax > categoryMax {
			maxOrder = builtInMax
		} else {
			maxOrder = categoryMax
		}
		return nil
	})
	return maxOrder + 10, err
}

func (s *Service) Backup(userID uint) ([]BackupRow, map[uint]int, error) {
	rows, err := s.List(userID)
	if err != nil {
		return nil, nil, err
	}
	result := make([]BackupRow, 0, len(rows))
	maskByCategory := make(map[uint]int)
	customIndex := 0
	for _, row := range rows {
		if row.Kind == "builtin" {
			definition, _ := builtInDefinition(row.Semantic)
			result = append(result, BackupRow{
				GroupID:   definition.GroupID,
				GroupName: row.Name,
				Order:     row.SortOrder,
				Show:      row.Show,
				Key:       row.Key,
			})
			continue
		}
		groupID := 0
		if row.CategoryID != nil && customIndex < 30 {
			groupID = 1 << customIndex
			maskByCategory[*row.CategoryID] = groupID
		}
		customIndex++
		result = append(result, BackupRow{
			GroupID:    groupID,
			GroupName:  row.Name,
			Order:      row.SortOrder,
			Show:       row.Show,
			CategoryID: row.CategoryID,
			Key:        row.Key,
		})
	}
	return result, maskByCategory, nil
}

func (s *Service) Restore(userID uint, rows []RestoreRow) (map[int]uint, int, error) {
	groupCategories := make(map[int]uint)
	count := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureBuiltIns(tx, userID); err != nil {
			return err
		}
		for _, row := range rows {
			name := strings.TrimSpace(row.GroupName)
			if row.GroupID < 0 {
				definition, ok := builtInDefinitionByGroupID(row.GroupID)
				if !ok {
					continue
				}
				if name == "" {
					name = definition.DefaultName
				}
				result := tx.Model(&models.BookGroupPreference{}).
					Where("user_id = ? AND `key` = ?", userID, definition.Key).
					Updates(map[string]any{"name": name, "show": row.Show, "sort_order": row.Order})
				if result.Error != nil {
					return result.Error
				}
				count++
				continue
			}
			if row.GroupID <= 0 || name == "" {
				continue
			}
			var category models.Category
			err := tx.Where("user_id = ? AND name = ?", userID, name).First(&category).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				category = models.Category{UserID: userID, Name: name, Show: row.Show, SortOrder: row.Order}
				if err := tx.Create(&category).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if err := tx.Model(&category).Updates(map[string]any{"show": row.Show, "sort_order": row.Order}).Error; err != nil {
				return err
			}
			groupCategories[row.GroupID] = category.ID
			count++
		}
		return nil
	})
	return groupCategories, count, err
}

func list(tx *gorm.DB, userID uint) ([]Row, error) {
	if err := ensureBuiltIns(tx, userID); err != nil {
		return nil, err
	}
	var preferences []models.BookGroupPreference
	if err := tx.Where("user_id = ?", userID).Find(&preferences).Error; err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(preferences)+4)
	for _, preference := range preferences {
		definition, ok := builtInDefinition(preference.Key)
		if !ok {
			continue
		}
		rows = append(rows, Row{
			Key:         builtInToken(preference.Key),
			Kind:        "builtin",
			Semantic:    preference.Key,
			Name:        preference.Name,
			DefaultName: definition.DefaultName,
			Show:        preference.Show,
			SortOrder:   preference.SortOrder,
			Assignable:  false,
			Deletable:   false,
		})
	}
	var categories []models.Category
	if err := tx.Where("user_id = ?", userID).Find(&categories).Error; err != nil {
		return nil, err
	}
	for index := range categories {
		category := categories[index]
		categoryID := category.ID
		rows = append(rows, Row{
			Key:        categoryToken(category.ID),
			Kind:       "category",
			Semantic:   "category",
			CategoryID: &categoryID,
			Name:       category.Name,
			Show:       category.Show,
			SortOrder:  category.SortOrder,
			Assignable: true,
			Deletable:  true,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SortOrder != rows[j].SortOrder {
			return rows[i].SortOrder < rows[j].SortOrder
		}
		return rows[i].Key < rows[j].Key
	})
	return rows, nil
}

func ensureBuiltIns(tx *gorm.DB, userID uint) error {
	now := time.Now()
	rows := make([]models.BookGroupPreference, 0, len(BuiltIns))
	for _, definition := range BuiltIns {
		rows = append(rows, models.BookGroupPreference{
			UserID: userID, Key: definition.Key, Name: definition.DefaultName,
			Show: true, SortOrder: definition.SortOrder, CreatedAt: now, UpdatedAt: now,
		})
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "key"}},
		DoNothing: true,
	}).Create(&rows).Error
}

func builtInDefinition(key string) (BuiltInDefinition, bool) {
	for _, definition := range BuiltIns {
		if definition.Key == key {
			return definition, true
		}
	}
	return BuiltInDefinition{}, false
}

func builtInDefinitionByGroupID(groupID int) (BuiltInDefinition, bool) {
	for _, definition := range BuiltIns {
		if definition.GroupID == groupID {
			return definition, true
		}
	}
	return BuiltInDefinition{}, false
}

func builtInToken(key string) string {
	return "builtin:" + key
}

func categoryToken(id uint) string {
	return "category:" + strconv.FormatUint(uint64(id), 10)
}

func CategoryIDFromToken(token string) (uint, error) {
	if !strings.HasPrefix(token, "category:") {
		return 0, fmt.Errorf("%w: category token", ErrInvalidOrder)
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(token, "category:"), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("%w: category token", ErrInvalidOrder)
	}
	return uint(id), nil
}
