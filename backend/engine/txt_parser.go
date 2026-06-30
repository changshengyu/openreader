package engine

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var ChapterTitlePattern = regexp.MustCompile(`^(?:第[零一二三四五六七八九十百千万两〇○0-9０-９]+[章回节卷集部]|序章|楔子|引子|前言|尾声|后记|番外(?:篇)?|第[零一二三四五六七八九十百千万两〇○0-9０-９]+卷|[上中下]卷).{0,64}$`)

var defaultTXTTitlePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^.{1,20}[(（][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}[)）][ 　\t]{0,4}$`),
	regexp.MustCompile(`^[ \t　]{0,4}(?:(?:内容|文章)?简介|文案|前言|序章|序言|卷首语|扉页|楔子|正文(?:完|结)?|终章|后记|尾声|番外|[卷章][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 　]{0,4}.{0,30}$`),
	regexp.MustCompile(`^[\s　]{0,4}(?:[☆★✦✧].{1,30}|(?:内容|文章)?简介|文案|前言|序章|楔子|正文|终章|后记|尾声|番外)[ 　]{0,4}$`),
	regexp.MustCompile(`^[\s　]*[【〔〖「『〈［\[](?:第|[Cc]hapter)[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,10}[章节].{0,20}$`),
	regexp.MustCompile(`^[ 　\t]{0,4}(?:[Cc]hapter|[Ss]ection|[Pp]art|ＰＡＲＴ|[Nn][oO]\.|[Ee]pisode|(?:内容|文章)?简介|文案|前言|序章|楔子|正文|终章|后记|尾声|番外)\s{0,4}\d{1,4}.{0,30}$`),
	regexp.MustCompile(`^[ 　\t]{0,4}正文[ 　]{1,4}.{0,20}$`),
	regexp.MustCompile(`^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文|终章|后记|尾声|番外|[〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 、_—\-].{1,30}$`),
	regexp.MustCompile(`^[ 　\t]{0,4}\d{1,5}[：:,.， 、_—\-].{1,30}$`),
	regexp.MustCompile(`^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节|卷|集|部|篇)).{0,30}$`),
	regexp.MustCompile(`^(?:序章|序言|卷首语|扉页|楔子|正文|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节|卷|集|部|篇)).{0,30}$`),
	ChapterTitlePattern,
}

type TXTChapter struct {
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Content string `json:"content"`
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
		{ID: -2, Enable: true, Name: "目录", Rule: `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节|卷|集|部|篇)).{0,30}$`, SerialNumber: 1},
		{ID: -6, Enable: true, Name: "数字 分隔符 标题名称", Rule: `^[ 　\t]{0,4}\d{1,5}[：:,.， 、_—\-].{1,30}$`, SerialNumber: 5},
		{ID: -7, Enable: true, Name: "大写数字 分隔符 标题名称", Rule: `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文|终章|后记|尾声|番外|[〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 、_—\-].{1,30}$`, SerialNumber: 6},
		{ID: -8, Enable: true, Name: "正文 标题/序号", Rule: `^[ 　\t]{0,4}正文[ 　]{1,4}.{0,20}$`, SerialNumber: 7},
		{ID: -9, Enable: true, Name: "Chapter/Section/Part/Episode 序号 标题", Rule: `^[ 　\t]{0,4}(?:[Cc]hapter|[Ss]ection|[Pp]art|ＰＡＲＴ|[Nn][oO]\.|[Ee]pisode|(?:内容|文章)?简介|文案|前言|序章|楔子|正文|终章|后记|尾声|番外)\s{0,4}\d{1,4}.{0,30}$`, SerialNumber: 8},
		{ID: -11, Enable: true, Name: "特殊符号 序号 标题", Rule: `^[\s　]*[【〔〖「『〈［\[](?:第|[Cc]hapter)[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,10}[章节].{0,20}$`, SerialNumber: 10},
		{ID: -13, Enable: true, Name: "特殊符号 标题(单个)", Rule: `^[\s　]{0,4}(?:[☆★✦✧].{1,30}|(?:内容|文章)?简介|文案|前言|序章|楔子|正文|终章|后记|尾声|番外)[ 　]{0,4}$`, SerialNumber: 12},
		{ID: -14, Enable: true, Name: "章/卷 序号 标题", Rule: `^[ \t　]{0,4}(?:(?:内容|文章)?简介|文案|前言|序章|序言|卷首语|扉页|楔子|正文(?:完|结)?|终章|后记|尾声|番外|[卷章][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 　]{0,4}.{0,30}$`, SerialNumber: 13},
		{ID: -18, Enable: true, Name: "标题 特殊符号 序号", Rule: `^.{1,20}[(（][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}[)）][ 　\t]{0,4}$`, SerialNumber: 17},
	}
}

func ParseTXT(data []byte) ([]TXTChapter, error) {
	text, err := decodeTXT(data)
	if err != nil {
		return nil, err
	}

	return parseTXTText(text, detectTXTTitlePattern(text)), nil
}

func ParseTXTWithRule(data []byte, rule string) ([]TXTChapter, error) {
	text, err := decodeTXT(data)
	if err != nil {
		return nil, err
	}
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return parseTXTText(text, detectTXTTitlePattern(text)), nil
	}
	pattern, err := compileTXTTitlePattern(rule)
	if err != nil {
		return nil, err
	}
	return parseTXTText(text, pattern), nil
}

func parseTXTText(text string, titlePattern *regexp.Regexp) []TXTChapter {
	chapters := make([]TXTChapter, 0)
	current := TXTChapter{Index: 0, Title: "正文", Start: 0}
	sawChapterTitle := false

	contentStart := 0
	for lineStart := 0; lineStart < len(text); {
		lineEnd := nextLineEnd(text, lineStart)
		lineText := strings.TrimRight(text[lineStart:lineEnd], "\r\n")
		line := strings.TrimSpace(lineText)
		if isChapterTitleWithRule(line, titlePattern) {
			if lineStart > contentStart {
				content := strings.TrimSpace(text[contentStart:lineStart])
				if sawChapterTitle || shouldKeepFrontMatter(content) {
					current.Index = len(chapters)
					current.End = lineStart
					current.Content = content
					chapters = append(chapters, current)
				}
			}
			current = TXTChapter{Title: line, Start: lineStart}
			contentStart = lineEnd
			sawChapterTitle = true
		}
		lineStart = lineEnd
	}

	if contentStart <= len(text) {
		content := strings.TrimSpace(text[contentStart:])
		if sawChapterTitle || len(chapters) == 0 || shouldKeepFrontMatter(content) {
			if content != "" || len(chapters) == 0 {
				current.Index = len(chapters)
				current.End = len(text)
				current.Content = content
				chapters = append(chapters, current)
			}
		}
	}

	return chapters
}

func detectTXTTitlePattern(text string) *regexp.Regexp {
	bestPattern := ChapterTitlePattern
	bestCount := countTXTTitleMatches(text, bestPattern)
	for _, pattern := range defaultTXTTitlePatterns {
		count := countTXTTitleMatches(text, pattern)
		if count >= bestCount && count > 1 {
			bestCount = count
			bestPattern = pattern
		}
	}
	return bestPattern
}

func countTXTTitleMatches(text string, pattern *regexp.Regexp) int {
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

func compileTXTTitlePattern(rule string) (*regexp.Regexp, error) {
	rule = normalizeTXTTitleRule(rule)
	return regexp.Compile(rule)
}

func normalizeTXTTitleRule(rule string) string {
	rule = strings.TrimSpace(rule)
	rule = strings.TrimPrefix(rule, "(?m)")
	rule = strings.TrimPrefix(rule, "(?M)")
	replacements := []string{
		`(?<=[　\s])`,
		`(?<=[\s　])`,
		`(?<=[\s　]{0,4})`,
		`(?<=[ \t　]{0,4})`,
	}
	for _, old := range replacements {
		rule = strings.ReplaceAll(rule, old, "^")
	}
	return rule
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

func isChapterTitleWithRule(line string, titlePattern *regexp.Regexp) bool {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" || utf8.RuneCountInString(line) > 72 {
		return false
	}
	if strings.ContainsAny(rightmostRune(line), "。！？!?；;") {
		return false
	}
	if titlePattern != nil {
		return titlePattern.MatchString(line)
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

func shouldKeepFrontMatter(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}

	lines := strings.Split(content, "\n")
	nonEmpty := 0
	totalRunes := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty++
		totalRunes += utf8.RuneCountInString(line)
	}

	return nonEmpty > 8 || totalRunes > 360
}

func decodeTXT(data []byte) (string, error) {
	decoded, _, err := detectAndDecodeText(data)
	return decoded, err
}
