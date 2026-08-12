package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	maxBookGroupWriteRequestBodyBytes int64 = 16 << 10
	maxBookGroupNameBytes                   = 80
	maxCategoryColorBytes                   = 24
)

func decodeBookGroupWriteRequest[T any](c *gin.Context, invalidMessage string) (*T, bool) {
	var request *T
	if err := decodeBoundedSingleJSON(c, &request, maxBookGroupWriteRequestBodyBytes); err != nil {
		if errors.Is(err, errJSONRequestTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return nil, false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
		return nil, false
	}
	if request == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
		return nil, false
	}
	return request, true
}
