package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	txtDetectionProbeBytes = 512000
	txtNoTocChunkBytes     = 10 * 1024
	txtNoTocShortTailBytes = 100
	MaxTXTTocRuleBytes     = 16 * 1024

	txtForwardDoubleTitleRule  = `(?m)(?<=[ \t　]{0,4})第[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$(?=[\s　]{0,8}第[\d零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章)`
	txtBackwardDoubleTitleRule = `(?m)(?<=[ \t　]{0,4}第[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$[\s　]{0,8})第[\d零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$`

	// txtZhangHuiRule is OpenReader rule -19 "目录(章回、话)"
	// (docs/compat/txt-toc-rule-selection-fixed-baseline-second-audit-p1-contract.md
	// section 6.1): upstream rule -2 "目录" plus 第x回/第x话 headings, which
	// may follow a 第x章 (节、卷、集、部、篇) prefix on the same line. reader-dev
	// and Legado only know 回 in disabled rules; Readest keeps 章 and 回 in
	// one alternation so a 章 that wraps several 回 keeps both levels.
	txtZhangHuiRuleID = -19
	txtZhangHuiRule   = `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))|(?:第\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张)).{0,30}?[ 　\t])?第\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:回(?![合来事去])|话)).{0,30}$`

	// Rule -19 only replaces reader-dev's automatic choice when, counting
	// headings the way Legado's current getTocRule does (a match counts only
	// when it starts more than txtSpacedHeadingDistance characters after the
	// previously counted one ends), it finds txtZhangHuiMinimumLead more
	// headings than every enabled upstream rule. A 章 line directly followed
	// by its 回 line, a leading table of contents or one stray 第一回
	// paragraph therefore keeps reader-dev's rule.
	txtSpacedHeadingDistance = 1000
	txtZhangHuiMinimumLead   = 2
)

var ChapterTitlePattern = regexp.MustCompile(`^(?:第[零一二三四五六七八九十百千万两〇○0-9０-９]+[章回节卷集部]|序章|楔子|引子|前言|尾声|后记|番外(?:篇)?|第[零一二三四五六七八九十百千万两〇○0-9０-９]+卷|[上中下]卷).{0,64}$`)

type txtTitleMatcher struct {
	pattern    *regexp.Regexp
	lookaheads []txtNegativeLookahead
	context    txtTitleContext
}

type txtTitleContext uint8

const (
	txtTitleContextNone txtTitleContext = iota
	txtTitleContextForward
	txtTitleContextBackward
)

var (
	txtForwardFirstTitlePattern  = regexp.MustCompile(`^第[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$`)
	txtForwardSecondTitlePattern = regexp.MustCompile(`^第[\d零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章`)
	txtBackwardTitlePattern      = regexp.MustCompile(`^第[\d零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$`)
)

// txtNegativeLookahead stands in for a Java negative lookahead such as
// 部(?![分赛游]): RE2 has no lookarounds, so the lookahead is compiled into an
// empty named group and, after a match, the text right after that group must
// not start with any excluded literal. Like Java, only the position the
// lookahead sits at is checked, not the rest of the line.
type txtNegativeLookahead struct {
	name     string
	group    int
	excluded []string
}

func (matcher *txtTitleMatcher) MatchString(line string) bool {
	if matcher == nil {
		return ChapterTitlePattern.MatchString(line)
	}
	if len(matcher.lookaheads) == 0 {
		return matcher.pattern.MatchString(line)
	}
	match := matcher.pattern.FindStringSubmatchIndex(line)
	if match == nil {
		return false
	}
	for _, lookahead := range matcher.lookaheads {
		if lookahead.group <= 0 || 2*lookahead.group >= len(match) {
			continue
		}
		position := match[2*lookahead.group]
		if position < 0 {
			continue
		}
		for _, excluded := range lookahead.excluded {
			if strings.HasPrefix(line[position:], excluded) {
				return false
			}
		}
	}
	return true
}

func (matcher *txtTitleMatcher) MatchText(text string, lineStart, lineEnd int, line string) bool {
	if !matcher.MatchString(line) {
		return false
	}
	switch matcher.context {
	case txtTitleContextForward:
		lineText := strings.TrimRight(text[lineStart:lineEnd], "\r\n")
		return nextTXTTitleWithinWhitespace(text, lineStart+len(lineText), 8)
	case txtTitleContextBackward:
		return previousTXTTitleWithinWhitespace(text, lineStart, 8)
	default:
		return true
	}
}

