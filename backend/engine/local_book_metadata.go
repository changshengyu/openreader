package engine

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// readerDevDirectUploadNameUnits is the length, in UTF-16 code units, that
// reader-dev BookController.importBookPreview keeps of an uploaded file name.
const readerDevDirectUploadNameUnits = 50

var (
	// LocalBook.analyzeNameAuthor patterns, tried in order with find().
	localBookNameAuthorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(.*?)《([^《》]+)》.*?作者：(.*)`),
		regexp.MustCompile(`(.*?)《([^《》]+)》(.*)`),
		regexp.MustCompile(`(^)(.+) 作者：(.+)$`),
		regexp.MustCompile(`(^)(.+) by (.+)$`),
	}
	// AppPattern.nameRegex, authorRegex and fileNameRegex.
	localBookNameSuffixPattern  = regexp.MustCompile(`\s+作\s*者.*|\s+\S+\s+著`)
	localBookAuthorAffixPattern = regexp.MustCompile(`^\s*作\s*者[:：\s]+|\s+著`)
	localBookFileNameCharacters = regexp.MustCompile(`[\\/:*?"<>|.]`)
	epubAuthorSeparatorPattern  = regexp.MustCompile(`^, |, $`)
)

// LocalBookNameAuthor derives a local book's name and author from its file
// name the way reader-dev does. Direct uploads first go through
// BookController.importBookPreview, which strips \/:*?"<>|. from the name
// and keeps its first 50 UTF-16 units; LocalStore and WebDAV imports
// (importFromLocalPathPreview) use the original name. The name is then split
// by LocalBook.analyzeNameAuthor.
func LocalBookNameAuthor(fileName string, directUpload bool) (name, author string) {
	stem := filepath.Base(fileName)
	if index := strings.LastIndex(stem, "."); index >= 0 {
		stem = stem[:index]
	}
	if directUpload {
		stem = truncateUTF16Units(localBookFileNameCharacters.ReplaceAllString(stem, ""), readerDevDirectUploadNameUnits)
	}
	return analyzeLocalBookNameAuthor(stem)
}

func analyzeLocalBookNameAuthor(stem string) (string, string) {
	for _, pattern := range localBookNameAuthorPatterns {
		if match := pattern.FindStringSubmatch(stem); match != nil {
			return match[2], formatLocalBookAuthor(match[1] + match[3])
		}
	}
	name := formatLocalBookName(stem)
	author := formatLocalBookAuthor(strings.ReplaceAll(stem, name, ""))
	// Removing the name or an author affix always shortens the text, so equal
	// lengths mean nothing was recognized as an author.
	if len(author) == len(stem) {
		author = ""
	}
	return name, author
}

// BookHelp.formatBookName and formatBookAuthor.
func formatLocalBookName(name string) string {
	return trimJavaControlSpace(localBookNameSuffixPattern.ReplaceAllString(name, ""))
}

func formatLocalBookAuthor(author string) string {
	return trimJavaControlSpace(localBookAuthorAffixPattern.ReplaceAllString(author, ""))
}

// trimJavaControlSpace is Kotlin's trim { it <= ' ' } and Java's String.trim().
func trimJavaControlSpace(value string) string {
	return strings.TrimFunc(value, func(r rune) bool { return r <= ' ' })
}

// truncateUTF16Units keeps at most limit UTF-16 code units. Unlike Java's
// substring it never keeps half of a surrogate pair.
func truncateUTF16Units(value string, limit int) string {
	units := 0
	for index, r := range value {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > limit {
			return value[:index]
		}
		units += width
	}
	return value
}

// readerDevEPUBAuthor reproduces EpubFile.upBookInfo: epublib keeps the
// first non-blank dc:creator, splits it at the last ASCII space into first
// and last name and prints "last, first"; reader-dev then removes a
// leading or trailing ", ".
func readerDevEPUBAuthor(creators []string) string {
	for _, creator := range creators {
		text := trimJavaControlSpace(creator)
		if strings.TrimFunc(text, unicode.IsSpace) == "" {
			continue
		}
		first, last := "", text
		if index := strings.LastIndex(text, " "); index >= 0 {
			first, last = text[:index], text[index+1:]
		}
		return strings.TrimSpace(epubAuthorSeparatorPattern.ReplaceAllString(last+", "+first, ""))
	}
	return ""
}

// readerDevEPUBIntro reproduces Jsoup.parse(descriptions[0]).text(): the
// first dc:description as HTML, tags removed, entities decoded and
// whitespace collapsed, with block boundaries read as spaces.
func readerDevEPUBIntro(descriptions []string) string {
	if len(descriptions) == 0 {
		return ""
	}
	nodes, err := html.ParseFragment(strings.NewReader(descriptions[0]), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return strings.Join(strings.Fields(descriptions[0]), " ")
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		switch node.Type {
		case html.TextNode:
			text.WriteString(node.Data)
		case html.ElementNode:
			if node.Data == "script" || node.Data == "style" {
				return
			}
			if readerDevTextBlockElements[node.Data] {
				text.WriteByte(' ')
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	return strings.Join(strings.Fields(text.String()), " ")
}

var readerDevTextBlockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "br": true, "dd": true,
	"div": true, "dl": true, "dt": true, "fieldset": true, "figcaption": true, "figure": true,
	"footer": true, "form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "header": true, "hr": true, "li": true, "main": true, "nav": true, "ol": true,
	"p": true, "pre": true, "section": true, "table": true, "tbody": true, "td": true,
	"tfoot": true, "th": true, "thead": true, "tr": true, "ul": true,
}

// ParseTXTBookWithLimits parses a TXT catalogue and reports the TOC rule it
// used (see parseTXTCatalogWithLimits).
func ParseTXTBookWithLimits(data []byte, rule string, limits LocalBookParseLimits) (ParsedBook, error) {
	chapters, effectiveRule, err := parseTXTCatalogWithLimits(data, rule, limits)
	if err != nil {
		return ParsedBook{}, err
	}
	return ParsedBook{TOCRule: effectiveRule, Chapters: chapters}, nil
}
