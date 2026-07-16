// create-old-volume-fixture writes a deliberately old OpenReader mounted
// volume for the local Docker release smoke. It is a test tool, not a runtime
// migration: the generated database is closed with the current local-book
// columns removed before the release image opens it.
package main

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
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
	fixtureUsername      = "legacy_owner"
	fixturePassword      = "legacy-volume-secret"
	fixtureOtherUsername = "legacy_other"
	fixtureOtherPassword = "legacy-other-volume-secret"
)

type historicalArchiveFixture struct {
	title            string
	directory        string
	filename         string
	tocRule          string
	archive          []byte
	relativeOriginal bool
	cachePath        string
	cacheContent     string
}

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

	fixtures, err := historicalArchiveFixtures()
	if err != nil {
		log.Fatalf("build archive fixtures: %v", err)
	}
	for _, fixture := range fixtures {
		libraryPath := filepath.Join("data", fixtureUsername, fixture.directory)
		archivePath := filepath.Join(cfg.LibraryDir, libraryPath, fixture.filename)
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			log.Fatalf("create %s archive directory: %v", fixture.title, err)
		}
		if err := os.WriteFile(archivePath, fixture.archive, 0o644); err != nil {
			log.Fatalf("write %s archive: %v", fixture.title, err)
		}
		if err := writeFixtureMetadata(filepath.Dir(archivePath), fixture.title); err != nil {
			log.Fatalf("write %s metadata: %v", fixture.title, err)
		}

		// The smoke mounts this directory at /retired-host. A vulnerable release
		// would prefer these readable absolute paths over the current library root.
		retiredSource := filepath.Join(rootPath, "retired-host", libraryPath, fixture.filename)
		retiredCache := filepath.Join(rootPath, "retired-host", "cache", fixture.directory+".txt")
		for path, content := range map[string]string{
			retiredSource: "绝不能读取 retired host source: " + fixture.title,
			retiredCache:  "绝不能读取 retired host cache: " + fixture.title,
		} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				log.Fatalf("create %s retired-host fixture: %v", fixture.title, err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				log.Fatalf("write %s retired-host fixture: %v", fixture.title, err)
			}
		}

		originalFile := filepath.Join("/retired-host", libraryPath, fixture.filename)
		if fixture.relativeOriginal {
			// Older TXT imports commonly retained a portable relative field, while
			// the binary formats below exercise stale host-absolute rebasing.
			originalFile = filepath.Join(libraryPath, fixture.filename)
		}
		chapterCachePath := filepath.Join("/retired-host", "cache", fixture.directory+".txt")
		if fixture.cachePath != "" {
			chapterCachePath = fixture.cachePath
			legacyCachePath := filepath.Join(cfg.CacheDir, fixture.cachePath)
			if err := os.MkdirAll(filepath.Dir(legacyCachePath), 0o755); err != nil {
				log.Fatalf("create %s legacy cache directory: %v", fixture.title, err)
			}
			if err := os.WriteFile(legacyCachePath, []byte(fixture.cacheContent), 0o644); err != nil {
				log.Fatalf("write %s legacy cache: %v", fixture.title, err)
			}
		}
		book := models.Book{
			UserID:       user.ID,
			SourceID:     0,
			Title:        fixture.title,
			Author:       "OpenReader 旧卷夹具",
			URL:          "local://" + fixture.directory,
			LibraryPath:  libraryPath,
			OriginalFile: originalFile,
			TOCFile:      filepath.Join(libraryPath, "chapters.json"),
			SourceFile:   filepath.Join(libraryPath, "bookSource.json"),
			TOCRule:      fixture.tocRule,
			LastChapter:  "旧目录",
			ChapterCount: 1,
			CanUpdate:    true,
		}
		if err := database.Create(&book).Error; err != nil {
			log.Fatalf("create %s fixture book: %v", fixture.title, err)
		}
		chapter := models.Chapter{
			BookID:    book.ID,
			Index:     0,
			Title:     "旧目录",
			URL:       book.URL + "/chapter_0",
			CachePath: chapterCachePath,
		}
		if err := database.Create(&chapter).Error; err != nil {
			log.Fatalf("create %s fixture chapter: %v", fixture.title, err)
		}
		if fixture.directory != "old-volume-txt" {
			continue
		}
		if err := database.Create(&models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 6, Percent: 0.3, ChapterTitle: chapter.Title}).Error; err != nil {
			log.Fatalf("create fixture progress: %v", err)
		}
		if err := database.Create(&models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 3, Percent: 0.1, Title: "旧卷书签", Excerpt: "旧卷"}).Error; err != nil {
			log.Fatalf("create fixture bookmark: %v", err)
		}
	}

	otherPasswordHash, err := bcrypt.GenerateFromPassword([]byte(fixtureOtherPassword), bcrypt.MinCost)
	if err != nil {
		log.Fatalf("hash other fixture password: %v", err)
	}
	otherUser := models.User{Username: fixtureOtherUsername, PasswordHash: string(otherPasswordHash)}
	if err := database.Create(&otherUser).Error; err != nil {
		log.Fatalf("create other fixture user: %v", err)
	}
	otherLibraryPath := filepath.Join("data", fixtureOtherUsername, "old-volume-other")
	otherArchivePath := filepath.Join(cfg.LibraryDir, otherLibraryPath, "legacy.txt")
	otherArchiveContent := "第一章\n用户 B 的旧卷正文必须保持私有。\n"
	if err := os.MkdirAll(filepath.Dir(otherArchivePath), 0o755); err != nil {
		log.Fatalf("create other archive directory: %v", err)
	}
	if err := os.WriteFile(otherArchivePath, []byte(otherArchiveContent), 0o644); err != nil {
		log.Fatalf("write other archive: %v", err)
	}
	if err := writeFixtureMetadata(filepath.Dir(otherArchivePath), "旧卷 用户B隔离验证书"); err != nil {
		log.Fatalf("write other archive metadata: %v", err)
	}
	otherBook := models.Book{
		UserID:       otherUser.ID,
		SourceID:     0,
		Title:        "旧卷 用户B隔离验证书",
		Author:       "OpenReader 旧卷夹具",
		URL:          "local://old-volume-other",
		LibraryPath:  otherLibraryPath,
		OriginalFile: filepath.Join(otherLibraryPath, "legacy.txt"),
		TOCFile:      filepath.Join(otherLibraryPath, "chapters.json"),
		SourceFile:   filepath.Join(otherLibraryPath, "bookSource.json"),
		TOCRule:      `^第.+章.*$`,
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := database.Create(&otherBook).Error; err != nil {
		log.Fatalf("create other fixture book: %v", err)
	}
	if err := database.Create(&models.Chapter{
		BookID:    otherBook.ID,
		Index:     0,
		Title:     "第一章",
		URL:       otherBook.URL + "/chapter_0",
		CachePath: filepath.Join("content", "missing.txt"),
	}).Error; err != nil {
		log.Fatalf("create other fixture chapter: %v", err)
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

	fmt.Printf("created old mounted-volume fixture: owner=%s archives=txt,epub,umd,cbz,relative-cache other=%s\n", fixtureUsername, fixtureOtherUsername)
}

func writeFixtureMetadata(directory, title string) error {
	chapters, err := json.Marshal([]map[string]any{{"title": "旧目录", "index": 0}})
	if err != nil {
		return err
	}
	source, err := json.Marshal([]map[string]string{{"name": title}})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "chapters.json"), append(chapters, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "bookSource.json"), append(source, '\n'), 0o644)
}

