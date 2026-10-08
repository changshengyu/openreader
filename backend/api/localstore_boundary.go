package api

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"openreader/backend/services/webdavfs"
)

const (
	localStoreUploadEnvelopeBytes  int64 = 1 << 20
	maxLocalStoreUploadFiles             = 64
	maxLocalStorePathBytes               = 4096
	maxLocalStoreNameBytes               = 255
	maxLocalStoreMetadataBodyBytes int64 = 16 << 10
	maxLocalStoreImportBodyBytes   int64 = 1 << 20
	maxLocalStoreImportItems             = 200
)

var (
	errLocalStoreUploadRequestTooLarge = errors.New("local store upload request body too large")
	errLocalStoreUploadRequestInvalid  = errors.New("invalid local store upload request")
	errLocalStoreUploadFileRequired    = errors.New("local store upload file required")
	errLocalStorePathInvalid           = errors.New("invalid local store path")
	errLocalStoreImportTooMany         = errors.New("too many local store import items")
	errLocalStoreImportRead            = errors.New("failed to read local store import")
)

type parsedLocalStoreUpload struct {
	form  *multipart.Form
	path  string
	files []*multipart.FileHeader
}

type localStoreImportTarget struct {
	relativePath string
	override     localBookImportItem
	file         localStoreImportFile
}

type localStoreImportPlan struct {
	service *webdavfs.Service
	scope   *webdavfs.ReadScope
	targets []localStoreImportTarget
}

func (p *localStoreImportPlan) Close() {
	for _, target := range p.targets {
		if target.file.reader != nil {
			_ = target.file.reader.Close()
		}
	}
	if p.scope != nil {
		_ = p.scope.Close()
	}
}

// Nonparallel observation seam at actual source planning/read boundaries.
// Nil in production; it adds no identity or cancellation checks.
var storageImportSourceReadTestHook func(stage, root, relative string)

type observedStorageImportReader struct {
	io.Reader
	ctx      context.Context
	service  *webdavfs.Service
	relative string
}

func (r observedStorageImportReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.Reader.Read(p)
	if n > 0 {
		runStorageImportSourceReadTestHook("source-read", r.service, r.relative)
	}
	if canceled := r.ctx.Err(); canceled != nil {
		return 0, canceled
	}
	return n, err
}

func runStorageImportSourceReadTestHook(stage string, service *webdavfs.Service, relative string) {
	if storageImportSourceReadTestHook != nil {
		storageImportSourceReadTestHook(stage, service.Root(), relative)
	}
}

func (s *Server) parseLocalStoreUpload(c *gin.Context) (*parsedLocalStoreUpload, error) {
	requestLimit := s.maxLocalImportBytes() + localStoreUploadEnvelopeBytes
	if requestLimit < s.maxLocalImportBytes() {
		requestLimit = int64(^uint64(0) >> 1)
	}
	if c.Request.ContentLength > requestLimit {
		return nil, errLocalStoreUploadRequestTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, requestLimit)
	form, err := c.MultipartForm()
	if form == nil {
		form = c.Request.MultipartForm
	}
	upload := &parsedLocalStoreUpload{form: form}
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return upload, errLocalStoreUploadRequestTooLarge
		}
		if errors.Is(err, http.ErrNotMultipart) {
			return upload, errLocalStoreUploadFileRequired
		}
		return upload, errLocalStoreUploadRequestInvalid
	}
	if form == nil {
		return upload, errLocalStoreUploadRequestInvalid
	}

	files := form.File["file"]
	if len(files) == 0 {
		return upload, errLocalStoreUploadFileRequired
	}
	fileCount := 0
	for field, headers := range form.File {
		fileCount += len(headers)
		if field != "file" && len(headers) > 0 {
			return upload, errLocalStoreUploadRequestInvalid
		}
	}
	if fileCount != len(files) || len(files) > maxLocalStoreUploadFiles {
		return upload, errLocalStoreUploadRequestInvalid
	}

	for field, values := range form.Value {
		if field != "path" && len(values) > 0 {
			return upload, errLocalStoreUploadRequestInvalid
		}
	}
	paths := form.Value["path"]
	if len(paths) > 1 {
		return upload, errLocalStoreUploadRequestInvalid
	}
	path := ""
	if len(paths) == 1 {
		var pathErr error
		path, pathErr = normalizeLocalStorePath(paths[0])
		if pathErr != nil {
			return upload, errLocalStoreUploadRequestInvalid
		}
	}
	for _, file := range files {
		name, nameErr := normalizeLocalStoreName(file.Filename)
		if nameErr != nil {
			return upload, errLocalStoreUploadRequestInvalid
		}
		file.Filename = name
	}
	upload.path = path
	upload.files = files
	return upload, nil
}

