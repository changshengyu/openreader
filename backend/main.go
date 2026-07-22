package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"

	"openreader/backend/api"
	"openreader/backend/config"
	"openreader/backend/db"
	"openreader/backend/middleware"
	"openreader/backend/services/backup"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

func main() {
	cfg := config.Load()
	cleanupContext, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		log.Fatalf("create cache dir: %v", err)
	}
	if err := os.MkdirAll(cfg.LibraryDir, 0o755); err != nil {
		log.Fatalf("create library dir: %v", err)
	}

	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(database); err != nil {
		log.Fatalf("migrate database: %v", err)
	}
	if err := db.MigrateLocalBookCache(database, cfg); err != nil {
		log.Fatalf("migrate local book cache: %v", err)
	}

	hub := readersync.NewHub()
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

	log.Printf("OpenReader listening on %s", cfg.Address)
	if err := router.Run(cfg.Address); err != nil {
		log.Fatal(err)
	}
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
