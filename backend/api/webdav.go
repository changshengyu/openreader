package api

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/bookgroups"
	"openreader/backend/services/localbook"
	"openreader/backend/services/webdavfs"
)

// ---------- WebDAV endpoints ----------

const webDAVAllow = "OPTIONS, DELETE, GET, PUT, PROPFIND, MKCOL, MOVE, COPY, LOCK, UNLOCK"

func (s *Server) registerWebDAVRoutes(router *gin.Engine, prefix string) {
	group := router.Group(prefix)
	group.Use(webDAVResponseHeaders)
	group.Use(middleware.WebDAVAuthRequired(s.cfg.JWTSecret, s.db))
	group.Use(middleware.TrackActivity(s.db))
	group.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if !s.requireWebDAVAccess(c) {
			return
		}
		c.Next()
	})
	group.Handle(http.MethodOptions, "/*path", s.webdavOptions)
	group.Handle("PROPFIND", "/*path", s.webdavPropfind)
	group.GET("/*path", s.webdavGetOrList)
	group.PUT("/*path", s.webdavPut)
	group.Handle("MKCOL", "/*path", s.webdavMkcol)
	group.Handle("MOVE", "/*path", s.webdavMove)
	group.Handle("COPY", "/*path", s.webdavCopy)
	group.Handle("LOCK", "/*path", s.webdavLock)
	group.Handle("UNLOCK", "/*path", s.webdavUnlock)
	group.DELETE("/*path", s.webdavDelete)
}

func webDAVResponseHeaders(c *gin.Context) {
	c.Header("DAV", "1,2")
	c.Header("Allow", webDAVAllow)
	c.Header("MS-Author-Via", "DAV")
	c.Next()
}

