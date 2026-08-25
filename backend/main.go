package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"

	"openreader/backend/api"
	"openreader/backend/config"
	"openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/services/backup"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

func main() {
	if err := run(); err != nil {
		log.Printf("OpenReader stopped with error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	if err := configureSourceRuntime(cfg); err != nil {
		return fmt.Errorf("configure source network policy: %w", err)
	}
	router := gin.New()
	if err := configureTrustedProxies(router, cfg.TrustedProxies); err != nil {
		return fmt.Errorf("configure trusted proxies: %w", err)
	}
	cleanupContext, cleanupCancel := context.WithCancel(context.Background())
	defer log.Println("OpenReader cleanup completed")
	defer cleanupCancel()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if err := os.MkdirAll(cfg.LibraryDir, 0o755); err != nil {
		return fmt.Errorf("create library dir: %w", err)
	}

	database, err := db.Open(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if err := db.AutoMigrate(database); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if err := db.MigrateLocalBookCache(database, cfg); err != nil {
		return fmt.Errorf("migrate local book cache: %w", err)
	}

	hub := readersync.NewHub()
	defer hub.Close()
	api.StartLocalImportStageCleanup(cleanupContext, cfg.CacheDir)

	interval, err := time.ParseDuration(cfg.CheckInterval)
	if err != nil {
		log.Printf("invalid check interval %q, using 30m default", cfg.CheckInterval)
		interval = 30 * time.Minute
	}
	sched := scheduler.New(database, interval)
	sched.Start()
	defer sched.Stop()

	backupSvc := backup.New(database, filepath.Join(cfg.DataDir, "webdav"), cfg)
	backupSvc.Start()
	defer backupSvc.Stop()

	router.Use(middleware.AccessLogger(), gin.Recovery(), middleware.NewRateLimiter(cfg.RateLimitPerMinute, time.Minute).Middleware(), cors(cfg))

	api.RegisterRoutes(router, cfg, database, hub, sched, backupSvc)
	serveFrontend(router, cfg.PublicDir)

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Address, err)
	}
	server := newHTTPServer(cfg.Address, router)
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	log.Printf("OpenReader listening on %s", listener.Addr())
	if err := serveHTTPServer(server, listener, signals, httpShutdownTimeout, hub.Close); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	log.Println("OpenReader stopped")
	return nil
}

func configureTrustedProxies(router *gin.Engine, configured string) error {
	if strings.TrimSpace(configured) == "" {
		return router.SetTrustedProxies(nil)
	}

	items := strings.Split(configured, ",")
	proxies := make([]string, 0, len(items))
	for index, item := range items {
		proxy := strings.TrimSpace(item)
		if proxy == "" {
			return fmt.Errorf("entry %d is empty", index+1)
		}
		proxies = append(proxies, proxy)
	}
	if err := router.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("invalid IP or CIDR: %w", err)
	}
	return nil
}

func configureSourceRuntime(cfg config.Config) error {
	engine.ConfigureSourceFetchLimits(engine.SourceFetchLimits{
		Timeout:          time.Duration(cfg.SourceRequestTimeoutSeconds) * time.Second,
		MaxResponseBytes: cfg.MaxSourceResponseBytes,
		MaxRedirects:     cfg.MaxSourceRedirects,
		MaxRetries:       cfg.MaxSourceRetries,
	})
	if _, err := engine.ConfigureSourceNetworkPolicy(cfg.SourceNetworkAllowlist); err != nil {
		return err
	}
	return nil
}

