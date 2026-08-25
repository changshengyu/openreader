package api

import (
	"errors"
	"math"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const maxBackupRestoreFilenameBytes = 255

var (
	errBackupRestoreUploadTooLarge     = errors.New("backup restore upload too large")
	errBackupRestoreUploadFileRequired = errors.New("backup restore upload file required")
	errBackupRestoreUploadInvalid      = errors.New("backup restore upload invalid")
	errBackupRestoreUploadFilename     = errors.New("backup restore upload filename invalid")
)

type parsedBackupRestoreMultipart struct {
	form *multipart.Form
	file *multipart.FileHeader
}

func backupRestoreMultipartRequestLimit(maxCompressed int64) int64 {
	if maxCompressed > math.MaxInt64-backupMultipartEnvelopeBytes {
		return math.MaxInt64
	}
	return maxCompressed + backupMultipartEnvelopeBytes
}

func prepareBackupRestoreMultipartBody(c *gin.Context, maxCompressed int64) error {
	requestLimit := backupRestoreMultipartRequestLimit(maxCompressed)
	if c.Request.ContentLength > requestLimit {
		return errBackupRestoreUploadTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, requestLimit)
	return nil
}

func parseBackupRestoreMultipart(c *gin.Context) (*parsedBackupRestoreMultipart, error) {
	form, err := c.MultipartForm()
	if form == nil {
		form = c.Request.MultipartForm
	}
	upload := &parsedBackupRestoreMultipart{form: form}
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return upload, errBackupRestoreUploadTooLarge
		}
		if errors.Is(err, http.ErrNotMultipart) {
			return upload, errBackupRestoreUploadFileRequired
		}
		return upload, errBackupRestoreUploadInvalid
	}
	if form == nil {
		return upload, errBackupRestoreUploadInvalid
	}
	if len(form.Value) != 0 {
		return upload, errBackupRestoreUploadInvalid
	}

	files := form.File["file"]
	fileCount := 0
	for field, headers := range form.File {
		fileCount += len(headers)
		if field != "file" && len(headers) > 0 {
			return upload, errBackupRestoreUploadInvalid
		}
	}
	if fileCount == 0 {
		return upload, errBackupRestoreUploadFileRequired
	}
	if fileCount != 1 || len(files) != 1 {
		return upload, errBackupRestoreUploadInvalid
	}

	filename := strings.TrimSpace(files[0].Filename)
	if filename == "" || !utf8.ValidString(filename) || len(filename) > maxBackupRestoreFilenameBytes ||
		!strings.EqualFold(filepath.Ext(filename), ".zip") {
		return upload, errBackupRestoreUploadFilename
	}
	files[0].Filename = filename
	upload.file = files[0]
	return upload, nil
}

func (upload *parsedBackupRestoreMultipart) removeAll() {
	if upload != nil && upload.form != nil {
		_ = upload.form.RemoveAll()
	}
}

func writeBackupRestoreMultipartError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errBackupRestoreUploadTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
	case errors.Is(err, errBackupRestoreUploadFileRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file is required"})
	case errors.Is(err, errBackupRestoreUploadFilename):
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file must be a zip archive"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup upload"})
	}
}
