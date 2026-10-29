package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/umdcover"
)

// umdCover serves a local UMD book's cover through its signed capability.
func (s *Server) umdCover(c *gin.Context) {
	resource, err := s.umdCovers.OpenCover(c.Param("capability"))
	if err != nil {
		status := http.StatusInternalServerError
		message := "failed to load UMD cover"
		switch {
		case errors.Is(err, umdcover.ErrMalformedCapability), errors.Is(err, umdcover.ErrUnsafePath):
			status = http.StatusBadRequest
			message = "invalid UMD cover request"
		case errors.Is(err, umdcover.ErrInvalidCapability), errors.Is(err, umdcover.ErrExpiredCapability):
			status = http.StatusForbidden
			message = "UMD cover authorization failed"
		case errors.Is(err, umdcover.ErrNotFound):
			status = http.StatusNotFound
			message = "UMD cover not found"
		}
		if c.Request.Method == http.MethodHead {
			c.Status(status)
			return
		}
		c.JSON(status, gin.H{"error": message})
		return
	}
	defer resource.File.Close()
	c.Header("Content-Type", resource.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cache-Control", "private, max-age=300")
	http.ServeContent(c.Writer, c.Request, resource.Name, resource.Info.ModTime(), resource.File)
}
