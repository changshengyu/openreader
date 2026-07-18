package engine

import (
	"regexp"
	"strings"
)

var (
	sourceBookNamePattern         = regexp.MustCompile(`\s+作\s*者.*|\s+\S+\s+著`)
	sourceBookAuthorPattern       = regexp.MustCompile(`^\s*作\s*者[:：\s]+|\s+著`)
	sourceIntroBlockTagPattern    = regexp.MustCompile(`(?i)<(br[\s/]*|/*p\b.*?|/*div\b.*?)>`)
	sourceIntroNoisePattern       = regexp.MustCompile(`<[script>]*.*?>|&nbsp;`)
	sourceIntroLineBreakPattern   = regexp.MustCompile(`\s*\n+\s*`)
	sourceIntroLeadingWhitespace  = regexp.MustCompile(`^[\n\s]+`)
	sourceIntroTrailingWhitespace = regexp.MustCompile(`[\n\s]+$`)
)

// formatSourceBookName mirrors reader-dev BookHelp.formatBookName for remote
// source results. It deliberately trims only ASCII control/space runes after
// applying the fixed-baseline suffix rules.
func formatSourceBookName(value string) string {
	return trimSourceASCIIWhitespace(sourceBookNamePattern.ReplaceAllString(value, ""))
}

// formatSourceBookAuthor mirrors reader-dev BookHelp.formatBookAuthor. This is
// intentionally narrower than the separate formatAuthor helper upstream uses
// in other contexts.
func formatSourceBookAuthor(value string) string {
	return trimSourceASCIIWhitespace(sourceBookAuthorPattern.ReplaceAllString(value, ""))
}

// formatSourceBookIntro mirrors the fixed-baseline htmlFormat transformation
// while keeping the result as inert text. Remote response limits bound input
// size before this linear sequence is reached.
func formatSourceBookIntro(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	value = sourceIntroBlockTagPattern.ReplaceAllString(value, "\n")
	value = sourceIntroNoisePattern.ReplaceAllString(value, "")
	value = sourceIntroLineBreakPattern.ReplaceAllString(value, "\n　　")
	value = sourceIntroLeadingWhitespace.ReplaceAllString(value, "　　")
	return sourceIntroTrailingWhitespace.ReplaceAllString(value, "")
}

func trimSourceASCIIWhitespace(value string) string {
	return strings.TrimFunc(value, func(r rune) bool { return r <= ' ' })
}
