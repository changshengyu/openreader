package engine

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"sort"
	"strings"
)

const (
	maxCBZArchiveBytes = 1 << 30
	maxCBZEntries      = 20_000
	maxCBZEntryBytes   = 128 << 20
	maxCBZTotalBytes   = 2 << 30
	maxCBZMetadataSize = 1 << 20
)

var cbzImageExtensions = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".png":  "image/png",
	".bmp":  "image/bmp",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

type cbzComicInfo struct {
	Title  string `xml:"Title"`
	Writer string `xml:"Writer"`
}

func ParseCBZ(data []byte) (ParsedBook, error) {
	if int64(len(data)) > maxCBZArchiveBytes {
		return ParsedBook{}, errors.New("cbz archive is too large")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ParsedBook{}, err
	}
	return parseCBZReader(reader)
}

func parseCBZReader(reader *zip.Reader) (ParsedBook, error) {
	if len(reader.File) > maxCBZEntries {
		return ParsedBook{}, errors.New("cbz contains too many entries")
	}

	seen := make(map[string]bool, len(reader.File))
	images := make([]string, 0)
	var parsed ParsedBook
	var total uint64

	for _, file := range reader.File {
		canonical, err := NormalizeCBZResourcePath(file.Name)
		if err != nil {
			return ParsedBook{}, err
		}
		if canonical == "" {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return ParsedBook{}, fmt.Errorf("unsafe cbz entry %q", file.Name)
		}
		isDir := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
		key := strings.ToLower(canonical)
		if seen[key] {
			return ParsedBook{}, fmt.Errorf("duplicate cbz entry %q", canonical)
		}
		seen[key] = true
		if isDir {
			continue
		}
		if file.UncompressedSize64 > maxCBZEntryBytes {
			return ParsedBook{}, fmt.Errorf("cbz entry %q is too large", canonical)
		}
		if ^uint64(0)-total < file.UncompressedSize64 {
			return ParsedBook{}, errors.New("cbz expanded size overflows")
		}
		total += file.UncompressedSize64
		if total > maxCBZTotalBytes {
			return ParsedBook{}, errors.New("cbz expands beyond the total limit")
		}

		if strings.EqualFold(path.Base(canonical), "ComicInfo.xml") {
			applyCBZComicInfo(&parsed, file)
			continue
		}
		if _, ok := CBZImageContentType(canonical); ok {
			images = append(images, canonical)
		}
	}

	sort.Strings(images)
	parsed.Chapters = make([]TXTChapter, 0, len(images))
	for index, imagePath := range images {
		parsed.Chapters = append(parsed.Chapters, TXTChapter{
			Index:        index,
			Title:        imagePath,
			ResourcePath: imagePath,
		})
	}
	if len(parsed.Chapters) == 0 {
		return ParsedBook{}, errors.New("no readable images found in CBZ file")
	}
	return parsed, nil
}

func applyCBZComicInfo(parsed *ParsedBook, file *zip.File) {
	if file.UncompressedSize64 > maxCBZMetadataSize {
		return
	}
	opened, err := file.Open()
	if err != nil {
		return
	}
	defer opened.Close()
	data, err := io.ReadAll(io.LimitReader(opened, maxCBZMetadataSize+1))
	if err != nil || len(data) > maxCBZMetadataSize {
		return
	}
	var info cbzComicInfo
	if err := xml.Unmarshal(data, &info); err != nil {
		return
	}
	if title := strings.TrimSpace(info.Title); title != "" {
		parsed.Title = title
	}
	if author := strings.TrimSpace(info.Writer); author != "" {
		parsed.Author = author
	}
}

func NormalizeCBZResourcePath(name string) (string, error) {
	if strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
		return "", fmt.Errorf("unsafe cbz path %q", name)
	}
	if strings.HasPrefix(name, "/") || hasCBZWindowsDrivePrefix(name) {
		return "", fmt.Errorf("unsafe cbz absolute path %q", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", fmt.Errorf("unsafe cbz traversal path %q", name)
		}
	}
	cleaned := path.Clean(name)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", fmt.Errorf("unsafe cbz escaped path %q", name)
	}
	return cleaned, nil
}

func hasCBZWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':'
}

func CBZImageContentType(resourcePath string) (string, bool) {
	contentType, ok := cbzImageExtensions[strings.ToLower(path.Ext(resourcePath))]
	if !ok {
		if detected := mime.TypeByExtension(path.Ext(resourcePath)); strings.HasPrefix(detected, "image/") {
			return strings.Split(detected, ";")[0], true
		}
		return "", false
	}
	return contentType, true
}