func (s *Server) webdavOptions(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (s *Server) webDAVFileService(c *gin.Context) (*webdavfs.Service, bool) {
	root, ok := s.storeRoot(c, s.webdavDir())
	if !ok {
		return nil, false
	}
	service, err := webdavfs.NewScoped(s.webdavDir(), root)
	if err != nil {
		writeWebDAVServiceError(c, err)
		return nil, false
	}
	if err := service.EnsureRoot(); err != nil {
		writeWebDAVServiceError(c, err)
		return nil, false
	}
	return service, true
}

func isUpstreamWebDAVRequest(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/reader3/webdav/") || c.Request.URL.Path == "/reader3/webdav"
}

func (s *Server) webdavGetOrList(c *gin.Context) {
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	resource, err := service.Stat(relPath)
	if err != nil {
		writeWebDAVServiceError(c, err)
		return
	}
	if resource.Info.IsDir() {
		if isUpstreamWebDAVRequest(c) {
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		s.webdavList(c, relPath)
		return
	}
	s.webdavGet(c)
}

func (s *Server) webdavList(c *gin.Context, relPath string) {
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	resources, err := service.List(relPath, 1)
	if err != nil {
		writeWebDAVServiceError(c, err)
		return
	}

	type fileEntry struct {
		Name         string `xml:"displayname"`
		IsDir        bool   `xml:"iscollection"`
		Size         int64  `xml:"getcontentlength"`
		LastModified string `xml:"lastmodified"`
	}

	response := struct {
		XMLName  xml.Name    `xml:"multistatus"`
		Response []fileEntry `xml:"response>propstat>prop"`
	}{
		Response: []fileEntry{{Name: "", IsDir: true}},
	}

	for _, resource := range resources[1:] {
		size := resource.Info.Size()
		if resource.Info.IsDir() {
			size = 0
		}
		response.Response = append(response.Response, fileEntry{
			Name:         filepath.Base(filepath.FromSlash(resource.RelativePath)),
			IsDir:        resource.Info.IsDir(),
			Size:         size,
			LastModified: resource.Info.ModTime().Format(time.RFC1123),
		})
	}

	c.XML(http.StatusMultiStatus, response)
}

func (s *Server) webdavGet(c *gin.Context) {
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	file, info, err := service.Open(relPath)
	if err != nil {
		writeWebDAVServiceError(c, err)
		return
	}
	defer file.Close()
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

func (s *Server) webdavPut(c *gin.Context) {
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	if c.Request.ContentLength > s.maxLocalImportBytes() {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}
	if err := service.Put(c.Request.Context(), strings.TrimPrefix(c.Param("path"), "/"), c.Request.Body, s.maxLocalImportBytes()); err != nil {
		writeWebDAVServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

func (s *Server) webdavMkcol(c *gin.Context) {
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	if err := service.Mkdir(strings.TrimPrefix(c.Param("path"), "/")); err != nil {
		writeWebDAVServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

func (s *Server) webdavMove(c *gin.Context) {
	s.webdavTransfer(c, false)
}

func (s *Server) webdavCopy(c *gin.Context) {
	s.webdavTransfer(c, true)
}

func (s *Server) webdavTransfer(c *gin.Context, copyResource bool) {
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	destinationRelPath, ok := webdavDestinationPath(c.GetHeader("Destination"))
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	overwrite := strings.EqualFold(strings.TrimSpace(c.GetHeader("Overwrite")), "T")
	sourceRelPath := strings.TrimPrefix(c.Param("path"), "/")
	var err error
	if copyResource {
		err = service.Copy(c.Request.Context(), sourceRelPath, destinationRelPath, overwrite)
	} else {
		err = service.Move(sourceRelPath, destinationRelPath, overwrite)
	}
	if err != nil {
		writeWebDAVServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

func (s *Server) webdavDelete(c *gin.Context) {
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	if err := service.Remove(strings.TrimPrefix(c.Param("path"), "/")); err != nil {
		writeWebDAVServiceError(c, err)
		return
	}
	if isUpstreamWebDAVRequest(c) {
		c.Status(http.StatusOK)
		return
	}
	c.Status(http.StatusNoContent)
}

func webdavDestinationPath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	if parsed.Path != "" {
		value = parsed.Path
	}
	hadLeadingSlash := strings.HasPrefix(value, "/")
	value = strings.TrimPrefix(value, "/")
	switch {
	case strings.HasPrefix(value, "reader3/webdav/"):
		value = strings.TrimPrefix(value, "reader3/webdav/")
	case value == "reader3/webdav":
		value = ""
	case strings.HasPrefix(value, "webdav/"):
		value = strings.TrimPrefix(value, "webdav/")
	case value == "webdav":
		value = ""
	case hadLeadingSlash:
		return "", false
	}
	return value, true
}

type davMultiStatus struct {
	XMLName   xml.Name      `xml:"DAV: multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string      `xml:"href"`
	PropStat davPropStat `xml:"propstat"`
}

type davPropStat struct {
	Status string  `xml:"status"`
	Prop   davProp `xml:"prop"`
}

type davProp struct {
	DisplayName   string          `xml:"displayname"`
	LastModified  string          `xml:"getlastmodified"`
	CreationDate  string          `xml:"creationdate"`
	ResourceType  davResourceType `xml:"resourcetype"`
	ContentLength *int64          `xml:"getcontentlength,omitempty"`
	ContentType   string          `xml:"getcontenttype,omitempty"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection,omitempty"`
}

func (s *Server) webdavPropfind(c *gin.Context) {
	service, ok := s.webDAVFileService(c)
	if !ok {
		return
	}
	depth := 1
	if strings.TrimSpace(c.GetHeader("Depth")) == "0" {
		depth = 0
	}
	resources, err := service.List(strings.TrimPrefix(c.Param("path"), "/"), depth)
	if err != nil {
		writeWebDAVServiceError(c, err)
		return
	}
	response := davMultiStatus{Responses: make([]davResponse, 0, len(resources))}
	for _, resource := range resources {
		property := davProp{
			DisplayName:  webDAVDisplayName(resource.RelativePath),
			LastModified: resource.Info.ModTime().UTC().Format(http.TimeFormat),
			CreationDate: resource.Info.ModTime().UTC().Format(time.RFC3339),
		}
		if resource.Info.IsDir() {
			property.ResourceType.Collection = &struct{}{}
		} else {
			length := resource.Info.Size()
			property.ContentLength = &length
			property.ContentType = mime.TypeByExtension(filepath.Ext(resource.Info.Name()))
			if property.ContentType == "" {
				property.ContentType = "application/octet-stream"
			}
		}
		response.Responses = append(response.Responses, davResponse{
			Href: webDAVHref(c, resource.RelativePath, resource.Info.IsDir()),
			PropStat: davPropStat{
				Status: "HTTP/1.1 200 OK",
				Prop:   property,
			},
		})
	}
	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Status(http.StatusMultiStatus)
	_, _ = c.Writer.Write([]byte(xml.Header))
	_ = xml.NewEncoder(c.Writer).Encode(response)
}

func webDAVDisplayName(relative string) string {
	if relative == "" {
		return ""
	}
	return filepath.Base(filepath.FromSlash(relative))
}

func webDAVHref(c *gin.Context, relative string, directory bool) string {
	prefix := "/webdav/"
	if isUpstreamWebDAVRequest(c) {
		prefix = "/reader3/webdav/"
	}
	if relative == "" {
		return prefix
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	href := prefix + strings.Join(parts, "/")
	if directory && !strings.HasSuffix(href, "/") {
		href += "/"
	}
	return href
}

type davLockProperty struct {
	XMLName       xml.Name         `xml:"DAV: prop"`
	LockDiscovery davLockDiscovery `xml:"lockdiscovery"`
}

type davLockDiscovery struct {
	ActiveLock davActiveLock `xml:"activelock"`
}

type davActiveLock struct {
	LockType  davWriteLock     `xml:"locktype"`
	LockScope davExclusiveLock `xml:"lockscope"`
	LockToken davHref          `xml:"locktoken"`
	LockRoot  davHref          `xml:"lockroot"`
	Depth     string           `xml:"depth"`
	Owner     davHref          `xml:"owner"`
	Timeout   string           `xml:"timeout"`
}

type davWriteLock struct {
	Write *struct{} `xml:"write"`
}

type davExclusiveLock struct {
	Exclusive *struct{} `xml:"exclusive"`
}

type davHref struct {
	Href string `xml:"href"`
}

func (s *Server) webdavLock(c *gin.Context) {
	token, err := randomWebDAVLockToken()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	timeout := strings.TrimSpace(c.GetHeader("Timeout"))
	if timeout == "" || (!strings.EqualFold(timeout, "Infinite") && !strings.HasPrefix(strings.ToLower(timeout), "second-")) {
		timeout = "Second-3600"
	}
	body := davLockProperty{LockDiscovery: davLockDiscovery{ActiveLock: davActiveLock{
		LockType:  davWriteLock{Write: &struct{}{}},
		LockScope: davExclusiveLock{Exclusive: &struct{}{}},
		LockToken: davHref{Href: token},
		LockRoot:  davHref{Href: webDAVHref(c, strings.TrimPrefix(c.Param("path"), "/"), false)},
		Depth:     "infinity",
		Owner:     davHref{Href: "http://www.apple.com/webdav_fs/"},
		Timeout:   timeout,
	}}}
	c.Header("Lock-Token", token)
	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write([]byte(xml.Header))
	_ = xml.NewEncoder(c.Writer).Encode(body)
}

func (s *Server) webdavUnlock(c *gin.Context) {
	token := strings.TrimSpace(c.GetHeader("Lock-Token"))
	if token == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	c.Header("Lock-Token", token)
	c.Status(http.StatusNoContent)
}

func randomWebDAVLockToken() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func writeWebDAVServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, webdavfs.ErrUnsafePath):
		c.Status(http.StatusForbidden)
	case errors.Is(err, webdavfs.ErrNotFound):
		c.Status(http.StatusNotFound)
	case errors.Is(err, webdavfs.ErrConflict), errors.Is(err, webdavfs.ErrNotDirectory):
		c.Status(http.StatusConflict)
	case errors.Is(err, webdavfs.ErrPrecondition):
		c.Status(http.StatusPreconditionFailed)
	case errors.Is(err, webdavfs.ErrIsDirectory):
		c.Status(http.StatusMethodNotAllowed)
	case errors.Is(err, webdavfs.ErrTooLarge):
		c.Status(http.StatusRequestEntityTooLarge)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return
	default:
		c.Status(http.StatusInternalServerError)
	}
}

// ---------- Reading app backup restoration ----------

const backupMultipartEnvelopeBytes int64 = 1 << 20

func (s *Server) importLegadoBackup(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	limits := s.portableLimits()
	if c.Request.ContentLength > limits.maxCompressed+backupMultipartEnvelopeBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limits.maxCompressed+backupMultipartEnvelopeBytes)
	userID, _ := middleware.UserID(c)
	user, ok := storeUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	username, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file is required"})
		return
	}
	if !strings.EqualFold(filepath.Ext(fileHeader.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file must be a zip archive"})
		return
	}
	if fileHeader.Size > limits.maxCompressed {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
		return
	}
	stagedPath, err := s.stageUploadedBackup(fileHeader, userID, limits.maxCompressed)
	if err != nil {
		writeBackupRestoreError(c, err)
		return
	}
	defer os.Remove(stagedPath)
	result, err := s.restoreBackupFileWithPermissions(stagedPath, userID, username, user.CanEditSources)
	if err != nil {
		writeBackupRestoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

type restoreWebDAVBackupRequest struct {
	Path string `json:"path" binding:"required"`
}

func (s *Server) restoreWebDAVBackup(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)
	user, ok := storeUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	username, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}
	var req restoreWebDAVBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(req.Path)), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file must be a zip archive"})
		return
	}
	filePath, _, ok := s.webdavPath(c, req.Path)
	if !ok {
		return
	}
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file not found"})
		return
	}
	limits := s.portableLimits()
	if info.Size() > limits.maxCompressed {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
		return
	}
	result, err := s.restoreBackupFileWithPermissions(filePath, userID, username, user.CanEditSources)
	if err != nil {
		writeBackupRestoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func writeBackupRestoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errPortableBackupConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "portable backup conflicts with an existing local book"})
	case errors.Is(err, errPortableBackupLimit):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
	case errors.Is(err, errBackupRestoreTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
	case errors.Is(err, errBackupArchiveLimit):
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup archive exceeds safety limits"})
	case errors.Is(err, errBackupRestorePersistence):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "backup restore failed"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup package"})
	}
}

func (s *Server) importFromWebDAV(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)

	var req localBookImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	paths := req.requestedPaths()
	if len(paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	categoryIDs := categoryIDsFromRequest(req.CategoryID, req.CategoryIDs)
	if len(req.CategoryIDs) > 0 {
		if !s.validateCategoryIDs(c, userID, categoryIDs) {
			return
		}
	} else if !s.validateCategory(c, userID, req.CategoryID) {
		return
	}
	var primaryCategoryID *uint
	if len(categoryIDs) > 0 {
		primaryCategoryID = &categoryIDs[0]
	}

	userName, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}

	importer := localbook.NewImporter(s.cfg, s.db)
	imported := make([]gin.H, 0)
	importedBooks := make([]bookListItem, 0)
	seen := make(map[string]bool)
	itemByPath := req.itemByPath()

	for _, rawPath := range paths {
		_, requestedPath, ok := s.webdavPath(c, rawPath)
		if !ok {
			return
		}
		override := itemByPath[requestedPath]
		if override.ImportToken != "" {
			if seen[requestedPath] {
				continue
			}
			seen[requestedPath] = true
			importRequest, err := s.stagedStorageImportRequest(userID, userName, override.ImportToken, override, primaryCategoryID)
			if err != nil {
				imported = append(imported, gin.H{"path": requestedPath, "error": err.Error()})
				continue
			}
			book, err := s.importStagedLocalBook(userID, override.ImportToken, importer, importRequest)
			if err != nil {
				imported = append(imported, gin.H{"path": requestedPath, "error": err.Error()})
				continue
			}
			s.removeStagedLocalImport(userID, override.ImportToken)
			if len(categoryIDs) > 0 {
				_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
			}
			item := s.bookShelfListItem(userID, book)
			imported = append(imported, gin.H{"path": requestedPath, "book": item})
			importedBooks = append(importedBooks, item)
			continue
		}
		files, ok := s.webDAVImportFiles(c, rawPath)
		if !ok {
			return
		}
		for _, file := range files {
			if seen[file.relativePath] {
				continue
			}
			seen[file.relativePath] = true
			if file.validationError != "" {
				imported = append(imported, gin.H{"path": file.relativePath, "error": file.validationError})
				continue
			}

			data, err := s.readBoundedLocalImportFile(file.filePath)
			if err != nil {
				imported = append(imported, gin.H{"path": file.relativePath, "error": err.Error()})
				continue
			}

			override := itemByPath[file.relativePath]
			book, err := importer.Import(localbook.ImportRequest{
				UserID:     userID,
				UserName:   userName,
				FileName:   filepath.Base(file.filePath),
				Extension:  file.extension,
				Data:       data,
				Title:      override.Title,
				Author:     override.Author,
				CategoryID: primaryCategoryID,
				TOCRule:    override.TOCRule,
			})
			if err != nil {
				imported = append(imported, gin.H{"path": file.relativePath, "error": err.Error()})
				continue
			}
			if len(categoryIDs) > 0 {
				_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
			}
			item := s.bookShelfListItem(userID, book)
			imported = append(imported, gin.H{"path": file.relativePath, "book": item})
			importedBooks = append(importedBooks, item)
		}
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": importedBooks})
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (s *Server) previewWebDAVImport(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)
	var req localBookImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	paths := req.requestedPaths()
	if len(paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	results := make([]gin.H, 0)
	seen := make(map[string]bool)
	itemByPath := req.itemByPath()
	for _, rawPath := range paths {
		_, requestedPath, ok := s.webdavPath(c, rawPath)
		if !ok {
			continue
		}
		if override, exists := itemByPath[requestedPath]; exists && override.ImportToken != "" {
			if seen[requestedPath] {
				continue
			}
			seen[requestedPath] = true
			preview, importToken, err := s.reparseStagedStorageImport(userID, override.ImportToken, override)
			if err != nil {
				results = append(results, gin.H{"path": requestedPath, "error": err.Error(), "importToken": importToken})
				continue
			}
			results = append(results, gin.H{"path": requestedPath, "book": preview, "importToken": importToken})
			continue
		}

		files, ok := s.webDAVImportFiles(c, rawPath)
		if !ok {
			continue
		}
		for _, file := range files {
			if seen[file.relativePath] {
				continue
			}
			seen[file.relativePath] = true
			if file.validationError != "" {
				results = append(results, gin.H{"path": file.relativePath, "error": file.validationError})
				continue
			}
			override := itemByPath[file.relativePath]
			preview, importToken, err := s.previewStagedStorageImport(userID, file, override)
			if err != nil {
				results = append(results, gin.H{"path": file.relativePath, "error": err.Error(), "importToken": importToken})
				continue
			}
			results = append(results, gin.H{"path": file.relativePath, "book": preview, "importToken": importToken})
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": results})
}

func (s *Server) webDAVImportFiles(c *gin.Context, rawPath string) ([]localStoreImportFile, bool) {
	filePath, relativePath, ok := s.webdavPath(c, rawPath)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, true
	}
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(filePath))
		if !isImportableExtension(ext) {
			return []localStoreImportFile{{filePath: filePath, relativePath: relativePath, extension: ext, validationError: "unsupported file type"}}, true
		}
		return []localStoreImportFile{{filePath: filePath, relativePath: relativePath, extension: ext}}, true
	}

	files := make([]localStoreImportFile, 0)
	_ = filepath.WalkDir(filePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !isImportableExtension(ext) {
			return nil
		}
		rel, err := filepath.Rel(filePath, path)
		if err != nil {
			return nil
		}
		files = append(files, localStoreImportFile{
			filePath:     path,
			relativePath: cleanRelativePath(filepath.Join(relativePath, rel)),
			extension:    ext,
		})
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i].relativePath) < strings.ToLower(files[j].relativePath)
	})
	return files, true
}

