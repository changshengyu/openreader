package engine

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	unicodeencoding "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
)

var htmlCharsetPattern = regexp.MustCompile(`(?i)<meta\b[^>]*\bcharset\s*=\s*["']?\s*([a-z0-9._:-]+)`)

var commonSimplifiedRunes = runeSet("的一是不了在人有我他这中大来上个国到说们为子和你地出道也时年得就那要下以生会自着去之家学对可里后小么心多天而能好都然没日于起还发成事只作当想看文无开手十用主行方又如前所本见经头面公同三已老从动两长知民样现分将外但身些与高意把法回")
var commonTraditionalRunes = runeSet("的一是不了在人有我他這中大來上個國到說們為子和你地出道也時年得就那要下以生會自著去之家學對可裡後小麼心多天而能好都然沒日於起還發成事只作當想看文無開手十用主行方又如前所本見經頭面公同三已老從動兩長知民樣現分將外但身些與高意把法回")

type legacyTextEncoding struct {
	name         string
	encoding     encoding.Encoding
	commonRunes  map[rune]struct{}
	primaryRange *unicode.RangeTable
}

func detectAndDecodeText(data []byte) (string, string, error) {
	if len(data) == 0 {
		return "", "utf-8", nil
	}
	if bytes.HasPrefix(data, []byte{0x00, 0x00, 0xFE, 0xFF}) {
		return decodeKnownText(data[4:], "utf-32be", utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM))
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE, 0x00, 0x00}) {
		return decodeKnownText(data[4:], "utf-32le", utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM))
	}
	if bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return decodeKnownText(data[2:], "utf-16be", unicodeencoding.UTF16(unicodeencoding.BigEndian, unicodeencoding.IgnoreBOM))
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
		return decodeKnownText(data[2:], "utf-16le", unicodeencoding.UTF16(unicodeencoding.LittleEndian, unicodeencoding.IgnoreBOM))
	}

	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(data) {
		return string(data), "utf-8", nil
	}
	if looksLikeUTF16(data, true) {
		return decodeKnownText(data, "utf-16le", unicodeencoding.UTF16(unicodeencoding.LittleEndian, unicodeencoding.IgnoreBOM))
	}
	if looksLikeUTF16(data, false) {
		return decodeKnownText(data, "utf-16be", unicodeencoding.UTF16(unicodeencoding.BigEndian, unicodeencoding.IgnoreBOM))
	}

	candidates := []legacyTextEncoding{
		{name: "gb18030", encoding: simplifiedchinese.GB18030, commonRunes: commonSimplifiedRunes, primaryRange: unicode.Han},
		{name: "big5", encoding: traditionalchinese.Big5, commonRunes: commonTraditionalRunes, primaryRange: unicode.Han},
		{name: "shift_jis", encoding: japanese.ShiftJIS, primaryRange: unicode.Hiragana},
		{name: "euc-kr", encoding: korean.EUCKR, primaryRange: unicode.Hangul},
	}
	bestScore := -1 << 30
	bestName := ""
	bestText := ""
	for _, candidate := range candidates {
		decoded, err := candidate.encoding.NewDecoder().Bytes(data)
		if err != nil || !utf8.Valid(decoded) || bytes.ContainsRune(decoded, utf8.RuneError) {
			continue
		}
		text := string(decoded)
		score := scoreDecodedText(text, candidate)
		if score > bestScore {
			bestScore = score
			bestName = candidate.name
			bestText = text
		}
	}
	if bestName == "" {
		return "", "", fmt.Errorf("unable to detect text encoding")
	}
	return bestText, bestName, nil
}

func detectHTMLCharset(data []byte) string {
	sample := data
	if len(sample) > 64*1024 {
		sample = sample[:64*1024]
	}
	lowerSample := bytes.ToLower(sample)
	if headStart := bytes.Index(lowerSample, []byte("<head")); headStart >= 0 {
		headEnd := bytes.Index(lowerSample[headStart:], []byte("</head>"))
		if headEnd >= 0 {
			headEnd += headStart + len("</head>")
			sample = sample[headStart:headEnd]
		}
	} else if bodyStart := bytes.Index(lowerSample, []byte("<body")); bodyStart >= 0 {
		sample = sample[:bodyStart]
	}
	match := htmlCharsetPattern.FindSubmatch(sample)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(string(match[1]))
}

func decodeKnownText(data []byte, name string, textEncoding encoding.Encoding) (string, string, error) {
	decoded, err := textEncoding.NewDecoder().Bytes(data)
	if err != nil {
		return "", "", err
	}
	return string(decoded), name, nil
}

// decodeTextWithEncoding decodes the complete staged input using the charset
// identified from TextFile's initial probe. Keeping detection and full-file
// decoding on the same charset matches reader-dev's local TXT pipeline.
func decodeTextWithEncoding(data []byte, name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "utf-8", "utf8":
		return string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})), nil
	case "utf-32be":
		data = bytes.TrimPrefix(data, []byte{0x00, 0x00, 0xFE, 0xFF})
		decoded, err := utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM).NewDecoder().Bytes(data)
		return string(decoded), err
	case "utf-32le":
		data = bytes.TrimPrefix(data, []byte{0xFF, 0xFE, 0x00, 0x00})
		decoded, err := utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM).NewDecoder().Bytes(data)
		return string(decoded), err
	case "utf-16be":
		data = bytes.TrimPrefix(data, []byte{0xFE, 0xFF})
		decoded, err := unicodeencoding.UTF16(unicodeencoding.BigEndian, unicodeencoding.IgnoreBOM).NewDecoder().Bytes(data)
		return string(decoded), err
	case "utf-16le":
		data = bytes.TrimPrefix(data, []byte{0xFF, 0xFE})
		decoded, err := unicodeencoding.UTF16(unicodeencoding.LittleEndian, unicodeencoding.IgnoreBOM).NewDecoder().Bytes(data)
		return string(decoded), err
	case "gb18030":
		decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
		return string(decoded), err
	case "big5":
		decoded, err := traditionalchinese.Big5.NewDecoder().Bytes(data)
		return string(decoded), err
	case "shift_jis":
		decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(data)
		return string(decoded), err
	case "euc-kr":
		decoded, err := korean.EUCKR.NewDecoder().Bytes(data)
		return string(decoded), err
	default:
		return "", fmt.Errorf("unsupported text encoding %q", name)
	}
}

func looksLikeUTF16(data []byte, littleEndian bool) bool {
	pairs := len(data) / 2
	if pairs < 4 {
		return false
	}
	zeroes := 0
	for index := 0; index+1 < len(data); index += 2 {
		zeroIndex := index
		if littleEndian {
			zeroIndex = index + 1
		}
		if data[zeroIndex] == 0 {
			zeroes++
		}
	}
	return zeroes*10 >= pairs*3
}

func scoreDecodedText(text string, candidate legacyTextEncoding) int {
	score := 0
	for _, current := range text {
		switch {
		case current == '\n' || current == '\r' || current == '\t' || current == ' ':
			score += 2
		case unicode.IsControl(current):
			score -= 40
		case unicode.Is(candidate.primaryRange, current):
			score += 8
		case unicode.Is(unicode.Han, current):
			score += 4
		case unicode.IsLetter(current) || unicode.IsDigit(current):
			score += 2
		case unicode.IsPunct(current):
			score++
		}
		if _, ok := candidate.commonRunes[current]; ok {
			score += 8
		}
	}
	return score
}

func runeSet(value string) map[rune]struct{} {
	result := make(map[rune]struct{}, utf8.RuneCountInString(value))
	for _, current := range value {
		result[current] = struct{}{}
	}
	return result
}
