package api

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
)

const bookSourceImportMultipartEnvelopeBytes int64 = 1 << 20

var (
	errBookSourceImportRequestTooLarge = errors.New("book source import request too large")
	errBookSourceImportRequestInvalid  = errors.New("invalid book source import request")
	errBookSourceImportFileRequired    = errors.New("book source import file is required")
)

type parsedBookSourceImportMultipart struct {
	form *multipart.Form
	file *multipart.FileHeader
}

func parseBookSourceImportMultipart(c *gin.Context) (*parsedBookSourceImportMultipart, error) {
	requestLimit := maxBookSourceImportBytes + bookSourceImportMultipartEnvelopeBytes
	if c.Request.ContentLength > requestLimit {
		return nil, errBookSourceImportRequestTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, requestLimit)

	form, err := c.MultipartForm()
	if form == nil {
		form = c.Request.MultipartForm
	}
	payload := &parsedBookSourceImportMultipart{form: form}
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return payload, errBookSourceImportRequestTooLarge
		}
		if errors.Is(err, http.ErrNotMultipart) {
			return payload, errBookSourceImportFileRequired
		}
		return payload, errBookSourceImportRequestInvalid
	}
	if form == nil {
		return payload, errBookSourceImportRequestInvalid
	}
	if len(form.Value) != 0 {
		return payload, errBookSourceImportRequestInvalid
	}

	files := form.File["file"]
	fileCount := 0
	for field, headers := range form.File {
		fileCount += len(headers)
		if field != "file" && len(headers) > 0 {
			return payload, errBookSourceImportRequestInvalid
		}
	}
	if fileCount == 0 {
		return payload, errBookSourceImportFileRequired
	}
	if fileCount != 1 || len(files) != 1 {
		return payload, errBookSourceImportRequestInvalid
	}

	payload.file = files[0]
	return payload, nil
}