func historicalArchiveFixtures() ([]historicalArchiveFixture, error) {
	epub, err := historicalEPUBArchive()
	if err != nil {
		return nil, err
	}
	umd, err := historicalReaderDevUMDArchive()
	if err != nil {
		return nil, err
	}
	cbz, err := historicalCBZArchive()
	if err != nil {
		return nil, err
	}
	return []historicalArchiveFixture{
		{
			title:            "旧卷 TXT 验证书",
			directory:        "old-volume-txt",
			filename:         "legacy.txt",
			tocRule:          `^第.+章.*$`,
			archive:          []byte("第一章\n旧卷归档正文只能从 library 读取。\n"),
			relativeOriginal: true,
		},
		{
			title:     "旧卷 EPUB 验证书",
			directory: "old-volume-epub",
			filename:  "legacy.epub",
			tocRule:   "toc",
			archive:   epub,
		},
		{
			title:     "旧卷 UMD 验证书",
			directory: "old-volume-umd",
			filename:  "legacy.umd",
			archive:   umd,
		},
		{
			title:     "旧卷 CBZ 验证书",
			directory: "old-volume-cbz",
			filename:  "legacy.cbz",
			archive:   cbz,
		},
		{
			title:            "旧卷 相对缓存验证书",
			directory:        "old-volume-relative-cache",
			filename:         "legacy.txt",
			tocRule:          `^第.+章.*$`,
			archive:          []byte("第一章\narchive 回退正文，不应覆盖旧 cache。\n"),
			relativeOriginal: true,
			cachePath:        filepath.Join("legacy-cache", "chapter.txt"),
			cacheContent:     "历史相对 cache 正文必须优先于 archive。",
		},
	}, nil
}

