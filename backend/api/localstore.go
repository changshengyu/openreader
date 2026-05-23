package api

import (
	"io"
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
	dst, err := os.Create(dstPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"path": cleanRelativePath(filepath.Join(c.PostForm("path"), filepath.Base(file.Filename)))})
}

func (s *Server) createLocalStoreDirectory(c *gin.Context) {
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
	userID, _ := middleware.UserID(c)

	var req struct {
		Paths      []string `json:"paths" binding:"required"`
		CategoryID *uint    `json:"categoryId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	if !s.validateCategory(c, userID, req.CategoryID) {
		return
	}

	userName, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}

	importer := localbook.NewImporter(s.cfg, s.db)
	imported := make([]gin.H, 0)

	for _, rawPath := range req.Paths {
		filePath, relativePath, ok := s.localStorePath(c, rawPath)
		if !ok {
			continue
		}

		info, err := os.Stat(filePath)
		if err != nil || info.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		if !isImportableExtension(ext) {
			imported = append(imported, gin.H{"path": relativePath, "error": "unsupported file type"})
			continue
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			imported = append(imported, gin.H{"path": relativePath, "error": err.Error()})
			continue
		}

		book, err := importer.Import(localbook.ImportRequest{
			UserID:     userID,
			UserName:   userName,
			FileName:   filepath.Base(filePath),
			Extension:  ext,
			Data:       data,
			CategoryID: req.CategoryID,
		})
		if err != nil {
			imported = append(imported, gin.H{"path": relativePath, "error": err.Error()})
			continue
		}
		imported = append(imported, gin.H{"path": relativePath, "book": book})
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (s *Server) localStorePath(c *gin.Context, rawPath string) (string, string, bool) {
	storeDir, err := filepath.Abs(s.cfg.LocalStoreDir)
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
	case ".txt", ".text", ".md", ".epub", ".pdf", ".umd":
		return true
	default:
		return false
	}
}
