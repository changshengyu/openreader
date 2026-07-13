package engine

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// readerDevUMDMagic is the little-endian 0xde9a9b89 header written by the
// UMD writer bundled with reader-dev (UmdHeader.buildHeader).
var readerDevUMDMagic = []byte{0x89, 0x9b, 0x9a, 0xde}

// legacyUMDMagic is not a reader-dev UMD signature. It is retained only so
// existing early OpenReader pseudo-UMD imports can still be explicitly
// reparsed; standard reader-dev UMD files always take the segmented parser.
var legacyUMDMagic = []byte{0x23, 0x54, 0x45, 0x58, 0x54, 0x4e, 0x4f, 0x56}

// umdMagic remains for the older allocation-limit regression fixture. New
// reader-dev compatibility fixtures must use readerDevUMDMagic instead.
var umdMagic = legacyUMDMagic

var errUMDContentLimit = errors.New("UMD decompressed content exceeds the limit")

type umdSegmentState struct {
	currentType        uint16
	textType           byte
	hasTextType        bool
	declaredContentLen int
	hasDeclaredLength  bool
	offsetCheck        uint32
	hasOffsetCheck     bool
	titleCheck         uint32
	hasTitleCheck      bool
	offsets            []int
	titles             []string
	content            bytes.Buffer
	segments           int
}

func ParseUMD(data []byte) (ParsedBook, error) {
	return ParseUMDWithLimits(data, LegacyLocalBookParseLimits())
}

func ParseUMDWithLimits(data []byte, limits LocalBookParseLimits) (ParsedBook, error) {
	limits = limits.normalized()
	switch {
	case bytes.HasPrefix(data, readerDevUMDMagic):
		return parseReaderDevUMDWithLimits(data, limits)
	case bytes.HasPrefix(data, legacyUMDMagic):
		return parseLegacyUMDWithLimits(data, limits)
	default:
		return ParsedBook{}, errors.New("not a valid UMD file")
	}
}

func parseReaderDevUMDWithLimits(data []byte, limits LocalBookParseLimits) (ParsedBook, error) {
	if len(data) < len(readerDevUMDMagic)+5 {
		return ParsedBook{}, errors.New("truncated UMD header")
	}

	state := umdSegmentState{declaredContentLen: -1}
	position := len(readerDevUMDMagic)
	maxSegments := maxUMDSegments(limits)
	var book ParsedBook

	for position < len(data) {
		state.segments++
		if state.segments > maxSegments {
			return ParsedBook{}, fmt.Errorf("%w: too many UMD sections", ErrLocalBookParseLimit)
		}
		marker := data[position]
		position++

		switch marker {
		case '#':
			if position+4 > len(data) {
				return ParsedBook{}, errors.New("truncated UMD section header")
			}
			segmentType := binary.LittleEndian.Uint16(data[position : position+2])
			// The flag is intentionally read even though text UMD semantics do
			// not require it. It is part of the reader-dev section framing.
			_ = data[position+2]
			sectionLength := int(data[position+3]) - 5
			position += 4
			if sectionLength < 0 || position+sectionLength > len(data) {
				return ParsedBook{}, errors.New("invalid UMD section length")
			}
			payload := data[position : position+sectionLength]
			position += sectionLength
			if segmentType != 0x00f1 && segmentType != 0x000a {
				state.currentType = segmentType
			}
			if err := parseReaderDevUMDSection(&state, segmentType, payload, &book, limits); err != nil {
				return ParsedBook{}, err
			}
		case '$':
			if position+8 > len(data) {
				return ParsedBook{}, errors.New("truncated UMD additional section header")
			}
			check := binary.LittleEndian.Uint32(data[position : position+4])
			encodedLength := binary.LittleEndian.Uint32(data[position+4 : position+8])
			position += 8
			if encodedLength < 9 {
				return ParsedBook{}, errors.New("invalid UMD additional section length")
			}
			payloadLength := int64(encodedLength - 9)
			if payloadLength > int64(len(data)-position) {
				return ParsedBook{}, errors.New("truncated UMD additional section")
			}
			payload := data[position : position+int(payloadLength)]
			position += int(payloadLength)
			if err := parseReaderDevUMDAdditional(&state, check, payload, limits); err != nil {
				return ParsedBook{}, err
			}
		default:
			return ParsedBook{}, fmt.Errorf("invalid UMD section marker 0x%x", marker)
		}
	}

	return buildReaderDevUMDBook(book, state, limits)
}

