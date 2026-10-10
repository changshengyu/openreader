package api

import (
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/services/localbook"
	"openreader/backend/services/webdavfs"
)

type localStoreItem struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Extension    string    `json:"extension"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
	IsDir        bool      `json:"isDir"`
	Importable   bool      `json:"importable"`
}

// Nonparallel tests cancel after authorized lazy-root initialization. This is
// an observation seam only; it does not implement request-aware file reading.
var beforeStoreReadTestHook func(action string)

func runStoreReadTestHook(action string) {
	if beforeStoreReadTestHook != nil {
		beforeStoreReadTestHook(action)
	}
}

func (s *Server) listLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	relativePath, err := normalizeLocalStorePath(c.Query("path"))
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to access local store")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	runStoreReadTestHook("local-list")
	reader, err := service.AdmitRead(c.Request.Context(), relativePath)
	if errors.Is(err, webdavfs.ErrNotFound) && relativePath != "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "local store path not found"})
		return
	}
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to read local store")
		return
	}
	defer reader.Close()
	resource := reader.Resource
	if !resource.Info.IsDir() {
		if !resource.Info.Mode().IsRegular() {
			writeLocalStoreFilesystemError(c, webdavfs.ErrUnsafePath, "failed to read local store")
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read local store"})
		return
	}
	recursive := c.Query("recursive") == "1" || strings.EqualFold(c.Query("recursive"), "true")
	resources, err := reader.ListLocal(recursive)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to read local store")
		return
	}
	items := make([]localStoreItem, 0)
	for _, resource := range resources[1:] {
		items = append(items, makeLocalStoreItem(resource.Info.Name(), resource.RelativePath, resource.Info, resource.Info.IsDir()))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Path) < strings.ToLower(items[j].Path)
	})

	if err := reader.Validate(); err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to read local store")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"path":      relativePath,
		"recursive": recursive,
		"items":     items,
	})
}

func makeLocalStoreItem(name string, itemPath string, info os.FileInfo, isDir bool) localStoreItem {
	ext := strings.ToLower(filepath.Ext(name))
	return localStoreItem{
		Name:         name,
		Path:         itemPath,
		Extension:    ext,
		Size:         info.Size(),
		LastModified: info.ModTime().UTC(),
		IsDir:        isDir,
		Importable:   !isDir && isImportableExtension(ext),
	}
}

func (s *Server) uploadToLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	upload, err := s.parseLocalStoreUpload(c)
	if upload != nil && upload.form != nil {
		defer func() {
			_ = upload.form.RemoveAll()
		}()
	}
	if err != nil {
		writeLocalStoreUploadRequestError(c, err)
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if upload.path != "" {
		if err := service.MkdirContext(c.Request.Context(), upload.path); err != nil {
			writeLocalStoreFilesystemError(c, err, "failed to create directory")
			return
		}
	}

	paths := make([]string, 0, len(upload.files))
	for _, file := range upload.files {
		path, err := s.saveLocalStoreUpload(c, service, upload.path, file)
		if err != nil {
			switch {
			case errors.Is(err, errLocalImportTooLarge):
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
				return
			case errors.Is(err, webdavfs.ErrUnsafePath),
				errors.Is(err, webdavfs.ErrNotDirectory),
				errors.Is(err, webdavfs.ErrIsDirectory),
				errors.Is(err, webdavfs.ErrConflict),
				errors.Is(err, errLocalStorePathInvalid):
				writeLocalStoreFilesystemError(c, err, "failed to save file")
				return
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
				return
			}
		}
		paths = append(paths, path)
	}

	c.JSON(http.StatusCreated, gin.H{"path": paths[0], "paths": paths})
}

func (s *Server) saveLocalStoreUpload(c *gin.Context, service *webdavfs.Service, parentPath string, file *multipart.FileHeader) (string, error) {
	if file.Size > s.maxLocalImportBytes() {
		return "", errLocalImportTooLarge
	}
	name, err := normalizeLocalStoreName(file.Filename)
	if err != nil {
		return "", err
	}
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	rawPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(parentPath), name))
	_, relativePath, err := service.Resolve(rawPath)
	if err != nil {
		return "", err
	}
	if err := service.Put(c.Request.Context(), relativePath, src, s.maxLocalImportBytes()); err != nil {
		if errors.Is(err, webdavfs.ErrTooLarge) {
			return "", errLocalImportTooLarge
		}
		return "", err
	}
	return relativePath, nil
}

func (s *Server) downloadFromLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	relativePath, err := normalizeLocalStorePath(c.Query("path"))
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to access local store")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot download local store root"})
		return
	}
	runStoreReadTestHook("local-download")
	file, info, err := service.OpenContext(c.Request.Context(), relativePath)
	if errors.Is(err, webdavfs.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "local store item not found"})
		return
	}
	if errors.Is(err, webdavfs.ErrIsDirectory) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot download directory"})
		return
	}
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to download local store item")
		return
	}
	defer file.Close()
	if disposition := mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}); disposition != "" {
		c.Header("Content-Disposition", disposition)
	}
	serveOpenedStoreFile(c, file, info)
}

func (s *Server) createLocalStoreDirectory(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	var req *struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decodeLocalStoreJSON(c, &req, maxLocalStoreMetadataBodyBytes, "directory name is required") {
		return
	}
	if req == nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "directory name is required"})
		return
	}
	name, ok := cleanLocalStoreName(c, req.Name)
	if !ok {
		return
	}
	parentPath, err := normalizeLocalStorePath(req.Path)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to create directory")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if parentPath != "" {
		if err := service.MkdirContext(c.Request.Context(), parentPath); err != nil {
			writeLocalStoreFilesystemError(c, err, "failed to create parent directory")
			return
		}
	}
	requestedPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(parentPath), name))
	_, relativePath, err := service.Resolve(requestedPath)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to create directory")
		return
	}
	if _, err := service.Stat(relativePath); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "failed to create directory"})
		return
	} else if !errors.Is(err, webdavfs.ErrNotFound) {
		writeLocalStoreFilesystemError(c, err, "failed to create directory")
		return
	}
	if err := service.MkdirContext(c.Request.Context(), relativePath); err != nil {
		if errors.Is(err, webdavfs.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "failed to create directory"})
		} else {
			writeLocalStoreFilesystemError(c, err, "failed to create directory")
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"path": relativePath})
}

func (s *Server) renameLocalStoreItem(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	var req *struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decodeLocalStoreJSON(c, &req, maxLocalStoreMetadataBodyBytes, "path and name are required") {
		return
	}
	if req == nil || strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and name are required"})
		return
	}
	relativePath, err := normalizeLocalStorePath(req.Path)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to rename local store item")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot rename local store root"})
		return
	}
	name, ok := cleanLocalStoreName(c, req.Name)
	if !ok {
		return
	}
	newRelativePath := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(relativePath)), name))
	if filepath.Dir(filepath.FromSlash(relativePath)) == "." {
		newRelativePath = name
	}
	_, newRelativePath, err = service.Resolve(newRelativePath)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to rename local store item")
		return
	}
	if err := service.MoveContext(c.Request.Context(), relativePath, newRelativePath, true); errors.Is(err, webdavfs.ErrMoveCleanupPending) {
		c.Header("X-OpenReader-WebDAV-Cleanup", "pending")
	} else if err != nil {
		if errors.Is(err, webdavfs.ErrUnsafePath) || errors.Is(err, webdavfs.ErrNotDirectory) {
			writeLocalStoreFilesystemError(c, err, "failed to rename local store item")
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "failed to rename local store item"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": newRelativePath})
}

func (s *Server) deleteFromLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	relativePath, err := normalizeLocalStorePath(c.Query("path"))
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to delete local store item")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete local store root"})
		return
	}
	if err := service.Remove(relativePath); err != nil && !errors.Is(err, webdavfs.ErrNotFound) {
		if errors.Is(err, webdavfs.ErrUnsafePath) || errors.Is(err, webdavfs.ErrNotDirectory) {
			writeLocalStoreFilesystemError(c, err, "failed to delete local store item")
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete local store item"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) importFromLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)

	req, ok := decodeLocalStoreImportRequest(c)
	if !ok {
		return
	}
	plan, ok := s.prepareLocalStoreImport(c, *req)
	if !ok {
		return
	}
	defer plan.Close()
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

	for _, target := range plan.targets {
		if target.override.ImportToken != "" {
			book, err := s.importStagedStorageBook(c.Request.Context(), userID, userName, target.override.ImportToken, target.override, primaryCategoryID, importer)
			if err != nil {
				if writeLocalImportStageCancellation(c, err, "") {
					if len(importedBooks) > 0 {
						_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": importedBooks})
					}
					return
				}
				imported = append(imported, gin.H{"path": target.relativePath, "error": err.Error()})
				continue
			}
			if len(categoryIDs) > 0 {
				_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
			}
			item := s.bookShelfListItem(userID, book)
			imported = append(imported, gin.H{"path": target.relativePath, "book": item})
			importedBooks = append(importedBooks, item)
			continue
		}
		file := target.file
		if file.validationError != "" {
			imported = append(imported, gin.H{"path": file.relativePath, "error": file.validationError})
			continue
		}
		data, err := s.readBoundedStorageImport(c.Request.Context(), plan.service, file, "local-file-read")
		if err != nil {
			if writeStorageImportLifecycleError(c, err) {
				if len(importedBooks) > 0 {
					_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": importedBooks})
				}
				return
			}
			imported = append(imported, gin.H{"path": file.relativePath, "error": localStoreImportReadError(err)})
			continue
		}
		book, err := importer.Import(localbook.ImportRequest{
			UserID:     userID,
			UserName:   userName,
			FileName:   filepath.Base(filepath.FromSlash(file.relativePath)),
			Extension:  file.extension,
			Data:       data,
			Title:      target.override.Title,
			Author:     target.override.Author,
			CategoryID: primaryCategoryID,
			TOCRule:    target.override.TOCRule,
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

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": importedBooks})
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (s *Server) previewLocalStoreImport(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)
	req, ok := decodeLocalStoreImportRequest(c)
	if !ok {
		return
	}
	plan, ok := s.prepareLocalStoreImport(c, *req)
	if !ok {
		return
	}
	defer plan.Close()
	results := make([]gin.H, 0)
	for _, target := range plan.targets {
		if target.override.ImportToken != "" {
			preview, importToken, err := s.reparseStagedStorageImport(c.Request.Context(), userID, target.override.ImportToken, target.override)
			if err != nil {
				if writeLocalImportStageCancellation(c, err, "") {
					return
				}
				results = append(results, gin.H{"path": target.relativePath, "error": err.Error(), "importToken": importToken})
				continue
			}
			results = append(results, gin.H{"path": target.relativePath, "book": preview, "importToken": importToken})
			continue
		}
		file := target.file
		if file.validationError != "" {
			results = append(results, gin.H{"path": file.relativePath, "error": file.validationError})
			continue
		}
		data, err := s.readBoundedStorageImport(c.Request.Context(), plan.service, file, "local-file-read")
		if err != nil {
			if writeStorageImportLifecycleError(c, err) {
				return
			}
			results = append(results, gin.H{"path": file.relativePath, "error": localStoreImportReadError(err)})
			continue
		}
		preview, importToken, err := s.previewStagedStorageImportData(
			c.Request.Context(),
			userID,
			filepath.Base(filepath.FromSlash(file.relativePath)),
			file.extension,
			data,
			target.override,
		)
		if err != nil {
			if writeLocalImportStageCancellation(c, err, "") {
				return
			}
			results = append(results, gin.H{"path": file.relativePath, "error": err.Error(), "importToken": importToken})
			continue
		}
		results = append(results, gin.H{"path": file.relativePath, "book": preview, "importToken": importToken})
	}
	c.JSON(http.StatusOK, gin.H{"items": results})
}

type localStoreImportFile struct {
	reader          *webdavfs.Reader
	filePath        string
	relativePath    string
	extension       string
	validationError string
}

func cleanLocalStoreName(c *gin.Context, value string) (string, bool) {
	name, err := normalizeLocalStoreName(value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return "", false
	}
	return name, true
}

func cleanRelativePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, sLocalStorePrefix(value))
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, "\\")
	if value == "" || value == "." {
		return ""
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func sLocalStorePrefix(value string) string {
	cleaned := filepath.Clean(value)
	if filepath.IsAbs(cleaned) {
		return filepath.VolumeName(cleaned)
	}
	return ""
}

func isImportableExtension(ext string) bool {
	switch ext {
	case ".txt", ".text", ".md", ".epub", ".pdf", ".umd", ".cbz":
		return true
	default:
		return false
	}
}