func historicalEPUBArchive() ([]byte, error) {
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	write := func(name, content string) error {
		file, err := writer.Create(name)
		if err != nil {
			return err
		}
		_, err = file.Write([]byte(content))
		return err
	}
	for _, entry := range []struct{ name, content string }{
		{"META-INF/container.xml", `<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`},
		{"OPS/content.opf", `<package><metadata><title>旧卷 EPUB</title></metadata><manifest><item id="one" href="one.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="one"/></spine></package>`},
		{"OPS/one.xhtml", `<html><body><h1>旧卷 EPUB 第一章</h1><p>旧卷 EPUB archive 正文。</p></body></html>`},
	} {
		if err := write(entry.name, entry.content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func historicalCBZArchive() ([]byte, error) {
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	write := func(name, content string) error {
		file, err := writer.Create(name)
		if err != nil {
			return err
		}
		_, err = file.Write([]byte(content))
		return err
	}
	for _, entry := range []struct{ name, content string }{
		{"ComicInfo.xml", `<ComicInfo><Title>旧卷 CBZ</Title><Writer>OpenReader</Writer></ComicInfo>`},
		{"pages/002.png", "old-volume-second-page"},
		{"pages/001.jpg", "old-volume-first-page"},
	} {
		if err := write(entry.name, entry.content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func historicalReaderDevUMDArchive() ([]byte, error) {
	type chapter struct {
		title   string
		content string
	}
	chapters := []chapter{
		{title: "第一章", content: "第一段\u2029第二段"},
		{title: "第二章", content: "第二章正文"},
	}
	utf16le := func(value string) ([]byte, error) {
		encoded := make([]byte, 0, len(value)*2)
		for _, unit := range value {
			if unit > 0xffff {
				return nil, fmt.Errorf("historical UMD fixture only supports BMP runes: %q", value)
			}
			encoded = append(encoded, byte(unit), byte(unit>>8))
		}
		return encoded, nil
	}

	var result bytes.Buffer
	writeSection := func(segmentType uint16, flag byte, payload []byte) error {
		if len(payload)+5 > 0xff {
			return fmt.Errorf("historical UMD section payload too large: %d", len(payload))
		}
		result.WriteByte('#')
		var number [2]byte
		binary.LittleEndian.PutUint16(number[:], segmentType)
		result.Write(number[:])
		result.WriteByte(flag)
		result.WriteByte(byte(len(payload) + 5))
		_, err := result.Write(payload)
		return err
	}
	writeAdditional := func(check uint32, payload []byte) error {
		result.WriteByte('$')
		var number [4]byte
		binary.LittleEndian.PutUint32(number[:], check)
		result.Write(number[:])
		binary.LittleEndian.PutUint32(number[:], uint32(len(payload)+9))
		result.Write(number[:])
		_, err := result.Write(payload)
		return err
	}
	uint32Payload := func(value uint32) []byte {
		var payload [4]byte
		binary.LittleEndian.PutUint32(payload[:], value)
		return payload[:]
	}

	result.Write([]byte{0x89, 0x9b, 0x9a, 0xde})
	if err := writeSection(0x01, 0, []byte{0x01, 0x11, 0x22}); err != nil {
		return nil, err
	}
	for _, value := range []string{"上游 UMD 导入", "导入作者"} {
		encoded, err := utf16le(value)
		if err != nil {
			return nil, err
		}
		segmentType := uint16(0x02)
		if value == "导入作者" {
			segmentType = 0x03
		}
		if err := writeSection(segmentType, 0, encoded); err != nil {
			return nil, err
		}
	}

	var contents bytes.Buffer
	offsets := make([]uint32, 0, len(chapters))
	titles := make([]byte, 0)
	for _, chapter := range chapters {
		offsets = append(offsets, uint32(contents.Len()))
		content, err := utf16le(chapter.content)
		if err != nil {
			return nil, err
		}
		contents.Write(content)
		title, err := utf16le(chapter.title)
		if err != nil {
			return nil, err
		}
		titles = append(titles, byte(len(title)))
		titles = append(titles, title...)
	}
	if err := writeSection(0x0b, 0, uint32Payload(uint32(contents.Len()))); err != nil {
		return nil, err
	}
	const offsetCheck uint32 = 0x11223344
	if err := writeSection(0x83, 0, uint32Payload(offsetCheck)); err != nil {
		return nil, err
	}
	offsetPayload := make([]byte, len(offsets)*4)
	for index, offset := range offsets {
		binary.LittleEndian.PutUint32(offsetPayload[index*4:], offset)
	}
	if err := writeAdditional(offsetCheck, offsetPayload); err != nil {
		return nil, err
	}

	const titleCheck uint32 = 0x55667788
	if err := writeSection(0x84, 1, uint32Payload(titleCheck)); err != nil {
		return nil, err
	}
	if err := writeAdditional(titleCheck, titles); err != nil {
		return nil, err
	}
	var compressed bytes.Buffer
	compressor := zlib.NewWriter(&compressed)
	if _, err := compressor.Write(contents.Bytes()); err != nil {
		return nil, err
	}
	if err := compressor.Close(); err != nil {
		return nil, err
	}
	const chunkCheck uint32 = 0x99aabbcc
	if err := writeAdditional(chunkCheck, compressed.Bytes()); err != nil {
		return nil, err
	}
	if err := writeSection(0x00f1, 0, make([]byte, 16)); err != nil {
		return nil, err
	}
	if err := writeSection(0x0081, 1, make([]byte, 4)); err != nil {
		return nil, err
	}
	if err := writeAdditional(0, uint32Payload(chunkCheck)); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}
