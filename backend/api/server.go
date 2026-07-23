package api

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/middleware"
	"openreader/backend/services/audioreader"
	"openreader/backend/services/backup"
	"openreader/backend/services/bookgroups"
	"openreader/backend/services/cbzreader"
	"openreader/backend/services/chapterimage"
	"openreader/backend/services/epubreader"
	"openreader/backend/services/readingprogress"
	"openreader/backend/services/scheduler"
	"openreader/backend/services/sourcefailure"
	readersync "openreader/backend/sync"
)

type Server struct {
	cfg            config.Config
	db             *gorm.DB
	hub            *readersync.Hub
	scheduler      *scheduler.Scheduler
	backupSvc      *backup.Service
	bookGroups     *bookgroups.Service
	audioReader    *audioreader.Service
	cbzReader      *cbzreader.Service
	chapterImages  *chapterimage.Service
	epubReader     *epubreader.Service
	progressSvc    *readingprogress.Service
	sourceFailures *sourcefailure.Service
	remoteReaders  *remoteReaderSessionStore
	registerMu     sync.Mutex
}

func RegisterRoutes(router *gin.Engine, cfg config.Config, database *gorm.DB, hub *readersync.Hub, sched *scheduler.Scheduler, backupSvc *backup.Service) *Server {
	server := &Server{
		cfg:            cfg,
		db:             database,
		hub:            hub,
		scheduler:      sched,
		backupSvc:      backupSvc,
		bookGroups:     bookgroups.New(database),
		audioReader:    audioreader.New(cfg, database),
		cbzReader:      cbzreader.New(cfg, database),
		chapterImages:  chapterimage.New(cfg, database),
		epubReader:     epubreader.New(cfg, database),
		progressSvc:    readingprogress.New(database, cfg.DataDir),
		sourceFailures: sourcefailure.New(database),
		remoteReaders:  newRemoteReaderSessionStore(),
	}
	server.cleanupPortableAssetRestoreJournals()

	api := router.Group("/api")
	api.GET("/health", server.health)
	api.GET("/cbz-resource/:capability/*resourcePath", server.cbzResource)
	api.HEAD("/cbz-resource/:capability/*resourcePath", server.cbzResource)
	api.GET("/epub-resource/:capability/*resourcePath", server.epubResource)
	api.HEAD("/epub-resource/:capability/*resourcePath", server.epubResource)
	api.GET("/audio-resource/:capability/*resourcePath", server.audioResource)
	api.HEAD("/audio-resource/:capability/*resourcePath", server.audioResource)
	api.GET("/chapter-image/:capability", server.chapterImageResource)
	api.HEAD("/chapter-image/:capability", server.chapterImageResource)

	auth := api.Group("/auth")
	auth.POST("/register", server.register)
	auth.POST("/login", server.login)

	protected := api.Group("")
	protected.Use(middleware.AuthRequired(cfg.JWTSecret))
	protected.Use(middleware.TrackActivity(database))
	protected.GET("/me", server.me)
	protected.GET("/settings/:key", server.getUserSetting)
	protected.PUT("/settings/:key", server.updateUserSetting)
	protected.GET("/admin/users", server.listUsers)
	protected.POST("/admin/users", server.createUser)
	protected.POST("/admin/users/batch-delete", server.deleteUsers)
	protected.PUT("/admin/users/:id", server.updateUser)
	protected.PUT("/admin/users/:id/password", server.resetUserPassword)
	protected.POST("/admin/cleanup-inactive", server.cleanupInactiveUsers)
	protected.GET("/sources", server.listSources)
	protected.GET("/sources/invalid", server.listInvalidSources)
	protected.POST("/sources", server.createSource)
	protected.DELETE("/sources", server.clearSources)
	protected.GET("/sources/default", server.defaultSourcesStatus)
	protected.POST("/sources/default/save", server.saveDefaultSources)
	protected.POST("/sources/default/restore", server.restoreDefaultSources)
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
	protected.GET("/book-groups", server.listBookGroups)
	protected.PUT("/book-groups/reorder", server.reorderBookGroups)
	protected.PUT("/book-groups/:key", server.updateBuiltInBookGroup)
	protected.GET("/books", server.listBooks)
	protected.POST("/books", server.createBook)
	protected.POST("/books/remote", server.createRemoteBook)
	protected.POST("/reader/remote-sessions", server.createRemoteReaderSession)
	protected.GET("/reader/remote-sessions/:id", server.getRemoteReaderSession)
	protected.GET("/reader/remote-sessions/:id/chapters/:index/content", server.remoteReaderSessionChapterContent)
	protected.POST("/books/check-updates", server.checkUpdates)
	protected.POST("/books/batch", server.batchBooks)
	protected.POST("/books/export", server.exportBooks)
	protected.GET("/books/:id", server.getBook)
	protected.PUT("/books/:id", server.updateBook)
	protected.DELETE("/books/:id", server.deleteBook)
	protected.POST("/books/:id/refresh", server.refreshBook)
	protected.POST("/books/:id/refresh-local", server.refreshLocalBook)
	protected.POST("/books/:id/cache", server.cacheBookContent)
	protected.POST("/books/:id/cache/stream", server.cacheBookContentStream)
	protected.GET("/books/:id/source-candidates", server.listBookSourceCandidates)
	protected.PUT("/books/:id/category", server.updateBookCategory)
	protected.POST("/books/:id/change-source", server.changeBookSource)
	protected.GET("/books/:id/search", server.searchBookContent)
	protected.GET("/reader3/searchBookContent", server.legacySearchBookContent)
	protected.POST("/reader3/searchBookContent", server.legacySearchBookContent)
	protected.POST("/reader3/getInvalidBookSources", server.legacyInvalidBookSources)
	protected.GET("/books/:id/chapters", server.listChapters)
	protected.POST("/search", server.search)
	protected.GET("/books/:id/chapters/:index/content", server.chapterContent)
	protected.GET("/books/:id/bookmarks", server.listBookmarks)
	protected.POST("/books/:id/bookmarks", server.createBookmark)
	protected.POST("/books/:id/bookmarks/batch", server.createBookmarks)
	protected.POST("/books/:id/bookmarks/batch-delete", server.deleteBookmarks)
	protected.PUT("/bookmarks/:id", server.updateBookmark)
	protected.DELETE("/bookmarks/:id", server.deleteBookmark)
	protected.GET("/local-store", server.listLocalStore)
	protected.GET("/local-store/download", server.downloadFromLocalStore)
	protected.POST("/local-store/directory", server.createLocalStoreDirectory)
	protected.PUT("/local-store/rename", server.renameLocalStoreItem)
	protected.POST("/local-store/upload", server.uploadToLocalStore)
	protected.DELETE("/local-store", server.deleteFromLocalStore)
	protected.POST("/local-store/import-preview", server.previewLocalStoreImport)
	protected.POST("/local-store/import", server.importFromLocalStore)
	protected.GET("/txt-toc-rules", server.listTXTTocRules)
	protected.POST("/imports/books/preview", server.previewTXTImport)
	protected.POST("/imports/books", server.importTXT)
	protected.POST("/imports/txt", server.importTXT)
	protected.POST("/uploads", server.uploadAsset)
	protected.DELETE("/uploads", server.deleteAsset)
	protected.GET("/progress/:bookID", server.getProgress)
	protected.PUT("/progress", server.updateProgress)
	protected.GET("/cache/stats", server.cacheStats)
	protected.DELETE("/cache", server.clearCache)
	protected.GET("/replace-rules", server.listReplaceRules)
	protected.POST("/replace-rules", server.createReplaceRule)
	protected.POST("/replace-rules/batch", server.upsertReplaceRules)
	protected.POST("/replace-rules/batch-delete", server.deleteReplaceRules)
	protected.POST("/replace-rules/test", server.testReplaceRule)
	protected.PUT("/replace-rules/:id", server.updateReplaceRule)
	protected.DELETE("/replace-rules/:id", server.deleteReplaceRule)
	protected.GET("/rss/sources", server.listRSSSources)
	protected.POST("/rss/sources", server.createRSSSource)
	protected.PUT("/rss/sources/:id", server.updateRSSSource)
	protected.DELETE("/rss/sources/:id", server.deleteRSSSource)
	protected.POST("/rss/sources/:id/refresh", server.refreshRSSSource)
	protected.GET("/rss/articles", server.listRSSArticles)
	protected.GET("/rss/articles/:id/content", server.getRSSArticleContent)
	protected.PUT("/rss/articles/:id", server.updateRSSArticleState)
	protected.GET("/explore/sources", server.listExploreSources)
	protected.GET("/explore/:sourceId", server.exploreBooks)

	server.registerWebDAVRoutes(router, "/webdav")
	server.registerWebDAVRoutes(router, "/reader3/webdav")

	protected.POST("/backup/trigger", server.triggerBackup)
	protected.POST("/backup/portable/trigger", server.triggerPortableBackup)
	protected.GET("/backup/list", server.listBackups)
	protected.GET("/backup/download/:name", server.downloadBackup)
	protected.POST("/backup/restore-legado", server.importLegadoBackup)
	protected.POST("/backup/restore-webdav", server.restoreWebDAVBackup)
	protected.POST("/webdav/import-preview", server.previewWebDAVImport)
	protected.POST("/webdav/import", server.importFromWebDAV)

	router.GET("/ws/sync", server.syncSocket)

	uploadsDir := filepath.Join(cfg.DataDir, "uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err == nil {
		router.Static("/uploads", uploadsDir)
	}
	return server
}
