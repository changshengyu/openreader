package engine

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	txtDetectionProbeBytes = 512000
	txtNoTocChunkBytes     = 10 * 1024
	txtNoTocShortTailBytes = 100
)

var ChapterTitlePattern = regexp.MustCompile(`^(?:第[零一二三四五六七八九十百千万两〇○0-9０-９]+[章回节卷集部]|序章|楔子|引子|前言|尾声|后记|番外(?:篇)?|第[零一二三四五六七八九十百千万两〇○0-9０-９]+卷|[上中下]卷).{0,64}$`)

var defaultTXTTitleRules = []string{
	`(?<=[　\s])(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))).{0,30}$`,
	`^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))).{0,30}$`,
	`^[ 　\t]{0,4}\d{1,5}[：:,.， 、_—\-].{1,30}$`,
	`^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|[〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 、_—\-].{1,30}$`,
	`^[ 　\t]{0,4}正文[ 　]{1,4}.{0,20}$`,
	`^[ 　\t]{0,4}(?:[Cc]hapter|[Ss]ection|[Pp]art|ＰＡＲＴ|[Nn][oO]\.|[Ee]pisode|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)\s{0,4}\d{1,4}.{0,30}$`,
	`(?<=[\s　])[【〔〖「『〈［\[](?:第|[Cc]hapter)[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,10}[章节].{0,20}$`,
	`(?<=[\s　]{0,4})(?:[☆★✦✧].{1,30}|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)[ 　]{0,4}$`,
	`^[ \t　]{0,4}(?:(?:内容|文章)?简介|文案|前言|序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|[卷章][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 　]{0,4}.{0,30}$`,
	`^.{1,20}[(（][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}[)）][ 　\t]{0,4}$`,
}

type txtTitleMatcher struct {
	pattern    *regexp.Regexp
	exclusions []txtTitleExclusion
}

type txtTitleExclusion struct {
	token  string
	chars  string
	phrase string
}

func (matcher *txtTitleMatcher) MatchString(line string) bool {
	if matcher == nil {
		return ChapterTitlePattern.MatchString(line)
	}
	if !matcher.pattern.MatchString(line) {
		return false
	}
	for _, exclusion := range matcher.exclusions {
		if exclusion.phrase != "" {
			if strings.Contains(line, exclusion.phrase) {
				return false
			}
			continue
		}
		index := strings.Index(line, exclusion.token)
		for index >= 0 {
			after := line[index+len(exclusion.token):]
			if after != "" {
				r, _ := utf8.DecodeRuneInString(after)
				if strings.ContainsRune(exclusion.chars, r) {
					return false
				}
			}
			nextOffset := index + len(exclusion.token)
			next := strings.Index(line[nextOffset:], exclusion.token)
			if next < 0 {
				break
			}
			index = nextOffset + next
		}
	}
	return true
}

