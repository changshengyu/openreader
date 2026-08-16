package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func decodeReplaceRuleRequest[T any](
	c *gin.Context,
	maxBytes int64,
	invalidMessage string,
) (*T, bool) {
	var request *T
	if err := decodeBoundedSingleUTF8JSON(c, &request, maxBytes); err != nil {
		writeReplaceRuleRequestError(c, err, invalidMessage)
		return nil, false
	}
	if request == nil {
		writeReplaceRuleRequestError(c, errJSONRequestInvalid, invalidMessage)
		return nil, false
	}
	return request, true
}

func writeReplaceRuleRequestError(c *gin.Context, err error, invalidMessage string) {
	if errors.Is(err, errJSONRequestTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
}

func replaceRuleRequestCancelled(c *gin.Context, err error) bool {
	return isRequestContextError(err) || c.Request.Context().Err() != nil
}
