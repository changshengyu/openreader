package engine

import (
	"path/filepath"
	"strings"
)

// LocalBookNameAuthor derives a local book's name and author from its file
// name.
func LocalBookNameAuthor(fileName string, directUpload bool) (name, author string) {
	base := filepath.Base(fileName)
	return strings.TrimSuffix(base, filepath.Ext(base)), ""
}

// ParseTXTBookWithLimits parses a TXT catalogue and reports the TOC rule it
// used.
func ParseTXTBookWithLimits(data []byte, rule string, limits LocalBookParseLimits) (ParsedBook, error) {
	chapters, err := ParseTXTWithLimits(data, rule, limits)
	if err != nil {
		return ParsedBook{}, err
	}
	return ParsedBook{Chapters: chapters}, nil
}