type TXTChapter struct {
	Index        int    `json:"index"`
	Title        string `json:"title"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	Content      string `json:"content"`
	ResourcePath string `json:"resourcePath,omitempty"`
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
		{ID: -1, Enable: true, Name: "目录(去空白)", Rule: defaultTXTTitleRules[0], SerialNumber: 0},
		{ID: -2, Enable: true, Name: "目录", Rule: defaultTXTTitleRules[1], SerialNumber: 1},
		{ID: -6, Enable: true, Name: "数字 分隔符 标题名称", Rule: `^[ 　\t]{0,4}\d{1,5}[：:,.， 、_—\-].{1,30}$`, SerialNumber: 5},
		{ID: -7, Enable: true, Name: "大写数字 分隔符 标题名称", Rule: defaultTXTTitleRules[3], SerialNumber: 6},
		{ID: -8, Enable: true, Name: "正文 标题/序号", Rule: `^[ 　\t]{0,4}正文[ 　]{1,4}.{0,20}$`, SerialNumber: 7},
		{ID: -9, Enable: true, Name: "Chapter/Section/Part/Episode 序号 标题", Rule: defaultTXTTitleRules[5], SerialNumber: 8},
		{ID: -11, Enable: true, Name: "特殊符号 序号 标题", Rule: defaultTXTTitleRules[6], SerialNumber: 10},
		{ID: -13, Enable: true, Name: "特殊符号 标题(单个)", Rule: defaultTXTTitleRules[7], SerialNumber: 12},
		{ID: -14, Enable: true, Name: "章/卷 序号 标题", Rule: defaultTXTTitleRules[8], SerialNumber: 13},
		{ID: -18, Enable: true, Name: "标题 特殊符号 序号", Rule: `^.{1,20}[(（][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}[)）][ 　\t]{0,4}$`, SerialNumber: 17},
	}
}

func ParseTXT(data []byte) ([]TXTChapter, error) {
	text, probe, err := decodeTXTForCatalog(data)
	if err != nil {
		return nil, err
	}

	return parseTXTText(text, detectTXTTitlePattern(probe)), nil
}

func ParseTXTWithRule(data []byte, rule string) ([]TXTChapter, error) {
	text, probe, err := decodeTXTForCatalog(data)
	if err != nil {
		return nil, err
	}
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return parseTXTText(text, detectTXTTitlePattern(probe)), nil
	}
	pattern, err := compileTXTTitleMatcher(rule)
	if err != nil {
		return nil, err
	}
	return parseTXTText(text, pattern), nil
}

func parseTXTText(text string, titlePattern *txtTitleMatcher) []TXTChapter {
	if titlePattern == nil {
		return parseTXTWithoutToc(text)
	}

	chapters := make([]TXTChapter, 0)
	var current TXTChapter
	hasCurrent := false
	contentStart := 0
	for lineStart := 0; lineStart < len(text); {
		lineEnd := nextLineEnd(text, lineStart)
		lineText := strings.TrimRight(text[lineStart:lineEnd], "\r\n")
		line := strings.TrimSpace(lineText)
		if titlePattern.MatchString(line) {
			if hasCurrent {
				content := strings.TrimSpace(text[contentStart:lineStart])
				current.Index = len(chapters)
				current.End = lineStart
				current.Content = content
				chapters = append(chapters, current)
			} else if preface := strings.TrimSpace(text[:lineStart]); preface != "" {
				chapters = append(chapters, TXTChapter{
					Index:   len(chapters),
					Title:   "前言",
					Start:   0,
					End:     lineStart,
					Content: preface,
				})
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
		chapters = append(chapters, current)
	}

	return chapters
}

// parseTXTWithoutToc mirrors reader-dev TextFile.analyze()'s local-text
// fallback. OpenReader intentionally performs it over its decoded, staged
// representation because it materializes each resulting chapter into cache
// files rather than seeking source-encoding offsets at read time.
func parseTXTWithoutToc(text string) []TXTChapter {
	if text == "" {
		return []TXTChapter{{Index: 0, Title: "第0章(0)", Start: 0, End: 0, Content: ""}}
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
			appendTXTChapter(&chapters, text, segmentStart, splitAt, fallbackTXTChapterTitle(blockPos, chapterPos))
			segmentStart = splitAt
		}

		blockEnd = nextBlockEnd
	}

	tailLength := len(text) - segmentStart
	if tailLength > txtNoTocShortTailBytes || len(chapters) == 0 {
		chapterPos++
		appendTXTChapter(&chapters, text, segmentStart, len(text), fallbackTXTChapterTitle(blockPos, chapterPos))
	} else {
		last := &chapters[len(chapters)-1]
		last.End = len(text)
		last.Content = text[last.Start:last.End]
	}
	return chapters
}

func fallbackTXTChapterTitle(blockPos, chapterPos int) string {
	return "第" + strconv.Itoa(blockPos) + "章(" + strconv.Itoa(chapterPos) + ")"
}

func appendTXTChapter(chapters *[]TXTChapter, text string, start, end int, title string) {
	if end < start {
		end = start
	}
	*chapters = append(*chapters, TXTChapter{
		Index:   len(*chapters),
		Title:   title,
		Start:   start,
		End:     end,
		Content: text[start:end],
	})
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

func detectTXTTitlePattern(text string) *txtTitleMatcher {
	bestCount := 1
	var bestPattern *txtTitleMatcher
	rules := DefaultTXTTocRules()
	for index := len(rules) - 1; index >= 0; index-- {
		rule := rules[index]
		if !rule.Enable {
			continue
		}
		pattern, err := compileTXTTitleMatcher(rule.Rule)
		if err == nil {
			if count := countTXTTitleMatches(text, pattern); count >= bestCount {
				bestCount = count
				bestPattern = pattern
			}
		}
	}
	return bestPattern
}

func countTXTTitleMatches(text string, pattern *txtTitleMatcher) int {
	count := 0
	for lineStart := 0; lineStart < len(text); {
		lineEnd := nextLineEnd(text, lineStart)
		lineText := strings.TrimRight(text[lineStart:lineEnd], "\r\n")
		if isChapterTitleWithRule(lineText, pattern) {
			count++
		}
		lineStart = lineEnd
	}
	return count
}

func compileTXTTitleMatcher(rule string) (*txtTitleMatcher, error) {
	exclusions := txtTitleExclusions(rule)
	rule = normalizeTXTTitleRule(rule)
	pattern, err := regexp.Compile(rule)
	if err != nil {
		return nil, err
	}
	return &txtTitleMatcher{pattern: pattern, exclusions: exclusions}, nil
}

func normalizeTXTTitleRule(rule string) string {
	rule = strings.TrimSpace(rule)
	rule = strings.TrimPrefix(rule, "(?m)")
	rule = strings.TrimPrefix(rule, "(?M)")
	replacements := []string{
		`(?<=[　\s])`, `^`,
		`(?<=[\s　])`, `^`,
		`(?<=[\s　]{0,4})`, `^`,
		`(?<=[ \t　]{0,4})`, `^`,
		`(?!完|结)`, ``,
		`(?!课)`, ``,
		`(?![合和])`, ``,
		`(?![分赛游])`, ``,
		`(?!张)`, ``,
		`(?![合来事去])`, ``,
		`(?![和合比电是])`, ``,
	}
	for index := 0; index < len(replacements); index += 2 {
		rule = strings.ReplaceAll(rule, replacements[index], replacements[index+1])
	}
	return rule
}

func txtTitleExclusions(rule string) []txtTitleExclusion {
	exclusions := make([]txtTitleExclusion, 0)
	add := func(needle string, exclusion txtTitleExclusion) {
		if strings.Contains(rule, needle) {
			exclusions = append(exclusions, exclusion)
		}
	}
	add(`正文(?!完|结)`, txtTitleExclusion{token: "正文", chars: "完结"})
	add(`节(?!课)`, txtTitleExclusion{phrase: "节课"})
	add(`集(?![合和])`, txtTitleExclusion{token: "集", chars: "合和"})
	add(`部(?![分赛游])`, txtTitleExclusion{token: "部", chars: "分赛游"})
	add(`篇(?!张)`, txtTitleExclusion{phrase: "篇张"})
	add(`回(?![合来事去])`, txtTitleExclusion{token: "回", chars: "合来事去"})
	add(`场(?![和合比电是])`, txtTitleExclusion{token: "场", chars: "和合比电是"})
	return exclusions
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
	probe, encodingName, err := detectAndDecodeText(probeData)
	if err != nil {
		return "", "", err
	}
	text, err := decodeTextWithEncoding(data, encodingName)
	if err != nil {
		return "", "", err
	}
	return text, probe, nil
}
