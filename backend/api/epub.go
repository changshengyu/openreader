package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/epubreader"
)

func (s *Server) epubResource(c *gin.Context) {
	resource, err := s.epubReader.OpenResource(
		c.Param("capability"),
		c.Param("resourcePath"),
	)
	if err != nil {
		writeEPUBServiceError(c, err, "failed to load EPUB resource")
		return
	}

	c.Header("Content-Type", resource.ContentType)
	c.Header("Content-Security-Policy", resource.CSP)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cache-Control", "private, max-age=300")
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	if resource.Document {
		c.Data(http.StatusOK, resource.ContentType, resource.Data)
		return
	}
	http.ServeFile(c.Writer, c.Request, resource.Path)
}

func writeEPUBServiceError(c *gin.Context, err error, fallback string) {
	status := http.StatusInternalServerError
	message := fallback
	switch {
	case errors.Is(err, epubreader.ErrMalformedCapability),
		errors.Is(err, epubreader.ErrUnsafePath):
		status = http.StatusBadRequest
		message = "invalid EPUB resource request"
	case errors.Is(err, epubreader.ErrInvalidCapability),
		errors.Is(err, epubreader.ErrExpiredCapability):
		status = http.StatusForbidden
		message = "EPUB resource authorization failed"
	case errors.Is(err, epubreader.ErrNotFound):
		status = http.StatusNotFound
		message = "EPUB resource not found"
	case errors.Is(err, epubreader.ErrUnsupportedMedia):
		status = http.StatusUnsupportedMediaType
		message = "unsupported EPUB resource type"
	case errors.Is(err, epubreader.ErrInvalidArchive),
		errors.Is(err, epubreader.ErrExtractionLimit),
		errors.Is(err, epubreader.ErrNotEPUB):
		status = http.StatusUnprocessableEntity
		message = "EPUB archive cannot be opened safely"
	}
	c.JSON(status, gin.H{"error": message})
}