func nextTXTTitleWithinWhitespace(text string, start, maxRunes int) bool {
	position := start
	spaces := 0
	for position < len(text) {
		r, size := utf8.DecodeRuneInString(text[position:])
		if !unicode.IsSpace(r) && r != '　' {
			break
		}
		spaces++
		if spaces > maxRunes {
			return false
		}
		position += size
	}
	return position > start && txtForwardSecondTitlePattern.MatchString(text[position:])
}

func previousTXTTitleWithinWhitespace(text string, start, maxRunes int) bool {
	position := start
	spaces := 0
	for position > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:position])
		if !unicode.IsSpace(r) && r != '　' {
			break
		}
		spaces++
		if spaces > maxRunes {
			return false
		}
		position -= size
	}
	if position == start || position == 0 {
		return false
	}
	previousStart := strings.LastIndex(text[:position], "\n") + 1
	previous := strings.TrimSpace(strings.TrimPrefix(text[previousStart:position], "\ufeff"))
	return txtForwardFirstTitlePattern.MatchString(previous)
}

type TXTChapter struct {
	Index               int    `json:"index"`
	Title               string `json:"title"`
	Start               int    `json:"start"`
	End                 int    `json:"end"`
	Content             string `json:"content"`
	ResourcePath        string `json:"resourcePath,omitempty"`
	ResourceFragment    string `json:"resourceFragment,omitempty"`
	ResourceEndFragment string `json:"resourceEndFragment,omitempty"`
}

type TXTTocRule struct {
	ID           int    `json:"id"`
	Enable       bool   `json:"enable"`
	Name         string `json:"name"`
	Rule         string `json:"rule"`
	SerialNumber int    `json:"serialNumber"`
}

func DefaultTXTTocRules() []TXTTocRule {
	return []TXTTocRule{
		{ID: -1, Enable: true, Name: "目录(去空白)", Rule: `(?<=[　\s])(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))).{0,30}$`, SerialNumber: 0},
		{ID: -2, Enable: true, Name: "目录", Rule: `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))).{0,30}$`, SerialNumber: 1},
		{ID: -3, Enable: false, Name: "目录(匹配简介)", Rule: `(?<=[　\s])(?:(?:内容|文章)?简介|文案|前言|序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|回(?![合来事去])|场(?![和合比电是])|篇(?!张))).{0,30}$`, SerialNumber: 2},
		{ID: -4, Enable: false, Name: "目录(古典、轻小说备用)", Rule: `^[ 　\t]{0,4}(?:序章|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|回(?![合来事去])|场(?![和合比电是])|话|篇(?!张))).{0,30}$`, SerialNumber: 3},
		{ID: -5, Enable: false, Name: "数字(纯数字标题)", Rule: `(?<=[　\s])\d+\.?[ 　\t]{0,4}$`, SerialNumber: 4},
		{ID: -6, Enable: true, Name: "数字 分隔符 标题名称", Rule: `^[ 　\t]{0,4}\d{1,5}[：:,.， 、_—\-].{1,30}$`, SerialNumber: 5},
		{ID: -7, Enable: true, Name: "大写数字 分隔符 标题名称", Rule: `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|[〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 、_—\-].{1,30}$`, SerialNumber: 6},
		{ID: -8, Enable: true, Name: "正文 标题/序号", Rule: `^[ 　\t]{0,4}正文[ 　]{1,4}.{0,20}$`, SerialNumber: 7},
		{ID: -9, Enable: true, Name: "Chapter/Section/Part/Episode 序号 标题", Rule: `^[ 　\t]{0,4}(?:[Cc]hapter|[Ss]ection|[Pp]art|ＰＡＲＴ|[Nn][oO]\.|[Ee]pisode|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)\s{0,4}\d{1,4}.{0,30}$`, SerialNumber: 8},
		{ID: -10, Enable: false, Name: "Chapter(去简介)", Rule: `^[ 　\t]{0,4}(?:[Cc]hapter|[Ss]ection|[Pp]art|ＰＡＲＴ|[Nn][Oo]\.|[Ee]pisode)\s{0,4}\d{1,4}.{0,30}$`, SerialNumber: 9},
		{ID: -11, Enable: true, Name: "特殊符号 序号 标题", Rule: `(?<=[\s　])[【〔〖「『〈［\[](?:第|[Cc]hapter)[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,10}[章节].{0,20}$`, SerialNumber: 10},
		{ID: -12, Enable: false, Name: "特殊符号 标题(成对)", Rule: `(?<=[\s　]{0,4})(?:[\[〈「『〖〔《（【\(].{1,30}[\)】）》〕〗』」〉\]]?|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)[ 　]{0,4}$`, SerialNumber: 11},
		{ID: -13, Enable: true, Name: "特殊符号 标题(单个)", Rule: `(?<=[\s　]{0,4})(?:[☆★✦✧].{1,30}|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)[ 　]{0,4}$`, SerialNumber: 12},
		{ID: -14, Enable: true, Name: "章/卷 序号 标题", Rule: `^[ \t　]{0,4}(?:(?:内容|文章)?简介|文案|前言|序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|[卷章][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 　]{0,4}.{0,30}$`, SerialNumber: 13},
		{ID: -15, Enable: false, Name: "顶格标题", Rule: `^\S.{1,20}$`, SerialNumber: 14},
		{ID: -16, Enable: false, Name: "双标题(前向)", Rule: txtForwardDoubleTitleRule, SerialNumber: 15},
		{ID: -17, Enable: false, Name: "双标题(后向)", Rule: txtBackwardDoubleTitleRule, SerialNumber: 16},
		{ID: -18, Enable: true, Name: "标题 特殊符号 序号", Rule: `^.{1,20}[(（][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}[)）][ 　\t]{0,4}$`, SerialNumber: 17},
		// OpenReader extension after the 18 fixed-upstream rules (see txtZhangHuiRule).
		{ID: txtZhangHuiRuleID, Enable: true, Name: "目录(章回、话)", Rule: txtZhangHuiRule, SerialNumber: 18},
	}
}

