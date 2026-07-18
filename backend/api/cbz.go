package api

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/cbzreader"
)

func (s *Server) cbzResource(c *gin.Context) {
	resource, err := s.cbzReader.OpenResource(
		c.Param("capability"),
		c.Param("resourcePath"),
	)
	if err != nil {
		writeCBZServiceError(c, err, "failed to load CBZ resource")
		return
	}

	c.Header("Content-Type", resource.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cache-Control", "private, max-age=300")
	http.ServeFile(c.Writer, c.Request, filepath.Clean(resource.Path))
}

func writeCBZServiceError(c *gin.Context, err error, fallback string) {
	status := http.StatusInternalServerError
	message := fallback
	switch {
	case errors.Is(err, cbzreader.ErrMalformedCapability),
		errors.Is(err, cbzreader.ErrUnsafePath):
		status = http.StatusBadRequest
		message = "invalid CBZ resource request"
	case errors.Is(err, cbzreader.ErrInvalidCapability),
		errors.Is(err, cbzreader.ErrExpiredCapability):
		status = http.StatusForbidden
		message = "CBZ resource authorization failed"
	case errors.Is(err, cbzreader.ErrNotFound):
		status = http.StatusNotFound
		message = "CBZ resource not found"
	case errors.Is(err, cbzreader.ErrUnsupportedMedia):
		status = http.StatusUnsupportedMediaType
		message = "unsupported CBZ resource type"
	case errors.Is(err, cbzreader.ErrInvalidArchive),
		errors.Is(err, cbzreader.ErrExtractionLimit),
		errors.Is(err, cbzreader.ErrNotCBZ):
		status = http.StatusUnprocessableEntity
		message = "CBZ archive cannot be opened safely"
	}
	c.JSON(status, gin.H{"error": message})
}
