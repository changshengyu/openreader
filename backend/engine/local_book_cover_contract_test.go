package engine

import (
	"archive/zip"
	"bytes"
	"testing"
)

// Contract: docs/compat/local-book-cover-fixed-baseline-second-audit-p1-contract.md

var (
	testJPEGCover = append([]byte{0xff, 0xd8, 0xff, 0xe0}, []byte("jpeg cover bytes")...)
	testPNGCover  = append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("png cover bytes")...)
	testGIFCover  = append([]byte("GIF89a"), []byte("gif cover bytes")...)
)

func coverTestEPUB(t *testing.T, metadata, manifest, guide string, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	write := func(name string, data []byte) {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	write("META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`))
	write("OEBPS/content.opf", []byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf"><dc:title>封面</dc:title>`+metadata+`</metadata>
  <manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>`+manifest+`</manifest>
  <spine><itemref idref="c1"/></spine>`+guide+`
</package>`))
	write("OEBPS/c1.xhtml", []byte(`<html><head><title>第一章</title></head><body><p>正文</p></body></html>`))
	for name, data := range files {
		write(name, data)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// epublib PackageDocumentReader.readCover: <meta name="cover" content="id or
// href"> and <guide><reference type="cover">; only JPG/PNG/GIF become the
// cover image (an XHTML candidate is a cover page, not an image).
func TestEPUBCoverImagePathFollowsEpublib(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata string
		manifest string
		guide    string
		files    map[string][]byte
		want     string
	}{
		{
			name:     "meta names a manifest id",
			metadata: `<meta name="cover" content="cover-img"/>`,
			manifest: `<item id="cover-img" href="images/cover.jpg" media-type="image/jpeg"/>`,
			files:    map[string][]byte{"OEBPS/images/cover.jpg": testJPEGCover},
			want:     "OEBPS/images/cover.jpg",
		},
		{
			name:     "meta content is an href",
			metadata: `<meta name="cover" content="images/cover.png"/>`,
			manifest: `<item id="art" href="images/cover.png" media-type="image/png"/>`,
			files:    map[string][]byte{"OEBPS/images/cover.png": testPNGCover},
			want:     "OEBPS/images/cover.png",
		},
		{
			name:     "guide cover image",
			manifest: `<item id="art" href="images/cover.gif" media-type="image/gif"/>`,
			guide:    `<guide><reference type="cover" title="封面" href="images/cover.gif"/></guide>`,
			files:    map[string][]byte{"OEBPS/images/cover.gif": testGIFCover},
			want:     "OEBPS/images/cover.gif",
		},
		{
			name:     "missing media type falls back to the extension",
			metadata: `<meta name="cover" content="cover-img"/>`,
			manifest: `<item id="cover-img" href="images/cover.jpeg"/>`,
			files:    map[string][]byte{"OEBPS/images/cover.jpeg": testJPEGCover},
			want:     "OEBPS/images/cover.jpeg",
		},
		{
			name:     "guide cover page is not an image",
			manifest: `<item id="page" href="cover.xhtml" media-type="application/xhtml+xml"/>`,
			guide:    `<guide><reference type="cover" href="cover.xhtml"/></guide>`,
			files:    map[string][]byte{"OEBPS/cover.xhtml": []byte(`<html><body><img src="images/cover.jpg"/></body></html>`)},
		},
		{
			name:     "svg cover is not a bitmap",
			metadata: `<meta name="cover" content="cover-img"/>`,
			manifest: `<item id="cover-img" href="images/cover.svg" media-type="image/svg+xml"/>`,
			files:    map[string][]byte{"OEBPS/images/cover.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
		},
		{
			name:     "webp cover is not a reader-dev bitmap",
			metadata: `<meta name="cover" content="cover-img"/>`,
			manifest: `<item id="cover-img" href="images/cover.webp" media-type="image/webp"/>`,
			files:    map[string][]byte{"OEBPS/images/cover.webp": []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")},
		},
		{
			name:     "EPUB3 cover-image property alone is ignored",
			manifest: `<item id="cover-img" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>`,
			files:    map[string][]byte{"OEBPS/images/cover.jpg": testJPEGCover},
		},
		{
			name:     "missing cover resource",
			metadata: `<meta name="cover" content="images/missing.jpg"/>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := coverTestEPUB(t, test.metadata, test.manifest, test.guide, test.files)
			got, err := EPUBCoverImagePath(data, DefaultLocalBookParseLimits())
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("cover path = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLocalBookCoverImageReadsEPUBAndCBZCovers(t *testing.T) {
	limits := DefaultLocalBookParseLimits()
	epub := coverTestEPUB(t, `<meta name="cover" content="cover-img"/>`, `<item id="cover-img" href="images/cover.jpg" media-type="image/jpeg"/>`, "",
		map[string][]byte{"OEBPS/images/cover.jpg": testJPEGCover})
	parsed, err := ParseEPUBCatalogWithLimits(epub, "", limits)
	if err != nil {
		t.Fatal(err)
	}
	contentType, image, ok := LocalBookCoverImage(".epub", epub, parsed, limits, 1<<20)
	if !ok || contentType != "image/jpeg" || !bytes.Equal(image, testJPEGCover) {
		t.Fatalf("EPUB cover = %q %q %v", contentType, image, ok)
	}
	if _, _, ok := LocalBookCoverImage(".epub", epub, parsed, limits, int64(len(testJPEGCover)-1)); ok {
		t.Fatal("an EPUB cover above the byte limit must be omitted")
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range []struct {
		name string
		data []byte
	}{{"pages/002.png", testPNGCover}, {"pages/001.jpg", testJPEGCover}} {
		file, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	cbz := buffer.Bytes()
	parsed, err = ParseCBZWithLimits(cbz, limits)
	if err != nil {
		t.Fatal(err)
	}
	contentType, image, ok = LocalBookCoverImage(".cbz", cbz, parsed, limits, 1<<20)
	if !ok || contentType != "image/png" || !bytes.Equal(image, testPNGCover) {
		t.Fatalf("CBZ cover must be the first archive image: %q %q %v", contentType, image, ok)
	}
}

// umdlib UmdReader: the additional section after segment 130 is the cover.
func TestUMDCoverImageReadsSegment130(t *testing.T) {
	chapters := []umdFixtureChapter{{title: "第一章", content: "正文"}}
	data := readerDevUMDFixtureWithCover(t, "封面 UMD", "作者", "", testPNGCover, chapters, 0)
	book, err := ParseUMDWithLimits(data, DefaultLocalBookParseLimits())
	if err != nil || book.Title != "封面 UMD" || len(book.Chapters) != 1 {
		t.Fatalf("a UMD with a cover must still parse: %+v %v", book, err)
	}
	contentType, image, ok := UMDCoverImage(data, 1<<20)
	if !ok || contentType != "image/png" || !bytes.Equal(image, testPNGCover) {
		t.Fatalf("UMD cover = %q %q %v", contentType, image, ok)
	}
	parsedType, parsedImage, ok := LocalBookCoverImage(".umd", data, book, DefaultLocalBookParseLimits(), 1<<20)
	if !ok || parsedType != "image/png" || !bytes.Equal(parsedImage, testPNGCover) {
		t.Fatalf("UMD local cover = %q %q %v", parsedType, parsedImage, ok)
	}
	if _, _, ok := UMDCoverImage(data, int64(len(testPNGCover)-1)); ok {
		t.Fatal("a UMD cover above the byte limit must be omitted")
	}
	if _, _, ok := UMDCoverImage(readerDevUMDFixture(t, "无封面", "作者", chapters, 0), 1<<20); ok {
		t.Fatal("a UMD without segment 130 has no cover")
	}
	if _, _, ok := UMDCoverImage(readerDevUMDFixtureWithCover(t, "坏封面", "作者", "", []byte("not an image"), chapters, 0), 1<<20); ok {
		t.Fatal("non-image cover bytes must not become a cover")
	}
}
