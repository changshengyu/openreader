package contentsearch

import (
	"strings"
	"unicode/utf16"
)

// Match describes one reader-dev-compatible exact chapter match. ByteOffset is
// retained for OpenReader's internal cache/progress calculations, while the
// query indexes and excerpt use Java/Kotlin UTF-16 positions for legacy clients.
type Match struct {
	ByteOffset          int
	QueryIndexInResult  int
	QueryIndexInChapter int
	Excerpt             string
}

// Find returns exact, case-sensitive matches in source order. Advancing from
// the previous byte position plus one preserves reader-dev's overlapping-match
// behavior while strings.Index still returns only valid query byte sequences.
func Find(content string, query string, limit int) ([]Match, bool) {
	if content == "" || query == "" || limit <= 0 {
		return nil, false
	}
	matches := make([]Match, 0, min(limit, 32))
	for start := 0; start <= len(content) && len(matches) <= limit; {
		relative := strings.Index(content[start:], query)
		if relative < 0 {
			break
		}
		byteOffset := start + relative
		excerpt, queryIndexInResult, queryIndexInChapter := excerptAround(
			content,
			byteOffset,
			query,
			20,
		)
		matches = append(matches, Match{
			ByteOffset:          byteOffset,
			QueryIndexInResult:  queryIndexInResult,
			QueryIndexInChapter: queryIndexInChapter,
			Excerpt:             excerpt,
		})
		start = byteOffset + 1
	}
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}
	return matches, truncated
}

func excerptAround(content string, byteOffset int, query string, radius int) (string, int, int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(content) {
		byteOffset = len(content)
	}
	contentUnits := utf16.Encode([]rune(content))
	queryIndexInChapter := len(utf16.Encode([]rune(content[:byteOffset])))
	queryWidth := len(utf16.Encode([]rune(query)))
	start := max(0, queryIndexInChapter-radius)
	end := min(len(contentUnits), queryIndexInChapter+queryWidth+radius)
	return string(utf16.Decode(contentUnits[start:end])), queryIndexInChapter - start, queryIndexInChapter
}
