// © 2016 and later: Unicode, Inc. and others.
// License & terms of use: http://www.unicode.org/copyright.html
// Copyright (C) 2005-2013, International Business Machines Corporation and
// others. All Rights Reserved.

package engine

import (
	"math"
	"sort"
)

// This file ports the charset recognizers of reader-dev's copy of the ICU4J
// CharsetDetector (src/main/java/io/legado/app/lib/icu4j at reader-dev
// fa22f271849d45f93349ae1636223e27b16a4691), so OpenReader ranks Asian
// multibyte, ISO-2022 and single-byte charsets the way upstream does. The
// UTF-8/16/32 recognizers are not ported: detectAndDecodeTextSample settles
// those encodings before it consults this ranking.

// icuInputLimit is CharsetDetector.kBufSize, the prefix the ISO-2022 and
// single-byte recognizers read; the multibyte recognizers read all the data.
const icuInputLimit = 8000

// icuNoEvidenceConfidence is the confidence ICU's multibyte recognizers give
// input that holds no more than ten multibyte characters and no malformed
// ones: not incompatible with the charset, but no evidence for it either.
const icuNoEvidenceConfidence = 10

// Positions in CharsetDetector's recognizer list. Of two matches with the same
// confidence, detectAll ranks the one later in this list first.
const (
	icuOrderShiftJIS = iota + 5
	icuOrderISO2022JP
	icuOrderISO2022CN
	icuOrderISO2022KR
	icuOrderGB18030
	icuOrderEUCJP
	icuOrderEUCKR
	icuOrderBig5
	icuOrder8859_1
	icuOrder8859_2
	icuOrder8859_5
	icuOrder8859_6
	icuOrder8859_7
	icuOrder8859_8I
	icuOrder8859_8
	icuOrderWindows1251
	icuOrderWindows1256
	icuOrderKOI8R
	icuOrder8859_9
)

type icuMatch struct {
	name       string
	confidence int
	order      int
}

// icuMultibyteMatch is a multibyte recognizer's result together with the
// counts it was computed from.
type icuMultibyteMatch struct {
	icuMatch
	doubleByte int
	malformed  int
	total      int
}

// icuCharsetAnalysis holds every ported recognizer's result for one sample.
// Recognizers that ICU would leave out of detectAll (confidence 0) have a
// zero confidence here.
type icuCharsetAnalysis struct {
	multibyte  []icuMultibyteMatch
	iso2022    []icuMatch
	singleByte []icuMatch
}

func analyzeICUCharsets(data []byte) icuCharsetAnalysis {
	input := data
	if len(input) > icuInputLimit {
		input = input[:icuInputLimit]
	}
	c1 := false
	for _, current := range input {
		if current >= 0x80 && current <= 0x9F {
			c1 = true
			break
		}
	}
	return icuCharsetAnalysis{
		multibyte: []icuMultibyteMatch{
			matchICUMultibyte(data, "Shift_JIS", icuOrderShiftJIS, nextICUShiftJISChar, icuSJISCommonChars),
			matchICUMultibyte(data, "GB18030", icuOrderGB18030, nextICUGB18030Char, icuGB18030CommonChars),
			matchICUMultibyte(data, "EUC-JP", icuOrderEUCJP, nextICUEUCChar, icuEUCJPCommonChars),
			matchICUMultibyte(data, "EUC-KR", icuOrderEUCKR, nextICUEUCChar, icuEUCKRCommonChars),
			matchICUMultibyte(data, "Big5", icuOrderBig5, nextICUBig5Char, icuBig5CommonChars),
		},
		iso2022: []icuMatch{
			{name: "ISO-2022-JP", confidence: matchICUISO2022(input, icuISO2022JPEscapes), order: icuOrderISO2022JP},
			{name: "ISO-2022-CN", confidence: matchICUISO2022(input, icuISO2022CNEscapes), order: icuOrderISO2022CN},
			{name: "ISO-2022-KR", confidence: matchICUISO2022(input, icuISO2022KREscapes), order: icuOrderISO2022KR},
		},
		singleByte: []icuMatch{
			{name: icuC1Name(c1, "windows-1252", "ISO-8859-1"), confidence: bestICULanguageConfidence(input, icu88591NGrams, &icu88591ByteMap), order: icuOrder8859_1},
			{name: icuC1Name(c1, "windows-1250", "ISO-8859-2"), confidence: bestICULanguageConfidence(input, icu88592NGrams, &icu88592ByteMap), order: icuOrder8859_2},
			{name: "ISO-8859-5", confidence: icuNGramConfidence(input, &icu88595ruNGrams, &icu88595ByteMap), order: icuOrder8859_5},
			{name: "ISO-8859-6", confidence: icuNGramConfidence(input, &icu88596arNGrams, &icu88596ByteMap), order: icuOrder8859_6},
			{name: icuC1Name(c1, "windows-1253", "ISO-8859-7"), confidence: icuNGramConfidence(input, &icu88597elNGrams, &icu88597ByteMap), order: icuOrder8859_7},
			{name: icuC1Name(c1, "windows-1255", "ISO-8859-8-I"), confidence: icuNGramConfidence(input, &icu88598IheNGrams, &icu88598ByteMap), order: icuOrder8859_8I},
			{name: icuC1Name(c1, "windows-1255", "ISO-8859-8"), confidence: icuNGramConfidence(input, &icu88598heNGrams, &icu88598ByteMap), order: icuOrder8859_8},
			{name: "windows-1251", confidence: icuNGramConfidence(input, &icuWindows1251NGrams, &icuWindows1251ByteMap), order: icuOrderWindows1251},
			{name: "windows-1256", confidence: icuNGramConfidence(input, &icuWindows1256NGrams, &icuWindows1256ByteMap), order: icuOrderWindows1256},
			{name: "KOI8-R", confidence: icuNGramConfidence(input, &icuKOI8RNGrams, &icuKOI8RByteMap), order: icuOrderKOI8R},
			{name: icuC1Name(c1, "windows-1254", "ISO-8859-9"), confidence: icuNGramConfidence(input, &icu88599trNGrams, &icu88599ByteMap), order: icuOrder8859_9},
		},
	}
}

