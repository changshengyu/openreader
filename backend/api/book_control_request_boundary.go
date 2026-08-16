package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	maxBookControlRequestBodyBytes       int64 = 16 << 10
	maxLocalRefreshRequestBodyBytes      int64 = 32 << 10
	maxRemoteBookControlRequestBodyBytes int64 = 1 << 20

	maxBookControlCategoryIDs = 200
)

type localRefreshRequest struct {
	TOCRule *string `json:"tocRule"`
}

func decodeBookControlRequest[T any](c *gin.Context, maxBytes int64, invalidMessage string) (*T, bool) {
	var request *T
	if err := decodeBoundedSingleUTF8JSON(c, &request, maxBytes); err != nil {
		writeBookControlPayloadError(c, err, invalidMessage)
		return nil, false
	}
	if request == nil {
		writeBookControlPayloadError(c, errJSONRequestInvalid, invalidMessage)
		return nil, false
	}
	return request, true
}

func decodeOptionalLocalRefreshRequest(c *gin.Context) (localRefreshRequest, bool) {
	if c.Request.ContentLength > maxLocalRefreshRequestBodyBytes {
		writeBookControlPayloadError(c, errJSONRequestTooLarge, "invalid local refresh payload")
		return localRefreshRequest{}, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxLocalRefreshRequestBodyBytes)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeBookControlPayloadError(c, classifyJSONRequestDecodeError(err), "invalid local refresh payload")
		return localRefreshRequest{}, false
	}
	if len(data) == 0 {
		return localRefreshRequest{}, true
	}
	if !utf8.Valid(data) {
		writeBookControlPayloadError(c, errJSONRequestInvalid, "invalid local refresh payload")
		return localRefreshRequest{}, false
	}

	var request *localRefreshRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&request); err != nil {
		writeBookControlPayloadError(c, errJSONRequestInvalid, "invalid local refresh payload")
		return localRefreshRequest{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil || !errors.Is(err, io.EOF) || request == nil {
		writeBookControlPayloadError(c, errJSONRequestInvalid, "invalid local refresh payload")
		return localRefreshRequest{}, false
	}
	return *request, true
}

func writeBookControlPayloadError(c *gin.Context, err error, invalidMessage string) {
	if errors.Is(err, errJSONRequestTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
}

func isRequestContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func normalizeRemoteBookRequest(c *gin.Context, request *remoteBookRequest) bool {
	var ok bool
	if request.Title, ok = normalizeBookWriteField(c, &request.Title, maxBookTitleBytes, "book title is too long"); !ok {
		return false
	}
	if request.Author, ok = normalizeBookWriteField(c, &request.Author, maxBookAuthorBytes, "book author is too long"); !ok {
		return false
	}
	if request.CoverURL, ok = normalizeBookWriteField(c, &request.CoverURL, maxBookCoverURLBytes, "book cover url is too long"); !ok {
		return false
	}
	if request.Kind, ok = normalizeBookWriteField(c, &request.Kind, maxBookKindBytes, "book kind is too long"); !ok {
		return false
	}
	if request.WordCount, ok = normalizeBookWriteField(c, &request.WordCount, maxBookWordCountBytes, "book word count is too long"); !ok {
		return false
	}
	if request.BookURL, ok = normalizeBookWriteField(c, &request.BookURL, maxBookURLBytes, "book url is too long"); !ok {
		return false
	}
	return true
}

func normalizeChangeSourceRequest(c *gin.Context, request *changeSourceRequest) bool {
	var ok bool
	if request.Title, ok = normalizeBookWriteField(c, &request.Title, maxBookTitleBytes, "book title is too long"); !ok {
		return false
	}
	if request.Author, ok = normalizeBookWriteField(c, &request.Author, maxBookAuthorBytes, "book author is too long"); !ok {
		return false
	}
	if request.CoverURL, ok = normalizeBookWriteField(c, &request.CoverURL, maxBookCoverURLBytes, "book cover url is too long"); !ok {
		return false
	}
	if request.Kind, ok = normalizeBookWriteField(c, &request.Kind, maxBookKindBytes, "book kind is too long"); !ok {
		return false
	}
	if request.WordCount, ok = normalizeBookWriteField(c, &request.WordCount, maxBookWordCountBytes, "book word count is too long"); !ok {
		return false
	}
	if request.BookURL, ok = normalizeBookWriteField(c, &request.BookURL, maxBookURLBytes, "book url is too long"); !ok {
		return false
	}
	return true
}
