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
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type ParsedBook struct {
	Title             string       `json:"title"`
	Author            string       `json:"author"`
	CoverResourcePath string       `json:"coverResourcePath,omitempty"`
	Chapters          []TXTChapter `json:"chapters"`
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
	Path        string
	Title       string
	Content     string
	Data        []byte
	Fragment    string
	EndFragment string
}

type epubTOCEntry struct {
	Path     string
	Fragment string
	Title    string
}

func ParseEPUBWithRule(data []byte, rule string) (ParsedBook, error) {
	return ParseEPUBWithLimits(data, rule, LegacyLocalBookParseLimits())
}

func ParseEPUBWithLimits(data []byte, rule string, limits LocalBookParseLimits) (ParsedBook, error) {
	catalog, err := ParseEPUBCatalogWithLimits(data, rule, limits)
	if err != nil {
		return ParsedBook{}, err
	}
	return MaterializeEPUBCatalogWithLimits(data, catalog, limits)
}

// ParseEPUBCatalogWithLimits reads only the package/catalogue metadata and the
// document <title> fallback required by reader-dev. It intentionally leaves
// chapter Content empty so upload preview and rule refresh do not build DOMs or
// serialize the whole book body before the user confirms the import.
func ParseEPUBCatalogWithLimits(data []byte, rule string, limits LocalBookParseLimits) (ParsedBook, error) {
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
	tocEntries := epubTOCEntries(reader, pkg, manifest, baseDir, limits.MaxArchiveEntryBytes)

	spineChapters := make([]epubChapter, 0, len(pkg.Spine.ItemRefs))
	for spineIndex, ref := range pkg.Spine.ItemRefs {
		item, ok := manifest[ref.IDRef]
		if !ok || !isReadableEPUBItem(item.MediaType) {
			continue
		}

		chapterPath := resolveEPUBPath(baseDir, item.Href)
		chapterPath, err = normalizeEPUBArchivePath(chapterPath)
		if err != nil {
			continue
		}
		title, err := readEPUBDocumentTitle(reader, chapterPath, limits.MaxArchiveEntryBytes)
		if err != nil {
			continue
		}
		// reader-dev keeps every readable spine resource. In particular, its
		// first title-less resource is the cover page, even when the XHTML has
		// only an image and therefore has no extractable text. Keep that
		// resource path for the protected EPUB iframe instead of silently
		// deleting the user's cover chapter during import.
		if spineIndex == 0 && strings.TrimSpace(title) == "" {
			title = "封面"
		}
		spineChapters = append(spineChapters, epubChapter{
			Path:  canonicalEPUBPath(chapterPath),
			Title: title,
		})
	}

	if len(spineChapters) == 0 {
		return ParsedBook{}, errors.New("no readable epub chapters found")
	}
	book.Chapters = buildEPUBChapters(spineChapters, tocEntries, rule)
	return book, nil
}

