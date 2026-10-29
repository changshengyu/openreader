package engine

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type upstreamNameAuthorFixtures struct {
	FileNames []struct {
		FileName     string `json:"fileName"`
		LocalName    string `json:"localName"`
		LocalAuthor  string `json:"localAuthor"`
		DirectName   string `json:"directName"`
		DirectAuthor string `json:"directAuthor"`
	} `json:"fileNames"`
	EPUBCreators []struct {
		Creator string  `json:"creator"`
		Author  *string `json:"author"`
	} `json:"epubCreators"`
}

func loadUpstreamNameAuthorFixtures(t *testing.T) upstreamNameAuthorFixtures {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "upstream_local_book_name_author", "upstream.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures upstreamNameAuthorFixtures
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures.FileNames) < 28 || len(fixtures.EPUBCreators) < 6 {
		t.Fatalf("fixture set is incomplete: %d file names, %d creators", len(fixtures.FileNames), len(fixtures.EPUBCreators))
	}
	return fixtures
}

// reader-dev splits a local book's name and author out of its file name
// (LocalBook.analyzeNameAuthor). Direct uploads first go through the
// importBookPreview normalization; LocalStore/WebDAV keep the original name.
func TestLocalBookNameAuthorMatchesReaderDev(t *testing.T) {
	fixtures := loadUpstreamNameAuthorFixtures(t)
	for _, fixture := range fixtures.FileNames {
		name, author := LocalBookNameAuthor(fixture.FileName, false)
		if name != fixture.LocalName || author != fixture.LocalAuthor {
			t.Errorf("storage %q = %q / %q, want %q / %q", fixture.FileName, name, author, fixture.LocalName, fixture.LocalAuthor)
		}
		name, author = LocalBookNameAuthor(fixture.FileName, true)
		if name != fixture.DirectName || author != fixture.DirectAuthor {
			t.Errorf("direct %q = %q / %q, want %q / %q", fixture.FileName, name, author, fixture.DirectName, fixture.DirectAuthor)
		}
	}
}

// Truncating a direct-upload name to 50 UTF-16 units must not keep half of
// a surrogate pair (reader-dev would); the rest of the rule is unchanged.
func TestLocalBookNameAuthorDirectTruncationKeepsWholeCharacters(t *testing.T) {
	prefix := strings.Repeat("书", 49)
	name, author := LocalBookNameAuthor(prefix+"😀尾 作者：甲.txt", true)
	if name != prefix || author != "" {
		t.Fatalf("direct name with a split surrogate = %q / %q, want %q / empty", name, author, prefix)
	}
	name, _ = LocalBookNameAuthor(strings.Repeat("书", 48)+"😀尾.txt", true)
	if name != strings.Repeat("书", 48)+"😀" {
		t.Fatalf("direct name ending on a whole surrogate pair = %q", name)
	}
}