// ranked returns the matches with a nonzero confidence in detectAll's order.
func (analysis icuCharsetAnalysis) ranked() []icuMatch {
	matches := make([]icuMatch, 0, len(analysis.multibyte)+len(analysis.iso2022)+len(analysis.singleByte))
	for _, match := range analysis.multibyte {
		matches = append(matches, match.icuMatch)
	}
	matches = append(matches, analysis.iso2022...)
	matches = append(matches, analysis.singleByte...)
	return rankICUMatches(matches)
}

func rankICUMatches(matches []icuMatch) []icuMatch {
	ranked := make([]icuMatch, 0, len(matches))
	for _, match := range matches {
		if match.confidence > 0 {
			ranked = append(ranked, match)
		}
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].confidence != ranked[right].confidence {
			return ranked[left].confidence > ranked[right].confidence
		}
		return ranked[left].order > ranked[right].order
	})
	return ranked
}

// bestSingleByte returns detectAll's first single-byte match, or a zero match.
func (analysis icuCharsetAnalysis) bestSingleByte() icuMatch {
	if ranked := rankICUMatches(analysis.singleByte); len(ranked) > 0 {
		return ranked[0]
	}
	return icuMatch{}
}

// shortCleanMultibyte reports whether a multibyte recognizer read the sample
// as at most ten multibyte characters, none malformed, making up at least half
// of its characters: short Chinese, Japanese or Korean text, for which ICU
// gives no evidence either way. ICU takes the same branch for single-byte text
// with a few accented letters ("ASCII or ISO file?"), where the multibyte
// characters are a small part of the sample.
func (analysis icuCharsetAnalysis) shortCleanMultibyte() bool {
	for _, match := range analysis.multibyte {
		if match.malformed == 0 && match.doubleByte > 0 && match.doubleByte <= 10 && 2*match.doubleByte >= match.total {
			return true
		}
	}
	return false
}

func icuC1Name(c1 bool, windowsName string, isoName string) string {
	if c1 {
		return windowsName
	}
	return isoName
}

// icuIteratedChar is CharsetRecog_mbcs.iteratedChar: a character as the raw
// bytes that make it up, packed into charValue.
type icuIteratedChar struct {
	charValue uint32
	nextIndex int
	malformed bool
	done      bool
}

func (it *icuIteratedChar) nextByte(data []byte) int {
	if it.nextIndex >= len(data) {
		it.done = true
		return -1
	}
	current := int(data[it.nextIndex])
	it.nextIndex++
	return current
}

type icuNextChar func(it *icuIteratedChar, data []byte) bool

