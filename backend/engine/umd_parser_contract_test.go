package engine

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"testing"
)

// readerDevUMDFixture mirrors the UMD writer bundled in the fixed reader-dev
// baseline (UmdHeader/UmdChapters). It deliberately uses the segmented
// 0xde9a9b89 format instead of the historical OpenReader-only #TEXTNOV layout.
func readerDevUMDFixture(t *testing.T, title, author string, chapters []umdFixtureChapter, chunkBytes int) []byte {
	t.Helper()
	if chunkBytes <= 0 {
		chunkBytes = 32 * 1024
	}

	var result bytes.Buffer
	writeSection := func(segmentType uint16, flag byte, payload []byte) {
		t.Helper()
		if len(payload)+5 > 0xff {
			t.Fatalf("fixture section payload too large: %d", len(payload))
		}
		result.WriteByte('#')
		var header [2]byte
		binary.LittleEndian.PutUint16(header[:], segmentType)
		result.Write(header[:])
		result.WriteByte(flag)
		result.WriteByte(byte(len(payload) + 5))
		result.Write(payload)
	}
	writeAdditional := func(check uint32, payload []byte) {
		t.Helper()
		result.WriteByte('$')
		var header [4]byte
		binary.LittleEndian.PutUint32(header[:], check)
		result.Write(header[:])
		binary.LittleEndian.PutUint32(header[:], uint32(len(payload)+9))
		result.Write(header[:])
		result.Write(payload)
	}
	writeUint32 := func(value uint32) []byte {
		var payload [4]byte
		binary.LittleEndian.PutUint32(payload[:], value)
		return payload[:]
	}

	// UmdHeader.buildHeader(): the UMD magic is little-endian 0xde9a9b89.
	result.Write([]byte{0x89, 0x9b, 0x9a, 0xde})
	writeSection(0x01, 0, []byte{0x01, 0x11, 0x22})
	if title != "" {
		writeSection(0x02, 0, umdUTF16LE(title))
	}
	if author != "" {
		writeSection(0x03, 0, umdUTF16LE(author))
	}

	var contents bytes.Buffer
	offsets := make([]uint32, 0, len(chapters))
	titles := make([]byte, 0)
	for _, chapter := range chapters {
		offsets = append(offsets, uint32(contents.Len()))
		contents.Write(umdUTF16LE(chapter.content))
		encodedTitle := umdUTF16LE(chapter.title)
		if len(encodedTitle) > 0xff {
			t.Fatalf("fixture chapter title too large: %d", len(encodedTitle))
		}
		titles = append(titles, byte(len(encodedTitle)))
		titles = append(titles, encodedTitle...)
	}

	writeSection(0x0b, 0, writeUint32(uint32(contents.Len())))
	const offsetsCheck uint32 = 0x11223344
	writeSection(0x83, 0, writeUint32(offsetsCheck))
	offsetPayload := make([]byte, len(offsets)*4)
	for index, offset := range offsets {
		binary.LittleEndian.PutUint32(offsetPayload[index*4:], offset)
	}
	writeAdditional(offsetsCheck, offsetPayload)

	const titlesCheck uint32 = 0x55667788
	writeSection(0x84, 1, writeUint32(titlesCheck))
	writeAdditional(titlesCheck, titles)
	chunkChecks := make([]uint32, 0)
	for start := 0; start < contents.Len(); start += chunkBytes {
		end := start + chunkBytes
		if end > contents.Len() {
			end = contents.Len()
		}
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		if _, err := writer.Write(contents.Bytes()[start:end]); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		chunkCheck := uint32(titlesCheck + uint32(start) + 1)
		chunkChecks = append(chunkChecks, chunkCheck)
		writeAdditional(chunkCheck, compressed.Bytes())
		// UmdChapters.writeChaptersChunks writes an F1 padding section after
		// every compressed body chunk before the next additional section.
		writeSection(0x00f1, 0, make([]byte, 16))
	}
	// The upstream writer ends the chunk stream with an 81 section followed
	// by the chunk checks in reverse order. Reader-dev does not use that
	// table while reading text, but its framing must be accepted.
	writeSection(0x0081, 1, make([]byte, 4))
	finalChecks := make([]byte, len(chunkChecks)*4)
	for index := range chunkChecks {
		binary.LittleEndian.PutUint32(finalChecks[index*4:], chunkChecks[len(chunkChecks)-index-1])
	}
	writeAdditional(0, finalChecks)
	return result.Bytes()
}

type umdFixtureChapter struct {
	title   string
	content string
}

func umdUTF16LE(value string) []byte {
	encoded := make([]byte, 0, len(value)*2)
	for _, unit := range []rune(value) {
		if unit <= 0xffff {
			encoded = append(encoded, byte(unit), byte(unit>>8))
			continue
		}
		adjusted := unit - 0x10000
		high := rune(0xd800 + (adjusted >> 10))
		low := rune(0xdc00 + (adjusted & 0x3ff))
		encoded = append(encoded, byte(high), byte(high>>8), byte(low), byte(low>>8))
	}
	return encoded
}

