package engine

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"path"
	"strings"
)

// epublib MediaTypes.isBitmapImage: only these become an EPUB cover image.
var epubBitmapCoverTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true}

// epublib MediaTypes, by name and by file extension. A manifest media type
// that epublib does not know falls back to the extension-based type.
var (
	epublibKnownMediaTypes = map[string]bool{
		"application/xhtml+xml": true, "application/epub+zip": true, "application/x-dtbncx+xml": true,
		"text/javascript": true, "text/css": true, "image/jpeg": true, "image/png": true, "image/gif": true,
		"image/svg+xml": true, "application/x-truetype-font": true, "application/vnd.ms-opentype": true,
		"application/font-woff": true, "audio/mpeg": true, "audio/ogg": true, "video/mp4": true,
		"application/smil+xml": true, "application/adobe-page-template+xml": true, "application/pls+xml": true,
	}
	epublibImageTypesByExtension = map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif", ".svg": "image/svg+xml",
	}
)

type epubCoverPackage struct {
	Metadata struct {
		Metas []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []epubManifestItem `xml:"item"`
	} `xml:"manifest"`
	Guide struct {
		References []struct {
			Type string `xml:"type,attr"`
			Href string `xml:"href,attr"`
		} `xml:"reference"`
	} `xml:"guide"`
}

// EPUBCoverImagePath reports the archive path of an EPUB's cover image, or ""
// when it has none (see EPUBCoverImagePathFromZip).
func EPUBCoverImagePath(data []byte, limits LocalBookParseLimits) (string, error) {
	limits = limits.normalized()
	reader, err := openEPUBArchive(data, limits)
	if err != nil {
		return "", err
	}
	return EPUBCoverImagePathFromZip(reader, limits)
}

// EPUBCoverImagePathFromZip follows epublib PackageDocumentReader.readCover,
// which reader-dev's EpubFile.updateCover writes out: the first
// <meta name="cover"> (a manifest id, otherwise an href) and then the first
// <guide><reference type="cover">; a candidate is the cover image only when
// it exists and is JPG, PNG or GIF. EPUB3 cover-image properties are not read.
func EPUBCoverImagePathFromZip(reader *zip.Reader, limits LocalBookParseLimits) (string, error) {
	limits = limits.normalized()
	opfPath, err := epubOPFPath(reader, limits)
	if err != nil {
		return "", err
	}
	opfBytes, err := readEPUBZipFile(reader, opfPath, limits.MaxArchiveEntryBytes)
	if err != nil {
		return "", err
	}
	return epubCoverImagePath(reader, opfPath, opfBytes)
}

func epubCoverImagePath(reader *zip.Reader, opfPath string, opfBytes []byte) (string, error) {
	var pkg epubCoverPackage
	if err := xml.Unmarshal(opfBytes, &pkg); err != nil {
		return "", err
	}
	baseDir := path.Dir(opfPath)
	if baseDir == "." {
		baseDir = ""
	}
	candidates := make([]string, 0, 2)
	for _, meta := range pkg.Metadata.Metas {
		if meta.Name != "cover" {
			continue
		}
		if strings.TrimSpace(meta.Content) != "" {
			href := meta.Content
			for _, item := range pkg.Manifest.Items {
				if item.ID == meta.Content && strings.TrimSpace(item.Href) != "" {
					href = item.Href
					break
				}
			}
			candidates = append(candidates, href)
		}
		break
	}
	for _, reference := range pkg.Guide.References {
		if reference.Type == "cover" {
			if strings.TrimSpace(reference.Href) != "" {
				candidates = append(candidates, reference.Href)
			}
			break
		}
	}

	entries := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		if canonical, err := normalizeEPUBArchivePath(file.Name); err == nil && canonical != "" {
			entries[canonical] = true
		}
	}
	manifestTypes := make(map[string]string, len(pkg.Manifest.Items))
	for _, item := range pkg.Manifest.Items {
		if itemPath, err := normalizeEPUBArchivePath(resolveEPUBPath(baseDir, item.Href)); err == nil && itemPath != "" {
			manifestTypes[itemPath] = strings.ToLower(strings.TrimSpace(item.MediaType))
		}
	}
	for _, candidate := range candidates {
		coverPath, err := normalizeEPUBArchivePath(resolveEPUBPath(baseDir, candidate))
		if err != nil || coverPath == "" || !entries[coverPath] {
			continue
		}
		mediaType := manifestTypes[coverPath]
		if !epublibKnownMediaTypes[mediaType] {
			mediaType = epublibImageTypesByExtension[strings.ToLower(path.Ext(coverPath))]
		}
		if epubBitmapCoverTypes[mediaType] {
			return coverPath, nil
		}
	}
	return "", nil
}

