package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/coverimage"
)

func (s *Server) coverImageResource(c *gin.Context) {
	resource, err := s.coverImages.Open(c.Request.Context(), c.Param("capability"))
	if err != nil {
		status := http.StatusInternalServerError
		message := "failed to load cover"
		switch {
		case errors.Is(err, coverimage.ErrMalformedCapability),
			errors.Is(err, coverimage.ErrInvalidCapability),
			errors.Is(err, coverimage.ErrExpiredCapability):
			status = http.StatusForbidden
			message = "cover capability is invalid"
		case errors.Is(err, coverimage.ErrUnavailable),
			errors.Is(err, coverimage.ErrUnsafeURL):
			status = http.StatusNotFound
			message = "cover unavailable"
		}
		if c.Request.Method == http.MethodHead {
			c.Status(status)
			return
		}
		c.JSON(status, gin.H{"error": message})
		return
	}

	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("Content-Length", strconv.FormatInt(resource.Size, 10))
	c.Header("Content-Type", resource.ContentType)
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, resource.ContentType, resource.Data)
}