// matchICUMultibyte ports CharsetRecog_mbcs.match.
func matchICUMultibyte(data []byte, name string, order int, nextChar icuNextChar, commonChars []uint32) icuMultibyteMatch {
	doubleByte, common, malformed, total := 0, 0, 0, 0
	result := func(confidence int) icuMultibyteMatch {
		return icuMultibyteMatch{icuMatch: icuMatch{name: name, confidence: confidence, order: order}, doubleByte: doubleByte, malformed: malformed, total: total}
	}
	it := icuIteratedChar{}
	for nextChar(&it, data) {
		total++
		if it.malformed {
			malformed++
		} else if it.charValue > 0xFF {
			doubleByte++
			index := sort.Search(len(commonChars), func(index int) bool { return commonChars[index] >= it.charValue })
			if index < len(commonChars) && commonChars[index] == it.charValue {
				common++
			}
		}
		if malformed >= 2 && malformed*5 >= doubleByte {
			return result(0)
		}
	}
	if doubleByte <= 10 && malformed == 0 {
		if doubleByte == 0 && total < 10 {
			return result(0)
		}
		return result(icuNoEvidenceConfidence)
	}
	if doubleByte < 20*malformed {
		return result(0)
	}
	maxVal := math.Log(float64(float32(doubleByte) / 4))
	scaleFactor := 90.0 / maxVal
	confidence := int(math.Log(float64(common+1))*scaleFactor + 10)
	return result(min(confidence, 100))
}

// nextICUShiftJISChar ports CharsetRecog_sjis.nextChar.
func nextICUShiftJISChar(it *icuIteratedChar, data []byte) bool {
	it.malformed = false
	first := it.nextByte(data)
	if first < 0 {
		return false
	}
	it.charValue = uint32(first)
	if first <= 0x7F || (first > 0xA0 && first <= 0xDF) {
		return true
	}
	second := it.nextByte(data)
	if second < 0 {
		return false
	}
	it.charValue = uint32(first<<8 | second)
	if !((second >= 0x40 && second <= 0x7F) || (second >= 0x80 && second <= 0xFF)) {
		it.malformed = true
	}
	return true
}

// nextICUBig5Char ports CharsetRecog_big5.nextChar.
func nextICUBig5Char(it *icuIteratedChar, data []byte) bool {
	it.malformed = false
	first := it.nextByte(data)
	if first < 0 {
		return false
	}
	it.charValue = uint32(first)
	if first <= 0x7F || first == 0xFF {
		return true
	}
	second := it.nextByte(data)
	if second < 0 {
		return false
	}
	it.charValue = uint32(first<<8 | second)
	if second < 0x40 || second == 0x7F || second == 0xFF {
		it.malformed = true
	}
	return true
}

// nextICUEUCChar ports CharsetRecog_euc.nextChar, shared by EUC-JP and EUC-KR.
func nextICUEUCChar(it *icuIteratedChar, data []byte) bool {
	it.malformed = false
	first := it.nextByte(data)
	if first < 0 {
		it.done = true
		return false
	}
	it.charValue = uint32(first)
	if first <= 0x8D {
		return true
	}
	second := it.nextByte(data)
	it.charValue = it.charValue<<8 | uint32(second)
	switch {
	case first >= 0xA1 && first <= 0xFE:
		if second < 0xA1 {
			it.malformed = true
		}
	case first == 0x8E:
		if second < 0xA1 {
			it.malformed = true
		}
	case first == 0x8F:
		third := it.nextByte(data)
		it.charValue = it.charValue<<8 | uint32(third)
		if third < 0xA1 {
			it.malformed = true
		}
	}
	return !it.done
}

// nextICUGB18030Char ports CharsetRecog_gb_18030.nextChar, including its test
// of the second byte against decimal 80.
func nextICUGB18030Char(it *icuIteratedChar, data []byte) bool {
	it.malformed = false
	first := it.nextByte(data)
	if first < 0 {
		it.done = true
		return false
	}
	it.charValue = uint32(first)
	if first <= 0x80 {
		return true
	}
	second := it.nextByte(data)
	it.charValue = it.charValue<<8 | uint32(second)
	if first >= 0x81 && first <= 0xFE {
		if (second >= 0x40 && second <= 0x7E) || (second >= 80 && second <= 0xFE) {
			return !it.done
		}
		if second >= 0x30 && second <= 0x39 {
			third := it.nextByte(data)
			if third >= 0x81 && third <= 0xFE {
				fourth := it.nextByte(data)
				if fourth >= 0x30 && fourth <= 0x39 {
					it.charValue = it.charValue<<16 | uint32(third<<8|fourth)
					return !it.done
				}
			}
		}
		it.malformed = true
	}
	return !it.done
}

