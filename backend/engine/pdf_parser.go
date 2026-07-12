package engine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ParsePDF extracts readable text from a PDF file buffer.
// It treats each page as a separate chapter and uses the PDF content as chapter text.
func ParsePDF(data []byte) (ParsedBook, error) {
	return ParsePDFWithLimits(data, LegacyLocalBookParseLimits())
}

func ParsePDFWithLimits(data []byte, limits LocalBookParseLimits) (ParsedBook, error) {
	limits = limits.normalized()
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ParsedBook{}, errors.New("failed to open PDF, treating as single chapter")
	}

	var builder strings.Builder
	pageCount := reader.NumPage()
	if pageCount > limits.MaxPDFPages {
		return ParsedBook{}, fmt.Errorf("%w: PDF contains too many pages", ErrLocalBookParseLimit)
	}

	for pageIndex := 1; pageIndex <= pageCount; pageIndex++ {
		page := reader.Page(pageIndex)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		separatorBytes := 0
		if builder.Len() > 0 {
			separatorBytes = 2
		}
		if int64(len(text)+separatorBytes) > limits.MaxParsedTextBytes-int64(builder.Len()) {
			return ParsedBook{}, fmt.Errorf("%w: PDF extracted text exceeds the limit", ErrLocalBookParseLimit)
		}

		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(text)
	}

	fullText := builder.String()
	if fullText == "" {
		return ParsedBook{}, errors.New("no readable text found in PDF")
	}

	chapters := splitPDFChapters(fullText)
	return ParsedBook{Chapters: chapters}, nil
}

// pdfReadAll wraps a simple reader for the pdf library
type pdfReadAll struct {
	io.Reader
}

func splitPDFChapters(text string) []TXTChapter {
	lines := strings.Split(text, "\n")
	chapters := make([]TXTChapter, 0)
	current := TXTChapter{Index: 0, Title: "正文", Start: 0}
	contentStart := 0
	pos := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineLen := len(line) + 1 // +1 for newline

		if isChapterTitle(trimmed) {
			if pos > contentStart {
				content := strings.TrimSpace(text[contentStart:pos])
				if content != "" {
					current.End = pos
					current.Content = content
					chapters = append(chapters, current)
				}
			}
			current = TXTChapter{Title: trimmed, Start: pos}
			contentStart = pos + lineLen
		}
		pos += lineLen
	}

	if contentStart <= len(text) {
		content := strings.TrimSpace(text[contentStart:])
		if content != "" || len(chapters) == 0 {
			current.Index = len(chapters)
			current.End = len(text)
			current.Content = content
			chapters = append(chapters, current)
		}
	}

	if len(chapters) == 0 {
		chapters = append(chapters, TXTChapter{
			Index:   0,
			Title:   "正文",
			Start:   0,
			End:     len(text),
			Content: text,
		})
	}

	return chapters
}
