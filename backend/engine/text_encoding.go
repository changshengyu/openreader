package engine

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	unicodeencoding "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"golang.org/x/text/transform"
)

var htmlCharsetPattern = regexp.MustCompile(`(?i)<meta\b[^>]*\bcharset\s*=\s*["']?\s*([a-z0-9._:-]+)`)

var commonSimplifiedRunes = runeSet("的一是不了在人有我他这中大来上个国到说们为子和你地出道也时年得就那要下以生会自着去之家学对可里后小么心多天而能好都然没日于起还发成事只作当想看文无开手十用主行方又如前所本见经头面公同三已老从动两长知民样现分将外但身些与高意把法回")
var commonTraditionalRunes = runeSet("的一是不了在人有我他這中大來上個國到說們為子和你地出道也時年得就那要下以生會自著去之家學對可裡後小麼心多天而能好都然沒日於起還發成事只作當想看文無開手十用主行方又如前所本見經頭面公同三已老從動兩長知民樣現分將外但身些與高意把法回")

var errUnableToDetectTextEncoding = errors.New("unable to detect text encoding")

type legacyTextEncoding struct {
	name         string
	encoding     encoding.Encoding
	commonRunes  map[rune]struct{}
	primaryRange *unicode.RangeTable
}

var (
	gb18030TextEncoding  = legacyTextEncoding{name: "gb18030", encoding: simplifiedchinese.GB18030, commonRunes: commonSimplifiedRunes, primaryRange: unicode.Han}
	big5TextEncoding     = legacyTextEncoding{name: "big5", encoding: traditionalchinese.Big5, commonRunes: commonTraditionalRunes, primaryRange: unicode.Han}
	shiftJISTextEncoding = legacyTextEncoding{name: "shift_jis", encoding: japanese.ShiftJIS, primaryRange: unicode.Hiragana}
	eucJPTextEncoding    = legacyTextEncoding{name: "euc-jp", encoding: japanese.EUCJP, primaryRange: unicode.Hiragana}
	eucKRTextEncoding    = legacyTextEncoding{name: "euc-kr", encoding: korean.EUCKR, primaryRange: unicode.Hangul}
)

// icuMultibyteTextEncodings maps the multibyte charsets of reader-dev's ICU
// detector to OpenReader's decoders.
var icuMultibyteTextEncodings = map[string]legacyTextEncoding{
	"Shift_JIS": shiftJISTextEncoding,
	"GB18030":   gb18030TextEncoding,
	"EUC-JP":    eucJPTextEncoding,
	"EUC-KR":    eucKRTextEncoding,
	"Big5":      big5TextEncoding,
}

// textEncodingsByName holds every non-Unicode charset detection can return.
var textEncodingsByName = map[string]encoding.Encoding{
	"gb18030":      simplifiedchinese.GB18030,
	"big5":         traditionalchinese.Big5,
	"shift_jis":    japanese.ShiftJIS,
	"euc-jp":       japanese.EUCJP,
	"iso-2022-jp":  japanese.ISO2022JP,
	"euc-kr":       korean.EUCKR,
	"iso-8859-1":   charmap.ISO8859_1,
	"windows-1252": charmap.Windows1252,
	"iso-8859-2":   charmap.ISO8859_2,
	"windows-1250": charmap.Windows1250,
	"iso-8859-5":   charmap.ISO8859_5,
	"iso-8859-6":   charmap.ISO8859_6,
	"iso-8859-7":   charmap.ISO8859_7,
	"windows-1253": charmap.Windows1253,
	"iso-8859-8":   charmap.ISO8859_8,
	"windows-1255": charmap.Windows1255,
	"windows-1251": charmap.Windows1251,
	"windows-1256": charmap.Windows1256,
	"koi8-r":       charmap.KOI8R,
	"iso-8859-9":   charmap.ISO8859_9,
	"windows-1254": charmap.Windows1254,
}

// icuTextEncodingName returns OpenReader's name for an ICU charset name.
// ISO-8859-8-I is logical-order Hebrew with the bytes of ISO-8859-8; upstream
// passes the -I name to Charset.forName, which rejects it.
func icuTextEncodingName(icuName string) string {
	if icuName == "ISO-8859-8-I" {
		return "iso-8859-8"
	}
	return strings.ToLower(icuName)
}

func detectAndDecodeText(data []byte) (string, string, error) {
	return detectAndDecodeTextSample(data, true)
}