func ParseTXT(data []byte) ([]TXTChapter, error) {
	return ParseTXTWithLimits(data, "", LegacyLocalBookParseLimits())
}

func ParseTXTWithRule(data []byte, rule string) ([]TXTChapter, error) {
	return ParseTXTWithLimits(data, rule, LegacyLocalBookParseLimits())
}

func ParseTXTWithLimits(data []byte, rule string, limits LocalBookParseLimits) ([]TXTChapter, error) {
	chapters, _, err := parseTXTCatalogWithLimits(data, rule, limits)
	return chapters, err
}

// parseTXTCatalogWithLimits also returns the rule the catalogue was split
// with: the trimmed explicit rule, or the automatically selected default rule
// (reader-dev TextFile.getChapterList writes it back to book.tocUrl). It is
// empty when no rule matched and the 10-KiB fallback was used.
func parseTXTCatalogWithLimits(data []byte, rule string, limits LocalBookParseLimits) ([]TXTChapter, string, error) {
	limits = limits.normalized()
	if int64(len(data)) > limits.MaxArchiveBytes {
		return nil, "", txtParseLimitError("TXT input exceeds the limit")
	}
	text, probe, err := decodeTXTForCatalog(data)
	if err != nil {
		return nil, "", err
	}
	if int64(len(text)) > limits.MaxParsedTextBytes {
		return nil, "", txtParseLimitError("TXT decoded text exceeds the limit")
	}
	rule = strings.TrimSpace(rule)
	if len(rule) > MaxTXTTocRuleBytes {
		return nil, "", txtParseLimitError("TXT TOC rule exceeds the limit")
	}
	if rule == "" {
		pattern, detected := detectTXTTitlePattern(probe)
		chapters, err := parseTXTText(text, pattern, limits.MaxParsedChapters)
		return chapters, detected, err
	}
	pattern, err := compileTXTTitleMatcher(rule)
	if err != nil {
		return nil, "", err
	}
	chapters, err := parseTXTText(text, pattern, limits.MaxParsedChapters)
	return chapters, rule, err
}

func txtParseLimitError(message string) error {
	return fmt.Errorf("%w: %s", ErrLocalBookParseLimit, message)
}

