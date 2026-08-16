package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

var (
	errJSONRequestInvalid  = errors.New("invalid JSON request")
	errJSONRequestTooLarge = errors.New("JSON request body too large")
)

func decodeBoundedSingleJSON(c *gin.Context, target any, maxBytes int64) error {
	if c.Request.ContentLength > maxBytes {
		return errJSONRequestTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(target); err != nil {
		return classifyJSONRequestDecodeError(err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errJSONRequestInvalid
	} else if !errors.Is(err, io.EOF) {
		return classifyJSONRequestDecodeError(err)
	}
	return nil
}

func decodeBoundedSingleUTF8JSON(c *gin.Context, target any, maxBytes int64) error {
	if c.Request.ContentLength > maxBytes {
		return errJSONRequestTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return classifyJSONRequestDecodeError(err)
	}
	if !utf8.Valid(data) {
		return errJSONRequestInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return classifyJSONRequestDecodeError(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errJSONRequestInvalid
	} else if !errors.Is(err, io.EOF) {
		return classifyJSONRequestDecodeError(err)
	}
	return nil
}

func classifyJSONRequestDecodeError(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return errJSONRequestTooLarge
	}
	return errJSONRequestInvalid
}