func (s *Server) restoreLegadoBackupData(data []byte, userID uint) (gin.H, error) {
	return s.restoreLegadoBackupDataWithBroadcast(data, userID, true)
}

// restoreLegadoBackupDataWithoutBroadcast lets a higher-level restore add durable local archive
// state before reader clients are told that their restored shelf is ready to load.
func (s *Server) restoreLegadoBackupDataWithoutBroadcast(data []byte, userID uint) (gin.H, error) {
	return s.restoreLegadoBackupDataWithBroadcast(data, userID, false)
}

func (s *Server) restoreLegadoBackupDataWithBroadcast(data []byte, userID uint, broadcast bool) (gin.H, error) {
	return s.restoreLegadoBackupDataWithPermissions(data, userID, true, broadcast)
}

func (s *Server) restoreRSSSourcesFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreRSSSourcesFromData(data, userID)
}

func (s *Server) restoreRSSSourcesFromData(data []byte, userID uint) (int, error) {
	sources, err := decodeRestoredRSSSources(data)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, sourceReq := range sources {
		sourceReq.normalize()
		url := strings.TrimSpace(sourceReq.URL)
		if url == "" {
			continue
		}
		title := strings.TrimSpace(sourceReq.Title)
		if title == "" {
			title = url
		}
		enabled := true
		if sourceReq.Enabled != nil {
			enabled = *sourceReq.Enabled
		}
		customOrder, err := sourceReq.orderOrDefaultStrict(s, userID)
		if err != nil {
			return count, err
		}
		source := models.RSSSource{
			UserID:          userID,
			Title:           title,
			URL:             url,
			Icon:            strings.TrimSpace(sourceReq.Icon),
			Group:           strings.TrimSpace(sourceReq.Group),
			Comment:         strings.TrimSpace(sourceReq.Comment),
			CustomOrder:     customOrder,
			ConcurrentRate:  strings.TrimSpace(sourceReq.ConcurrentRate),
			Header:          sourceReq.headerText(),
			LoginURL:        strings.TrimSpace(sourceReq.LoginURL),
			LoginCheckJS:    strings.TrimSpace(sourceReq.LoginCheckJS),
			SingleURL:       sourceReq.singleURLOr(false),
			ArticleStyle:    sourceReq.articleStyleOrDefault(),
			SortURL:         strings.TrimSpace(sourceReq.SortURL),
			RuleArticles:    strings.TrimSpace(sourceReq.RuleArticles),
			RuleNextPage:    strings.TrimSpace(sourceReq.RuleNextPage),
			RuleTitle:       strings.TrimSpace(sourceReq.RuleTitle),
			RulePubDate:     strings.TrimSpace(sourceReq.RulePubDate),
			RuleDescription: strings.TrimSpace(sourceReq.RuleDescription),
			RuleImage:       strings.TrimSpace(sourceReq.RuleImage),
			RuleLink:        strings.TrimSpace(sourceReq.RuleLink),
			RuleContent:     strings.TrimSpace(sourceReq.RuleContent),
			Style:           strings.TrimSpace(sourceReq.Style),
			EnableJS:        sourceReq.enableJSOrDefault(),
			LoadWithBaseURL: sourceReq.loadWithBaseURLOrDefault(),
			Enabled:         enabled,
			UpdatedAt:       time.Now(),
		}
		query := s.db.Where("user_id = ? AND url = ?", userID, url)
		if err := query.Assign(source).FirstOrCreate(&source).Error; err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func decodeRestoredRSSSources(data []byte) ([]rssSourceRequest, error) {
	var sources []rssSourceRequest
	if err := json.Unmarshal(data, &sources); err != nil {
		var source rssSourceRequest
		if err := json.Unmarshal(data, &source); err != nil {
			return nil, err
		}
		sources = []rssSourceRequest{source}
	}
	return sources, nil
}

func (s *Server) broadcastRestoreUpdates(userID uint, result gin.H) {
	if s.hub == nil {
		return
	}
	if restoreResultCount(result, "sources") > 0 {
		s.broadcastSourcesUpdate("restore-backup")
	}
	if restoreResultCount(result, "settings") > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "settings_update", "payload": gin.H{"key": "all"}})
	}
	if restoreResultCount(result, "categories")+restoreResultCount(result, "bookGroups") > 0 {
		var categories []models.Category
		if err := s.db.Where("user_id = ?", userID).Order("sort_order asc, name asc").Find(&categories).Error; err == nil {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "categories_update", "payload": categories})
		} else {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "categories_update"})
		}
		s.broadcastBookGroupsUpdate(userID)
	}
	if restoreResultCount(result, "books")+restoreResultCount(result, "progress") > 0 {
		if items, err := s.listAllBookShelfItems(userID); err == nil {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
		} else {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
		}
	}
	if restoreResultCount(result, "bookmarks") > 0 {
		s.broadcastBookmarksUpdate(userID, "restore-backup", 0, nil)
	}
	if restoreResultCount(result, "replaceRules") > 0 {
		s.broadcastReplaceRulesUpdate(userID, "restore-backup")
	}
	if restoreResultCount(result, "rssSources") > 0 {
		s.broadcastRSSUpdate(userID, "restore-backup", gin.H{"sources": true})
	}
}