func writeLocalStoreUploadRequestError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errLocalStoreUploadRequestTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "local store upload request is too large"})
	case errors.Is(err, errLocalStoreUploadFileRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload request"})
	}
}

func normalizeLocalStoreName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || name == "." || name == ".." ||
		!utf8.ValidString(name) || len(name) > maxLocalStoreNameBytes ||
		strings.ContainsRune(name, '\x00') || hasWindowsPathVolume(name) {
		return "", errLocalStorePathInvalid
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return "", errLocalStorePathInvalid
	}
	return name, nil
}

func normalizeLocalStorePath(value string) (string, error) {
	if !utf8.ValidString(value) || len(value) > maxLocalStorePathBytes || strings.ContainsRune(value, '\x00') {
		return "", errLocalStorePathInvalid
	}
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if strings.HasPrefix(value, "//") {
		return "", errLocalStorePathInvalid
	}
	value = strings.TrimPrefix(value, "/")
	if hasWindowsPathVolume(value) {
		return "", errLocalStorePathInvalid
	}
	if value == "" || value == "." {
		return "", nil
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", errLocalStorePathInvalid
		}
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", errLocalStorePathInvalid
	}
	return cleaned, nil
}

func hasWindowsPathVolume(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	first := value[0]
	return first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z'
}

func (s *Server) localStoreFileService(c *gin.Context) (*webdavfs.Service, bool) {
	root, ok := s.storeRoot(c, s.cfg.LocalStoreDir)
	if !ok {
		return nil, false
	}
	service, err := webdavfs.NewScoped(s.cfg.LocalStoreDir, root)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to access local store")
		return nil, false
	}
	if err := service.EnsureRootContext(c.Request.Context()); err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to create local store")
		return nil, false
	}
	return service, true
}

func writeLocalStoreFilesystemError(c *gin.Context, err error, internalMessage string) {
	switch {
	case errors.Is(err, webdavfs.ErrUnsafePath),
		errors.Is(err, webdavfs.ErrNotDirectory),
		errors.Is(err, errLocalStorePathInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": internalMessage})
	}
}

func decodeLocalStoreJSON(c *gin.Context, target any, maxBytes int64, invalidMessage string) bool {
	if err := decodeBoundedSingleJSON(c, target, maxBytes); err != nil {
		if errors.Is(err, errJSONRequestTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
		}
		return false
	}
	return true
}

func decodeLocalStoreImportRequest(c *gin.Context) (*localBookImportRequest, bool) {
	var request *localBookImportRequest
	if !decodeLocalStoreJSON(c, &request, maxLocalStoreImportBodyBytes, "paths is required") {
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
	if len(request.Paths) > maxLocalStoreImportItems ||
		len(request.Items) > maxLocalStoreImportItems ||
		len(request.CategoryIDs) > maxLocalStoreImportItems {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many paths"})
		return nil, false
	}
	if len(request.requestedPaths()) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return nil, false
	}
	return request, true
}

func (s *Server) prepareLocalStoreImport(c *gin.Context, request localBookImportRequest) (localStoreImportPlan, bool) {
	rawPaths := request.requestedPaths()
	normalizedPaths := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		relativePath, err := normalizeLocalStorePath(rawPath)
		if err != nil {
			writeLocalStoreFilesystemError(c, err, "failed to access local store")
			return localStoreImportPlan{}, false
		}
		normalizedPaths = append(normalizedPaths, relativePath)
	}

	overrides := make(map[string]localBookImportItem, len(request.Items))
	for _, item := range request.Items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		relativePath, err := normalizeLocalStorePath(item.Path)
		if err != nil {
			writeLocalStoreFilesystemError(c, err, "failed to access local store")
			return localStoreImportPlan{}, false
		}
		item.Path = relativePath
		overrides[relativePath] = item
	}

	plan := localStoreImportPlan{targets: make([]localStoreImportTarget, 0, len(normalizedPaths))}
	complete := false
	defer func() {
		if !complete {
			plan.Close()
		}
	}()
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
			service, ok := s.localStoreFileService(c)
			if !ok {
				return localStoreImportPlan{}, false
			}
			plan.service = service
		}
		files, err := plan.collectImportFiles(c.Request.Context(), relativePath, seen)
		if err != nil {
			writeLocalStoreImportPlanError(c, err)
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
			if len(plan.targets) > maxLocalStoreImportItems {
				writeLocalStoreImportPlanError(c, errLocalStoreImportTooMany)
				return localStoreImportPlan{}, false
			}
		}
	}
	complete = true
	return plan, true
}

