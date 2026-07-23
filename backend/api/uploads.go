package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
	assetservice "openreader/backend/services/assets"
)

func (s *Server) uploadAsset(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
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
	input, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file content does not match type"})
		return
	}
	validationErr := assetservice.ValidateUpload(input, fileHeader.Size, kind, ext)
	_ = input.Close()
	if validationErr != nil {
		if errors.Is(validationErr, assetservice.ErrImageDimensions) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image dimensions are too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "file content does not match type"})
		return
	}

	kindDir := uploadKindDir(kind)
	dir := filepath.Join(s.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(userID), 10), kindDir)
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
		"url":  fmt.Sprintf("/uploads/users/%d/%s/%s", userID, kindDir, name),
		"name": fileHeader.Filename,
		"size": fileHeader.Size,
		"type": kindDir,
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
	userID, _ := middleware.UserID(c)
	asset, err := s.userUploadAsset(payload.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported upload url"})
		return
	}
	if asset.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "upload not found"})
		return
	}
	if referenced, err := s.userUploadAssetReferenced(userID, asset.URL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check upload references"})
		return
	} else if referenced {
		c.JSON(http.StatusConflict, gin.H{"error": "upload is still in use"})
		return
	}
	if err := os.Remove(asset.Path); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete upload"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

type userUploadAsset struct {
	UserID uint
	Kind   string
	URL    string
	Path   string
}

func (s *Server) userUploadAsset(rawURL string) (userUploadAsset, error) {
	cleanURL := strings.TrimSpace(rawURL)
	if strings.ContainsAny(cleanURL, "?#") || !strings.HasPrefix(cleanURL, "/uploads/users/") {
		return userUploadAsset{}, os.ErrPermission
	}
	parts := strings.Split(strings.TrimPrefix(cleanURL, "/uploads/"), "/")
	if len(parts) != 4 || parts[0] != "users" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return userUploadAsset{}, os.ErrPermission
	}
	ownerID, err := strconv.ParseUint(parts[1], 10, 0)
	if err != nil || ownerID == 0 {
		return userUploadAsset{}, os.ErrPermission
	}
	kind := parts[2]
	if !isUploadKindDir(kind) || parts[3] != filepath.Base(parts[3]) || strings.Contains(parts[3], `\\`) {
		return userUploadAsset{}, os.ErrPermission
	}
	uploadsRoot := filepath.Join(s.cfg.DataDir, "uploads")
	target := filepath.Join(uploadsRoot, "users", parts[1], kind, parts[3])
	if relative, err := filepath.Rel(uploadsRoot, target); err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return userUploadAsset{}, os.ErrPermission
	}
	return userUploadAsset{UserID: uint(ownerID), Kind: kind, URL: cleanURL, Path: target}, nil
}

func (s *Server) userUploadAssetReferenced(userID uint, url string) (bool, error) {
	var count int64
	if err := s.db.Model(&models.Book{}).
		Where("user_id = ? AND custom_cover_url = ?", userID, url).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	escapedURL := strings.NewReplacer(`\\`, `\\\\`, `%`, `\\%`, `_`, `\\_`).Replace(url)
	if err := s.db.Model(&models.UserSetting{}).
		Where("user_id = ? AND value LIKE ? ESCAPE '\\'", userID, "%"+escapedURL+"%").
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func uploadSizeLimit(kind string) int64 {
	return assetservice.SizeLimitForKind(kind)
}

func uploadKindDir(kind string) string {
	return assetservice.KindDirectory(kind)
}

func isUploadKindDir(kind string) bool {
	return assetservice.IsKindDirectory(kind)
}

func allowedUploadExtension(kind, ext string) bool {
	return assetservice.AllowedExtension(kind, ext)
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	return hex.EncodeToString(buf)
}
