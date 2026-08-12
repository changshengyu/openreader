package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	maxBookWriteRequestBodyBytes int64 = 1 << 20

	maxBookTitleBytes          = 240
	maxBookAuthorBytes         = 160
	maxBookCoverURLBytes       = 600
	maxBookCustomCoverURLBytes = 600
	maxBookKindBytes           = 400
	maxBookWordCountBytes      = 120
	maxBookURLBytes            = 800
)

type bookCreateRequest struct {
	Title          *string `json:"title"`
	Author         *string `json:"author"`
	CoverURL       *string `json:"coverUrl"`
	CustomCoverURL *string `json:"customCoverUrl"`
	Intro          *string `json:"intro"`
	Kind           *string `json:"kind"`
	WordCount      *string `json:"wordCount"`
	URL            *string `json:"url"`
	CategoryID     *uint   `json:"categoryId"`
	CategoryIDs    []uint  `json:"categoryIds"`
	CanUpdate      *bool   `json:"canUpdate"`
}

func decodeBookCreateRequest(c *gin.Context) (*bookCreateRequest, bool) {
	var request *bookCreateRequest
	if err := decodeBoundedSingleJSON(c, &request, maxBookWriteRequestBodyBytes); err != nil {
		writeBookPayloadError(c, err)
		return nil, false
	}
	if request == nil {
		writeBookPayloadError(c, errJSONRequestInvalid)
		return nil, false
	}
	return request, true
}

func decodeBookUpdateRequest(c *gin.Context) (map[string]json.RawMessage, bookUpdateRequest, bool) {
	var raw map[string]json.RawMessage
	if err := decodeBoundedSingleJSON(c, &raw, maxBookWriteRequestBodyBytes); err != nil {
		writeBookPayloadError(c, err)
		return nil, bookUpdateRequest{}, false
	}
	if raw == nil {
		writeBookPayloadError(c, errJSONRequestInvalid)
		return nil, bookUpdateRequest{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		writeBookPayloadError(c, errJSONRequestInvalid)
		return nil, bookUpdateRequest{}, false
	}
	var request bookUpdateRequest
	if err := json.Unmarshal(data, &request); err != nil {
		writeBookPayloadError(c, errJSONRequestInvalid)
		return nil, bookUpdateRequest{}, false
	}
	return raw, request, true
}

func writeBookPayloadError(c *gin.Context, err error) {
	if errors.Is(err, errJSONRequestTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
}

func normalizeBookWriteField(c *gin.Context, value *string, maxBytes int, tooLongError string) (string, bool) {
	if value == nil {
		return "", true
	}
	normalized := strings.TrimSpace(*value)
	if len(normalized) > maxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": tooLongError})
		return "", false
	}
	return normalized, true
}
