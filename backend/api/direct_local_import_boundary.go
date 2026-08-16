package api

import (
	"errors"
	"math"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	localImportMultipartEnvelopeBytes int64 = 1 << 20
	maxLocalImportFilenameBytes             = 255
	maxLocalImportTitleBytes                = 240
	maxLocalImportAuthorBytes               = 160
	maxLocalImportTOCRuleBytes              = 16 << 10
	maxLocalImportCategoryValueBytes        = 32
	maxLocalImportCategoryValues            = 200
)

var (
	errLocalImportRequestTooLarge = errors.New("local import request body too large")
	errLocalImportRequestInvalid  = errors.New("invalid local import request")
	errLocalImportSourceRequired  = errors.New("local import source is required")
)

type parsedLocalImportMultipart struct {
	form        *multipart.Form
	file        *multipart.FileHeader
	importToken string
	title       string
	author      string
	tocRule     string
	categoryID  *uint
	categoryIDs []uint
}

func (s *Server) parseLocalImportMultipart(c *gin.Context, preview bool) (*parsedLocalImportMultipart, error) {
	requestLimit := s.maxLocalImportBytes() + localImportMultipartEnvelopeBytes
	if requestLimit < s.maxLocalImportBytes() {
		requestLimit = math.MaxInt64
	}
	if c.Request.ContentLength > requestLimit {
		return nil, errLocalImportRequestTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, requestLimit)
	form, err := c.MultipartForm()
	if form == nil {
		form = c.Request.MultipartForm
	}
	payload := &parsedLocalImportMultipart{form: form}
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return payload, errLocalImportRequestTooLarge
		}
		if errors.Is(err, http.ErrNotMultipart) {
			return payload, errLocalImportSourceRequired
		}
		return payload, errLocalImportRequestInvalid
	}
	if form == nil {
		return payload, errLocalImportRequestInvalid
	}

	allowedValues := map[string]bool{
		"importToken": true,
		"title":       true,
		"author":      true,
		"tocRule":     true,
	}
	if !preview {
		allowedValues["categoryId"] = true
		allowedValues["categoryIds"] = true
	}
	for field, values := range form.Value {
		if len(values) > 0 && !allowedValues[field] {
			return payload, errLocalImportRequestInvalid
		}
	}

	files := form.File["file"]
	fileCount := 0
	for field, headers := range form.File {
		fileCount += len(headers)
		if field != "file" && len(headers) > 0 {
			return payload, errLocalImportRequestInvalid
		}
	}
	if len(files) > 1 || fileCount != len(files) {
		return payload, errLocalImportRequestInvalid
	}

	importToken, err := localImportUniqueField(form, "importToken", 48)
	if err != nil {
		return payload, err
	}
	hasFile := len(files) == 1
	hasToken := importToken != ""
	if !hasFile && !hasToken {
		return payload, errLocalImportSourceRequired
	}
	if hasFile && hasToken {
		return payload, errLocalImportRequestInvalid
	}
	if hasToken && !validLocalImportToken(importToken) {
		return payload, errLocalImportRequestInvalid
	}

	title, err := localImportUniqueField(form, "title", maxLocalImportTitleBytes)
	if err != nil {
		return payload, err
	}
	author, err := localImportUniqueField(form, "author", maxLocalImportAuthorBytes)
	if err != nil {
		return payload, err
	}
	tocRule, err := localImportUniqueField(form, "tocRule", maxLocalImportTOCRuleBytes)
	if err != nil {
		return payload, err
	}

	if hasFile {
		filename, err := normalizeDirectLocalImportFilename(files[0].Filename)
		if err != nil {
			return payload, errLocalImportRequestInvalid
		}
		files[0].Filename = filename
		payload.file = files[0]
	}

	payload.importToken = importToken
	payload.title = title
	payload.author = author
	payload.tocRule = tocRule
	if !preview {
		categoryID, categoryIDs, err := parseDirectLocalImportCategories(form)
		if err != nil {
			return payload, err
		}
		payload.categoryID = categoryID
		payload.categoryIDs = categoryIDs
	}
	return payload, nil
}

