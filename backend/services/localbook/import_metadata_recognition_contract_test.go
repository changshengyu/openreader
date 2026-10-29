package localbook

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/models"
)

const readerDevUserNovelFileName = "前浪(校对加料版全本 1-425) 作者：刀一耕.txt"

var metadataTestTXT = []byte("第1章 开端\n这里是开端的内容。\n第2章 尾声\n这里是尾声的内容。\n")

func metadataImporter(t *testing.T) (Importer, *gorm.DB, models.User, config.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "metadata-recognition", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return NewImporter(cfg, database), database, user, cfg
}

func metadataRecognitionEPUB(t *testing.T, metadata string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	write := func(name, body string) {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("META-INF/container.xml", `<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`)
	write("OPS/content.opf", `<package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">`+metadata+`</metadata><manifest>
  <item id="one" href="one.xhtml" media-type="application/xhtml+xml"/>
</manifest><spine><itemref idref="one"/></spine></package>`)
	write("OPS/one.xhtml", `<html><head><title>第一章</title></head><body><p>正文。</p></body></html>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

// The user's own book: reader-dev imports it as 前浪(校对加料版全本 1-425) by
// 刀一耕 and shows the automatically selected TXT rule in the dialog.
func TestImporterPreviewSplitsReaderDevFileNameAndReportsTheTOCRule(t *testing.T) {
	preview, err := (Importer{}).Preview(ImportRequest{
		FileName: readerDevUserNovelFileName, Extension: ".txt", Data: metadataTestTXT, DirectUpload: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Title != "前浪(校对加料版全本 1-425)" || preview.Author != "刀一耕" {
		t.Fatalf("preview name/author = %q / %q", preview.Title, preview.Author)
	}
	if want := engine.DefaultTXTTocRules()[0].Rule; preview.TOCRule != want || preview.ChapterCount != 2 {
		t.Fatalf("preview TOC rule = %q with %d chapters, want %q", preview.TOCRule, preview.ChapterCount, want)
	}

	edited, err := (Importer{}).Preview(ImportRequest{
		FileName: readerDevUserNovelFileName, Extension: ".txt", Data: metadataTestTXT,
		Title: "用户书名", Author: "用户作者", TOCRule: `^第.+章.*$`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Title != "用户书名" || edited.Author != "用户作者" || edited.TOCRule != `^第.+章.*$` {
		t.Fatalf("edited preview = %q / %q / %q", edited.Title, edited.Author, edited.TOCRule)
	}

	for _, test := range []struct {
		direct bool
		title  string
	}{
		{direct: true, title: "三体第一部"},
		{direct: false, title: "三体.第一部"},
	} {
		preview, err := (Importer{}).Preview(ImportRequest{
			FileName: "三体.第一部 作者：刘慈欣.txt", Extension: ".txt", Data: metadataTestTXT, DirectUpload: test.direct,
		})
		if err != nil {
			t.Fatal(err)
		}
		if preview.Title != test.title || preview.Author != "刘慈欣" {
			t.Fatalf("direct=%v preview = %q / %q, want %q / 刘慈欣", test.direct, preview.Title, preview.Author, test.title)
		}
	}

	epub, err := (Importer{}).Preview(ImportRequest{
		FileName: "目录.epub", Extension: ".epub", Data: metadataRecognitionEPUB(t, `<dc:title>目录</dc:title>`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if epub.TOCRule != "spin+toc" {
		t.Fatalf("EPUB preview TOC rule = %q, want spin+toc", epub.TOCRule)
	}
}

func TestImporterPersistsReaderDevLocalBookMetadata(t *testing.T) {
	importer, _, user, _ := metadataImporter(t)

	book, err := importer.Import(ImportRequest{
		UserID: user.ID, UserName: user.Username, FileName: readerDevUserNovelFileName, Extension: ".txt",
		Data: metadataTestTXT, DirectUpload: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "前浪(校对加料版全本 1-425)" || book.Author != "刀一耕" || book.TOCRule != engine.DefaultTXTTocRules()[0].Rule {
		t.Fatalf("TXT book = %q / %q / %q", book.Title, book.Author, book.TOCRule)
	}

	book, err = importer.Import(ImportRequest{
		UserID: user.ID, UserName: user.Username, FileName: "随便.epub", Extension: ".epub", TOCRule: "spin+toc",
		Data: metadataRecognitionEPUB(t, `<dc:title>元数据书名</dc:title><dc:creator>John Smith</dc:creator><dc:description>&lt;p&gt;第一段 &amp;amp; 简介&lt;/p&gt;</dc:description>`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "元数据书名" || book.Author != "Smith, John" || book.Intro != "第一段 & 简介" {
		t.Fatalf("EPUB book = %q / %q / %q", book.Title, book.Author, book.Intro)
	}

	book, err = importer.Import(ImportRequest{
		UserID: user.ID, UserName: user.Username, FileName: "斗罗大陆 唐家三少 著.epub", Extension: ".epub", TOCRule: "spin+toc",
		Data: metadataRecognitionEPUB(t, `<dc:title>斗罗大陆</dc:title>`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "斗罗大陆" || book.Author != "唐家三少" {
		t.Fatalf("EPUB without creator = %q / %q, want the file-name author", book.Title, book.Author)
	}

	for _, test := range []struct {
		fileName string
		parsed   engine.ParsedBook
		title    string
		author   string
		kind     string
	}{
		{
			fileName: "UMD小说 作者：甲.umd",
			parsed:   engine.ParsedBook{Title: "头部书名", Author: "头部作者", Kind: "都市言情"},
			title:    "头部书名", author: "头部作者", kind: "都市言情",
		},
		{
			fileName: "UMD小说 作者：甲.umd",
			parsed:   engine.ParsedBook{},
			title:    "UMD小说", author: "甲",
		},
	} {
		test.parsed.Chapters = []engine.TXTChapter{{Index: 0, Title: "第一章", Content: "正文"}}
		request := ImportRequest{
			UserID: user.ID, UserName: user.Username, FileName: test.fileName, Extension: ".umd",
			Data: []byte(test.title + "|" + test.kind),
		}
		book, err := importer.ImportPrepared(request, NewPreparedImport(request, test.parsed))
		if err != nil {
			t.Fatal(err)
		}
		if book.Title != test.title || book.Author != test.author || book.Kind != test.kind {
			t.Fatalf("UMD book = %q / %q / %q, want %q / %q / %q", book.Title, book.Author, book.Kind, test.title, test.author, test.kind)
		}
	}
}

// reader-dev logical backups restore local books as name+author placeholders;
// re-uploading the original file must reattach by the same derived identity.
func TestImporterReattachesReaderDevPlaceholderByFileNameDerivedIdentity(t *testing.T) {
	importer, database, user, _ := metadataImporter(t)
	placeholder := models.Book{
		UserID: user.ID, SourceID: 0, Title: "前浪(校对加料版全本 1-425)", Author: "刀一耕",
		URL: "storage/data/default/前浪.txt", Intro: "备份简介",
	}
	if err := database.Create(&placeholder).Error; err != nil {
		t.Fatal(err)
	}
	book, err := importer.Import(ImportRequest{
		UserID: user.ID, UserName: user.Username, FileName: readerDevUserNovelFileName, Extension: ".txt",
		Data: metadataTestTXT, DirectUpload: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if book.ID != placeholder.ID || book.OriginalFile == "" || book.Intro != "备份简介" {
		t.Fatalf("reattached book = %+v, want placeholder %d with its intro", book, placeholder.ID)
	}
}
