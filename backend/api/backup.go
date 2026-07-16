package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/backup"
)

func (s *Server) triggerBackup(c *gin.Context) {
	if !s.requireStoreAccess(c) {
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
	if user.Role == "admin" {
		path, err = s.backupSvc.RunNow()
	} else {
		path, err = s.backupSvc.RunNowForUser(user.ID, user.Username)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "backup failed: " + err.Error()})
		return
	}
	name := filepath.Base(path)
	c.JSON(http.StatusOK, gin.H{"message": "backup created", "path": name, "name": name})
}

func (s *Server) triggerPortableBackup(c *gin.Context) {
	if !s.requireStoreAccess(c) {
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
	path, localBooks, err := s.backupSvc.RunPortableForUser(user.ID, user.Username, backupDir)
	if err != nil {
		switch {
		case errors.Is(err, backup.ErrPortableArchiveUnavailable):
			c.JSON(http.StatusConflict, gin.H{"error": "local archive unavailable for portable backup"})
		case errors.Is(err, backup.ErrPortableBackupUnavailable):
			c.JSON(http.StatusConflict, gin.H{"error": "portable backup storage is unavailable"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "portable backup failed"})
		}
		return
	}
	name := filepath.Base(path)
	c.JSON(http.StatusOK, gin.H{
		"message":    "portable backup created",
		"path":       name,
		"name":       name,
		"format":     "openreader-portable-v1",
		"localBooks": localBooks,
	})
}

func (s *Server) listBackups(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	webdavDir, ok := s.backupDir(c)
	if !ok {
		return
	}
	entries, err := os.ReadDir(webdavDir)
	if err != nil {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	var backups []gin.H
	for _, entry := range entries {
		if entry.IsDir() || !backupFileNameAllowed(entry.Name()) {
			continue
		}
		info, _ := entry.Info()
		format := "logical"
		if strings.HasPrefix(entry.Name(), "portable_backup_") {
			format = "openreader-portable-v1"
		}
		backups = append(backups, gin.H{
			"name":   entry.Name(),
			"size":   info.Size(),
			"time":   info.ModTime(),
			"format": format,
		})
	}
	c.JSON(http.StatusOK, backups)
}

func (s *Server) downloadBackup(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	name := filepath.Base(c.Param("name"))
	if !backupFileNameAllowed(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup name"})
		return
	}
	backupDir, ok := s.backupDir(c)
	if !ok {
		return
	}
	path := filepath.Join(backupDir, name)
	c.File(path)
}

func backupFileNameAllowed(name string) bool {
	return strings.HasPrefix(name, "backup_") || strings.HasPrefix(name, "portable_backup_")
}

func (s *Server) backupDir(c *gin.Context) (string, bool) {
	return s.storeRoot(c, filepath.Join(s.cfg.DataDir, "webdav"))
}
