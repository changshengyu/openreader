package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxProgressRequestBodyBytes int64 = 16 << 10
	maxProgressModeBytes              = 20
	maxProgressClientIDBytes          = 128
	maxProgressTimestampBytes         = 64
)

func decodeProgressRequest(c *gin.Context) (*progressRequest, bool) {
	var request *progressRequest
	if err := decodeBoundedSingleUTF8JSON(c, &request, maxProgressRequestBodyBytes); err != nil {
		if errors.Is(err, errJSONRequestTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid progress payload"})
		}
		return nil, false
	}
	if request == nil || request.BookID == 0 || request.ChapterIndex == nil || *request.ChapterIndex < 0 ||
		request.Offset < 0 || len(request.Mode) > maxProgressModeBytes ||
		len(request.ClientID) > maxProgressClientIDBytes ||
		!validProgressTimestamp(request.BaseUpdatedAt) ||
		!validProgressTimestamp(request.ClientUpdatedAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid progress payload"})
		return nil, false
	}
	return request, true
}

func validProgressTimestamp(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > maxProgressTimestampBytes {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