func parseTXTText(text string, titlePattern *txtTitleMatcher, maxChapters int) ([]TXTChapter, error) {
	if titlePattern == nil {
		return parseTXTWithoutToc(text, maxChapters)
	}

	chapters := make([]TXTChapter, 0)
	var current TXTChapter
	hasCurrent := false
	contentStart := 0
	for lineStart := 0; lineStart < len(text); {
		lineEnd := nextLineEnd(text, lineStart)
		lineText := strings.TrimRight(text[lineStart:lineEnd], "\r\n")
		line := strings.TrimSpace(lineText)
		if titlePattern.MatchText(text, lineStart, lineEnd, line) {
			if hasCurrent {
				content := strings.TrimSpace(text[contentStart:lineStart])
				current.Index = len(chapters)
				current.End = lineStart
				current.Content = content
				if err := appendLimitedTXTChapter(&chapters, current, maxChapters); err != nil {
					return nil, err
				}
			} else if preface := strings.TrimSpace(text[:lineStart]); preface != "" {
				if err := appendLimitedTXTChapter(&chapters, TXTChapter{
					Index:   len(chapters),
					Title:   "前言",
					Start:   0,
					End:     lineStart,
					Content: preface,
				}, maxChapters); err != nil {
					return nil, err
				}
			}
			current = TXTChapter{Title: line, Start: lineStart}
			contentStart = lineEnd
			hasCurrent = true
		}
		lineStart = lineEnd
	}

	if hasCurrent {
		content := strings.TrimSpace(text[contentStart:])
		current.Index = len(chapters)
		current.End = len(text)
		current.Content = content
		if err := appendLimitedTXTChapter(&chapters, current, maxChapters); err != nil {
			return nil, err
		}
	}

	return chapters, nil
}

// parseTXTWithoutToc mirrors reader-dev TextFile.analyze()'s local-text
// fallback. OpenReader intentionally performs it over its decoded, staged
// representation because it materializes each resulting chapter into cache
// files rather than seeking source-encoding offsets at read time.
func parseTXTWithoutToc(text string, maxChapters int) ([]TXTChapter, error) {
	if text == "" {
		chapters := make([]TXTChapter, 0, 1)
		if err := appendLimitedTXTChapter(&chapters, TXTChapter{Title: "第0章(0)", Start: 0, End: 0, Content: ""}, maxChapters); err != nil {
			return nil, err
		}
		return chapters, nil
	}

	chapters := make([]TXTChapter, 0)
	segmentStart := 0
	blockPos := 0
	blockEnd := 0
	chapterPos := 0

	for blockEnd < len(text) {
		blockPos++
		nextBlockEnd := blockEnd + txtDetectionProbeBytes
		if nextBlockEnd > len(text) {
			nextBlockEnd = len(text)
		}
		nextBlockEnd = safeTXTBoundary(text, nextBlockEnd)
		if nextBlockEnd <= blockEnd {
			nextBlockEnd = len(text)
		}

		chapterPos = 0
		for nextBlockEnd-segmentStart > txtNoTocChunkBytes {
			chapterPos++
			splitAt := nextTXTNoTocSplit(text, segmentStart, nextBlockEnd)
			if err := appendTXTChapter(&chapters, text, segmentStart, splitAt, fallbackTXTChapterTitle(blockPos, chapterPos), maxChapters); err != nil {
				return nil, err
			}
			segmentStart = splitAt
		}

		blockEnd = nextBlockEnd
	}

	tailLength := len(text) - segmentStart
	if tailLength > txtNoTocShortTailBytes || len(chapters) == 0 {
		chapterPos++
		if err := appendTXTChapter(&chapters, text, segmentStart, len(text), fallbackTXTChapterTitle(blockPos, chapterPos), maxChapters); err != nil {
			return nil, err
		}
	} else {
		last := &chapters[len(chapters)-1]
		last.End = len(text)
		last.Content = text[last.Start:last.End]
	}
	return chapters, nil
}

func fallbackTXTChapterTitle(blockPos, chapterPos int) string {
	return "第" + strconv.Itoa(blockPos) + "章(" + strconv.Itoa(chapterPos) + ")"
}

func appendTXTChapter(chapters *[]TXTChapter, text string, start, end int, title string, maxChapters int) error {
	if end < start {
		end = start
	}
	return appendLimitedTXTChapter(chapters, TXTChapter{
		Index:   len(*chapters),
		Title:   title,
		Start:   start,
		End:     end,
		Content: text[start:end],
	}, maxChapters)
}

func appendLimitedTXTChapter(chapters *[]TXTChapter, chapter TXTChapter, maxChapters int) error {
	if len(*chapters) >= maxChapters {
		return txtParseLimitError("TXT contains too many chapters")
	}
	chapter.Index = len(*chapters)
	*chapters = append(*chapters, chapter)
	return nil
}

