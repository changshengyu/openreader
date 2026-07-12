package engine

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// UMD magic bytes: header starts with these bytes for text novels.
var umdMagic = []byte{0x23, 0x54, 0x45, 0x58, 0x54, 0x4E, 0x4F, 0x56}

func ParseUMD(data []byte) (ParsedBook, error) {
	return ParseUMDWithLimits(data, LegacyLocalBookParseLimits())
}

func ParseUMDWithLimits(data []byte, limits LocalBookParseLimits) (ParsedBook, error) {
	limits = limits.normalized()
	if len(data) < 256 {
		return ParsedBook{}, errors.New("file too small for UMD format")
	}

	if !bytes.HasPrefix(data, umdMagic) {
		return ParsedBook{}, errors.New("not a valid UMD file")
	}

	umdContent := data
	book := ParsedBook{}

	pos := 8 // skip magic

	// --- content length and key ---
	if pos+4 > len(umdContent) {
		return ParsedBook{}, errors.New("truncated UMD header")
	}
	pos += 4 // skip content length
	pos += 2 // skip key

	// --- title ---
	title, advance, err := readUMDString(umdContent, pos)
	if err != nil {
		return ParsedBook{}, err
	}
	book.Title = title
	pos = advance

	// --- author ---
	author, advance, err := readUMDString(umdContent, pos)
	if err != nil {
		return ParsedBook{}, err
	}
	book.Author = author
	pos = advance

	// --- dates ---
	if pos+5 > len(umdContent) {
		return ParsedBook{}, errors.New("truncated UMD date")
	}
	pos += 5 // year(2) + month(1) + day(1) + reserved(1)

	// --- content type IDs ---
	if pos+3 > len(umdContent) {
		return ParsedBook{}, errors.New("truncated UMD content types")
	}
	typeCount := int(umdContent[pos])
	pos++

	// skip content type IDs
	for i := 0; i < typeCount; i++ {
		if pos+3 > len(umdContent) {
			break
		}
		pos += 3
	}

	// --- chapter count ---
	if pos+4 > len(umdContent) {
		return ParsedBook{}, errors.New("truncated UMD chapter count")
	}
	chapterCountValue := uint64(binary.LittleEndian.Uint32(umdContent[pos : pos+4]))
	pos += 4
	if chapterCountValue > uint64(limits.MaxUMDChapters) {
		return ParsedBook{}, fmt.Errorf("%w: UMD declares too many chapters", ErrLocalBookParseLimit)
	}
	if chapterCountValue+1 > uint64((len(umdContent)-pos)/4) {
		return ParsedBook{}, errors.New("truncated UMD offset table")
	}
	chapterCount := int(chapterCountValue)

	// --- chapter offsets ---
	offsets := make([]int, chapterCount+1)
	for i := 0; i <= chapterCount; i++ {
		if pos+4 > len(umdContent) {
			return ParsedBook{}, errors.New("truncated UMD offset table")
		}
		offsets[i] = int(binary.LittleEndian.Uint32(umdContent[pos : pos+4]))
		pos += 4
	}

	// --- chapter titles ---
	titles := make([]string, 0, chapterCount)
	for range chapterCount {
		if pos+1 > len(umdContent) {
			break
		}
		strLen := int(umdContent[pos])
		pos++
		if strLen == 0 {
			titles = append(titles, "")
			continue
		}
		if pos+strLen > len(umdContent) {
			break
		}
		titles = append(titles, decodeGBK(umdContent[pos:pos+strLen]))
		pos += strLen
	}

	// --- chapter content ---
	for i := 0; i < chapterCount && i < len(offsets)-1; i++ {
		startOffset := offsets[i]
		endOffset := offsets[i+1]
		if endOffset < startOffset {
			return ParsedBook{}, errors.New("invalid UMD chapter offsets")
		}
		if startOffset >= len(umdContent) {
			continue
		}
		if endOffset > len(umdContent) {
			endOffset = len(umdContent)
		}

		rawContent := umdContent[startOffset:endOffset]
		content := decodeGBK(rawContent)
		if strings.TrimSpace(content) == "" {
			continue
		}

		title := fmt.Sprintf("第 %d 章", i+1)
		if i < len(titles) && titles[i] != "" {
			title = titles[i]
		}

		book.Chapters = append(book.Chapters, TXTChapter{
			Index:   i,
			Title:   title,
			Start:   startOffset,
			End:     endOffset,
			Content: content,
		})
	}

	if len(book.Chapters) == 0 {
		return ParsedBook{}, errors.New("no readable chapters found in UMD file")
	}

	if book.Title == "" {
		book.Title = "未命名 UMD 书"
	}

	return book, nil
}

func readUMDString(data []byte, pos int) (string, int, error) {
	if pos+1 > len(data) {
		return "", pos, errors.New("truncated UMD string length")
	}
	strLen := int(data[pos])
	pos++
	if strLen == 0 {
		return "", pos, nil
	}
	if pos+strLen > len(data) {
		return "", pos, errors.New("truncated UMD string")
	}
	raw := data[pos : pos+strLen]
	pos += strLen
	return decodeGBK(raw), pos, nil
}

func decodeGBK(data []byte) string {
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
	if err != nil {
		return string(data)
	}
	return string(decoded)
}
