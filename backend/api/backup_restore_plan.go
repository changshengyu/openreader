package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/models"
	"openreader/backend/services/bookgroups"
)

type plannedBackupArtifact struct {
	data     []byte
	score    int
	upstream bool
}

type logicalBackupRestorePlan struct {
	sources          plannedBackupArtifact
	rssSources       plannedBackupArtifact
	settings         plannedBackupArtifact
	categories       plannedBackupArtifact
	bookGroups       plannedBackupArtifact
	bookshelf        plannedBackupArtifact
	chapterVariables plannedBackupArtifact
	bookmarks        plannedBackupArtifact
	replaceRules     plannedBackupArtifact
	progress         []plannedBackupArtifact
}

func (s *Server) restoreLegadoBackupDataWithPermissions(data []byte, userID uint, canEditSources bool, broadcast bool) (gin.H, error) {
	archive, err := newBackupRestoreArchive(data, s.backupRestoreLimits())
	if err != nil {
		return nil, err
	}
	plan, err := buildLogicalBackupRestorePlan(archive)
	if err != nil {
		return nil, err
	}
	if err := validateLogicalBackupRestorePlan(plan); err != nil {
		return nil, errInvalidBackupArchive
	}

	var result gin.H
	err = s.db.Transaction(func(tx *gorm.DB) error {
		worker := *s
		worker.db = tx
		worker.bookGroups = bookgroups.New(tx)
		var executeErr error
		result, executeErr = worker.executeLogicalBackupRestorePlan(plan, userID, canEditSources)
		if executeErr != nil {
			if errors.Is(executeErr, errInvalidBackupArchive) {
				return executeErr
			}
			return fmt.Errorf("%w: %v", errBackupRestorePersistence, executeErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if broadcast {
		s.broadcastRestoreUpdates(userID, result)
	}
	return result, nil
}

func buildLogicalBackupRestorePlan(archive *backupRestoreArchive) (logicalBackupRestorePlan, error) {
	plan := logicalBackupRestorePlan{}
	for _, entry := range archive.entries {
		entryData, err := archive.dataFor(entry.file)
		if err != nil {
			return logicalBackupRestorePlan{}, err
		}
		name := strings.ToLower(entry.name)
		base := path.Base(name)
		depthScore := strings.Count(name, "/")
		switch base {
		case "booksource.json":
			selectPlannedArtifact(&plan.sources, entryData, depthScore, false)
		case "rsssources.json":
			selectPlannedArtifact(&plan.rssSources, entryData, depthScore, false)
		case "usersettings.json":
			selectPlannedArtifact(&plan.settings, entryData, depthScore, false)
		case "categories.json":
			selectPlannedArtifact(&plan.categories, entryData, depthScore, false)
		case "bookgroup.json":
			selectPlannedArtifact(&plan.bookGroups, entryData, depthScore, false)
		case "bookshelf.json":
			selectPlannedArtifact(&plan.bookshelf, entryData, depthScore, false)
		case "mybookshelf.json":
			selectPlannedArtifact(&plan.bookshelf, entryData, 100+depthScore, true)
		case "chaptervariables.json":
			selectPlannedArtifact(&plan.chapterVariables, entryData, depthScore, false)
		case "bookmarks.json":
			selectPlannedArtifact(&plan.bookmarks, entryData, depthScore, false)
		case "bookmark.json":
			selectPlannedArtifact(&plan.bookmarks, entryData, 100+depthScore, true)
		case "replacerules.json":
			selectPlannedArtifact(&plan.replaceRules, entryData, depthScore, false)
		case "replacerule.json":
			selectPlannedArtifact(&plan.replaceRules, entryData, 100+depthScore, true)
		case "readingprogress.json":
			plan.progress = append(plan.progress, plannedBackupArtifact{data: entryData, score: depthScore})
		default:
			if strings.HasPrefix(name, "bookprogress/") || strings.Contains(name, "/bookprogress/") {
				plan.progress = append(plan.progress, plannedBackupArtifact{data: entryData, score: depthScore, upstream: true})
			}
		}
	}
	sort.SliceStable(plan.progress, func(i, j int) bool { return plan.progress[i].score < plan.progress[j].score })
	return plan, nil
}

func selectPlannedArtifact(target *plannedBackupArtifact, data []byte, score int, upstream bool) {
	if len(target.data) > 0 && target.score <= score {
		return
	}
	*target = plannedBackupArtifact{data: data, score: score, upstream: upstream}
}

func validateLogicalBackupRestorePlan(plan logicalBackupRestorePlan) error {
	if len(plan.sources.data) > 0 {
		if _, err := decodeBookSources(plan.sources.data); err != nil {
			return err
		}
	}
	for _, artifact := range []plannedBackupArtifact{plan.settings, plan.categories, plan.bookGroups, plan.bookshelf, plan.chapterVariables, plan.bookmarks, plan.replaceRules} {
		if len(artifact.data) > 0 && !isJSONArray(artifact.data) {
			return errInvalidBackupArchive
		}
	}
	if len(plan.rssSources.data) > 0 {
		if _, err := decodeRestoredRSSSources(plan.rssSources.data); err != nil {
			return err
		}
	}
	if len(plan.settings.data) > 0 {
		var rows []models.UserSetting
		if err := json.Unmarshal(plan.settings.data, &rows); err != nil {
			return err
		}
	}
	if len(plan.categories.data) > 0 {
		var rows []models.Category
		if err := json.Unmarshal(plan.categories.data, &rows); err != nil {
			return err
		}
	}
	if len(plan.bookshelf.data) > 0 {
		if err := validateRestoredBookshelfVariables(plan.bookshelf.data); err != nil {
			return err
		}
	}
	if len(plan.chapterVariables.data) > 0 {
		if err := validateRestoredChapterVariables(plan.chapterVariables.data); err != nil {
			return err
		}
	}
	if len(plan.bookmarks.data) > 0 {
		var rows []restoredBookmarkRow
		if err := json.Unmarshal(plan.bookmarks.data, &rows); err != nil {
			return err
		}
	}
	if len(plan.replaceRules.data) > 0 {
		var rows []restoredReplaceRuleRow
		if err := json.Unmarshal(plan.replaceRules.data, &rows); err != nil {
			return err
		}
	}
	if len(plan.bookGroups.data) > 0 {
		var rows []bookgroups.RestoreRow
		if err := json.Unmarshal(plan.bookGroups.data, &rows); err != nil {
			return err
		}
	}
	for _, progress := range plan.progress {
		if _, err := decodeRestoredProgressPayloads(progress.data); err != nil {
			return err
		}
	}
	return nil
}

func isJSONArray(data []byte) bool {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return false
	}
	_, ok := value.([]any)
	return ok
}

func (s *Server) executeLogicalBackupRestorePlan(plan logicalBackupRestorePlan, userID uint, canEditSources bool) (gin.H, error) {
	result := gin.H{
		"sources": 0, "rssSources": 0, "books": 0, "bookGroups": 0,
		"chapterVariables": 0, "progress": 0, "settings": 0, "categories": 0,
		"bookmarks": 0, "replaceRules": 0,
	}
	groupCategoryMap := make(map[int]uint)

	if len(plan.sources.data) > 0 {
		if canEditSources {
			n, err := s.restoreSourcesFromDataStrict(plan.sources.data)
			if err != nil {
				return nil, err
			}
			result["sources"] = n
		} else {
			result["sourcesSkipped"] = true
		}
	}
	if len(plan.rssSources.data) > 0 {
		n, err := s.restoreRSSSourcesFromData(plan.rssSources.data, userID)
		if err != nil {
			return nil, err
		}
		result["rssSources"] = n
	}
	if len(plan.settings.data) > 0 {
		n, err := s.restoreUserSettingsFromData(plan.settings.data, userID)
		if err != nil {
			return nil, err
		}
		result["settings"] = n
	}
	if len(plan.categories.data) > 0 {
		n, err := s.restoreCategoriesFromData(plan.categories.data, userID)
		if err != nil {
			return nil, err
		}
		result["categories"] = n
	}
	if len(plan.bookGroups.data) > 0 {
		mapping, n, err := s.restoreBookGroupsFromData(plan.bookGroups.data, userID)
		if err != nil {
			return nil, err
		}
		groupCategoryMap = mapping
		result["bookGroups"] = n
	}
	if len(plan.bookshelf.data) > 0 {
		books, progress, err := s.restoreBookshelfFromDataWithGroupMap(plan.bookshelf.data, userID, groupCategoryMap)
		if err != nil {
			return nil, err
		}
		result["books"] = books
		result["progress"] = progress
	}
	if len(plan.chapterVariables.data) > 0 {
		n, err := s.restoreChapterVariablesFromData(plan.chapterVariables.data, userID)
		if err != nil {
			return nil, err
		}
		result["chapterVariables"] = n
	}
	if len(plan.bookmarks.data) > 0 {
		n, err := s.restoreBookmarksFromDataWithFormat(plan.bookmarks.data, userID, plan.bookmarks.upstream)
		if err != nil {
			return nil, err
		}
		result["bookmarks"] = n
	}
	if len(plan.replaceRules.data) > 0 {
		n, err := s.restoreReplaceRulesFromData(plan.replaceRules.data, userID)
		if err != nil {
			return nil, err
		}
		result["replaceRules"] = n
	}
	progressCount := result["progress"].(int)
	for _, artifact := range plan.progress {
		n, err := s.restoreProgressFromData(artifact.data, userID)
		if err != nil {
			return nil, err
		}
		progressCount += n
	}
	result["progress"] = progressCount
	return result, nil
}

func (s *Server) restoreSourcesFromDataStrict(data []byte) (int, error) {
	sources, err := decodeBookSources(data)
	if err != nil {
		return 0, err
	}
	result, err := importBookSourcesStrictWithDB(s.db, sources)
	if err != nil {
		return 0, err
	}
	return result["imported"].(int) + result["updated"].(int), nil
}
