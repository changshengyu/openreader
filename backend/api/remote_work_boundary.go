package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	maxRemoteSearchRequestBytes  int64 = 64 << 10
	maxRemoteControlRequestBytes int64 = 16 << 10

	maxRemoteSearchKeywordBytes = 1024
	maxRemoteProbeURLBytes      = 8192
	maxRemoteSearchSourceIDs    = 5000
	maxRemoteHealthSources      = 300
	maxRemoteSearchConcurrent   = 60
	maxRemoteSearchWindows      = 8
)

func decodeRemoteWorkRequest[T any](c *gin.Context, maxBytes int64, invalidMessage string) (*T, bool) {
	var request *T
	if err := decodeBoundedSingleUTF8JSON(c, &request, maxBytes); err != nil {
		if errors.Is(err, errJSONRequestTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
		}
		return nil, false
	}
	if request == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
		return nil, false
	}
	return request, true
}
