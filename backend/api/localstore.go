package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/services/localbook"
)

type localStoreItem struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Extension  string `json:"extension"`
	Size       int64  `json:"size"`
	IsDir      bool   `json:"isDir"`
	Importable bool   `json:"importable"`
}

func (s *Server) listLocalStore(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	targetDir, relativePath, ok := s.localStorePath(c, c.Query("path"))
	if !ok {
		return
	}
	recursive := c.Query("recursive") == "1" || strings.EqualFold(c.Query("recursive"), "true")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create local store"})
			return
		}
	}

	items := make([]localStoreItem, 0)
	if recursive {
		err := filepath.WalkDir(targetDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if path == targetDir {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			rel, err := filepath.Rel(targetDir, path)
			if err != nil {
				return nil
			}
			items = append(items, makeLocalStoreItem(entry.Name(), cleanRelativePath(filepath.Join(relativePath, rel)), info, entry.IsDir()))
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read local store"})
			return
		}
	} else {
		entries, err := os.ReadDir(targetDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read local store"})
			return
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			itemPath := cleanRelativePath(filepath.Join(relativePath, entry.Name()))
			items = append(items, makeLocalStoreItem(entry.Name(), itemPath, info, entry.IsDir()))
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Path) < strings.ToLower(items[j].Path)
	})

	c.JSON(http.StatusOK, gin.H{
		"path":      relativePath,
		"recursive": recursive,
		"items":     items,
	})
}

func makeLocalStoreItem(name string, itemPath string, info os.FileInfo, isDir bool) localStoreItem {
	ext := strings.ToLower(filepath.Ext(name))
	return localStoreItem{
		Name:       name,
		Path:       itemPath,
		Extension:  ext,
		Size:       info.Size(),
		IsDir:      isDir,
		Importable: !isDir && isImportableExtension(ext),
	}
}

func (s *Server) uploadToLocalStore(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	targetDir, _, ok := s.localStorePath(c, c.PostForm("path"))
	if !ok {
		return
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create directory"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	if file.Size > s.maxLocalImportBytes() {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": errLocalImportTooLarge.Error()})
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !isImportableExtension(ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open upload"})
		return
	}
	defer src.Close()

	dstPath, _, ok := s.localStorePath(c, filepath.Join(c.PostForm("path"), filepath.Base(file.Filename)))
	if !ok {
		return
	}
	dst, err := os.CreateTemp(filepath.Dir(dstPath), ".localstore-upload-")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}
	stagedPath := dst.Name()
	defer os.Remove(stagedPath)
	if err := s.copyBoundedLocalImport(dst, src); err != nil {
		_ = dst.Close()
		if errors.Is(err, errLocalImportTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}
	if err := dst.Chmod(0o644); err != nil {
		_ = dst.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}
	if err := dst.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}
	if err := os.Rename(stagedPath, dstPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"path": cleanRelativePath(filepath.Join(c.PostForm("path"), filepath.Base(file.Filename)))})
}

func (s *Server) downloadFromLocalStore(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	targetPath, relativePath, ok := s.localStorePath(c, c.Query("path"))
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot download local store root"})
		return
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "local store item not found"})
		return
	}
	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot download directory"})
		return
	}
	c.FileAttachment(targetPath, filepath.Base(relativePath))
}

func (s *Server) createLocalStoreDirectory(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	var req struct {
		Path string `json:"path"`
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "directory name is required"})
		return
	}
	name, ok := cleanLocalStoreName(c, req.Name)
	if !ok {
		return
	}
	targetDir, relativePath, ok := s.localStorePath(c, filepath.Join(req.Path, name))
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create parent directory"})
		return
	}
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "failed to create directory"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"path": relativePath})
}

func (s *Server) renameLocalStoreItem(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	var req struct {
		Path string `json:"path" binding:"required"`
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and name are required"})
		return
	}
	oldPath, relativePath, ok := s.localStorePath(c, req.Path)
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
	newPath, newRelativePath, ok := s.localStorePath(c, filepath.Join(filepath.Dir(relativePath), name))
	if !ok {
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "failed to rename local store item"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": newRelativePath})
}

func (s *Server) deleteFromLocalStore(c *gin.Context) {
	if !s.requireStoreAccess(c) {
		return
	}
	targetPath, relativePath, ok := s.localStorePath(c, c.Query("path"))
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete local store root"})
		return
	}
	if err := os.RemoveAll(targetPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete local store item"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) importFromLocalStore(c *gin.Context) {
	if !s.requireStoreAccess(c) {
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
		_, requestedPath, ok := s.localStorePath(c, rawPath)
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
			book, err := importer.Import(importRequest)
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
		files, ok := s.localStoreImportFiles(c, rawPath)
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

func (s *Server) previewLocalStoreImport(c *gin.Context) {
	if !s.requireStoreAccess(c) {
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
		_, requestedPath, ok := s.localStorePath(c, rawPath)
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

		files, ok := s.localStoreImportFiles(c, rawPath)
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

type localStoreImportFile struct {
	filePath        string
	relativePath    string
	extension       string
	validationError string
}

func (s *Server) localStoreImportFiles(c *gin.Context, rawPath string) ([]localStoreImportFile, bool) {
	filePath, relativePath, ok := s.localStorePath(c, rawPath)
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

func (s *Server) localStorePath(c *gin.Context, rawPath string) (string, string, bool) {
	storeRoot, ok := s.storeRoot(c, s.cfg.LocalStoreDir)
	if !ok {
		return "", "", false
	}
	storeDir, err := filepath.Abs(storeRoot)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid local store directory"})
		return "", "", false
	}
	relativePath := cleanRelativePath(rawPath)
	targetPath := filepath.Join(storeDir, relativePath)
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return "", "", false
	}
	if targetAbs != storeDir && !strings.HasPrefix(targetAbs, storeDir+string(os.PathSeparator)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path escapes local store"})
		return "", "", false
	}
	return targetAbs, relativePath, true
}

func cleanLocalStoreName(c *gin.Context, value string) (string, bool) {
	name := strings.TrimSpace(value)
	if name == "" || name == "." || name == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return "", false
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not contain path separators"})
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
