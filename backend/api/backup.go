package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) triggerBackup(c *gin.Context) {
	path, err := s.backupSvc.RunNow()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "backup failed: " + err.Error()})
		return
	}
	name := filepath.Base(path)
	c.JSON(http.StatusOK, gin.H{"message": "backup created", "path": name, "name": name})
}

func (s *Server) listBackups(c *gin.Context) {
	webdavDir := filepath.Join(s.cfg.DataDir, "webdav")
	entries, err := os.ReadDir(webdavDir)
	if err != nil {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	var backups []gin.H
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "backup_") {
			continue
		}
		info, _ := entry.Info()
		backups = append(backups, gin.H{
			"name": entry.Name(),
			"size": info.Size(),
			"time": info.ModTime(),
		})
	}
	c.JSON(http.StatusOK, backups)
}

func (s *Server) downloadBackup(c *gin.Context) {
	name := filepath.Base(c.Param("name"))
	if !strings.HasPrefix(name, "backup_") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup name"})
		return
	}
	path := filepath.Join(s.cfg.DataDir, "webdav", name)
	c.File(path)
}
