package api

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

func (s *Server) requireAdmin(c *gin.Context) bool {
	userID, ok := middleware.UserID(c)
	if !ok {
		unauthorized(c, "login required")
		return false
	}
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil || user.Role != "admin" {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "admin access required"))
		return false
	}
	return true
}

func (s *Server) listUsers(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	var users []models.User
	if err := s.db.Order("created_at desc").Find(&users).Error; err != nil {
		internalError(c, "failed to list users")
		return
	}

	type userSummary struct {
		ID              uint      `json:"id"`
		Username        string    `json:"username"`
		Role            string    `json:"role"`
		BookLimit       int       `json:"bookLimit"`
		SourceLimit     int       `json:"sourceLimit"`
		CanEditSources  bool      `json:"canEditSources"`
		CanAccessStore  bool      `json:"canAccessStore"`
		CanAccessWebDAV bool      `json:"canAccessWebdav"`
		BookCount       int64     `json:"bookCount"`
		SourceCount     int64     `json:"sourceCount"`
		LastActiveAt    time.Time `json:"lastActiveAt"`
		CreatedAt       time.Time `json:"createdAt"`
	}

	var sourceCount int64
	_ = s.db.Model(&models.BookSource{}).Count(&sourceCount).Error

	results := make([]userSummary, 0, len(users))
	for _, u := range users {
		var bookCount int64
		_ = s.db.Model(&models.Book{}).Where("user_id = ?", u.ID).Count(&bookCount).Error
		results = append(results, userSummary{
			ID:              u.ID,
			Username:        u.Username,
			Role:            u.Role,
			BookLimit:       u.BookLimit,
			SourceLimit:     u.SourceLimit,
			CanEditSources:  u.CanEditSources,
			CanAccessStore:  u.CanAccessStore,
			CanAccessWebDAV: effectiveWebDAVAccess(u),
			BookCount:       bookCount,
			SourceCount:     sourceCount,
			LastActiveAt:    u.LastActiveAt,
			CreatedAt:       u.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, results)
}

type createAdminUserRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	Role            string `json:"role"`
	BookLimit       int    `json:"bookLimit"`
	SourceLimit     int    `json:"sourceLimit"`
	CanEditSources  *bool  `json:"canEditSources"`
	CanAccessStore  *bool  `json:"canAccessStore"`
	CanAccessWebDAV *bool  `json:"canAccessWebdav"`
}

func (s *Server) createUser(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	var req createAdminUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}

	username, validationError := validateNewAccountCredentials(req.Username, req.Password)
	if validationError != "" {
		badRequest(c, validationError)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(c, "failed to hash password")
		return
	}

	role := strings.TrimSpace(req.Role)
	if role != "" && role != "user" {
		badRequest(c, "administrator role assignment is not allowed")
		return
	}

	user := models.User{
		Username:        username,
		PasswordHash:    string(hash),
		Role:            "user",
		BookLimit:       req.BookLimit,
		SourceLimit:     req.SourceLimit,
		CanEditSources:  true,
		CanAccessStore:  true,
		CanAccessWebDAV: boolValue(true),
		LastActiveAt:    time.Now(),
	}
	if req.CanEditSources != nil {
		user.CanEditSources = *req.CanEditSources
	}
	if req.CanAccessStore != nil {
		user.CanAccessStore = *req.CanAccessStore
	}
	if req.CanAccessWebDAV != nil {
		user.CanAccessWebDAV = boolValue(*req.CanAccessWebDAV)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if req.CanEditSources != nil {
			updates["can_edit_sources"] = *req.CanEditSources
		}
		if req.CanAccessStore != nil {
			updates["can_access_store"] = *req.CanAccessStore
		}
		if req.CanAccessWebDAV != nil {
			updates["can_access_webdav"] = *req.CanAccessWebDAV
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&user).Updates(updates).Error
	}); err != nil {
		c.JSON(http.StatusConflict, errResp("CONFLICT", "username already exists"))
		return
	}
	s.broadcastUsersUpdate("create", []uint{user.ID})
	c.JSON(http.StatusCreated, user)
}

type updateUserRequest struct {
	BookLimit       *int  `json:"bookLimit"`
	SourceLimit     *int  `json:"sourceLimit"`
	CanEditSources  *bool `json:"canEditSources"`
	CanAccessStore  *bool `json:"canAccessStore"`
	CanAccessWebDAV *bool `json:"canAccessWebdav"`
}