func metadataTestEPUB(t *testing.T, metadata string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	writeZipFile(t, writer, "META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)
	writeZipFile(t, writer, "OEBPS/content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">`+metadata+`</metadata>
  <manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c1"/></spine>
</package>`)
	writeZipFile(t, writer, "OEBPS/c1.xhtml", `<html><head><title>第一章</title></head><body><p>正文</p></body></html>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// EpubFile.upBookInfo takes epublib's Author.toString() ("last, first",
// split at the last ASCII space) with a leading/trailing ", " removed.
func TestParseEPUBAuthorUsesReaderDevEpublibFormat(t *testing.T) {
	fixtures := loadUpstreamNameAuthorFixtures(t)
	for _, fixture := range fixtures.EPUBCreators {
		if fixture.Author == nil {
			continue
		}
		data := metadataTestEPUB(t, `<dc:title>作者格式</dc:title><dc:creator>`+fixture.Creator+`</dc:creator>`)
		for name, parse := range map[string]func([]byte, string, LocalBookParseLimits) (ParsedBook, error){
			"catalog": ParseEPUBCatalogWithLimits,
			"full":    ParseEPUBWithLimits,
		} {
			book, err := parse(data, "", DefaultLocalBookParseLimits())
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.TrimSpace(*fixture.Author); book.Author != want {
				t.Errorf("%s author for creator %q = %q, want %q", name, fixture.Creator, book.Author, want)
			}
		}
	}
}

// EpubFile.upBookInfo stores Jsoup.parse(descriptions[0]).text(): tags
// removed, entities decoded, whitespace collapsed.
func TestParseEPUBReadsFirstDescriptionAsPlainIntro(t *testing.T) {
	for _, test := range []struct {
		name        string
		description string
		want        string
	}{
		{name: "escaped html", description: `&lt;p&gt;第一段 &amp;amp; 简介&lt;/p&gt;&lt;p&gt;第二段&lt;br/&gt;第三行&lt;/p&gt;`, want: "第一段 & 简介 第二段 第三行"},
		{name: "plain text", description: "  纯文本\n\n   简介  ", want: "纯文本 简介"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := metadataTestEPUB(t, `<dc:title>简介</dc:title><dc:description>`+test.description+`</dc:description><dc:description>第二个简介</dc:description>`)
			for name, parse := range map[string]func([]byte, string, LocalBookParseLimits) (ParsedBook, error){
				"catalog": ParseEPUBCatalogWithLimits,
				"full":    ParseEPUBWithLimits,
			} {
				book, err := parse(data, "", DefaultLocalBookParseLimits())
				if err != nil {
					t.Fatal(err)
				}
				if book.Intro != test.want {
					t.Errorf("%s intro = %q, want %q", name, book.Intro, test.want)
				}
			}
		})
	}
}

// UmdReader.ReadSection type 7 is the book type that UmdFile.upBookInfo
// stores as Book.kind.
func TestParseUMDReadsReaderDevBookType(t *testing.T) {
	data := readerDevUMDFixtureWithKind(t, "类型 UMD", "测试作者", "都市言情", []umdFixtureChapter{
		{title: "第一章", content: "正文一"},
	}, 0)
	book, err := ParseUMDWithLimits(data, DefaultLocalBookParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "类型 UMD" || book.Author != "测试作者" || book.Kind != "都市言情" {
		t.Fatalf("UMD metadata = %q / %q / %q", book.Title, book.Author, book.Kind)
	}
}

// TextFile.getChapterList writes the automatically selected rule back to
// book.tocUrl; an explicit rule is kept as given and no match leaves it empty.
func TestParseTXTBookReportsTheEffectiveTOCRule(t *testing.T) {
	limits := DefaultLocalBookParseLimits()
	data := []byte("第1章 开端\n这里是开端的内容。\n第2章 尾声\n这里是尾声的内容。\n")
	book, err := ParseTXTBookWithLimits(data, "", limits)
	if err != nil {
		t.Fatal(err)
	}
	if want := DefaultTXTTocRules()[0].Rule; book.TOCRule != want || len(book.Chapters) != 2 {
		t.Fatalf("auto TOC rule = %q with %d chapters, want rule %q and two chapters", book.TOCRule, len(book.Chapters), want)
	}

	explicit := `^第.+章.*$`
	book, err = ParseTXTBookWithLimits(data, "  "+explicit+"  ", limits)
	if err != nil {
		t.Fatal(err)
	}
	if book.TOCRule != explicit || len(book.Chapters) != 2 {
		t.Fatalf("explicit TOC rule = %q with %d chapters", book.TOCRule, len(book.Chapters))
	}

	book, err = ParseTXTBookWithLimits([]byte("没有任何目录标题的一段正文。\n"), "", limits)
	if err != nil {
		t.Fatal(err)
	}
	if book.TOCRule != "" || len(book.Chapters) != 1 {
		t.Fatalf("no-match TOC rule = %q with %d chapters, want empty rule and one fallback chapter", book.TOCRule, len(book.Chapters))
	}
}
