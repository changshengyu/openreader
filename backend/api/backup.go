package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/backup"
	"openreader/backend/services/webdavfs"
)

func (s *Server) triggerBackup(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	user, ok := storeUser(c)
	if !ok {
		unauthorized(c, "store user missing")
		return
	}
	var (
		path string
		err  error
	)
	ctx := c.Request.Context()
	if user.Role == "admin" {
		path, err = s.backupSvc.RunNowForUserAtRootContext(ctx, user.ID)
	} else {
		path, err = s.backupSvc.RunNowForUserContext(ctx, user.ID, user.Username)
	}
	if err != nil {
		if requestCanceled(err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "backup failed"})
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	name := filepath.Base(path)
	c.JSON(http.StatusOK, gin.H{"message": "backup created", "path": name, "name": name})
}

func (s *Server) triggerPortableBackup(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	user, ok := storeUser(c)
	if !ok {
		unauthorized(c, "store user missing")
		return
	}
	backupDir, ok := s.backupDir(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	result, err := s.backupSvc.RunPortableV2ForUserContext(ctx, user.ID, user.Username, backupDir)
	if err != nil {
		switch {
		case requestCanceled(err):
			return
		case errors.Is(err, backup.ErrPortableArchiveUnavailable):
			c.JSON(http.StatusConflict, gin.H{"error": "local archive unavailable for portable backup"})
		case errors.Is(err, backup.ErrPortableAssetUnavailable):
			c.JSON(http.StatusConflict, gin.H{"error": "custom asset unavailable for portable backup"})
		case errors.Is(err, backup.ErrPortableBackupUnavailable):
			c.JSON(http.StatusConflict, gin.H{"error": "portable backup storage is unavailable"})
		case errors.Is(err, backup.ErrPortableBackupLimit):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "portable backup exceeds safety limits"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "portable backup failed"})
		}
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	name := filepath.Base(result.Path)
	c.JSON(http.StatusOK, gin.H{
		"message":      "portable backup created",
		"path":         name,
		"name":         name,
		"format":       "openreader-portable-v2",
		"localBooks":   result.LocalBooks,
		"assets":       result.Assets,
		"legacyAssets": result.LegacyAssets,
	})
}

func requestCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (s *Server) listBackups(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	service, err := s.backupFileService(c)
	if err != nil {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}
	root, err := service.Stat("")
	if err != nil || !root.Info.IsDir() {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}
	entries, err := os.ReadDir(service.Root())
	if err != nil {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	backups := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !backupFileNameAllowed(name) {
			continue
		}
		file, info, err := service.Open(name)
		if err != nil {
			continue
		}
		format := "logical"
		if strings.HasPrefix(name, "portable_backup_") {
			format = portableBackupFormat(file, info.Size())
		}
		_ = file.Close()
		backups = append(backups, gin.H{
			"name":   name,
			"size":   info.Size(),
			"time":   info.ModTime(),
			"format": format,
		})
	}
	c.JSON(http.StatusOK, backups)
}

func (s *Server) backupFileService(c *gin.Context) (*webdavfs.Service, error) {
	root, ok := s.backupDir(c)
	if !ok {
		return nil, webdavfs.ErrUnsafePath
	}
	return webdavfs.NewScoped(s.webdavDir(), root)
}

func (s *Server) downloadBackup(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	name := c.Param("name")
	if !backupFileNameAllowed(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup name"})
		return
	}
	service, err := s.backupFileService(c)
	if err != nil {
		writeBackupOpenError(c, err)
		return
	}
	file, info, err := service.Open(name)
	if err != nil {
		writeBackupOpenError(c, err)
		return
	}
	defer file.Close()
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), file)
}

func backupFileNameAllowed(name string) bool {
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) || !strings.HasSuffix(strings.ToLower(name), ".zip") {
		return false
	}
	return strings.HasPrefix(name, "backup_") || strings.HasPrefix(name, "portable_backup_")
}

func writeBackupOpenError(c *gin.Context, err error) {
	if errors.Is(err, webdavfs.ErrNotFound) || errors.Is(err, webdavfs.ErrUnsafePath) ||
		errors.Is(err, webdavfs.ErrIsDirectory) || errors.Is(err, webdavfs.ErrNotDirectory) {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open backup"})
}

func (s *Server) backupDir(c *gin.Context) (string, bool) {
	return s.storeRoot(c, filepath.Join(s.cfg.DataDir, "webdav"))
}