func nextTXTNoTocSplit(text string, start, end int) int {
	probe := start + txtNoTocChunkBytes
	if probe >= end {
		return safeTXTBoundary(text, end)
	}
	for index := probe; index < end; index++ {
		if text[index] == '\n' {
			return index
		}
	}
	return safeTXTBoundary(text, end)
}

func safeTXTBoundary(text string, index int) int {
	if index >= len(text) {
		return len(text)
	}
	for index > 0 && index < len(text) && (text[index]&0xc0) == 0x80 {
		index--
	}
	return index
}

// detectTXTTitlePattern keeps reader-dev TextFile.getTocRule among the
// fixed-upstream rules (enabled rules in reverse, raw per-line counts, `>=`
// overwrite from at least one match) and lets OpenReader rule -19 replace
// that choice only with txtZhangHuiMinimumLead more spaced headings than
// every enabled upstream rule.
func detectTXTTitlePattern(text string) (*txtTitleMatcher, string) {
	bestCount := 1
	var bestPattern *txtTitleMatcher
	bestRule := ""
	var zhangHuiPattern *txtTitleMatcher
	zhangHuiSpaced := 0
	upstreamSpaced := 0
	rules := DefaultTXTTocRules()
	for index := len(rules) - 1; index >= 0; index-- {
		rule := rules[index]
		if !rule.Enable {
			continue
		}
		pattern, err := compileTXTTitleMatcher(rule.Rule)
		if err != nil {
			continue
		}
		count, spaced := countTXTTitleMatches(text, pattern)
		if rule.ID == txtZhangHuiRuleID {
			zhangHuiPattern, zhangHuiSpaced = pattern, spaced
			continue
		}
		if spaced > upstreamSpaced {
			upstreamSpaced = spaced
		}
		if count >= bestCount {
			bestCount = count
			bestPattern = pattern
			bestRule = rule.Rule
		}
	}
	if zhangHuiPattern != nil && zhangHuiSpaced >= upstreamSpaced+txtZhangHuiMinimumLead {
		return zhangHuiPattern, txtZhangHuiRule
	}
	return bestPattern, bestRule
}

// countTXTTitleMatches returns reader-dev's raw count of matching lines and
// Legado's spaced count, in which a matching line only counts when it starts
// more than txtSpacedHeadingDistance characters after the end of the
// previously counted line.
func countTXTTitleMatches(text string, pattern *txtTitleMatcher) (int, int) {
	count := 0
	spaced := 0
	offset := 0
	lastCountedEnd := -1
	for lineStart := 0; lineStart < len(text); {
		lineEnd := nextLineEnd(text, lineStart)
		lineText := strings.TrimRight(text[lineStart:lineEnd], "\r\n")
		line := strings.TrimSpace(strings.TrimPrefix(lineText, "\ufeff"))
		if line != "" && pattern.MatchText(text, lineStart, lineEnd, line) {
			count++
			if lastCountedEnd < 0 || offset-lastCountedEnd > txtSpacedHeadingDistance {
				spaced++
				lastCountedEnd = offset + utf8.RuneCountInString(lineText)
			}
		}
		offset += utf8.RuneCountInString(text[lineStart:lineEnd])
		lineStart = lineEnd
	}
	return count, spaced
}

func compileTXTTitleMatcher(rule string) (*txtTitleMatcher, error) {
	rawRule := strings.TrimSpace(rule)
	if rawRule == txtForwardDoubleTitleRule {
		return &txtTitleMatcher{pattern: txtForwardFirstTitlePattern, context: txtTitleContextForward}, nil
	}
	if rawRule == txtBackwardDoubleTitleRule {
		return &txtTitleMatcher{pattern: txtBackwardTitlePattern, context: txtTitleContextBackward}, nil
	}
	normalized, lookaheads := markTXTNegativeLookaheads(normalizeTXTTitleRule(rule))
	pattern, err := regexp.Compile(normalized)
	if err != nil {
		return nil, err
	}
	for index := range lookaheads {
		lookaheads[index].group = pattern.SubexpIndex(lookaheads[index].name)
	}
	return &txtTitleMatcher{pattern: pattern, lookaheads: lookaheads}, nil
}

// normalizeTXTTitleRule turns the upstream lookbehind prefixes into line
// anchors: OpenReader matches each trimmed line on its own.
func normalizeTXTTitleRule(rule string) string {
	rule = strings.TrimSpace(rule)
	rule = strings.TrimPrefix(rule, "(?m)")
	rule = strings.TrimPrefix(rule, "(?M)")
	replacements := []string{
		`(?<=[　\s])`, `^`,
		`(?<=[\s　])`, `^`,
		`(?<=[\s　]{0,4})`, `^`,
		`(?<=[ \t　]{0,4})`, `^`,
	}
	for index := 0; index < len(replacements); index += 2 {
		rule = strings.ReplaceAll(rule, replacements[index], replacements[index+1])
	}
	return rule
}