func restoreResultCount(result gin.H, key string) int {
	switch value := result[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func (s *Server) restoreUserSettingsFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreUserSettingsFromData(data, userID)
}

func (s *Server) restoreUserSettingsFromData(data []byte, userID uint) (int, error) {
	var settings []models.UserSetting
	if err := json.Unmarshal(data, &settings); err != nil {
		return 0, err
	}

	count := 0
	for _, setting := range settings {
		key := normalizeUserSettingKey(setting.Key)
		if key == "" || !json.Valid([]byte(setting.Value)) {
			continue
		}
		next := models.UserSetting{
			UserID:    userID,
			Key:       key,
			Value:     string(sanitizeUserSettingValue(key, json.RawMessage(setting.Value))),
			UpdatedAt: time.Now(),
		}
		if err := s.db.Where("user_id = ? AND key = ?", userID, key).Assign(next).FirstOrCreate(&next).Error; err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Server) restoreCategoriesFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreCategoriesFromData(data, userID)
}

func (s *Server) restoreCategoriesFromData(data []byte, userID uint) (int, error) {
	var categories []models.Category
	if err := json.Unmarshal(data, &categories); err != nil {
		return 0, err
	}

	count := 0
	for _, category := range categories {
		name := strings.TrimSpace(category.Name)
		if name == "" {
			continue
		}
		next := models.Category{
			UserID:    userID,
			Name:      name,
			Color:     category.Color,
			SortOrder: category.SortOrder,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := s.db.Where("user_id = ? AND name = ?", userID, name).Assign(next).FirstOrCreate(&next).Error; err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Server) restoreBookshelfFromZip(file *zip.File, userID uint) (int, int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, 0, err
	}
	return s.restoreBookshelfFromData(data, userID)
}

type restoredBookshelfRow struct {
	Title              string   `json:"title"`
	Name               string   `json:"name"`
	Author             string   `json:"author"`
	URL                string   `json:"url"`
	BookURL            string   `json:"bookUrl"`
	Origin             string   `json:"origin"`
	SourceID           uint     `json:"sourceId"`
	SourceName         string   `json:"sourceName"`
	Type               int      `json:"type"`
	CoverURL           string   `json:"coverUrl"`
	CustomCoverURL     string   `json:"customCoverUrl"`
	Intro              string   `json:"intro"`
	Kind               string   `json:"kind"`
	WordCount          string   `json:"wordCount"`
	Variable           string   `json:"variable"`
	LastChapter        string   `json:"lastChapter"`
	LatestChapterTitle string   `json:"latestChapterTitle"`
	ChapterCount       int      `json:"chapterCount"`
	TotalChapterNum    int      `json:"totalChapterNum"`
	CanUpdate          *bool    `json:"canUpdate"`
	CategoryName       string   `json:"categoryName"`
	CategoryNames      []string `json:"categoryNames"`
	Group              int      `json:"group"`
	OriginName         string   `json:"originName"`
	DurChapter         int      `json:"durChapter"`
	DurChapterIndex    int      `json:"durChapterIndex"`
	DurChapterPos      int      `json:"durChapterPos"`
	DurChapterTitle    string   `json:"durChapterTitle"`
	DurChapterTime     int64    `json:"durChapterTime"`
	LastCheckTime      int64    `json:"lastCheckTime"`
}

type restoredBookmarkRow struct {
	models.Bookmark
	BookTitle   string `json:"bookTitle"`
	BookURL     string `json:"bookUrl"`
	Time        int64  `json:"time"`
	BookName    string `json:"bookName"`
	BookAuthor  string `json:"bookAuthor"`
	ChapterPos  int    `json:"chapterPos"`
	ChapterName string `json:"chapterName"`
	BookText    string `json:"bookText"`
	Content     string `json:"content"`
}

type restoredReplaceRuleRow struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Scope       string `json:"scope"`
	IsRegex     *bool  `json:"isRegex"`
	Enabled     *bool  `json:"enabled"`
	IsEnabled   *bool  `json:"isEnabled"`
	Order       int    `json:"order"`
}

type restoredProgressPayload struct {
	models.ReadingProgress
	Name            string `json:"name"`
	BookName        string `json:"bookName"`
	BookTitle       string `json:"bookTitle"`
	Title           string `json:"title"`
	BookURL         string `json:"bookUrl"`
	URL             string `json:"url"`
	DurChapter      int    `json:"durChapter"`
	DurChapterIndex int    `json:"durChapterIndex"`
	ChapterIndex    int    `json:"chapterIndex"`
	DurChapterPos   int    `json:"durChapterPos"`
	Offset          int    `json:"offset"`
	DurChapterTitle string `json:"durChapterTitle"`
	DurChapterTime  int64  `json:"durChapterTime"`
}

type restoredChapterVariableRow struct {
	SourceName   string `json:"sourceName"`
	BookURL      string `json:"bookUrl"`
	BookTitle    string `json:"bookTitle"`
	ChapterURL   string `json:"chapterUrl"`
	ChapterTitle string `json:"chapterTitle"`
	ChapterIndex int    `json:"chapterIndex"`
	Variable     string `json:"variable"`
}

func positiveTimestamp(value int64) int64 {
	if value > 0 {
		return value
	}
	return 0
}

func validateRestoredBookshelfVariables(data []byte) error {
	var books []restoredBookshelfRow
	if err := json.Unmarshal(data, &books); err != nil {
		return err
	}
	for index := range books {
		variable, err := models.NormalizeSourceRuleVariables(books[index].Variable)
		if err != nil {
			return err
		}
		books[index].Variable = variable
	}
	return nil
}

func validateRestoredChapterVariables(data []byte) error {
	var rows []restoredChapterVariableRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	for index := range rows {
		variable, err := models.NormalizeSourceRuleVariables(rows[index].Variable)
		if err != nil {
			return err
		}
		rows[index].Variable = variable
	}
	return nil
}

func (s *Server) restoreBookshelfFromData(data []byte, userID uint) (int, int, error) {
	return s.restoreBookshelfFromDataWithGroupMap(data, userID, nil)
}

func (s *Server) restoreBookshelfFromDataWithGroupMap(data []byte, userID uint, groupCategoryMap map[int]uint) (int, int, error) {
	var books []restoredBookshelfRow
	if err := json.Unmarshal(data, &books); err != nil {
		return 0, 0, err
	}
	if err := validateRestoredBookshelfVariables(data); err != nil {
		return 0, 0, errInvalidBackupArchive
	}
	for index := range books {
		variable, _ := models.NormalizeSourceRuleVariables(books[index].Variable)
		books[index].Variable = variable
	}

	count := 0
	progressCount := 0
	for _, b := range books {
		title := strings.TrimSpace(b.Title)
		if title == "" {
			title = strings.TrimSpace(b.Name)
		}
		if title == "" {
			continue
		}
		bookURL := strings.TrimSpace(b.URL)
		if bookURL == "" {
			bookURL = strings.TrimSpace(b.BookURL)
		}
		canUpdate := true
		if b.CanUpdate != nil {
			canUpdate = *b.CanUpdate
		}
		sourceName := strings.TrimSpace(firstNonBlank(b.SourceName, b.OriginName))
		sourceID, err := s.restoredBookSourceIDStrict(sourceName, b.Origin)
		if err != nil {
			return count, progressCount, err
		}
		if strings.EqualFold(strings.TrimSpace(b.Origin), "loc_book") {
			sourceID = 0
		}
		variable := b.Variable
		if sourceID == 0 || sourceName == "" {
			// A source token is meaningful only with a resolved remote source. This
			// also makes old/local backups safe when they contain unknown fields or
			// only a legacy numeric source ID.
			variable = ""
		}
		chapterCount := b.ChapterCount
		if chapterCount == 0 && b.TotalChapterNum > 0 {
			chapterCount = b.TotalChapterNum
		}
		book := models.Book{
			UserID:         userID,
			SourceID:       sourceID,
			Type:           b.Type,
			Title:          title,
			Author:         strings.TrimSpace(b.Author),
			URL:            bookURL,
			CoverURL:       strings.TrimSpace(b.CoverURL),
			CustomCoverURL: strings.TrimSpace(b.CustomCoverURL),
			Intro:          strings.TrimSpace(b.Intro),
			Kind:           strings.TrimSpace(b.Kind),
			WordCount:      strings.TrimSpace(b.WordCount),
			Variable:       variable,
			LastChapter:    strings.TrimSpace(firstNonBlank(b.LastChapter, b.LatestChapterTitle)),
			ChapterCount:   chapterCount,
			LastCheckTime:  positiveTimestamp(b.LastCheckTime),
			CanUpdate:      canUpdate,
		}
		categoryIDs, err := s.restoredCategoryIDsStrict(userID, b.CategoryName, b.CategoryNames)
		if err != nil {
			return count, progressCount, err
		}
		if len(categoryIDs) == 0 && b.Group > 0 {
			categoryIDs = restoredCategoryIDsFromGroupMask(b.Group, groupCategoryMap)
		}
		if len(categoryIDs) > 0 {
			book.CategoryID = &categoryIDs[0]
		}
		query := s.db.Where("user_id = ? AND title = ?", userID, book.Title)
		if book.URL != "" {
			query = s.db.Where("user_id = ? AND url = ?", userID, book.URL)
		}
		var existing models.Book
		existingErr := query.First(&existing).Error
		if existingErr == nil {
			existing.SourceID = book.SourceID
			existing.Type = book.Type
			existing.Author = book.Author
			existing.CoverURL = book.CoverURL
			existing.CustomCoverURL = book.CustomCoverURL
			existing.Intro = book.Intro
			existing.Kind = book.Kind
			existing.WordCount = book.WordCount
			existing.Variable = book.Variable
			existing.LastChapter = book.LastChapter
			existing.ChapterCount = book.ChapterCount
			if book.LastCheckTime > 0 {
				existing.LastCheckTime = book.LastCheckTime
			}
			existing.CanUpdate = book.CanUpdate
			existing.CategoryID = book.CategoryID
			if book.URL != "" {
				existing.URL = book.URL
			}
			if err := s.db.Save(&existing).Error; err != nil {
				return count, progressCount, err
			}
			if err := s.setBookCategories(s.db, userID, existing.ID, categoryIDs); err != nil {
				return count, progressCount, err
			}
			count++
			chapterIndex := b.DurChapter
			if b.DurChapterIndex > 0 || chapterIndex == 0 {
				chapterIndex = b.DurChapterIndex
			}
			restoredProgress, err := s.restoreBookshelfProgressStrictAt(userID, existing.ID, chapterIndex, b.DurChapterPos, b.DurChapterTitle, timeFromUnixMilli(b.DurChapterTime))
			if err != nil {
				return count, progressCount, err
			}
			if restoredProgress {
				progressCount++
			}
			continue
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return count, progressCount, existingErr
		}
		if err := s.db.Create(&book).Error; err != nil {
			return count, progressCount, err
		}
		if err := s.setBookCategories(s.db, userID, book.ID, categoryIDs); err != nil {
			return count, progressCount, err
		}
		chapterIndex := b.DurChapter
		if b.DurChapterIndex > 0 || chapterIndex == 0 {
			chapterIndex = b.DurChapterIndex
		}
		restoredProgress, err := s.restoreBookshelfProgressStrictAt(userID, book.ID, chapterIndex, b.DurChapterPos, b.DurChapterTitle, timeFromUnixMilli(b.DurChapterTime))
		if err != nil {
			return count, progressCount, err
		}
		if restoredProgress {
			progressCount++
		}
		count++
	}
	return count, progressCount, nil
}

func (s *Server) restoreBookGroupsFromData(data []byte, userID uint) (map[int]uint, int, error) {
	var rows []bookgroups.RestoreRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, 0, err
	}
	return s.bookGroups.Restore(userID, rows)
}

