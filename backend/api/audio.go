package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/audioreader"
)

func (s *Server) audioResource(c *gin.Context) {
	resource, err := s.audioReader.OpenResource(
		c.Param("capability"),
		c.Param("resourcePath"),
	)
	if err != nil {
		writeAudioServiceError(c, err, "failed to load audio resource")
		return
	}

	if resource.File == nil || resource.Info == nil {
		writeAudioServiceError(c, audioreader.ErrNotFound, "failed to load audio resource")
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

func writeAudioServiceError(c *gin.Context, err error, fallback string) {
	status := http.StatusInternalServerError
	message := fallback
	switch {
	case errors.Is(err, audioreader.ErrMalformedCapability),
		errors.Is(err, audioreader.ErrUnsafePath):
		status = http.StatusBadRequest
		message = "invalid audio resource request"
	case errors.Is(err, audioreader.ErrInvalidCapability),
		errors.Is(err, audioreader.ErrExpiredCapability):
		status = http.StatusForbidden
		message = "audio resource authorization failed"
	case errors.Is(err, audioreader.ErrNotFound),
		errors.Is(err, audioreader.ErrNotAudio):
		status = http.StatusNotFound
		message = "audio resource not found"
	case errors.Is(err, audioreader.ErrUnsupportedMedia):
		status = http.StatusUnsupportedMediaType
		message = "unsupported audio resource type"
	}
	c.JSON(status, gin.H{"error": message})
}

func writeAudioChapterPrepareError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "failed to prepare audio resource"
	switch {
	case errors.Is(err, audioreader.ErrUnsafePath):
		status = http.StatusBadRequest
		message = "invalid audio resource request"
	case errors.Is(err, audioreader.ErrNotFound),
		errors.Is(err, audioreader.ErrNotAudio):
		status = http.StatusNotFound
		message = "audio resource not found"
	case errors.Is(err, audioreader.ErrUnsupportedMedia):
		status = http.StatusUnsupportedMediaType
		message = "unsupported audio resource type"
	}
	c.JSON(status, gin.H{"error": message})
}
