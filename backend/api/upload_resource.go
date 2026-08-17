package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/webdavfs"
)

func (s *Server) uploadPublicResource(c *gin.Context) {
	rawPath := c.Param("resourcePath")
	if !strings.HasPrefix(rawPath, "/") {
		c.Status(http.StatusNotFound)
		return
	}
	relativePath := strings.TrimPrefix(rawPath, "/")
	invalidPath := relativePath == "" ||
		strings.HasPrefix(relativePath, "/") ||
		strings.Contains(relativePath, `\`) ||
		strings.ContainsRune(relativePath, '\x00')
	if invalidPath {
		c.Status(http.StatusNotFound)
		return
	}

	uploadsRoot := filepath.Join(s.cfg.DataDir, "uploads")
	service, err := webdavfs.New(uploadsRoot)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	file, info, err := service.Open(relativePath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()

	http.ServeContent(c.Writer, c.Request, filepath.Base(relativePath), info.ModTime(), file)
}
