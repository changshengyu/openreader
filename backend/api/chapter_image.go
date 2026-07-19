package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/chapterimage"
)

func (s *Server) chapterImageResource(c *gin.Context) {
	resource, err := s.chapterImages.OpenResource(c.Param("capability"))
	if err != nil {
		switch {
		case errors.Is(err, chapterimage.ErrMalformedCapability),
			errors.Is(err, chapterimage.ErrInvalidCapability),
			errors.Is(err, chapterimage.ErrExpiredCapability),
			errors.Is(err, chapterimage.ErrUnsafePath):
			c.JSON(http.StatusForbidden, gin.H{"error": "chapter image capability is invalid"})
		case errors.Is(err, chapterimage.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "chapter image not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load chapter image"})
		}
		return
	}
	c.Header("Content-Type", resource.ContentType)
	c.Header("Content-Length", strconv.FormatInt(resource.Size, 10))
	c.Header("Cache-Control", "private, max-age=300")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, resource.ContentType, resource.Data)
}