// Metadata-only compatibility helper. Live handlers retain the owned readers
// through collectImportFiles instead of reopening these paths after planning.
func (s *Server) localStoreImportFilesWithService(service *webdavfs.Service, relativePath string) ([]localStoreImportFile, error) {
	plan := localStoreImportPlan{service: service}
	defer plan.Close()
	files, err := plan.collectImportFiles(context.Background(), relativePath, nil)
	for i := range files {
		_ = files[i].reader.Close()
		files[i].reader = nil
	}
	return files, err
}

func (plan *localStoreImportPlan) collectImportFiles(ctx context.Context, relativePath string, seen map[string]bool) ([]localStoreImportFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if plan.scope == nil {
		scope, err := plan.service.AdmitReadScope(ctx)
		if errors.Is(err, webdavfs.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		plan.scope = scope
		runStorageImportSourceReadTestHook("source-admission", plan.service, relativePath)
	}
	reader, err := plan.scope.Admit(relativePath)
	if errors.Is(err, webdavfs.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	resource := reader.Resource
	if !resource.Info.IsDir() {
		if seen[resource.RelativePath] {
			_ = reader.Close()
			return nil, nil
		}
		extension := strings.ToLower(filepath.Ext(resource.RelativePath))
		file := localStoreImportFile{relativePath: resource.RelativePath, extension: extension, reader: reader}
		if !isImportableExtension(extension) {
			file.validationError = "unsupported file type"
		}
		return []localStoreImportFile{file}, nil
	}

	defer reader.Close()
	files := make([]localStoreImportFile, 0)
	err = reader.WalkFiles(func(resource webdavfs.Resource) bool {
		return !seen[resource.RelativePath] && isImportableExtension(strings.ToLower(filepath.Ext(resource.RelativePath)))
	}, func(child *webdavfs.Reader) error {
		if len(plan.targets)+len(files) >= maxLocalStoreImportItems {
			return errLocalStoreImportTooMany
		}
		files = append(files, localStoreImportFile{
			relativePath: child.Resource.RelativePath,
			extension:    strings.ToLower(filepath.Ext(child.Resource.RelativePath)),
			reader:       child,
		})
		return nil
	}, func(relative string) {
		runStorageImportSourceReadTestHook("directory-scan", plan.service, relative)
	})
	if err != nil {
		for _, file := range files {
			_ = file.reader.Close()
		}
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i].relativePath) < strings.ToLower(files[j].relativePath)
	})
	return files, nil
}

func (s *Server) readBoundedStorageImport(ctx context.Context, service *webdavfs.Service, source localStoreImportFile, stage string) ([]byte, error) {
	if service == nil || source.reader == nil {
		return nil, errLocalStoreImportRead
	}
	defer source.reader.Close()
	runStorageImportSourceReadTestHook(stage, service, source.relativePath)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, _, err := source.reader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := s.readBoundedLocalImport(observedStorageImportReader{Reader: file, ctx: ctx, service: service, relative: source.relativePath})
	if err != nil {
		return nil, err
	}
	runStorageImportSourceReadTestHook("source-handoff", service, source.relativePath)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := source.reader.ValidateAncestors(); err != nil {
		return nil, err
	}
	return data, nil
}

func writeStorageImportLifecycleError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import source read canceled"})
	case errors.Is(err, webdavfs.ErrUnsafePath), errors.Is(err, webdavfs.ErrNotDirectory):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
	default:
		return false
	}
	return true
}

func writeLocalStoreImportPlanError(c *gin.Context, err error) {
	if writeStorageImportLifecycleError(c, err) {
		return
	}
	if errors.Is(err, errLocalStoreImportTooMany) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many paths"})
		return
	}
	writeLocalStoreFilesystemError(c, err, "failed to access local store")
}

func localStoreImportReadError(err error) string {
	if errors.Is(err, errLocalImportTooLarge) {
		return errLocalImportTooLarge.Error()
	}
	return errLocalStoreImportRead.Error()
}