func restoredCategoryIDsFromGroupMask(group int, mapping map[int]uint) []uint {
	if group <= 0 || len(mapping) == 0 {
		return nil
	}
	masks := make([]int, 0, len(mapping))
	for mask := range mapping {
		if mask > 0 && group&mask != 0 {
			masks = append(masks, mask)
		}
	}
	sort.Ints(masks)
	ids := make([]uint, 0, len(masks))
	seen := make(map[uint]struct{}, len(masks))
	for _, mask := range masks {
		id := mapping[mask]
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func isBookGroupBackupEntry(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), "bookgroup.json")
}

func (s *Server) restoreChapterVariablesFromData(data []byte, userID uint) (int, error) {
	var rows []restoredChapterVariableRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, err
	}
	if err := validateRestoredChapterVariables(data); err != nil {
		return 0, errInvalidBackupArchive
	}
	for index := range rows {
		variable, _ := models.NormalizeSourceRuleVariables(rows[index].Variable)
		rows[index].Variable = variable
	}

	count := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if row.ChapterIndex < 0 || strings.TrimSpace(row.Variable) == "" || strings.TrimSpace(row.SourceName) == "" {
				continue
			}
			book, ok, err := findRestoredBookWithDB(tx, userID, row.BookURL, row.BookTitle)
			if err != nil {
				return err
			}
			if !ok || book.SourceID == 0 {
				continue
			}
			var source models.BookSource
			if err := tx.Select("name").First(&source, book.SourceID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return err
			}
			if source.Name != strings.TrimSpace(row.SourceName) {
				continue
			}

			chapterURL := strings.TrimSpace(row.ChapterURL)
			chapterTitle := strings.TrimSpace(row.ChapterTitle)
			var chapter models.Chapter
			query := tx.Where("book_id = ? AND `index` = ?", book.ID, row.ChapterIndex)
			if err := query.First(&chapter).Error; err == nil {
				if chapterURL != "" && strings.TrimSpace(chapter.URL) != "" && chapter.URL != chapterURL {
					continue
				}
				chapter.Variable = row.Variable
				if chapter.URL == "" {
					chapter.URL = chapterURL
				}
				if chapter.Title == "" {
					chapter.Title = chapterTitle
				}
				if err := tx.Save(&chapter).Error; err != nil {
					return err
				}
				count++
				continue
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			if chapterURL == "" {
				continue
			}
			if chapterTitle == "" {
				chapterTitle = fmt.Sprintf("第 %d 章", row.ChapterIndex+1)
			}
			chapter = models.Chapter{
				BookID:   book.ID,
				Index:    row.ChapterIndex,
				Title:    chapterTitle,
				URL:      chapterURL,
				Variable: row.Variable,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func (s *Server) restoredBookSourceIDStrict(sourceName string, sourceURL string) (uint, error) {
	name := strings.TrimSpace(sourceName)
	if name != "" {
		var source models.BookSource
		if err := s.db.Select("id").Where("name = ?", name).First(&source).Error; err == nil {
			return source.ID, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
	}
	url := strings.TrimSpace(sourceURL)
	if url == "" || strings.EqualFold(url, "loc_book") {
		return 0, nil
	}
	var source models.BookSource
	if err := s.db.Select("id").Where("base_url = ?", url).First(&source).Error; err == nil {
		return source.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	return 0, nil
}

func (s *Server) restoreBookmarksFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreBookmarksFromData(data, userID)
}

func (s *Server) restoreBookmarksFromData(data []byte, userID uint) (int, error) {
	return s.restoreBookmarksFromDataWithFormat(data, userID, false)
}

func (s *Server) restoreBookmarksFromDataWithFormat(data []byte, userID uint, upstream bool) (int, error) {
	var rows []restoredBookmarkRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, err
	}

	count := 0
	for _, row := range rows {
		bookTitle := strings.TrimSpace(row.BookTitle)
		if bookTitle == "" {
			bookTitle = strings.TrimSpace(row.BookName)
		}
		book, ok, err := s.findRestoredBookStrict(userID, row.BookURL, bookTitle)
		if err != nil {
			return count, err
		}
		if !ok {
			continue
		}
		chapterIndex := row.ChapterIndex
		if chapterIndex < 0 {
			chapterIndex = 0
		}
		offset := row.Offset
		if upstream || (offset == 0 && row.ChapterPos > 0) {
			offset = row.ChapterPos
		}
		if offset < 0 {
			offset = 0
		}
		chapterID := uint(0)
		var chapter models.Chapter
		if err := s.db.Where("book_id = ? AND `index` = ?", book.ID, chapterIndex).First(&chapter).Error; err == nil {
			chapterID = chapter.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return count, err
		}
		createdAt := row.CreatedAt
		if createdAt.IsZero() && row.Time > 0 {
			createdAt = time.UnixMilli(row.Time)
		}
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		updatedAt := row.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}
		bookmark := models.Bookmark{
			UserID:       userID,
			BookID:       book.ID,
			ChapterID:    chapterID,
			ChapterIndex: chapterIndex,
			Offset:       offset,
			Percent:      clampProgressPercent(row.Percent),
			Title:        strings.TrimSpace(firstNonBlank(row.Title, row.ChapterName)),
			Excerpt:      strings.TrimSpace(firstNonBlank(row.Excerpt, row.BookText)),
			Note:         strings.TrimSpace(firstNonBlank(row.Note, row.Content)),
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		// Modern OpenReader exports include CreatedAt, which is the stable identity
		// needed to retain more than one bookmark at the same chapter/offset.  Old
		// reader-dev exports may omit it, so their narrower, legacy identity remains
		// available as a read-compatibility fallback.
		identity := models.Bookmark{
			UserID:       userID,
			BookID:       book.ID,
			ChapterIndex: bookmark.ChapterIndex,
			Offset:       bookmark.Offset,
			Title:        bookmark.Title,
			Excerpt:      bookmark.Excerpt,
			Note:         bookmark.Note,
		}
		if !createdAt.IsZero() && (!row.CreatedAt.IsZero() || row.Time > 0) {
			identity.CreatedAt = createdAt
		}
		if err := s.db.Where(&identity).FirstOrCreate(&bookmark).Error; err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Server) restoreReplaceRulesFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreReplaceRulesFromData(data, userID)
}

func (s *Server) restoreReplaceRulesFromData(data []byte, userID uint) (int, error) {
	var rules []restoredReplaceRuleRow
	if err := json.Unmarshal(data, &rules); err != nil {
		return 0, err
	}

	count := 0
	for _, rule := range rules {
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			continue
		}
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			name = pattern
		}
		enabled := true
		if rule.Enabled != nil {
			enabled = *rule.Enabled
		}
		if rule.IsEnabled != nil {
			enabled = *rule.IsEnabled
		}
		isRegex := false
		if rule.IsRegex != nil {
			isRegex = *rule.IsRegex
		}
		scope := strings.TrimSpace(rule.Scope)
		if scope == "" {
			scope = "*"
		}
		next := models.ReplaceRule{
			UserID:      userID,
			Name:        name,
			Group:       strings.TrimSpace(rule.Group),
			Pattern:     pattern,
			Replacement: rule.Replacement,
			Scope:       scope,
			IsRegex:     &isRegex,
			Enabled:     enabled,
			Order:       rule.Order,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := s.db.Where("user_id = ? AND pattern = ?", userID, pattern).Assign(next).FirstOrCreate(&next).Error; err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Server) restoreBookshelfProgress(userID uint, bookID uint, chapterIndex int, offset int, chapterTitle string) bool {
	restored, _ := s.restoreBookshelfProgressStrict(userID, bookID, chapterIndex, offset, chapterTitle)
	return restored
}

func (s *Server) restoreBookshelfProgressStrict(userID uint, bookID uint, chapterIndex int, offset int, chapterTitle string) (bool, error) {
	return s.restoreBookshelfProgressStrictAt(userID, bookID, chapterIndex, offset, chapterTitle, time.Time{})
}

func (s *Server) restoreBookshelfProgressStrictAt(userID uint, bookID uint, chapterIndex int, offset int, chapterTitle string, restoredAt time.Time) (bool, error) {
	if chapterIndex <= 0 && offset <= 0 && strings.TrimSpace(chapterTitle) == "" {
		return false, nil
	}
	if chapterIndex < 0 {
		chapterIndex = 0
	}
	if offset < 0 {
		offset = 0
	}
	return s.restoreReadingProgressStrict(models.ReadingProgress{
		UserID:       userID,
		BookID:       bookID,
		ChapterIndex: chapterIndex,
		Offset:       offset,
		ChapterTitle: strings.TrimSpace(chapterTitle),
		Mode:         "scroll",
		UpdatedAt:    restoredAt,
	}, !restoredAt.IsZero())
}

func (s *Server) restoreReadingProgressStrict(progress models.ReadingProgress, hasArchiveTime bool) (bool, error) {
	if progress.UserID == 0 || progress.BookID == 0 {
		return false, nil
	}
	if progress.Mode == "" {
		progress.Mode = "scroll"
	}
	if !hasArchiveTime || progress.UpdatedAt.IsZero() {
		progress.UpdatedAt = time.Now()
		hasArchiveTime = false
	}

	var existing models.ReadingProgress
	err := s.db.Where("user_id = ? AND book_id = ?", progress.UserID, progress.BookID).First(&existing).Error
	if err == nil {
		if hasArchiveTime && progress.UpdatedAt.UnixMilli() <= existing.UpdatedAt.UnixMilli() {
			return false, nil
		}
		updates := map[string]any{
			"chapter_id":      progress.ChapterID,
			"chapter_index":   progress.ChapterIndex,
			"offset":          progress.Offset,
			"percent":         clampProgressPercent(progress.Percent),
			"chapter_percent": clampProgressPercent(progress.ChapterPercent),
			"chapter_title":   progress.ChapterTitle,
			"mode":            progress.Mode,
			"updated_at":      progress.UpdatedAt,
		}
		if err := s.db.Model(&existing).Updates(updates).Error; err != nil {
			return false, err
		}
		return true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	progress.ID = 0
	progress.Percent = clampProgressPercent(progress.Percent)
	progress.ChapterPercent = clampProgressPercent(progress.ChapterPercent)
	if err := s.db.Create(&progress).Error; err != nil {
		return false, err
	}
	return true, nil
}

func timeFromUnixMilli(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}

func (s *Server) restoreProgressFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreProgressFromData(data, userID)
}

func (s *Server) restoreProgressFromData(data []byte, userID uint) (int, error) {
	payloads, err := decodeRestoredProgressPayloads(data)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, payload := range payloads {
		bookURL := strings.TrimSpace(payload.BookURL)
		if bookURL == "" {
			bookURL = strings.TrimSpace(payload.URL)
		}
		title := strings.TrimSpace(payload.BookName)
		if title == "" {
			title = strings.TrimSpace(payload.BookTitle)
		}
		if title == "" {
			title = strings.TrimSpace(payload.Name)
		}
		if title == "" {
			title = strings.TrimSpace(payload.Title)
		}

		book, ok, err := s.findRestoredBookStrict(userID, bookURL, title)
		if err != nil {
			return count, err
		}
		if !ok {
			continue
		}

		chapterIndex := payload.ChapterIndex
		if chapterIndex == 0 && payload.DurChapterIndex > 0 {
			chapterIndex = payload.DurChapterIndex
		}
		if chapterIndex == 0 && payload.DurChapter > 0 {
			chapterIndex = payload.DurChapter
		}
		offset := payload.Offset
		if offset == 0 && payload.DurChapterPos > 0 {
			offset = payload.DurChapterPos
		}
		chapterTitle := strings.TrimSpace(payload.ReadingProgress.ChapterTitle)
		if chapterTitle == "" {
			chapterTitle = strings.TrimSpace(payload.DurChapterTitle)
		}
		restoredAt := payload.UpdatedAt
		if restoredAt.IsZero() {
			restoredAt = timeFromUnixMilli(payload.DurChapterTime)
		}
		progress := models.ReadingProgress{
			UserID:         userID,
			BookID:         book.ID,
			ChapterIndex:   chapterIndex,
			Offset:         offset,
			Percent:        payload.Percent,
			ChapterPercent: payload.ChapterPercent,
			ChapterTitle:   chapterTitle,
			Mode:           payload.Mode,
			UpdatedAt:      restoredAt,
		}
		restored, err := s.restoreReadingProgressStrict(progress, !restoredAt.IsZero())
		if err != nil {
			return count, err
		}
		if restored {
			count++
		}
	}
	return count, nil
}

func decodeRestoredProgressPayloads(data []byte) ([]restoredProgressPayload, error) {
	var payloads []restoredProgressPayload
	if err := json.Unmarshal(data, &payloads); err == nil {
		return payloads, nil
	}
	var payload restoredProgressPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return []restoredProgressPayload{payload}, nil
}

func (s *Server) findRestoredCategoryID(userID uint, categoryName string) *uint {
	categoryID, _, _ := s.findRestoredCategoryIDStrict(userID, categoryName)
	return categoryID
}

func (s *Server) findRestoredCategoryIDStrict(userID uint, categoryName string) (*uint, bool, error) {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return nil, false, nil
	}
	var category models.Category
	if err := s.db.Where("user_id = ? AND name = ?", userID, categoryName).First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &category.ID, true, nil
}

func (s *Server) restoredCategoryIDs(userID uint, categoryName string, categoryNames []string) []uint {
	ids, _ := s.restoredCategoryIDsStrict(userID, categoryName, categoryNames)
	return ids
}

func (s *Server) restoredCategoryIDsStrict(userID uint, categoryName string, categoryNames []string) ([]uint, error) {
	names := make([]string, 0, len(categoryNames)+1)
	names = append(names, categoryNames...)
	if strings.TrimSpace(categoryName) != "" {
		names = append(names, categoryName)
	}
	seen := make(map[string]struct{}, len(names))
	ids := make([]uint, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		categoryID, found, err := s.findRestoredCategoryIDStrict(userID, name)
		if err != nil {
			return nil, err
		}
		if found {
			ids = append(ids, *categoryID)
		}
	}
	return ids, nil
}

func (s *Server) findRestoredBook(userID uint, bookURL string, title string) (models.Book, bool) {
	book, found, _ := s.findRestoredBookStrict(userID, bookURL, title)
	return book, found
}

func (s *Server) findRestoredBookStrict(userID uint, bookURL string, title string) (models.Book, bool, error) {
	return findRestoredBookWithDB(s.db, userID, bookURL, title)
}

func findRestoredBookWithDB(db *gorm.DB, userID uint, bookURL string, title string) (models.Book, bool, error) {
	bookURL = strings.TrimSpace(bookURL)
	title = strings.TrimSpace(title)
	var book models.Book
	query := db.Where("user_id = ?", userID)
	if bookURL != "" {
		query = query.Where("url = ?", bookURL)
	} else if title != "" {
		query = query.Where("title = ?", title)
	} else {
		return book, false, nil
	}
	if err := query.First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return book, false, nil
		}
		return book, false, err
	}
	return book, true, nil
}

func (s *Server) webdavDir() string {
	return filepath.Join(s.cfg.DataDir, "webdav")
}

func (s *Server) webdavPath(c *gin.Context, rawPath string) (string, string, bool) {
	storeRoot, ok := s.storeRoot(c, s.webdavDir())
	if !ok {
		return "", "", false
	}
	service, err := webdavfs.NewScoped(s.webdavDir(), storeRoot)
	if err != nil {
		writeWebDAVServiceError(c, err)
		return "", "", false
	}
	target, relative, err := service.Resolve(rawPath)
	if err != nil {
		writeWebDAVServiceError(c, err)
		return "", "", false
	}
	return target, relative, true
}
