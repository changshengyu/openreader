package engine

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type ParsedBook struct {
	Title    string
	Author   string
	Chapters []TXTChapter
}

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Metadata struct {
		Titles   []string `xml:"title"`
		Creators []string `xml:"creator"`
	} `xml:"metadata"`
	Manifest struct {
		Items []epubManifestItem `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		TOC      string `xml:"toc,attr"`
		ItemRefs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

type epubManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

func ParseEPUB(data []byte) (ParsedBook, error) {
	return ParseEPUBWithLimits(data, "", LegacyLocalBookParseLimits())
}

type epubChapter struct {
	Path    string
	Title   string
	Content string
}

type epubTOCEntry struct {
	Path  string
	Title string
}

func ParseEPUBWithRule(data []byte, rule string) (ParsedBook, error) {
	return ParseEPUBWithLimits(data, rule, LegacyLocalBookParseLimits())
}

func ParseEPUBWithLimits(data []byte, rule string, limits LocalBookParseLimits) (ParsedBook, error) {
	limits = limits.normalized()
	reader, err := openEPUBArchive(data, limits)
	if err != nil {
		return ParsedBook{}, err
	}

	opfPath, err := epubOPFPath(reader, limits)
	if err != nil {
		return ParsedBook{}, err
	}

	opfBytes, err := readEPUBZipFile(reader, opfPath, limits.MaxArchiveEntryBytes)
	if err != nil {
		return ParsedBook{}, err
	}

	var pkg epubPackage
	if err := xml.Unmarshal(opfBytes, &pkg); err != nil {
		return ParsedBook{}, err
	}

	manifest := make(map[string]epubManifestItem)
	for _, item := range pkg.Manifest.Items {
		manifest[item.ID] = item
	}

	book := ParsedBook{
		Title:  firstNonEmpty(pkg.Metadata.Titles),
		Author: firstNonEmpty(pkg.Metadata.Creators),
	}
	baseDir := path.Dir(opfPath)
	if baseDir == "." {
		baseDir = ""
	}

	spineChapters := make([]epubChapter, 0, len(pkg.Spine.ItemRefs))
	var parsedTextBytes int64
	for _, ref := range pkg.Spine.ItemRefs {
		item, ok := manifest[ref.IDRef]
		if !ok || !isReadableEPUBItem(item.MediaType) {
			continue
		}

		href, err := url.PathUnescape(item.Href)
		if err != nil {
			href = item.Href
		}
		chapterPath := path.Clean(path.Join(baseDir, href))
		chapterBytes, err := readEPUBZipFile(reader, chapterPath, limits.MaxArchiveEntryBytes)
		if err != nil {
			continue
		}

		title, content := extractEPUBChapter(chapterBytes)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if int64(len(content)) > limits.MaxParsedTextBytes-parsedTextBytes {
			return ParsedBook{}, fmt.Errorf("%w: EPUB extracted text exceeds the limit", ErrLocalBookParseLimit)
		}
		parsedTextBytes += int64(len(content))
		spineChapters = append(spineChapters, epubChapter{
			Path:    canonicalEPUBPath(chapterPath),
			Title:   title,
			Content: content,
		})
	}

	if len(spineChapters) == 0 {
		return ParsedBook{}, errors.New("no readable epub chapters found")
	}
	tocEntries := epubTOCEntries(reader, pkg, manifest, baseDir, limits.MaxArchiveEntryBytes)
	book.Chapters = buildEPUBChapters(spineChapters, tocEntries, rule)
	return book, nil
}

func buildEPUBChapters(spine []epubChapter, toc []epubTOCEntry, rule string) []TXTChapter {
	rule = normalizeEPUBRule(rule)
	tocTitleByPath := make(map[string]string, len(toc))
	for _, entry := range toc {
		if entry.Path != "" && strings.TrimSpace(entry.Title) != "" {
			if _, exists := tocTitleByPath[entry.Path]; !exists {
				tocTitleByPath[entry.Path] = strings.TrimSpace(entry.Title)
			}
		}
	}
	spineByPath := make(map[string]epubChapter, len(spine))
	for _, chapter := range spine {
		spineByPath[chapter.Path] = chapter
	}

	ordered := make([]epubChapter, 0, len(spine))
	if strings.HasPrefix(rule, "toc") && len(toc) > 0 {
		seen := make(map[string]struct{}, len(toc))
		for _, entry := range toc {
			chapter, ok := spineByPath[entry.Path]
			if !ok {
				continue
			}
			if _, exists := seen[entry.Path]; exists {
				continue
			}
			seen[entry.Path] = struct{}{}
			tocTitle := strings.TrimSpace(entry.Title)
			switch rule {
			case "toc":
				chapter.Title = tocTitle
			case "toc+spin":
				if tocTitle != "" {
					chapter.Title = tocTitle
				}
			case "toc<spin":
				// Keep the title extracted from the spine document.
			}
			ordered = append(ordered, chapter)
		}
	} else {
		for _, chapter := range spine {
			tocTitle := tocTitleByPath[chapter.Path]
			switch rule {
			case "spin+toc":
				if strings.TrimSpace(chapter.Title) == "" && tocTitle != "" {
					chapter.Title = tocTitle
				}
			case "spin<toc":
				if tocTitle != "" {
					chapter.Title = tocTitle
				}
			}
			ordered = append(ordered, chapter)
		}
	}
	if len(ordered) == 0 {
		ordered = append(ordered, spine...)
	}

	chapters := make([]TXTChapter, 0, len(ordered))
	for index, chapter := range ordered {
		title := strings.TrimSpace(chapter.Title)
		if title == "" {
			title = fmt.Sprintf("第 %d 章", index+1)
		}
		chapters = append(chapters, TXTChapter{
			Index:        index,
			Title:        title,
			Content:      chapter.Content,
			ResourcePath: chapter.Path,
		})
	}
	return chapters
}

func normalizeEPUBRule(rule string) string {
	switch strings.ToLower(strings.TrimSpace(rule)) {
	case "spin", "spin+toc", "spin<toc", "toc", "toc+spin", "toc<spin":
		return strings.ToLower(strings.TrimSpace(rule))
	default:
		return "spin+toc"
	}
}

func epubTOCEntries(reader *zip.Reader, pkg epubPackage, manifest map[string]epubManifestItem, baseDir string, maxEntryBytes int64) []epubTOCEntry {
	for _, item := range pkg.Manifest.Items {
		if hasEPUBProperty(item.Properties, "nav") {
			if entries := parseEPUBNav(reader, baseDir, item.Href, maxEntryBytes); len(entries) > 0 {
				return entries
			}
		}
	}
	ncxID := strings.TrimSpace(pkg.Spine.TOC)
	if ncxID != "" {
		if item, ok := manifest[ncxID]; ok {
			if entries := parseEPUBNCX(reader, baseDir, item.Href, maxEntryBytes); len(entries) > 0 {
				return entries
			}
		}
	}
	for _, item := range pkg.Manifest.Items {
		if strings.EqualFold(item.MediaType, "application/x-dtbncx+xml") {
			if entries := parseEPUBNCX(reader, baseDir, item.Href, maxEntryBytes); len(entries) > 0 {
				return entries
			}
		}
	}
	return nil
}

func hasEPUBProperty(properties string, target string) bool {
	for _, property := range strings.Fields(properties) {
		if property == target {
			return true
		}
	}
	return false
}

func parseEPUBNav(reader *zip.Reader, baseDir string, href string, maxEntryBytes int64) []epubTOCEntry {
	navPath := resolveEPUBPath(baseDir, href)
	data, err := readEPUBZipFile(reader, navPath, maxEntryBytes)
	if err != nil {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	navDir := path.Dir(navPath)
	if navDir == "." {
		navDir = ""
	}
	entries := make([]epubTOCEntry, 0)
	selection := doc.Find("nav").FilterFunction(func(_ int, nav *goquery.Selection) bool {
		for _, attr := range nav.Get(0).Attr {
			if (attr.Key == "epub:type" || attr.Key == "type") && attr.Val == "toc" {
				return true
			}
		}
		return false
	}).First()
	if selection.Length() == 0 {
		selection = doc.Find("nav").First()
	}
	selection.Find("a[href]").Each(func(_ int, link *goquery.Selection) {
		target, ok := link.Attr("href")
		if !ok {
			return
		}
		resolved := canonicalEPUBPath(resolveEPUBPath(navDir, target))
		if resolved == "." || resolved == "" {
			return
		}
		entries = append(entries, epubTOCEntry{
			Path:  resolved,
			Title: strings.Join(strings.Fields(link.Text()), " "),
		})
	})
	return entries
}

type epubNCX struct {
	NavMap struct {
		Points []epubNCXPoint `xml:"navPoint"`
	} `xml:"navMap"`
}

type epubNCXPoint struct {
	Label struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Points []epubNCXPoint `xml:"navPoint"`
}

func parseEPUBNCX(reader *zip.Reader, baseDir string, href string, maxEntryBytes int64) []epubTOCEntry {
	ncxPath := resolveEPUBPath(baseDir, href)
	data, err := readEPUBZipFile(reader, ncxPath, maxEntryBytes)
	if err != nil {
		return nil
	}
	var ncx epubNCX
	if err := xml.Unmarshal(data, &ncx); err != nil {
		return nil
	}
	ncxDir := path.Dir(ncxPath)
	if ncxDir == "." {
		ncxDir = ""
	}
	entries := make([]epubTOCEntry, 0)
	var appendPoints func([]epubNCXPoint)
	appendPoints = func(points []epubNCXPoint) {
		for _, point := range points {
			resolved := canonicalEPUBPath(resolveEPUBPath(ncxDir, point.Content.Src))
			if resolved != "." && resolved != "" {
				entries = append(entries, epubTOCEntry{
					Path:  resolved,
					Title: strings.Join(strings.Fields(point.Label.Text), " "),
				})
			}
			appendPoints(point.Points)
		}
	}
	appendPoints(ncx.NavMap.Points)
	return entries
}

func resolveEPUBPath(baseDir string, href string) string {
	href, err := url.PathUnescape(strings.TrimSpace(href))
	if err != nil {
		href = strings.TrimSpace(href)
	}
	href = strings.SplitN(href, "#", 2)[0]
	href = strings.SplitN(href, "?", 2)[0]
	return path.Clean(path.Join(baseDir, href))
}

func canonicalEPUBPath(value string) string {
	value = strings.SplitN(value, "#", 2)[0]
	value = strings.SplitN(value, "?", 2)[0]
	return path.Clean(value)
}

func epubOPFPath(reader *zip.Reader, limits LocalBookParseLimits) (string, error) {
	data, err := readEPUBZipFile(reader, "META-INF/container.xml", limits.MaxArchiveEntryBytes)
	if err != nil {
		return "", err
	}

	var container epubContainer
	if err := xml.Unmarshal(data, &container); err != nil {
		return "", err
	}
	for _, rootfile := range container.Rootfiles {
		if rootfile.FullPath != "" {
			return normalizeEPUBArchivePath(rootfile.FullPath)
		}
	}
	return "", errors.New("missing opf rootfile")
}

func readEPUBZipFile(reader *zip.Reader, name string, maxBytes int64) ([]byte, error) {
	for _, file := range reader.File {
		canonical, err := normalizeEPUBArchivePath(file.Name)
		if err != nil {
			return nil, err
		}
		if canonical != name {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer opened.Close()
		data, err := io.ReadAll(io.LimitReader(opened, maxBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > maxBytes {
			return nil, fmt.Errorf("%w: EPUB entry exceeds the limit", ErrLocalBookParseLimit)
		}
		return data, nil
	}
	return nil, fmt.Errorf("zip file not found: %s", name)
}

func openEPUBArchive(data []byte, limits LocalBookParseLimits) (*zip.Reader, error) {
	if int64(len(data)) > limits.MaxArchiveBytes {
		return nil, fmt.Errorf("%w: EPUB archive is too large", ErrLocalBookParseLimit)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(reader.File) > limits.MaxArchiveEntries {
		return nil, fmt.Errorf("%w: EPUB contains too many entries", ErrLocalBookParseLimit)
	}

	seen := make(map[string]struct{}, len(reader.File))
	var total uint64
	for _, file := range reader.File {
		canonical, err := normalizeEPUBArchivePath(file.Name)
		if err != nil {
			return nil, err
		}
		if canonical == "" {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: EPUB symbolic links are not allowed", ErrLocalBookParseLimit)
		}
		key := strings.ToLower(canonical)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate EPUB archive path", ErrLocalBookParseLimit)
		}
		seen[key] = struct{}{}
		if file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/") {
			continue
		}
		if file.UncompressedSize64 > uint64(limits.MaxArchiveEntryBytes) {
			return nil, fmt.Errorf("%w: EPUB entry is too large", ErrLocalBookParseLimit)
		}
		if ^uint64(0)-total < file.UncompressedSize64 {
			return nil, fmt.Errorf("%w: EPUB expanded size overflows", ErrLocalBookParseLimit)
		}
		total += file.UncompressedSize64
		if total > uint64(limits.MaxArchiveExpandedBytes) {
			return nil, fmt.Errorf("%w: EPUB expands beyond the total limit", ErrLocalBookParseLimit)
		}
	}
	return reader, nil
}

func normalizeEPUBArchivePath(name string) (string, error) {
	if strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
		return "", fmt.Errorf("%w: malformed EPUB archive path", ErrLocalBookParseLimit)
	}
	if strings.HasPrefix(name, "/") || hasEPUBWindowsDrivePrefix(name) {
		return "", fmt.Errorf("%w: absolute EPUB archive path", ErrLocalBookParseLimit)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: EPUB archive path traversal", ErrLocalBookParseLimit)
		}
	}
	cleaned := path.Clean(name)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", fmt.Errorf("%w: EPUB archive path escaped root", ErrLocalBookParseLimit)
	}
	return cleaned, nil
}

func hasEPUBWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':'
}

func isReadableEPUBItem(mediaType string) bool {
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/xhtml+xml" || mediaType == "text/html" || mediaType == ""
}

func extractEPUBChapter(data []byte) (string, string) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return "", ""
	}
	doc.Find("script, style, nav").Remove()

	title := strings.TrimSpace(doc.Find("h1, h2, title").First().Text())
	lines := make([]string, 0)
	doc.Find("h1, h2, h3, p, li, blockquote").Each(func(_ int, selection *goquery.Selection) {
		text := strings.Join(strings.Fields(selection.Text()), " ")
		if text != "" {
			lines = append(lines, text)
		}
	})
	if len(lines) == 0 {
		bodyText := strings.Join(strings.Fields(doc.Find("body").Text()), " ")
		if bodyText != "" {
			lines = append(lines, bodyText)
		}
	}
	return title, strings.Join(lines, "\n")
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