func parseReaderDevUMDSection(state *umdSegmentState, segmentType uint16, payload []byte, book *ParsedBook, limits LocalBookParseLimits) error {
	switch segmentType {
	case 0x0001:
		if len(payload) < 1 {
			return errors.New("truncated UMD type section")
		}
		state.textType = payload[0]
		state.hasTextType = true
		if state.textType != 0x01 {
			return errors.New("unsupported non-text UMD file")
		}
	case 0x0002:
		title, err := decodeUMDUTF16LE(payload)
		if err != nil {
			return fmt.Errorf("decode UMD title: %w", err)
		}
		if int64(len(title)) > limits.MaxParsedTextBytes {
			return fmt.Errorf("%w: UMD title exceeds the limit", ErrLocalBookParseLimit)
		}
		book.Title = title
	case 0x0003:
		author, err := decodeUMDUTF16LE(payload)
		if err != nil {
			return fmt.Errorf("decode UMD author: %w", err)
		}
		if int64(len(author)) > limits.MaxParsedTextBytes {
			return fmt.Errorf("%w: UMD author exceeds the limit", ErrLocalBookParseLimit)
		}
		book.Author = author
	case 0x000b:
		if len(payload) != 4 {
			return errors.New("invalid UMD content-length section")
		}
		declared := uint64(binary.LittleEndian.Uint32(payload))
		if declared > uint64(limits.MaxParsedTextBytes) {
			return fmt.Errorf("%w: UMD declared content exceeds the limit", ErrLocalBookParseLimit)
		}
		state.declaredContentLen = int(declared)
		state.hasDeclaredLength = true
	case 0x0083:
		if len(payload) != 4 {
			return errors.New("invalid UMD chapter-offset section")
		}
		state.offsetCheck = binary.LittleEndian.Uint32(payload)
		state.hasOffsetCheck = true
	case 0x0084:
		if len(payload) != 4 {
			return errors.New("invalid UMD chapter-title section")
		}
		state.titleCheck = binary.LittleEndian.Uint32(payload)
		state.hasTitleCheck = true
	}
	return nil
}

func parseReaderDevUMDAdditional(state *umdSegmentState, check uint32, payload []byte, limits LocalBookParseLimits) error {
	switch state.currentType {
	case 0x0083:
		if !state.hasOffsetCheck || check != state.offsetCheck {
			return errors.New("UMD chapter-offset check mismatch")
		}
		if state.offsets != nil || len(payload)%4 != 0 {
			return errors.New("invalid UMD chapter-offset payload")
		}
		count := len(payload) / 4
		if count == 0 {
			return errors.New("UMD has no chapter offsets")
		}
		if count > limits.MaxUMDChapters {
			return fmt.Errorf("%w: UMD declares too many chapters", ErrLocalBookParseLimit)
		}
		state.offsets = make([]int, count)
		for index := range state.offsets {
			offset := uint64(binary.LittleEndian.Uint32(payload[index*4 : index*4+4]))
			if offset > uint64(limits.MaxParsedTextBytes) {
				return fmt.Errorf("%w: UMD chapter offset exceeds the limit", ErrLocalBookParseLimit)
			}
			state.offsets[index] = int(offset)
		}
	case 0x0084:
		if !state.hasTitleCheck {
			return errors.New("UMD chapter titles have no check section")
		}
		if check == state.titleCheck {
			if state.titles != nil {
				return errors.New("duplicate UMD chapter-title payload")
			}
			titles, err := parseUMDTitles(payload, limits)
			if err != nil {
				return err
			}
			state.titles = titles
			return nil
		}
		if err := appendUMDCompressedChunk(&state.content, payload, limits.MaxParsedTextBytes); err != nil {
			if errors.Is(err, errUMDContentLimit) {
				return fmt.Errorf("%w: %v", ErrLocalBookParseLimit, err)
			}
			return fmt.Errorf("decompress UMD content: %w", err)
		}
	}
	return nil
}