func (s *Server) updateUser(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		notFound(c, "user not found")
		return
	}
	if user.Role == "admin" {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "protected administrator cannot be modified"))
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}

	if req.BookLimit != nil {
		user.BookLimit = *req.BookLimit
	}
	if req.SourceLimit != nil {
		user.SourceLimit = *req.SourceLimit
	}
	if req.CanEditSources != nil {
		user.CanEditSources = *req.CanEditSources
	}
	if req.CanAccessStore != nil {
		user.CanAccessStore = *req.CanAccessStore
	}
	if req.CanAccessWebDAV != nil {
		user.CanAccessWebDAV = boolValue(*req.CanAccessWebDAV)
	}

	if err := s.db.Save(&user).Error; err != nil {
		internalError(c, "failed to update user")
		return
	}
	s.broadcastUsersUpdate("update", []uint{user.ID})
	c.JSON(http.StatusOK, user)
}

type resetUserPasswordRequest struct {
	Password string `json:"password"`
}

func (s *Server) resetUserPassword(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var req resetUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}
	if validationError := validateResetPassword(req.Password); validationError != "" {
		badRequest(c, validationError)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(c, "failed to hash password")
		return
	}

	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		notFound(c, "user not found")
		return
	}
	if user.Role == "admin" {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "protected administrator password cannot be reset"))
		return
	}
	if err := s.db.Model(&user).Update("password_hash", string(hash)).Error; err != nil {
		internalError(c, "failed to reset password")
		return
	}
	s.broadcastUsersUpdate("password", []uint{id})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type deleteUsersRequest struct {
	IDs []uint `json:"ids"`
}

var errNoDeletableUsers = errors.New("no deletable users selected")

type userWorkspaceCleanupPlan struct {
	user  models.User
	paths []string
}

var removeUserWorkspace = os.RemoveAll

// privateUserWorkspacePath can only return a descendant below an internal
// configured root. It is intentionally built from persisted user identity,
// never from an HTTP path or a client-provided username.
func privateUserWorkspacePath(root string, parts ...string) (string, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(append([]string{base}, parts...)...))
	if err != nil {
		return "", err
	}
	if target == base || !strings.HasPrefix(target, base+string(os.PathSeparator)) {
		return "", os.ErrPermission
	}
	return target, nil
}

func (s *Server) userWorkspaceCleanupPlans(users []models.User) ([]userWorkspaceCleanupPlan, error) {
	plans := make([]userWorkspaceCleanupPlan, 0, len(users))
	for _, user := range users {
		username := engine.SafeFilename(user.Username)
		paths := make([]string, 0, 4)
		for _, rootAndParts := range []struct {
			root  string
			parts []string
		}{
			{root: filepath.Join(s.cfg.DataDir, "webdav", "users"), parts: []string{username}},
			{root: filepath.Join(s.cfg.LocalStoreDir, "users"), parts: []string{username}},
			{root: filepath.Join(s.cfg.LibraryDir, "data"), parts: []string{username}},
			{root: filepath.Join(s.cfg.DataDir, "uploads", "users"), parts: []string{strconv.FormatUint(uint64(user.ID), 10)}},
		} {
			path, err := privateUserWorkspacePath(rootAndParts.root, rootAndParts.parts...)
			if err != nil {
				return nil, err
			}
			paths = append(paths, path)
		}
		plans = append(plans, userWorkspaceCleanupPlan{user: user, paths: paths})
	}
	return plans, nil
}

