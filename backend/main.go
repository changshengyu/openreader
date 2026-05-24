package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

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

	interval, err := time.ParseDuration(cfg.CheckInterval)
	if err != nil {
		log.Printf("invalid check interval %q, using 30m default", cfg.CheckInterval)
		interval = 30 * time.Minute
	}
	sched := scheduler.New(database, interval)
	sched.Start()
	defer sched.Stop()

	backupSvc := backup.New(database, filepath.Join(cfg.DataDir, "webdav"))
	backupSvc.Start()
	defer backupSvc.Stop()

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), middleware.NewRateLimiter(cfg.RateLimitPerMinute, time.Minute).Middleware(), cors(cfg))

	api.RegisterRoutes(router, cfg, database, hub, sched, backupSvc)
	serveFrontend(router, cfg.PublicDir)

	log.Printf("OpenReader listening on %s", cfg.Address)
	if err := router.Run(cfg.Address); err != nil {
		log.Fatal(err)
	}
}

func cors(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := cfg.CORSOrigin
		if origin == "" {
			origin = c.GetHeader("Origin")
		}
		if origin == "" {
			origin = "*"
		}

		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
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