// detectAndDecodeTextSample detects the charset of data and decodes it. atEOF
// is false when data is a probe cut from the start of a longer document: the
// cut can split the probe's final multibyte character, whose remaining bytes
// follow in the document, so legacy candidates leave that character out
// instead of failing on the U+FFFD it would decode to at end of input.
func detectAndDecodeTextSample(data []byte, atEOF bool) (string, string, error) {
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
	if text, ok := decodeISO2022JP(data, atEOF); ok {
		return text, "iso-2022-jp", nil
	}
	if utf8.Valid(data) {
		return string(data), "utf-8", nil
	}
	if looksLikeUTF16(data, true) {
		return decodeKnownText(data, "utf-16le", unicodeencoding.UTF16(unicodeencoding.LittleEndian, unicodeencoding.IgnoreBOM))
	}
	if looksLikeUTF16(data, false) {
		return decodeKnownText(data, "utf-16be", unicodeencoding.UTF16(unicodeencoding.BigEndian, unicodeencoding.IgnoreBOM))
	}
	// Corrupted UTF-8 is recognized before the legacy charsets, which would
	// also tolerate its malformed bytes and decode it as mojibake.
	if mostlyUTF8(data) {
		return string(data), "utf-8", nil
	}

	analysis := analyzeICUCharsets(data)
	if text, name, decided := decodeICUMultibyte(analysis, data, atEOF); decided {
		if name == "" {
			return "", "", errUnableToDetectTextEncoding
		}
		return text, name, nil
	}
	// A single-byte charset needs n-gram evidence, and is not trusted over a
	// sample that also reads as a few clean multibyte characters: ICU's
	// single-byte confidences on short Chinese, Japanese or Korean text are as
	// high as on real single-byte text.
	if single := analysis.bestSingleByte(); single.confidence > icuNoEvidenceConfidence && !analysis.shortCleanMultibyte() {
		name := icuTextEncodingName(single.name)
		decoded, err := textEncodingsByName[name].NewDecoder().Bytes(data)
		if err != nil {
			return "", "", err
		}
		return string(decoded), name, nil
	}

	// Without ICU evidence, keep OpenReader's earlier choice among its first
	// four legacy charsets.
	candidates := []legacyTextEncoding{gb18030TextEncoding, big5TextEncoding, shiftJISTextEncoding, eucKRTextEncoding}
	bestScore := -1 << 30
	bestName := ""
	bestText := ""
	bestClean := false
	for _, candidate := range candidates {
		decoded, err := decodeLegacyText(candidate.encoding, data, atEOF)
		if err != nil || !utf8.Valid(decoded) {
			continue
		}
		text := string(decoded)
		malformed, multibyte := countDecodedRunes(text)
		// reader-dev's ICU CharsetRecog_mbcs keeps a charset while its valid
		// multibyte characters outnumber malformed ones at least 20 to 1.
		if multibyte < 20*malformed {
			continue
		}
		// A candidate that decodes without replacement characters still wins,
		// so every file that decoded cleanly before keeps its charset.
		clean := malformed == 0
		score := scoreDecodedText(text, candidate)
		if (clean && !bestClean) || (clean == bestClean && score > bestScore) {
			bestScore = score
			bestName = candidate.name
			bestText = text
			bestClean = clean
		}
	}
	if bestName == "" {
		return "", "", errUnableToDetectTextEncoding
	}
	return bestText, bestName, nil
}

// icuASCIIConfidence is what reader-dev's ICU UTF-8 recognizer gives plain
// ASCII.
const icuASCIIConfidence = 15

// decodeISO2022JP recognizes ISO-2022-JP, which is 7-bit and would otherwise
// pass as UTF-8. Like reader-dev's ICU it is chosen when its escape sequences
// score at least as high as plain ASCII and above every single-byte charset.
func decodeISO2022JP(data []byte, atEOF bool) (string, bool) {
	if bytes.IndexByte(data, 0x1B) < 0 {
		return "", false
	}
	for _, current := range data {
		if current >= utf8.RuneSelf {
			return "", false
		}
	}
	analysis := analyzeICUCharsets(data)
	ranked := rankICUMatches(analysis.iso2022)
	if len(ranked) == 0 || ranked[0].name != "ISO-2022-JP" || ranked[0].confidence < icuASCIIConfidence || ranked[0].confidence <= analysis.bestSingleByte().confidence {
		return "", false
	}
	decoded, err := decodeLegacyText(japanese.ISO2022JP, data, atEOF)
	if err != nil || !utf8.Valid(decoded) {
		return "", false
	}
	text := string(decoded)
	if malformed, multibyte := countDecodedRunes(text); multibyte < 20*malformed {
		return "", false
	}
	return text, true
}