// UMDCoverImage returns the cover stored in a reader-dev UMD: umdlib
// UmdReader keeps the bytes of the additional section that follows segment
// 130 (the last one wins). Sections are walked without decompressing text.
func UMDCoverImage(data []byte, maxBytes int64) (string, []byte, bool) {
	if !bytes.HasPrefix(data, readerDevUMDMagic) {
		return "", nil, false
	}
	maxSegments := maxUMDSegments(DefaultLocalBookParseLimits().normalized())
	position := len(readerDevUMDMagic)
	var currentType uint16
	var cover []byte
	for segments := 0; position < len(data); segments++ {
		if segments >= maxSegments {
			return "", nil, false
		}
		switch data[position] {
		case '#':
			if position+5 > len(data) {
				return "", nil, false
			}
			segmentType := binary.LittleEndian.Uint16(data[position+1 : position+3])
			length := int(data[position+4]) - 5
			position += 5
			if length < 0 || position+length > len(data) {
				return "", nil, false
			}
			position += length
			if segmentType != 0x00f1 && segmentType != 0x000a {
				currentType = segmentType
			}
		case '$':
			if position+9 > len(data) {
				return "", nil, false
			}
			encodedLength := binary.LittleEndian.Uint32(data[position+5 : position+9])
			position += 9
			if encodedLength < 9 || int64(encodedLength-9) > int64(len(data)-position) {
				return "", nil, false
			}
			payloadLength := int(encodedLength - 9)
			if currentType == 0x0082 {
				cover = data[position : position+payloadLength]
			}
			position += payloadLength
		default:
			return "", nil, false
		}
	}
	if len(cover) == 0 || int64(len(cover)) > maxBytes {
		return "", nil, false
	}
	contentType := SniffCoverImage(cover)
	if contentType == "" {
		return "", nil, false
	}
	return contentType, append([]byte(nil), cover...), true
}

// LocalBookCoverImage returns a parsed local book's cover image: the EPUB
// cover image, the first CBZ archive image or the UMD segment-130 cover. A
// cover over maxBytes or without a known raster signature is omitted.
func LocalBookCoverImage(extension string, data []byte, parsed ParsedBook, limits LocalBookParseLimits, maxBytes int64) (string, []byte, bool) {
	limits = limits.normalized()
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".epub":
		return zipCoverImage(data, parsed.CoverResourcePath, normalizeEPUBArchivePath, limits, maxBytes)
	case ".cbz":
		return zipCoverImage(data, parsed.CoverResourcePath, NormalizeCBZResourcePath, limits, maxBytes)
	case ".umd":
		return UMDCoverImage(data, maxBytes)
	}
	return "", nil, false
}

func zipCoverImage(data []byte, coverPath string, normalize func(string) (string, error), limits LocalBookParseLimits, maxBytes int64) (string, []byte, bool) {
	if coverPath == "" || int64(len(data)) > limits.MaxArchiveBytes {
		return "", nil, false
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", nil, false
	}
	for _, file := range reader.File {
		if canonical, err := normalize(file.Name); err != nil || canonical != coverPath {
			continue
		}
		if file.UncompressedSize64 > uint64(maxBytes) {
			return "", nil, false
		}
		opened, err := file.Open()
		if err != nil {
			return "", nil, false
		}
		image, err := io.ReadAll(io.LimitReader(opened, maxBytes+1))
		_ = opened.Close()
		if err != nil || int64(len(image)) > maxBytes {
			return "", nil, false
		}
		contentType := SniffCoverImage(image)
		if contentType == "" {
			return "", nil, false
		}
		return contentType, image, true
	}
	return "", nil, false
}

// SniffCoverImage identifies the raster formats served as local book covers.
func SniffCoverImage(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif"
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}
