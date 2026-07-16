// create-old-volume-fixture writes a deliberately old OpenReader mounted
// volume for the local Docker release smoke. It is a test tool, not a runtime
// migration: the generated database is closed with the current local-book
// columns removed before the release image opens it.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

const (
	fixtureUsername  = "legacy_owner"
	fixturePassword  = "legacy-volume-secret"
	fixtureBookTitle = "旧卷 TXT 验证书"
)

func main() {
	root := flag.String("root", "", "mounted-volume root containing data, cache, and library")
	flag.Parse()
	if *root == "" {
		log.Fatal("-root is required")
	}
	rootPath, err := filepath.Abs(*root)
	if err != nil {
		log.Fatalf("resolve root: %v", err)
	}
	cfg := config.Config{
		DataDir:       filepath.Join(rootPath, "data"),
		CacheDir:      filepath.Join(rootPath, "cache"),
		LibraryDir:    filepath.Join(rootPath, "library"),
		DatabasePath:  filepath.Join(rootPath, "data", "openreader.db"),
		LocalStoreDir: filepath.Join(rootPath, "library", "localStore"),
	}
	for _, directory := range []string{cfg.DataDir, cfg.CacheDir, cfg.LibraryDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			log.Fatalf("create fixture directory: %v", err)
		}
	}
	if _, err := os.Stat(cfg.DatabasePath); err == nil {
		log.Fatalf("refusing to replace existing fixture database %q", cfg.DatabasePath)
	} else if !os.IsNotExist(err) {
		log.Fatalf("inspect fixture database: %v", err)
	}

	database, err := readerdb.Open(cfg)
	if err != nil {
		log.Fatalf("open fixture database: %v", err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		log.Fatalf("create fixture schema: %v", err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(fixturePassword), bcrypt.MinCost)
	if err != nil {
		log.Fatalf("hash fixture password: %v", err)
	}
	user := models.User{Username: fixtureUsername, PasswordHash: string(passwordHash)}
	if err := database.Create(&user).Error; err != nil {
		log.Fatalf("create fixture user: %v", err)
	}

	libraryPath := filepath.Join("data", fixtureUsername, "old-volume-txt")
	archivePath := filepath.Join(cfg.LibraryDir, libraryPath, "legacy.txt")
	archiveContent := "第一章\n旧卷归档正文只能从 library 读取。\n"
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		log.Fatalf("create archive directory: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte(archiveContent), 0o644); err != nil {
		log.Fatalf("write archive: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(archivePath), "chapters.json"), []byte("[{\"title\":\"第一章\",\"index\":0}]\n"), 0o644); err != nil {
		log.Fatalf("write chapter metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(archivePath), "bookSource.json"), []byte("[{\"name\":\"旧卷 TXT 验证书\"}]\n"), 0o644); err != nil {
		log.Fatalf("write source metadata: %v", err)
	}

	// The smoke mounts this directory at /retired-host. A vulnerable release
	// would prefer these readable absolute paths over the current library root.
	retiredSource := filepath.Join(rootPath, "retired-host", libraryPath, "legacy.txt")
	retiredCache := filepath.Join(rootPath, "retired-host", "cache", "chapter.txt")
	for path, content := range map[string]string{
		retiredSource: "绝不能读取 retired host source",
		retiredCache:  "绝不能读取 retired host cache",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			log.Fatalf("create retired-host fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			log.Fatalf("write retired-host fixture: %v", err)
		}
	}

	book := models.Book{
		UserID:       user.ID,
		SourceID:     0,
		Title:        fixtureBookTitle,
		Author:       "OpenReader 旧卷夹具",
		URL:          "local://old-volume-txt",
		LibraryPath:  libraryPath,
		OriginalFile: filepath.Join("/retired-host", libraryPath, "legacy.txt"),
		TOCFile:      filepath.Join(libraryPath, "chapters.json"),
		SourceFile:   filepath.Join(libraryPath, "bookSource.json"),
		TOCRule:      `^第.+章.*$`,
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := database.Create(&book).Error; err != nil {
		log.Fatalf("create fixture book: %v", err)
	}
	chapter := models.Chapter{
		BookID:    book.ID,
		Index:     0,
		Title:     "第一章",
		URL:       book.URL + "/chapter_0",
		CachePath: "/retired-host/cache/chapter.txt",
	}
	if err := database.Create(&chapter).Error; err != nil {
		log.Fatalf("create fixture chapter: %v", err)
	}
	if err := database.Create(&models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 6, Percent: 0.3, ChapterTitle: chapter.Title}).Error; err != nil {
		log.Fatalf("create fixture progress: %v", err)
	}
	if err := database.Create(&models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 3, Percent: 0.1, Title: "旧卷书签", Excerpt: "旧卷"}).Error; err != nil {
		log.Fatalf("create fixture bookmark: %v", err)
	}

	for _, field := range []string{"ResourcePath", "ResourceFragment", "ResourceEndFragment", "Variable"} {
		if err := database.Migrator().DropColumn(&models.Chapter{}, field); err != nil {
			log.Fatalf("downgrade fixture chapter column %s: %v", field, err)
		}
	}
	if err := database.Migrator().DropColumn(&models.Book{}, "Variable"); err != nil {
		log.Fatalf("downgrade fixture book column: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		log.Fatalf("unwrap fixture database: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		log.Fatalf("close fixture database: %v", err)
	}

	fmt.Printf("created old mounted-volume fixture: user=%s password=%s title=%s\n", fixtureUsername, fixturePassword, fixtureBookTitle)
}