func localImportUniqueField(form *multipart.Form, key string, maxBytes int) (string, error) {
	values := form.Value[key]
	if len(values) > 1 {
		return "", errLocalImportRequestInvalid
	}
	if len(values) == 0 {
		return "", nil
	}
	value := strings.TrimSpace(values[0])
	if !utf8.ValidString(value) || len(value) > maxBytes || strings.ContainsRune(value, '\x00') {
		return "", errLocalImportRequestInvalid
	}
	return value, nil
}

func normalizeDirectLocalImportFilename(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || name == "." || name == ".." ||
		!utf8.ValidString(name) || len(name) > maxLocalImportFilenameBytes ||
		strings.ContainsRune(name, '\x00') || hasWindowsPathVolume(name) ||
		name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return "", errLocalImportRequestInvalid
	}
	return name, nil
}

func parseDirectLocalImportCategories(form *multipart.Form) (*uint, []uint, error) {
	categoryIDValues := form.Value["categoryId"]
	categoryIDsValues := form.Value["categoryIds"]
	if len(categoryIDValues) > 1 || len(categoryIDValues)+len(categoryIDsValues) > maxLocalImportCategoryValues {
		return nil, nil, errLocalImportRequestInvalid
	}
	var categoryID *uint
	if len(categoryIDValues) == 1 {
		value, err := parseDirectLocalImportCategoryValue(categoryIDValues[0])
		if err != nil {
			return nil, nil, err
		}
		categoryID = value
	}

	categoryIDs := make([]uint, 0, len(categoryIDsValues))
	seen := make(map[uint]struct{}, len(categoryIDsValues))
	for _, rawValue := range categoryIDsValues {
		if !utf8.ValidString(rawValue) || len(rawValue) > maxLocalImportCategoryValueBytes || strings.ContainsRune(rawValue, '\x00') {
			return nil, nil, errLocalImportRequestInvalid
		}
		for _, part := range strings.Split(rawValue, ",") {
			value, err := parseDirectLocalImportCategoryValue(part)
			if err != nil {
				return nil, nil, err
			}
			if value == nil {
				continue
			}
			if _, ok := seen[*value]; ok {
				continue
			}
			seen[*value] = struct{}{}
			categoryIDs = append(categoryIDs, *value)
			if len(categoryIDs) > maxLocalImportCategoryValues {
				return nil, nil, errLocalImportRequestInvalid
			}
		}
	}
	return categoryID, categoryIDs, nil
}

func parseDirectLocalImportCategoryValue(rawValue string) (*uint, error) {
	value := strings.TrimSpace(rawValue)
	if !utf8.ValidString(value) || len(value) > maxLocalImportCategoryValueBytes || strings.ContainsRune(value, '\x00') {
		return nil, errLocalImportRequestInvalid
	}
	if value == "" || value == "0" {
		return nil, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 || uint64(uint(parsed)) != parsed {
		return nil, errLocalImportRequestInvalid
	}
	result := uint(parsed)
	return &result, nil
}

func (s *Server) readLocalImportPayload(payload *parsedLocalImportMultipart, userID uint, createStage bool) (string, string, []byte, string, error) {
	if payload.importToken != "" {
		metadata, data, err := s.loadStagedLocalImport(userID, payload.importToken)
		if err != nil {
			return "", "", nil, "", err
		}
		return metadata.FileName, metadata.Extension, data, payload.importToken, nil
	}

	fileHeader := payload.file
	if fileHeader == nil {
		return "", "", nil, "", errLocalImportSourceRequired
	}
	if fileHeader.Size > s.maxLocalImportBytes() {
		return "", "", nil, "", errLocalImportTooLarge
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	file, err := fileHeader.Open()
	if err != nil {
		return "", "", nil, "", errors.New("failed to open file")
	}
	defer file.Close()
	data, err := s.readBoundedLocalImport(file)
	if err != nil {
		if errors.Is(err, errLocalImportTooLarge) {
			return "", "", nil, "", err
		}
		return "", "", nil, "", errors.New("failed to read file")
	}
	if !createStage {
		return fileHeader.Filename, ext, data, "", nil
	}
	importToken, err := s.stageLocalImport(userID, fileHeader.Filename, ext, data)
	if err != nil {
		return "", "", nil, "", errors.New("failed to stage import")
	}
	return fileHeader.Filename, ext, data, importToken, nil
}

func writeLocalImportRequestError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errLocalImportRequestTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "local import request is too large"})
	case errors.Is(err, errLocalImportSourceRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": "file or importToken is required"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid local import request"})
	}
}