// decodeICUMultibyte follows reader-dev's ICU when a multibyte recognizer
// found frequent characters of its language (a confidence above
// icuNoEvidenceConfidence) and outranks every single-byte charset. Charsets
// tied for the top are told apart by scoreDecodedText rather than ICU's list
// order, and the decode must keep the malformed tolerance. decided reports
// whether the ICU evidence settled the charset; name is empty when every
// decode it settled on is too damaged.
func decodeICUMultibyte(analysis icuCharsetAnalysis, data []byte, atEOF bool) (text string, name string, decided bool) {
	best := 0
	for _, match := range analysis.multibyte {
		best = max(best, match.confidence)
	}
	if best <= icuNoEvidenceConfidence || best <= analysis.bestSingleByte().confidence {
		return "", "", false
	}
	bestScore := 0
	for _, match := range analysis.multibyte {
		if match.confidence != best {
			continue
		}
		candidate := icuMultibyteTextEncodings[match.name]
		decoded, err := decodeLegacyText(candidate.encoding, data, atEOF)
		if err != nil || !utf8.Valid(decoded) {
			continue
		}
		candidateText := string(decoded)
		if malformed, multibyte := countDecodedRunes(candidateText); multibyte < 20*malformed {
			continue
		}
		if score := scoreDecodedText(candidateText, candidate); name == "" || score > bestScore {
			text, name, bestScore = candidateText, candidate.name, score
		}
	}
	return text, name, true
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

// decodeLegacyText decodes data with a legacy multibyte charset. When atEOF is
// false, an incomplete multibyte sequence at the end of data is left undecoded
// rather than replaced with U+FFFD.
func decodeLegacyText(textEncoding encoding.Encoding, data []byte, atEOF bool) ([]byte, error) {
	if atEOF {
		return textEncoding.NewDecoder().Bytes(data)
	}
	decoder := textEncoding.NewDecoder()
	decoded := make([]byte, 0, len(data)*3/2)
	buffer := make([]byte, 64*1024)
	for {
		nDst, nSrc, err := decoder.Transform(buffer, data, false)
		decoded = append(decoded, buffer[:nDst]...)
		data = data[nSrc:]
		switch {
		case errors.Is(err, transform.ErrShortDst):
			// buffer is full; decode the rest of data.
		case errors.Is(err, transform.ErrShortSrc):
			// Only the incomplete final character is left in data.
			return decoded, nil
		default:
			return decoded, err
		}
	}
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
	}
	textEncoding, ok := textEncodingsByName[name]
	if !ok {
		return "", fmt.Errorf("unsupported text encoding %q", name)
	}
	decoded, err := textEncoding.NewDecoder().Bytes(data)
	return string(decoded), err
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

// mostlyUTF8 reports whether reader-dev's ICU CharsetRecog_UTF8 still matches
// data: its valid multibyte sequences outnumber invalid ones more than
// tenfold. As in ICU, a sequence cut off by the end of data counts as neither.
func mostlyUTF8(data []byte) bool {
	valid, invalid := 0, 0
	for index := 0; index < len(data); index++ {
		current := data[index]
		if current < utf8.RuneSelf {
			continue
		}
		var trailing int
		switch {
		case current&0xE0 == 0xC0:
			trailing = 1
		case current&0xF0 == 0xE0:
			trailing = 2
		case current&0xF8 == 0xF0:
			trailing = 3
		default:
			invalid++
			continue
		}
		for trailing > 0 {
			index++
			if index >= len(data) {
				break
			}
			if data[index]&0xC0 != 0x80 {
				invalid++
				break
			}
			trailing--
		}
		if trailing == 0 {
			valid++
		}
	}
	return valid > 0 && valid > 10*invalid
}

// countDecodedRunes returns how many U+FFFD replacement characters and how
// many other non-ASCII characters text contains.
func countDecodedRunes(text string) (malformed, multibyte int) {
	for _, current := range text {
		switch {
		case current == utf8.RuneError:
			malformed++
		case current >= utf8.RuneSelf:
			multibyte++
		}
	}
	return malformed, multibyte
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
