package main

import (
	"context"
	"fmt"
	"log"
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

	router := gin.New()
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
	indexPath := filepath.Join(publicDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	assetsDir := filepath.Join(publicDir, "assets")
	if _, err := os.Stat(assetsDir); err == nil {
		router.Static("/assets", assetsDir)
	}

	router.NoRoute(func(c *gin.Context) {
		c.File(indexPath)
	})
}