var (
	icuISO2022JPEscapes = [][]byte{
		{0x1B, 0x24, 0x28, 0x43}, {0x1B, 0x24, 0x28, 0x44}, {0x1B, 0x24, 0x40}, {0x1B, 0x24, 0x41},
		{0x1B, 0x24, 0x42}, {0x1B, 0x26, 0x40}, {0x1B, 0x28, 0x42}, {0x1B, 0x28, 0x48},
		{0x1B, 0x28, 0x49}, {0x1B, 0x28, 0x4A}, {0x1B, 0x2E, 0x41}, {0x1B, 0x2E, 0x46},
	}
	icuISO2022KREscapes = [][]byte{{0x1B, 0x24, 0x29, 0x43}}
	icuISO2022CNEscapes = [][]byte{
		{0x1B, 0x24, 0x29, 0x41}, {0x1B, 0x24, 0x29, 0x47}, {0x1B, 0x24, 0x2A, 0x48}, {0x1B, 0x24, 0x29, 0x45},
		{0x1B, 0x24, 0x2B, 0x49}, {0x1B, 0x24, 0x2B, 0x4A}, {0x1B, 0x24, 0x2B, 0x4B}, {0x1B, 0x24, 0x2B, 0x4C},
		{0x1B, 0x24, 0x2B, 0x4D}, {0x1B, 0x4E}, {0x1B, 0x4F},
	}
)

// matchICUISO2022 ports CharsetRecog_2022.match.
func matchICUISO2022(text []byte, escapes [][]byte) int {
	hits, misses, shifts := 0, 0, 0
scan:
	for index := 0; index < len(text); index++ {
		if text[index] == 0x1B {
		escapeSequences:
			for _, sequence := range escapes {
				if len(text)-index < len(sequence) {
					continue
				}
				for offset := 1; offset < len(sequence); offset++ {
					if sequence[offset] != text[index+offset] {
						continue escapeSequences
					}
				}
				hits++
				index += len(sequence) - 1
				continue scan
			}
			misses++
		}
		if text[index] == 0x0E || text[index] == 0x0F {
			shifts++
		}
	}
	if hits == 0 {
		return 0
	}
	quality := (100*hits - 100*misses) / (hits + misses)
	if hits+shifts < 5 {
		quality -= (5 - (hits + shifts)) * 10
	}
	return max(quality, 0)
}

type icuLanguageNGrams struct {
	lang   string
	ngrams [64]uint32
}

// bestICULanguageConfidence ports the language loop of CharsetRecog_8859_1
// and CharsetRecog_8859_2: the best confidence of their languages.
func bestICULanguageConfidence(input []byte, languages []icuLanguageNGrams, byteMap *[256]byte) int {
	best := -1
	for index := range languages {
		best = max(best, icuNGramConfidence(input, &languages[index].ngrams, byteMap))
	}
	return max(best, 0)
}

// icuNGramConfidence ports CharsetRecog_sbcs.NGramParser.parse with the space
// character 0x20.
func icuNGramConfidence(input []byte, ngrams *[64]uint32, byteMap *[256]byte) int {
	const space = 0x20
	var ngram uint32
	ngramCount, hitCount := 0, 0
	addByte := func(current byte) {
		ngram = (ngram<<8 + uint32(current)) & 0xFFFFFF
		ngramCount++
		if searchICUNGrams(ngrams, ngram) {
			hitCount++
		}
	}
	ignoreSpace := false
	for _, current := range input {
		mapped := byteMap[current]
		if mapped == 0 {
			continue
		}
		if !(mapped == space && ignoreSpace) {
			addByte(mapped)
		}
		ignoreSpace = mapped == space
	}
	addByte(space)
	rawPercent := float64(hitCount) / float64(ngramCount)
	if rawPercent > 0.33 {
		return 98
	}
	return int(rawPercent * 300.0)
}

// searchICUNGrams ports NGramParser.search over its 64-entry sorted table.
func searchICUNGrams(table *[64]uint32, value uint32) bool {
	index := 0
	for _, step := range [...]int{32, 16, 8, 4, 2, 1} {
		if table[index+step] <= value {
			index += step
		}
	}
	if table[index] > value {
		index--
	}
	return index >= 0 && table[index] == value
}