func buildReaderDevUMDBook(book ParsedBook, state umdSegmentState, limits LocalBookParseLimits) (ParsedBook, error) {
	if !state.hasTextType || state.textType != 0x01 {
		return ParsedBook{}, errors.New("unsupported non-text UMD file")
	}
	if len(state.offsets) == 0 || len(state.titles) == 0 || len(state.offsets) != len(state.titles) {
		return ParsedBook{}, errors.New("UMD chapter offsets and titles do not match")
	}
	if state.hasDeclaredLength && state.declaredContentLen != state.content.Len() {
		return ParsedBook{}, errors.New("UMD decompressed content length does not match header")
	}
	if int64(state.content.Len()) > limits.MaxParsedTextBytes {
		return ParsedBook{}, fmt.Errorf("%w: UMD content exceeds the limit", ErrLocalBookParseLimit)
	}

	contents := state.content.Bytes()
	book.Chapters = make([]TXTChapter, 0, len(state.offsets))
	var parsedTextBytes int64
	for index, start := range state.offsets {
		end := len(contents)
		if index+1 < len(state.offsets) {
			end = state.offsets[index+1]
		}
		if start < 0 || end < start || end > len(contents) {
			return ParsedBook{}, errors.New("invalid UMD chapter offsets")
		}
		content, err := decodeUMDUTF16LE(contents[start:end])
		if err != nil {
			return ParsedBook{}, fmt.Errorf("decode UMD chapter content: %w", err)
		}
		content = strings.ReplaceAll(content, "\u2029", "\n")
		if int64(len(content)) > limits.MaxParsedTextBytes-parsedTextBytes {
			return ParsedBook{}, fmt.Errorf("%w: UMD parsed text exceeds the limit", ErrLocalBookParseLimit)
		}
		parsedTextBytes += int64(len(content))
		book.Chapters = append(book.Chapters, TXTChapter{
			Index:   index,
			Title:   state.titles[index],
			Start:   start,
			End:     end,
			Content: content,
		})
	}
	return book, nil
}

func parseUMDTitles(payload []byte, limits LocalBookParseLimits) ([]string, error) {
	titles := make([]string, 0)
	position := 0
	var totalBytes int64
	for position < len(payload) {
		length := int(payload[position])
		position++
		if position+length > len(payload) {
			return nil, errors.New("truncated UMD chapter title")
		}
		title, err := decodeUMDUTF16LE(payload[position : position+length])
		if err != nil {
			return nil, fmt.Errorf("decode UMD chapter title: %w", err)
		}
		if len(titles) >= limits.MaxUMDChapters || int64(len(title)) > limits.MaxParsedTextBytes-totalBytes {
			return nil, fmt.Errorf("%w: UMD chapter titles exceed the limit", ErrLocalBookParseLimit)
		}
		totalBytes += int64(len(title))
		titles = append(titles, title)
		position += length
	}
	if len(titles) == 0 {
		return nil, errors.New("UMD has no chapter titles")
	}
	return titles, nil
}

func appendUMDCompressedChunk(destination *bytes.Buffer, payload []byte, maxBytes int64) error {
	reader, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer reader.Close()
	writer := &umdLimitedWriter{destination: destination, remaining: maxBytes - int64(destination.Len())}
	if writer.remaining < 0 {
		return errUMDContentLimit
	}
	_, err = io.Copy(writer, reader)
	if err != nil {
		return err
	}
	return reader.Close()
}

type umdLimitedWriter struct {
	destination *bytes.Buffer
	remaining   int64
}

func (writer *umdLimitedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.remaining {
		allowed := int(writer.remaining)
		if allowed > 0 {
			_, _ = writer.destination.Write(data[:allowed])
			writer.remaining = 0
		}
		return allowed, errUMDContentLimit
	}
	count, err := writer.destination.Write(data)
	writer.remaining -= int64(count)
	return count, err
}

func decodeUMDUTF16LE(data []byte) (string, error) {
	if len(data)%2 != 0 {
		return "", errors.New("odd-length UTF-16LE value")
	}
	units := make([]uint16, len(data)/2)
	for index := range units {
		units[index] = binary.LittleEndian.Uint16(data[index*2 : index*2+2])
	}
	return string(utf16.Decode(units)), nil
}

func maxUMDSegments(limits LocalBookParseLimits) int {
	const overhead = 1024
	if limits.MaxUMDChapters > (int(^uint(0)>>1)-overhead)/4 {
		return int(^uint(0) >> 1)
	}
	return limits.MaxUMDChapters*4 + overhead
}