// deleteUserData atomically removes every SQLite row owned by the requested
// ordinary users. Files are intentionally a post-commit cleanup: rollback can
// protect database data, but it cannot safely undo a removed mounted file.
func (s *Server) deleteUserData(ids []uint, protectedUserID uint) ([]models.User, []userWorkspaceCleanupPlan, error) {
	var deletedUsers []models.User
	var plans []userWorkspaceCleanupPlan
	err := s.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id IN ? AND role <> ?", ids, "admin")
		if protectedUserID != 0 {
			query = query.Where("id <> ?", protectedUserID)
		}
		if err := query.Find(&deletedUsers).Error; err != nil {
			return err
		}
		if len(deletedUsers) == 0 {
			return errNoDeletableUsers
		}
		var err error
		plans, err = s.userWorkspaceCleanupPlans(deletedUsers)
		if err != nil {
			return err
		}
		deletedIDs := make([]uint, 0, len(deletedUsers))
		for _, user := range deletedUsers {
			deletedIDs = append(deletedIDs, user.ID)
		}

		var bookIDs []uint
		if err := tx.Model(&models.Book{}).Where("user_id IN ?", deletedIDs).Pluck("id", &bookIDs).Error; err != nil {
			return err
		}
		if len(bookIDs) > 0 {
			if err := tx.Where("book_id IN ?", bookIDs).Delete(&models.Chapter{}).Error; err != nil {
				return err
			}
		}
		for _, deletion := range []struct {
			model any
			where string
			args  []any
		}{
			{model: &models.BookCategory{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.Bookmark{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.ReadingProgress{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.Book{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.Category{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.BookGroupPreference{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.RSSArticle{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.RSSSource{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.ReplaceRule{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.UserSetting{}, where: "user_id IN ?", args: []any{deletedIDs}},
			{model: &models.SourceFailure{}, where: "user_id IN ?", args: []any{deletedIDs}},
		} {
			if err := tx.Where(deletion.where, deletion.args...).Delete(deletion.model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id IN ?", deletedIDs).Delete(&models.User{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return deletedUsers, plans, nil
}

func cleanupUserWorkspaces(plans []userWorkspaceCleanupPlan) int {
	failed := 0
	for _, plan := range plans {
		for _, path := range plan.paths {
			if err := removeUserWorkspace(path); err != nil {
				// Do not place an internal path or a filesystem error (which often
				// includes that path) in the log or API response. The durable database
				// deletion is already complete and this cleanup is safe to retry only
				// through a future, explicitly scoped maintenance operation.
				log.Printf("openreader: post-commit private workspace cleanup failed for deleted user id %d", plan.user.ID)
				failed++
			}
		}
	}
	return failed
}

func (s *Server) deleteUsers(c *gin.Context) {
	currentUserID, ok := middleware.UserID(c)
	if !ok {
		unauthorized(c, "login required")
		return
	}
	if !s.requireAdmin(c) {
		return
	}

	var req deleteUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}

	ids := make([]uint, 0, len(req.IDs))
	seen := map[uint]bool{}
	for _, id := range req.IDs {
		if id == 0 || id == currentUserID || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		badRequest(c, "no deletable users selected")
		return
	}
	deletedUsers, plans, err := s.deleteUserData(ids, currentUserID)
	if errors.Is(err, errNoDeletableUsers) {
		badRequest(c, err.Error())
		return
	}
	if err != nil {
		internalError(c, "failed to delete users")
		return
	}
	cleanupFailures := cleanupUserWorkspaces(plans)
	deletedIDs := make([]uint, 0, len(deletedUsers))
	for _, user := range deletedUsers {
		deletedIDs = append(deletedIDs, user.ID)
	}
	s.broadcastUsersUpdate("delete", deletedIDs)
	c.JSON(http.StatusOK, gin.H{"deleted": len(deletedUsers), "cleanupFailures": cleanupFailures})
}

func (s *Server) cleanupInactiveUsers(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	var ids []uint
	if err := s.db.Model(&models.User{}).Where("role <> ? AND last_active_at < ?", "admin", cutoff).Pluck("id", &ids).Error; err != nil {
		internalError(c, "cleanup failed")
		return
	}
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"deleted": 0})
		return
	}
	deletedUsers, plans, err := s.deleteUserData(ids, 0)
	if err != nil {
		internalError(c, "cleanup failed")
		return
	}
	cleanupFailures := cleanupUserWorkspaces(plans)
	deletedIDs := make([]uint, 0, len(deletedUsers))
	for _, user := range deletedUsers {
		deletedIDs = append(deletedIDs, user.ID)
	}
	s.broadcastUsersUpdate("cleanup", deletedIDs)
	c.JSON(http.StatusOK, gin.H{"deleted": len(deletedUsers), "cleanupFailures": cleanupFailures})
}

func (s *Server) broadcastUsersUpdate(kind string, userIDs []uint) {
	if s.hub == nil {
		return
	}
	_ = s.hub.BroadcastAll(nil, gin.H{
		"type": "users_update",
		"payload": gin.H{
			"kind":    kind,
			"userIds": userIDs,
		},
	})
}