func cors(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(cfg.CORSOrigin)
		if origin == "" {
			origin = strings.TrimSpace(c.GetHeader("Origin"))
		}
		if origin == "" {
			origin = "*"
		}

		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		if origin != "*" {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Depth, Destination, Overwrite, Timeout, Lock-Token, If")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, PROPFIND, MKCOL, MOVE, COPY, LOCK, UNLOCK")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "DAV, Allow, MS-Author-Via, Lock-Token, Content-Length")

		if c.Request.Method == http.MethodOptions {
			if isWebDAVProtocolPath(c.Request.URL.Path) {
				c.Next()
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func isWebDAVProtocolPath(path string) bool {
	return path == "/webdav" || strings.HasPrefix(path, "/webdav/") ||
		path == "/reader3/webdav" || strings.HasPrefix(path, "/reader3/webdav/")
}

func serveFrontend(router *gin.Engine, publicDir string) {
	router.HandleMethodNotAllowed = true
	router.NoMethod(frontendMethodNotAllowed)
	router.NoRoute(func(c *gin.Context) {
		frontendRouteNotFound(c)
	})

	index, _, err := openFrontendFile(publicDir, "index.html")
	if err != nil {
		return
	}
	_ = index.Close()

	assetsHandler := func(c *gin.Context) {
		name := strings.TrimPrefix(c.Param("filepath"), "/")
		if name == "" || !serveFrontendFile(c, publicDir, "assets/"+name) {
			frontendRouteNotFound(c)
		}
	}
	router.GET("/assets/*filepath", assetsHandler)
	router.HEAD("/assets/*filepath", assetsHandler)

	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			path := c.Request.URL.Path
			if isFrontendHistoryRoute(path) && serveFrontendFile(c, publicDir, "index.html") {
				return
			}
			if name, ok := frontendPublicFilename(path); ok && serveFrontendFile(c, publicDir, name) {
				return
			}
		}
		frontendRouteNotFound(c)
	})
}

func frontendRouteNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"error": gin.H{"code": "NOT_FOUND", "message": "route not found"},
	})
}

func frontendMethodNotAllowed(c *gin.Context) {
	c.JSON(http.StatusMethodNotAllowed, gin.H{
		"error": gin.H{"code": "METHOD_NOT_ALLOWED", "message": "method not allowed"},
	})
}

func isFrontendHistoryRoute(requestPath string) bool {
	switch requestPath {
	case "/", "/login", "/search", "/discover", "/local-store", "/sources",
		"/source-debug", "/bookSourceDebug", "/bookSourceDebug/", "/settings":
		return true
	}

	parts := strings.Split(strings.TrimPrefix(requestPath, "/"), "/")
	if len(parts) == 2 && parts[0] == "books" {
		return validFrontendRouteSegment(parts[1])
	}
	if len(parts) == 3 && parts[0] == "books" && parts[2] == "read" {
		return validFrontendRouteSegment(parts[1])
	}
	return len(parts) == 3 && parts[0] == "reader" && parts[1] == "remote" && validFrontendRouteSegment(parts[2])
}

func validFrontendRouteSegment(segment string) bool {
	return segment != "" && segment != "." && segment != ".." &&
		!strings.ContainsAny(segment, `/\`) && !strings.ContainsRune(segment, 0)
}

func frontendPublicFilename(requestPath string) (string, bool) {
	if !strings.HasPrefix(requestPath, "/") {
		return "", false
	}
	name := strings.TrimPrefix(requestPath, "/")
	if name == "" || !fs.ValidPath(name) || strings.ContainsRune(name, 0) || strings.Contains(name, `\`) {
		return "", false
	}
	rootSegment, _, _ := strings.Cut(name, "/")
	switch rootSegment {
	case "api", "ws", "webdav", "reader3", "uploads", "assets":
		return "", false
	}
	return name, true
}

func serveFrontendFile(c *gin.Context, publicDir, name string) bool {
	file, info, err := openFrontendFile(publicDir, name)
	if err != nil {
		return false
	}
	defer file.Close()

	if extension := strings.ToLower(filepath.Ext(name)); extension == ".webmanifest" {
		c.Header("Content-Type", "application/manifest+json")
	} else if contentType := mime.TypeByExtension(extension); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	http.ServeContent(c.Writer, c.Request, filepath.Base(name), info.ModTime(), file)
	return true
}

func openFrontendFile(publicDir, name string) (*os.File, os.FileInfo, error) {
	if name == "" || !fs.ValidPath(name) || strings.ContainsRune(name, 0) || strings.Contains(name, `\`) {
		return nil, nil, fs.ErrNotExist
	}

	root, err := os.OpenRoot(publicDir)
	if err != nil {
		return nil, nil, fs.ErrNotExist
	}
	defer root.Close()

	parts := strings.Split(name, "/")
	for index := range parts {
		current := strings.Join(parts[:index+1], "/")
		info, statErr := root.Lstat(current)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fs.ErrNotExist
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, nil, fs.ErrNotExist
		}
	}

	expected, err := root.Lstat(name)
	if err != nil || !expected.Mode().IsRegular() || expected.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fs.ErrNotExist
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, fs.ErrNotExist
	}
	actual, err := file.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		_ = file.Close()
		return nil, nil, fs.ErrNotExist
	}
	return file, actual, nil
}
