package api

import (
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/middleware"
	"openreader/backend/services/backup"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

type Server struct {
	cfg       config.Config
	db        *gorm.DB
	hub       *readersync.Hub
	scheduler *scheduler.Scheduler
	backupSvc *backup.Service
}

func RegisterRoutes(router *gin.Engine, cfg config.Config, database *gorm.DB, hub *readersync.Hub, sched *scheduler.Scheduler, backupSvc *backup.Service) {
	server := &Server{cfg: cfg, db: database, hub: hub, scheduler: sched, backupSvc: backupSvc}

	api := router.Group("/api")
	api.GET("/health", server.health)

	auth := api.Group("/auth")
	auth.POST("/register", server.register)
	auth.POST("/login", server.login)

	protected := api.Group("")
	protected.Use(middleware.AuthRequired(cfg.JWTSecret))
	protected.Use(middleware.TrackActivity(database))
	protected.GET("/me", server.me)
	protected.GET("/admin/users", server.listUsers)
	protected.PUT("/admin/users/:id", server.updateUser)
	protected.POST("/admin/cleanup-inactive", server.cleanupInactiveUsers)
	protected.GET("/sources", server.listSources)
	protected.POST("/sources", server.createSource)
	protected.DELETE("/sources", server.clearSources)
	protected.POST("/sources/batch", server.batchSources)
	protected.POST("/sources/import", server.importSources)
	protected.GET("/sources/export", server.exportSources)
	protected.POST("/sources/remote", server.importRemoteSource)
	protected.POST("/sources/remote-preview", server.previewRemoteSource)
	protected.POST("/sources/batch-test", server.batchTestSources)
	protected.GET("/sources/:id", server.getSource)
	protected.PUT("/sources/:id", server.updateSource)
	protected.DELETE("/sources/:id", server.deleteSource)
	protected.POST("/sources/:id/test", server.testSourceSearch)
	protected.POST("/sources/:id/test-chapter", server.testSourceChapter)
	protected.POST("/sources/:id/test-content", server.testSourceContent)
	protected.GET("/categories", server.listCategories)
	protected.POST("/categories", server.createCategory)
	protected.PUT("/categories/reorder", server.reorderCategories)
	protected.PUT("/categories/:id", server.updateCategory)
	protected.DELETE("/categories/:id", server.deleteCategory)
	protected.GET("/books", server.listBooks)
	protected.POST("/books", server.createBook)
	protected.POST("/books/remote", server.createRemoteBook)
	protected.POST("/books/check-updates", server.checkUpdates)
	protected.POST("/books/batch", server.batchBooks)
	protected.POST("/books/export", server.exportBooks)
	protected.GET("/books/:id", server.getBook)
	protected.PUT("/books/:id", server.updateBook)
	protected.DELETE("/books/:id", server.deleteBook)
	protected.POST("/books/:id/refresh", server.refreshBook)
	protected.POST("/books/:id/cache", server.cacheBookContent)
	protected.GET("/books/:id/source-candidates", server.listBookSourceCandidates)
	protected.PUT("/books/:id/category", server.updateBookCategory)
	protected.POST("/books/:id/change-source", server.changeBookSource)
	protected.GET("/books/:id/search", server.searchBookContent)
	protected.GET("/books/:id/chapters", server.listChapters)
	protected.POST("/search", server.search)
	protected.GET("/books/:id/chapters/:index/content", server.chapterContent)
	protected.GET("/books/:id/bookmarks", server.listBookmarks)
	protected.POST("/books/:id/bookmarks", server.createBookmark)
	protected.PUT("/bookmarks/:id", server.updateBookmark)
	protected.DELETE("/bookmarks/:id", server.deleteBookmark)
	protected.GET("/local-store", server.listLocalStore)
	protected.POST("/local-store/directory", server.createLocalStoreDirectory)
	protected.PUT("/local-store/rename", server.renameLocalStoreItem)
	protected.POST("/local-store/upload", server.uploadToLocalStore)
	protected.DELETE("/local-store", server.deleteFromLocalStore)
	protected.POST("/local-store/import", server.importFromLocalStore)
	protected.POST("/imports/books", server.importTXT)
	protected.POST("/imports/txt", server.importTXT)
	protected.POST("/uploads", server.uploadAsset)
	protected.GET("/progress/:bookID", server.getProgress)
	protected.PUT("/progress", server.updateProgress)
	protected.GET("/cache/stats", server.cacheStats)
	protected.DELETE("/cache", server.clearCache)
	protected.GET("/replace-rules", server.listReplaceRules)
	protected.POST("/replace-rules", server.createReplaceRule)
	protected.POST("/replace-rules/test", server.testReplaceRule)
	protected.PUT("/replace-rules/:id", server.updateReplaceRule)
	protected.DELETE("/replace-rules/:id", server.deleteReplaceRule)
	protected.GET("/rss/sources", server.listRSSSources)
	protected.POST("/rss/sources", server.createRSSSource)
	protected.PUT("/rss/sources/:id", server.updateRSSSource)
	protected.DELETE("/rss/sources/:id", server.deleteRSSSource)
	protected.POST("/rss/sources/:id/refresh", server.refreshRSSSource)
	protected.GET("/rss/articles", server.listRSSArticles)
	protected.PUT("/rss/articles/:id", server.updateRSSArticleState)
	protected.GET("/explore/sources", server.listExploreSources)
	protected.GET("/explore/:sourceId", server.exploreBooks)

	webdav := router.Group("/webdav")
	webdav.GET("/*path", server.webdavGetOrList)
	webdav.PUT("/*path", server.webdavPut)
	webdav.Handle("MKCOL", "/*path", server.webdavMkcol)
	webdav.Handle("MOVE", "/*path", server.webdavMove)
	webdav.DELETE("/*path", server.webdavDelete)

	protected.POST("/backup/trigger", server.triggerBackup)
	protected.GET("/backup/list", server.listBackups)
	protected.GET("/backup/download/:name", server.downloadBackup)
	protected.POST("/backup/restore-legado", server.importLegadoBackup)
	protected.POST("/backup/restore-webdav", server.restoreWebDAVBackup)

	router.GET("/ws/sync", server.syncSocket)

	uploadsDir := filepath.Join(cfg.DataDir, "uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err == nil {
		router.Static("/uploads", uploadsDir)
	}
}
