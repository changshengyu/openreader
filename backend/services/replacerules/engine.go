package replacerules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxCaptureGroups      = 32
	DefaultMaxMatches     = 20_000
	DefaultMaxOutputBytes = 64 << 20
)

var (
	ErrCaptureLimit = errors.New("replace rule capture-group limit exceeded")
	ErrMatchLimit   = errors.New("replace rule match limit exceeded")
	ErrOutputLimit  = errors.New("replace rule output limit exceeded")
)

type Limits struct {
	MaxMatches     int
	MaxOutputBytes int
}

func DefaultLimits(inputBytes int) Limits {
	maxOutputBytes := DefaultMaxOutputBytes
	if inputBytes > maxOutputBytes {
		maxOutputBytes = inputBytes
	}
	return Limits{
		MaxMatches:     DefaultMaxMatches,
		MaxOutputBytes: maxOutputBytes,
	}
}

// Compile builds the bounded RE2 subset used for reader-global regular
// expressions. reader-dev applies the browser's /ig flags, so matching is
// global at Apply time and case-insensitive here.
func Compile(pattern string) (*regexp.Regexp, error) {
	compiled, err := regexp.Compile("(?i:" + pattern + ")")
	if err != nil {
		return nil, err
	}
	if compiled.NumSubexp() > MaxCaptureGroups {
		return nil, fmt.Errorf("%w: %d > %d", ErrCaptureLimit, compiled.NumSubexp(), MaxCaptureGroups)
	}
	return compiled, nil
}

// Apply follows JavaScript String.replace replacement-string semantics for the
// pattern subset accepted by Compile. Plain rules replace only the first exact
// match. Regex rules replace every case-insensitive match.
//
// It returns the original input on every execution-limit or compile failure,
// so callers can stop a rule pipeline without exposing partial output.
func Apply(
	content string,
	pattern string,
	replacement string,
	isRegex bool,
	limits Limits,
) (string, error) {
	if limits.MaxMatches <= 0 {
		return content, ErrMatchLimit
	}
	if limits.MaxOutputBytes <= 0 {
		return content, ErrOutputLimit
	}
	if !isRegex {
		start := strings.Index(content, pattern)
		if start < 0 {
			return content, nil
		}
		match := []int{start, start + len(pattern)}
		output := newBoundedBuilder(limits.MaxOutputBytes)
		if err := output.WriteString(content[:start]); err != nil {
			return content, err
		}
		if err := expandJavaScriptReplacement(output, replacement, content, match, nil); err != nil {
			return content, err
		}
		if err := output.WriteString(content[start+len(pattern):]); err != nil {
			return content, err
		}
		return output.String(), nil
	}

	re, err := Compile(pattern)
	if err != nil {
		return content, err
	}
	matches := re.FindAllStringSubmatchIndex(content, limits.MaxMatches+1)
	if len(matches) == 0 {
		return content, nil
	}
	if len(matches) > limits.MaxMatches {
		return content, ErrMatchLimit
	}
	output := newBoundedBuilder(limits.MaxOutputBytes)
	previousEnd := 0
	for _, match := range matches {
		if err := output.WriteString(content[previousEnd:match[0]]); err != nil {
			return content, err
		}
		if err := expandJavaScriptReplacement(output, replacement, content, match, re); err != nil {
			return content, err
		}
		previousEnd = match[1]
	}
	if err := output.WriteString(content[previousEnd:]); err != nil {
		return content, err
	}
	return output.String(), nil
}

func expandJavaScriptReplacement(
	output *boundedBuilder,
	replacement string,
	source string,
	match []int,
	re *regexp.Regexp,
) error {
	if len(match) < 2 {
		return output.WriteString(replacement)
	}
	captureCount := len(match)/2 - 1
	names := []string(nil)
	hasNamedCaptures := false
	if re != nil {
		names = re.SubexpNames()
		for index := 1; index < len(names); index++ {
			if names[index] != "" {
				hasNamedCaptures = true
				break
			}
		}
	}

	for index := 0; index < len(replacement); index++ {
		if replacement[index] != '$' || index+1 >= len(replacement) {
			if err := output.WriteByte(replacement[index]); err != nil {
				return err
			}
			continue
		}

		next := replacement[index+1]
		switch next {
		case '$':
			if err := output.WriteByte('$'); err != nil {
				return err
			}
			index++
		case '&':
			if err := output.WriteString(source[match[0]:match[1]]); err != nil {
				return err
			}
			index++
		case '`':
			if err := output.WriteString(source[:match[0]]); err != nil {
				return err
			}
			index++
		case '\'':
			if err := output.WriteString(source[match[1]:]); err != nil {
				return err
			}
			index++
		case '<':
			end := strings.IndexByte(replacement[index+2:], '>')
			if end < 0 || !hasNamedCaptures {
				if err := output.WriteByte('$'); err != nil {
					return err
				}
				continue
			}
			end += index + 2
			name := replacement[index+2 : end]
			capture := namedCaptureIndex(names, name)
			if capture > 0 {
				if err := output.WriteString(captureText(source, match, capture)); err != nil {
					return err
				}
			}
			index = end
		default:
			if next < '0' || next > '9' {
				if err := output.WriteByte('$'); err != nil {
					return err
				}
				continue
			}
			first := int(next - '0')
			capture := 0
			consumedDigits := 0
			if index+2 < len(replacement) &&
				replacement[index+2] >= '0' &&
				replacement[index+2] <= '9' {
				second := int(replacement[index+2] - '0')
				candidate := first*10 + second
				if candidate <= captureCount {
					capture = candidate
					consumedDigits = 2
				}
			}
			if capture == 0 && first > 0 && first <= captureCount {
				capture = first
				consumedDigits = 1
			}
			if capture == 0 {
				if err := output.WriteByte('$'); err != nil {
					return err
				}
				continue
			}
			if err := output.WriteString(captureText(source, match, capture)); err != nil {
				return err
			}
			index += consumedDigits
		}
	}
	return nil
}

func namedCaptureIndex(names []string, name string) int {
	for index := 1; index < len(names); index++ {
		if names[index] == name {
			return index
		}
	}
	return 0
}

func captureText(source string, match []int, capture int) string {
	offset := capture * 2
	if offset+1 >= len(match) || match[offset] < 0 || match[offset+1] < 0 {
		return ""
	}
	return source[match[offset]:match[offset+1]]
}

type boundedBuilder struct {
	builder strings.Builder
	limit   int
}

func newBoundedBuilder(limit int) *boundedBuilder {
	return &boundedBuilder{limit: limit}
}

func (builder *boundedBuilder) WriteByte(value byte) error {
	if builder.builder.Len()+1 > builder.limit {
		return ErrOutputLimit
	}
	return builder.builder.WriteByte(value)
}

func (builder *boundedBuilder) WriteString(value string) error {
	if len(value) > builder.limit-builder.builder.Len() {
		return ErrOutputLimit
	}
	_, err := builder.builder.WriteString(value)
	return err
}

func (builder *boundedBuilder) String() string {
	return builder.builder.String()
}
