package engine

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestImportParsersRejectArchiveLimitsBeforeParsing(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"one.xhtml", "two.xhtml"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("content")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	limits := DefaultLocalBookParseLimits()
	limits.MaxArchiveEntries = 1
	if _, err := ParseEPUBWithLimits(archive.Bytes(), "", limits); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("EPUB entry limit error = %v, want %v", err, ErrLocalBookParseLimit)
	}
	if _, err := ParseCBZWithLimits(archive.Bytes(), limits); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("CBZ entry limit error = %v, want %v", err, ErrLocalBookParseLimit)
	}

	perEntry := DefaultLocalBookParseLimits()
	perEntry.MaxArchiveEntryBytes = 4
	if _, err := ParseEPUBWithLimits(makeImportLimitArchive(t, map[string]string{"chapter.xhtml": "12345"}), "", perEntry); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("EPUB per-entry limit error = %v, want %v", err, ErrLocalBookParseLimit)
	}

	total := DefaultLocalBookParseLimits()
	total.MaxArchiveEntryBytes = 8
	total.MaxArchiveExpandedBytes = 8
	if _, err := ParseEPUBWithLimits(makeImportLimitArchive(t, map[string]string{"one.xhtml": "12345", "two.xhtml": "67890"}), "", total); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("EPUB expanded-size limit error = %v, want %v", err, ErrLocalBookParseLimit)
	}

	if _, err := ParseEPUBWithLimits(makeImportLimitArchive(t, map[string]string{"../escape.xhtml": "content"}), "", DefaultLocalBookParseLimits()); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("EPUB unsafe-path limit error = %v, want %v", err, ErrLocalBookParseLimit)
	}
}

func TestParseUMDWithLimitsRejectsDeclaredChapterCountBeforeOffsetAllocation(t *testing.T) {
	data := make([]byte, 256)
	copy(data, umdMagic)
	pos := 8 + 4 + 2
	data[pos] = 0 // title length
	pos++
	data[pos] = 0 // author length
	pos++
	pos += 5      // date fields
	data[pos] = 0 // content-type count
	pos++
	binary.LittleEndian.PutUint32(data[pos:pos+4], 2)

	limits := DefaultLocalBookParseLimits()
	limits.MaxUMDChapters = 1
	if _, err := ParseUMDWithLimits(data, limits); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("UMD chapter-count limit error = %v, want %v", err, ErrLocalBookParseLimit)
	}
}

func TestParsePDFWithLimitsRejectsExtractedTextOverBudget(t *testing.T) {
	data := makeMinimalPDF("parser budget text")
	if book, err := ParsePDF(data); err != nil || len(book.Chapters) == 0 {
		t.Fatalf("valid PDF control parse = %+v, %v", book, err)
	}
	limits := DefaultLocalBookParseLimits()
	limits.MaxParsedTextBytes = 1
	if _, err := ParsePDFWithLimits(data, limits); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("PDF text limit error = %v, want %v", err, ErrLocalBookParseLimit)
	}
}

func TestLegacyLocalBookLimitsRemainWiderThanNewImportDefaults(t *testing.T) {
	strict := DefaultLocalBookParseLimits()
	legacy := LegacyLocalBookParseLimits()
	if legacy.MaxArchiveBytes <= strict.MaxArchiveBytes || legacy.MaxArchiveExpandedBytes <= strict.MaxArchiveExpandedBytes ||
		legacy.MaxPDFPages <= strict.MaxPDFPages || legacy.MaxParsedTextBytes <= strict.MaxParsedTextBytes ||
		legacy.MaxUMDChapters <= strict.MaxUMDChapters {
		t.Fatalf("legacy limits must preserve a wider bounded recovery path: strict=%+v legacy=%+v", strict, legacy)
	}
}

func makeMinimalPDF(text string) []byte {
	var body strings.Builder
	body.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	writeObject := func(number int, value string) {
		offsets = append(offsets, body.Len())
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", number, value)
	}
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")
	writeObject(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	stream := "BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET\n"
	writeObject(5, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	xref := body.Len()
	fmt.Fprintf(&body, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&body, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(body.String())
}

func makeImportLimitArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
