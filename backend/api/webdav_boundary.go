package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/webdavfs"
)

const (
	maxWebDAVImportBodyBytes  int64 = 1 << 20
	maxWebDAVRestoreBodyBytes int64 = 16 << 10
	maxWebDAVImportItems            = 200
)

func decodeWebDAVJSON(c *gin.Context, target any, maxBytes int64, invalidMessage string) bool {
	if err := decodeBoundedSingleUTF8JSON(c, target, maxBytes); err != nil {
		if errors.Is(err, errJSONRequestTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
		}
		return false
	}
	return true
}

func decodeWebDAVImportRequest(c *gin.Context) (*localBookImportRequest, bool) {
	var request *localBookImportRequest
	if !decodeWebDAVJSON(c, &request, maxWebDAVImportBodyBytes, "paths is required") {
		return nil, false
	}
	if request == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return nil, false
	}
	if len(request.Paths) > 0 && len(request.Items) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid import request"})
		return nil, false
	}
	if len(request.Paths) > maxWebDAVImportItems ||
		len(request.Items) > maxWebDAVImportItems ||
		len(request.CategoryIDs) > maxWebDAVImportItems {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many paths"})
		return nil, false
	}
	if len(request.requestedPaths()) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return nil, false
	}
	return request, true
}

func (s *Server) webDAVReadService(c *gin.Context) (*webdavfs.Service, bool) {
	root, ok := s.storeRoot(c, s.webdavDir())
	if !ok {
		return nil, false
	}
	service, err := webdavfs.NewScoped(s.webdavDir(), root)
	if err != nil {
		writeWebDAVImportFilesystemError(c, err)
		return nil, false
	}
	return service, true
}

func writeWebDAVImportFilesystemError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, webdavfs.ErrUnsafePath),
		errors.Is(err, webdavfs.ErrNotDirectory):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access WebDAV"})
	}
}

func writeWebDAVImportPlanError(c *gin.Context, err error) {
	if errors.Is(err, errLocalStoreImportTooMany) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many paths"})
		return
	}
	writeWebDAVImportFilesystemError(c, err)
}

func (s *Server) prepareWebDAVImport(c *gin.Context, request localBookImportRequest) (localStoreImportPlan, bool) {
	rawPaths := request.requestedPaths()
	normalizedPaths := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		relativePath, err := webdavfs.NormalizeImportPath(rawPath)
		if err != nil {
			writeWebDAVImportFilesystemError(c, err)
			return localStoreImportPlan{}, false
		}
		normalizedPaths = append(normalizedPaths, relativePath)
	}

	overrides := make(map[string]localBookImportItem, len(request.Items))
	for _, item := range request.Items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		relativePath, err := webdavfs.NormalizeImportPath(strings.TrimSpace(item.Path))
		if err != nil {
			writeWebDAVImportFilesystemError(c, err)
			return localStoreImportPlan{}, false
		}
		item.Path = relativePath
		overrides[relativePath] = item
	}

	plan := localStoreImportPlan{targets: make([]localStoreImportTarget, 0, len(normalizedPaths))}
	seen := make(map[string]bool)
	for _, relativePath := range normalizedPaths {
		requestedOverride := overrides[relativePath]
		if requestedOverride.ImportToken != "" {
			if seen[relativePath] {
				continue
			}
			seen[relativePath] = true
			plan.targets = append(plan.targets, localStoreImportTarget{
				relativePath: relativePath,
				override:     requestedOverride,
			})
			continue
		}

		if plan.service == nil {
			service, ok := s.webDAVReadService(c)
			if !ok {
				return localStoreImportPlan{}, false
			}
			plan.service = service
		}
		files, err := s.localStoreImportFilesWithService(plan.service, relativePath)
		if err != nil {
			writeWebDAVImportPlanError(c, err)
			return localStoreImportPlan{}, false
		}
		for _, file := range files {
			if seen[file.relativePath] {
				continue
			}
			seen[file.relativePath] = true
			override := overrides[file.relativePath]
			override.ImportToken = ""
			plan.targets = append(plan.targets, localStoreImportTarget{
				relativePath: file.relativePath,
				override:     override,
				file:         file,
			})
			if len(plan.targets) > maxWebDAVImportItems {
				writeWebDAVImportPlanError(c, errLocalStoreImportTooMany)
				return localStoreImportPlan{}, false
			}
		}
	}
	return plan, true
}

func (s *Server) webDAVImportFiles(c *gin.Context, rawPath string) ([]localStoreImportFile, bool) {
	relativePath, err := webdavfs.NormalizeImportPath(rawPath)
	if err != nil {
		writeWebDAVImportFilesystemError(c, err)
		return nil, false
	}
	service, ok := s.webDAVReadService(c)
	if !ok {
		return nil, false
	}
	files, err := s.localStoreImportFilesWithService(service, relativePath)
	if err != nil {
		writeWebDAVImportPlanError(c, err)
		return nil, false
	}
	return files, true
}

func (s *Server) readBoundedWebDAVImport(service *webdavfs.Service, relativePath string) ([]byte, error) {
	if service == nil {
		return nil, errLocalStoreImportRead
	}
	file, _, err := service.Open(relativePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return s.readBoundedLocalImport(file)
}

func webDAVImportReadError(err error) string {
	if errors.Is(err, errLocalImportTooLarge) {
		return errLocalImportTooLarge.Error()
	}
	return "failed to read WebDAV import"
}

func writeWebDAVRestoreSourceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, webdavfs.ErrNotFound),
		errors.Is(err, webdavfs.ErrIsDirectory),
		errors.Is(err, webdavfs.ErrUnsafePath),
		errors.Is(err, webdavfs.ErrNotDirectory):
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "backup restore failed"})
	}
}

func webDAVImportFileName(relativePath string) string {
	return filepath.Base(filepath.FromSlash(relativePath))
}