func parseLegacyUMDWithLimits(data []byte, limits LocalBookParseLimits) (ParsedBook, error) {
	if len(data) < 256 {
		return ParsedBook{}, errors.New("file too small for legacy UMD format")
	}

	book := ParsedBook{}
	position := 8 // skip legacy #TEXTNOV magic
	if position+6 > len(data) {
		return ParsedBook{}, errors.New("truncated legacy UMD header")
	}
	position += 4 // skip content length
	position += 2 // skip key

	title, advance, err := readLegacyUMDString(data, position)
	if err != nil {
		return ParsedBook{}, err
	}
	book.Title = title
	position = advance
	author, advance, err := readLegacyUMDString(data, position)
	if err != nil {
		return ParsedBook{}, err
	}
	book.Author = author
	position = advance

	if position+5 > len(data) {
		return ParsedBook{}, errors.New("truncated legacy UMD date")
	}
	position += 5
	if position+1 > len(data) {
		return ParsedBook{}, errors.New("truncated legacy UMD content types")
	}
	typeCount := int(data[position])
	position++
	for index := 0; index < typeCount; index++ {
		if position+3 > len(data) {
			return ParsedBook{}, errors.New("truncated legacy UMD content types")
		}
		position += 3
	}
	if position+4 > len(data) {
		return ParsedBook{}, errors.New("truncated legacy UMD chapter count")
	}
	chapterCountValue := uint64(binary.LittleEndian.Uint32(data[position : position+4]))
	position += 4
	if chapterCountValue > uint64(limits.MaxUMDChapters) {
		return ParsedBook{}, fmt.Errorf("%w: legacy UMD declares too many chapters", ErrLocalBookParseLimit)
	}
	if chapterCountValue+1 > uint64((len(data)-position)/4) {
		return ParsedBook{}, errors.New("truncated legacy UMD offset table")
	}
	chapterCount := int(chapterCountValue)
	offsets := make([]int, chapterCount+1)
	for index := range offsets {
		offsets[index] = int(binary.LittleEndian.Uint32(data[position : position+4]))
		position += 4
	}

	titles := make([]string, 0, chapterCount)
	for range chapterCount {
		if position >= len(data) {
			break
		}
		length := int(data[position])
		position++
		if position+length > len(data) {
			break
		}
		titles = append(titles, decodeGBK(data[position:position+length]))
		position += length
	}

	var parsedTextBytes int64
	for index := 0; index < chapterCount; index++ {
		start := offsets[index]
		end := offsets[index+1]
		if start < 0 || end < start {
			return ParsedBook{}, errors.New("invalid legacy UMD chapter offsets")
		}
		if start >= len(data) {
			continue
		}
		if end > len(data) {
			end = len(data)
		}
		content := decodeGBK(data[start:end])
		if strings.TrimSpace(content) == "" {
			continue
		}
		if int64(len(content)) > limits.MaxParsedTextBytes-parsedTextBytes {
			return ParsedBook{}, fmt.Errorf("%w: legacy UMD parsed text exceeds the limit", ErrLocalBookParseLimit)
		}
		parsedTextBytes += int64(len(content))
		chapterTitle := fmt.Sprintf("第 %d 章", index+1)
		if index < len(titles) && titles[index] != "" {
			chapterTitle = titles[index]
		}
		book.Chapters = append(book.Chapters, TXTChapter{Index: index, Title: chapterTitle, Start: start, End: end, Content: content})
	}
	if len(book.Chapters) == 0 {
		return ParsedBook{}, errors.New("no readable chapters found in legacy UMD file")
	}
	if book.Title == "" {
		book.Title = "未命名 UMD 书"
	}
	return book, nil
}

func readLegacyUMDString(data []byte, position int) (string, int, error) {
	if position >= len(data) {
		return "", position, errors.New("truncated legacy UMD string length")
	}
	length := int(data[position])
	position++
	if position+length > len(data) {
		return "", position, errors.New("truncated legacy UMD string")
	}
	return decodeGBK(data[position : position+length]), position + length, nil
}

func decodeGBK(data []byte) string {
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
	if err != nil {
		return string(data)
	}
	return string(decoded)
}