// MaterializeEPUBCatalogWithLimits fills searchable chapter text for an
// already validated catalogue. Every unique XHTML resource is decompressed
// once per call, while separate fragment chapters retain their own boundaries.
// The catalogue order/title is authoritative because staged snapshots are
// bound to the exact source SHA-256 and TOC rule before this function is used.
func MaterializeEPUBCatalogWithLimits(data []byte, catalog ParsedBook, limits LocalBookParseLimits) (ParsedBook, error) {
	limits = limits.normalized()
	reader, err := openEPUBArchive(data, limits)
	if err != nil {
		return ParsedBook{}, err
	}
	result := catalog
	result.Chapters = append([]TXTChapter(nil), catalog.Chapters...)
	indexesByPath := make(map[string][]int, len(result.Chapters))
	pathOrder := make([]string, 0, len(result.Chapters))
	for index, chapter := range result.Chapters {
		resourcePath, err := normalizeEPUBArchivePath(strings.TrimSpace(chapter.ResourcePath))
		if err != nil || resourcePath == "" {
			return ParsedBook{}, fmt.Errorf("%w: invalid EPUB chapter resource path", ErrLocalBookParseLimit)
		}
		result.Chapters[index].ResourcePath = resourcePath
		if _, exists := indexesByPath[resourcePath]; !exists {
			pathOrder = append(pathOrder, resourcePath)
		}
		indexesByPath[resourcePath] = append(indexesByPath[resourcePath], index)
	}

	var parsedTextBytes int64
	for _, resourcePath := range pathOrder {
		chapterBytes, err := readEPUBZipFile(reader, resourcePath, limits.MaxArchiveEntryBytes)
		if err != nil {
			return ParsedBook{}, err
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(chapterBytes))
		if err != nil {
			return ParsedBook{}, fmt.Errorf("invalid EPUB chapter document: %w", err)
		}
		for _, index := range indexesByPath[resourcePath] {
			chapter := &result.Chapters[index]
			content := strings.TrimSpace(extractEPUBChapterRangeFromDocument(
				doc,
				chapter.ResourceFragment,
				chapter.ResourceEndFragment,
			))
			if int64(len(content)) > limits.MaxParsedTextBytes-parsedTextBytes {
				return ParsedBook{}, fmt.Errorf("%w: EPUB extracted text exceeds the limit", ErrLocalBookParseLimit)
			}
			parsedTextBytes += int64(len(content))
			chapter.Content = content
		}
	}
	return result, nil
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
		selected := make([]epubTOCEntry, 0, len(toc))
		for _, entry := range toc {
			if _, ok := spineByPath[entry.Path]; !ok {
				continue
			}
			fragmentKey := entry.Path + "\x00" + entry.Fragment
			if _, exists := seen[fragmentKey]; exists {
				continue
			}
			seen[fragmentKey] = struct{}{}
			selected = append(selected, entry)
		}
		for tocIndex, entry := range selected {
			chapter, ok := spineByPath[entry.Path]
			if !ok {
				continue
			}
			chapterEndFragment := ""
			if tocIndex+1 < len(selected) && selected[tocIndex+1].Path == entry.Path {
				chapterEndFragment = selected[tocIndex+1].Fragment
			}
			chapter.Content = extractEPUBChapterRange(chapter.Data, entry.Fragment, chapterEndFragment)
			chapterFragment := entry.Fragment
			chapter.Path = canonicalEPUBPath(chapter.Path)
			chapter.Title = strings.TrimSpace(chapter.Title)
			chapter.Content = strings.TrimSpace(chapter.Content)
			chapter.Data = nil
			chapter.Fragment = chapterFragment
			chapter.EndFragment = chapterEndFragment
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
			Index:               index,
			Title:               title,
			Content:             chapter.Content,
			ResourcePath:        chapter.Path,
			ResourceFragment:    chapter.Fragment,
			ResourceEndFragment: chapter.EndFragment,
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
		resolved, fragment, ok := resolveEPUBReference(navDir, target)
		if !ok {
			return
		}
		if resolved == "." || resolved == "" {
			return
		}
		entries = append(entries, epubTOCEntry{
			Path:     resolved,
			Fragment: fragment,
			Title:    strings.Join(strings.Fields(link.Text()), " "),
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
			resolved, fragment, ok := resolveEPUBReference(ncxDir, point.Content.Src)
			if !ok {
				appendPoints(point.Points)
				continue
			}
			if resolved != "." && resolved != "" {
				entries = append(entries, epubTOCEntry{
					Path:     resolved,
					Fragment: fragment,
					Title:    strings.Join(strings.Fields(point.Label.Text), " "),
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

func resolveEPUBReference(baseDir string, href string) (string, string, bool) {
	href = strings.TrimSpace(href)
	resourceHref, rawFragment, hasFragment := strings.Cut(href, "#")
	resourceHref = strings.SplitN(resourceHref, "?", 2)[0]
	resourceHref, err := url.PathUnescape(resourceHref)
	if err != nil {
		return "", "", false
	}
	resourcePath := canonicalEPUBPath(path.Clean(path.Join(baseDir, resourceHref)))
	if resourcePath == "" || resourcePath == "." {
		return "", "", false
	}
	if !hasFragment || rawFragment == "" {
		return resourcePath, "", true
	}
	fragment, err := NormalizeEPUBFragment(rawFragment)
	if err != nil {
		return "", "", false
	}
	return resourcePath, fragment, true
}

// NormalizeEPUBFragment accepts only a bounded decoded DOM id. EPUB fragments
// are display metadata, never archive paths, and must remain safe to persist.
func NormalizeEPUBFragment(value string) (string, error) {
	decoded, err := url.PathUnescape(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if decoded == "" {
		return "", nil
	}
	if len(decoded) > 512 || strings.ContainsRune(decoded, '\x00') || !utf8.ValidString(decoded) {
		return "", errors.New("invalid EPUB fragment")
	}
	return decoded, nil
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

func readEPUBDocumentTitle(reader *zip.Reader, name string, maxBytes int64) (string, error) {
	for _, file := range reader.File {
		canonical, err := normalizeEPUBArchivePath(file.Name)
		if err != nil {
			return "", err
		}
		if canonical != name {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			return "", err
		}
		defer opened.Close()
		limited := &io.LimitedReader{R: opened, N: maxBytes + 1}
		tokenizer := html.NewTokenizer(limited)
		insideTitle := false
		var title strings.Builder
		for {
			switch tokenizer.Next() {
			case html.ErrorToken:
				if limited.N <= 0 {
					return "", fmt.Errorf("%w: EPUB entry exceeds the limit", ErrLocalBookParseLimit)
				}
				if err := tokenizer.Err(); err != nil && !errors.Is(err, io.EOF) {
					return "", err
				}
				return strings.Join(strings.Fields(title.String()), " "), nil
			case html.StartTagToken:
				token := tokenizer.Token()
				if strings.EqualFold(token.Data, "title") {
					insideTitle = true
				}
			case html.TextToken:
				if insideTitle {
					title.Write(tokenizer.Text())
				}
			case html.EndTagToken:
				token := tokenizer.Token()
				if insideTitle && strings.EqualFold(token.Data, "title") {
					return strings.Join(strings.Fields(title.String()), " "), nil
				}
			}
		}
	}
	return "", fmt.Errorf("zip file not found: %s", name)
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
	return extractEPUBChapterDocument(doc)
}

func extractEPUBChapterRange(data []byte, startFragment, endFragment string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return extractEPUBChapterRangeFromDocument(doc, startFragment, endFragment)
}

// extractEPUBChapterRangeFromDocument clones the already parsed resource before
// applying fragment slicing and text cleanup. A resource shared by multiple TOC
// fragments therefore incurs one HTML parse while each chapter keeps an
// isolated mutable tree and the existing fragment semantics.
func extractEPUBChapterRangeFromDocument(doc *goquery.Document, startFragment, endFragment string) string {
	if doc == nil || doc.Selection == nil {
		return ""
	}
	cloned := doc.Selection.Clone()
	if cloned.Length() == 0 {
		return ""
	}
	chapterDoc := goquery.NewDocumentFromNode(cloned.Get(0))
	if startFragment == "" && endFragment == "" {
		_, content := extractEPUBChapterDocument(chapterDoc)
		return content
	}
	body := chapterDoc.Find("body").First()
	if body.Length() == 0 {
		return ""
	}
	if start := findEPUBElementByID(body, startFragment); start.Length() > 0 {
		start.PrevAll().Remove()
	}
	if endFragment != "" && endFragment != startFragment {
		if end := findEPUBElementByID(body, endFragment); end.Length() > 0 {
			end.NextAll().Remove()
			end.Remove()
		}
	}
	_, content := extractEPUBChapterDocument(chapterDoc)
	return content
}

func findEPUBElementByID(root *goquery.Selection, id string) *goquery.Selection {
	if root == nil || id == "" {
		return &goquery.Selection{}
	}
	return root.Find("[id]").FilterFunction(func(_ int, selection *goquery.Selection) bool {
		return selection.AttrOr("id", "") == id
	}).First()
}

func extractEPUBChapterDocument(doc *goquery.Document) (string, string) {
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