func TestParseUMDWithLimitsReadsReaderDevSegmentedTextUMD(t *testing.T) {
	data := readerDevUMDFixture(t, "上游 UMD", "测试作者", []umdFixtureChapter{
		{title: "第一章", content: "第一段\u2029第二段"},
		{title: "第二章", content: "第二章正文"},
	}, 32*1024)

	book, err := ParseUMDWithLimits(data, DefaultLocalBookParseLimits())
	if err != nil {
		t.Fatalf("reader-dev UMD parse: %v", err)
	}
	if book.Title != "上游 UMD" || book.Author != "测试作者" {
		t.Fatalf("reader-dev UMD metadata = %+v", book)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("reader-dev UMD chapters = %+v", book.Chapters)
	}
	if book.Chapters[0].Title != "第一章" || book.Chapters[0].Content != "第一段\n第二段" {
		t.Fatalf("first reader-dev UMD chapter = %+v", book.Chapters[0])
	}
	if book.Chapters[1].Index != 1 || book.Chapters[1].Title != "第二章" || book.Chapters[1].Content != "第二章正文" {
		t.Fatalf("second reader-dev UMD chapter = %+v", book.Chapters[1])
	}
}

func TestParseUMDWithLimitsConcatenatesReaderDevCompressedChunksInOrder(t *testing.T) {
	data := readerDevUMDFixture(t, "分块 UMD", "作者", []umdFixtureChapter{
		{title: "第一章", content: "跨分块正文一"},
		{title: "第二章", content: "跨分块正文二"},
	}, 6)

	book, err := ParseUMDWithLimits(data, DefaultLocalBookParseLimits())
	if err != nil {
		t.Fatalf("chunked reader-dev UMD parse: %v", err)
	}
	if len(book.Chapters) != 2 || book.Chapters[0].Content != "跨分块正文一" || book.Chapters[1].Content != "跨分块正文二" {
		t.Fatalf("chunked reader-dev UMD chapters = %+v", book.Chapters)
	}
}

func TestParseUMDWithLimitsRejectsMalformedOrOverBudgetReaderDevUMD(t *testing.T) {
	fixture := readerDevUMDFixture(t, "边界 UMD", "作者", []umdFixtureChapter{
		{title: "第一章", content: "可验证正文"},
	}, 32*1024)

	truncated := fixture[:len(fixture)-2]
	if _, err := ParseUMDWithLimits(truncated, DefaultLocalBookParseLimits()); err == nil {
		t.Fatal("truncated reader-dev UMD unexpectedly parsed")
	}

	badCompressedChunk := append([]byte(nil), fixture...)
	compressedStart := bytes.Index(badCompressedChunk, []byte{0x78, 0x9c})
	if compressedStart < 0 || compressedStart+2 >= len(badCompressedChunk) {
		t.Fatal("fixture has no zlib body to corrupt")
	}
	badCompressedChunk[compressedStart+2] ^= 0xff
	if _, err := ParseUMDWithLimits(badCompressedChunk, DefaultLocalBookParseLimits()); err == nil {
		t.Fatal("corrupted reader-dev UMD zlib chunk unexpectedly parsed")

	}

	imageUMD := append([]byte(nil), fixture...)
	imageUMD[len(readerDevUMDMagic)+5] = 0x02
	if _, err := ParseUMDWithLimits(imageUMD, DefaultLocalBookParseLimits()); err == nil {
		t.Fatal("image UMD unexpectedly parsed as text")
	}

	strict := DefaultLocalBookParseLimits()
	strict.MaxParsedTextBytes = 4
	if _, err := ParseUMDWithLimits(fixture, strict); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("over-budget reader-dev UMD error = %v, want %v", err, ErrLocalBookParseLimit)
	}
}

func TestParseUMDWithLimitsKeepsLegacyPseudoUMDFallback(t *testing.T) {
	data := makeLegacyOpenReaderUMDFixture(t)
	book, err := ParseUMDWithLimits(data, DefaultLocalBookParseLimits())
	if err != nil {
		t.Fatalf("legacy pseudo-UMD parse: %v", err)
	}
	if book.Title != "Legacy UMD" || book.Author != "Legacy Author" || len(book.Chapters) != 1 {
		t.Fatalf("legacy pseudo-UMD metadata = %+v", book)
	}
	if book.Chapters[0].Title != "Legacy Chapter" || book.Chapters[0].Content != "legacy content" {
		t.Fatalf("legacy pseudo-UMD chapter = %+v", book.Chapters[0])
	}
}

func makeLegacyOpenReaderUMDFixture(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 320)
	copy(data, legacyUMDMagic)
	position := 8 + 4 + 2
	writeString := func(value string) {
		data[position] = byte(len(value))
		position++
		copy(data[position:], value)
		position += len(value)
	}
	writeString("Legacy UMD")
	writeString("Legacy Author")
	position += 5
	data[position] = 0
	position++
	binary.LittleEndian.PutUint32(data[position:position+4], 1)
	position += 4
	const contentStart = 260
	const contentEnd = contentStart + len("legacy content")
	binary.LittleEndian.PutUint32(data[position:position+4], uint32(contentStart))
	position += 4
	binary.LittleEndian.PutUint32(data[position:position+4], uint32(contentEnd))
	position += 4
	writeString("Legacy Chapter")
	copy(data[contentStart:contentEnd], "legacy content")
	return data
}