// markTXTNegativeLookaheads replaces every negative lookahead whose body is a
// literal alternation (完|结) or a literal character class ([合来事去]) with
// an empty named group and returns what may not follow each group. Escaped
// text and character classes are copied unchanged; any other lookaround is
// left for regexp.Compile to reject explicitly.
func markTXTNegativeLookaheads(rule string) (string, []txtNegativeLookahead) {
	var marked strings.Builder
	lookaheads := make([]txtNegativeLookahead, 0)
	inClass := false
	for index := 0; index < len(rule); {
		char := rule[index]
		switch {
		case char == '\\':
			_, size := utf8.DecodeRuneInString(rule[index+1:])
			end := min(index+1+size, len(rule))
			marked.WriteString(rule[index:end])
			index = end
			continue
		case inClass:
			if char == ']' {
				inClass = false
			}
		case char == '[':
			inClass = true
			marked.WriteByte(char)
			index++
			if index < len(rule) && rule[index] == '^' {
				marked.WriteByte('^')
				index++
			}
			if index < len(rule) && rule[index] == ']' {
				marked.WriteByte(']')
				index++
			}
			continue
		case strings.HasPrefix(rule[index:], "(?!"):
			if end := strings.IndexByte(rule[index+3:], ')'); end >= 0 {
				if excluded, ok := txtLookaheadLiterals(rule[index+3 : index+3+end]); ok {
					name := "openreaderNegativeLookahead" + strconv.Itoa(len(lookaheads))
					marked.WriteString("(?P<" + name + ">)")
					lookaheads = append(lookaheads, txtNegativeLookahead{name: name, excluded: excluded})
					index += 3 + end + 1
					continue
				}
			}
		}
		marked.WriteByte(char)
		index++
	}
	return marked.String(), lookaheads
}

func txtLookaheadLiterals(body string) ([]string, bool) {
	if strings.HasPrefix(body, "[") && strings.HasSuffix(body, "]") && len(body) > 2 {
		class := body[1 : len(body)-1]
		if strings.ContainsAny(class, `\[]^-`) {
			return nil, false
		}
		literals := make([]string, 0, utf8.RuneCountInString(class))
		for _, char := range class {
			literals = append(literals, string(char))
		}
		return literals, true
	}
	literals := strings.Split(body, "|")
	for _, literal := range literals {
		if literal == "" || strings.ContainsAny(literal, `\.+*?()[]{}^$|`) {
			return nil, false
		}
	}
	return literals, true
}

func nextLineEnd(text string, start int) int {
	for index := start; index < len(text); index++ {
		if text[index] == '\n' {
			return index + 1
		}
	}
	return len(text)
}

func isChapterTitle(line string) bool {
	return isChapterTitleWithRule(line, nil)
}

func isChapterTitleWithRule(line string, titlePattern *txtTitleMatcher) bool {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" {
		return false
	}
	if titlePattern != nil {
		return titlePattern.MatchString(line)
	}
	if utf8.RuneCountInString(line) > 72 {
		return false
	}
	if strings.ContainsAny(rightmostRune(line), "。！？!?；;") {
		return false
	}
	return ChapterTitlePattern.MatchString(line)
}

func rightmostRune(value string) string {
	for len(value) > 0 {
		r, size := utf8.DecodeLastRuneInString(value)
		if r == utf8.RuneError && size == 0 {
			return ""
		}
		return string(r)
	}
	return ""
}

func decodeTXTForCatalog(data []byte) (string, string, error) {
	probeData := data
	if len(probeData) > txtDetectionProbeBytes {
		probeData = probeData[:txtDetectionProbeBytes]
		if utf8.Valid(data) {
			for len(probeData) > 0 && !utf8.Valid(probeData) {
				probeData = probeData[:len(probeData)-1]
			}
		}
	}
	probe, encodingName, err := detectAndDecodeTextSample(probeData, len(probeData) == len(data))
	if err != nil {
		return "", "", err
	}
	text, err := decodeTextWithEncoding(data, encodingName)
	if err != nil {
		return "", "", err
	}
	return text, probe, nil
}
