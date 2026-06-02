package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) uploadAsset(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	kind := strings.TrimSpace(c.PostForm("type"))
	if fileHeader.Size > uploadSizeLimit(kind) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is too large"})
		return
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedUploadExtension(kind, ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
		return
	}

	dir := filepath.Join(s.cfg.DataDir, "uploads", uploadKindDir(kind))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload directory"})
		return
	}
	name := time.Now().Format("20060102150405") + "-" + randomHex(6) + ext
	target := filepath.Join(dir, name)
	if err := c.SaveUploadedFile(fileHeader, target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save upload"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"url":  "/uploads/" + uploadKindDir(kind) + "/" + name,
		"name": fileHeader.Filename,
		"size": fileHeader.Size,
		"type": uploadKindDir(kind),
	})
}

func (s *Server) deleteAsset(c *gin.Context) {
	var payload struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || strings.TrimSpace(payload.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	target, err := s.uploadAssetPath(payload.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported upload url"})
		return
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete upload"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) uploadAssetPath(url string) (string, error) {
	cleanURL := strings.TrimSpace(url)
	if strings.HasPrefix(cleanURL, "http://") || strings.HasPrefix(cleanURL, "https://") || strings.HasPrefix(cleanURL, "//") {
		return "", os.ErrPermission
	}
	if !strings.HasPrefix(cleanURL, "/uploads/") {
		return "", os.ErrPermission
	}
	uploadsRoot := filepath.Join(s.cfg.DataDir, "uploads")
	rel := filepath.Clean(strings.TrimPrefix(cleanURL, "/uploads/"))
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", os.ErrPermission
	}
	target := filepath.Join(uploadsRoot, rel)
	if relative, err := filepath.Rel(uploadsRoot, target); err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return "", os.ErrPermission
	}
	return target, nil
}

func uploadSizeLimit(kind string) int64 {
	if kind == "font" {
		return 32 * 1024 * 1024
	}
	return 8 * 1024 * 1024
}

func uploadKindDir(kind string) string {
	switch kind {
	case "cover":
		return "covers"
	case "background":
		return "backgrounds"
	case "font":
		return "fonts"
	default:
		return "misc"
	}
}

func allowedUploadExtension(kind, ext string) bool {
	imageExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	fontExts := map[string]bool{".ttf": true, ".otf": true, ".woff": true, ".woff2": true}
	switch kind {
	case "cover", "background":
		return imageExts[ext]
	case "font":
		return fontExts[ext]
	default:
		return imageExts[ext] || fontExts[ext]
	}
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	return hex.EncodeToString(buf)
}
