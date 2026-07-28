package replacerules

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyMatchesJavaScriptReplacementTokens(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		pattern     string
		replacement string
		regex       bool
		want        string
	}{
		{
			name:        "plain special tokens",
			content:     "left ad right ad",
			pattern:     "ad",
			replacement: "[$$][$&][$`][$']",
			want:        "left [$][ad][left ][ right ad] right ad",
		},
		{
			name:        "regex captures and whole match",
			content:     "Ad1 ad2",
			pattern:     `(ad)(\d)`,
			replacement: "$2-$1-$&-$$",
			regex:       true,
			want:        "1-Ad-Ad1-$ 2-ad-ad2-$",
		},
		{
			name:        "two digit fallback and unknown captures",
			content:     "ab",
			pattern:     `(a)(b)`,
			replacement: "$01-$21-$3-$0",
			regex:       true,
			want:        "a-b1-$3-$0",
		},
		{
			name:        "global prefix and suffix",
			content:     "aa",
			pattern:     `(a)`,
			replacement: "<$`|$'>",
			regex:       true,
			want:        "<|a><a|>",
		},
		{
			name:        "unmatched capture",
			content:     "b",
			pattern:     `(a)?b`,
			replacement: "[$1]",
			regex:       true,
			want:        "[]",
		},
		{
			name:        "plain numeric capture stays literal",
			content:     "ad",
			pattern:     "ad",
			replacement: "$1-$0",
			want:        "$1-$0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Apply(
				test.content,
				test.pattern,
				test.replacement,
				test.regex,
				DefaultLimits(len(test.content)),
			)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("Apply() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplyLeavesContentUntouchedForInvalidRegex(t *testing.T) {
	const content = "[broken"
	got, err := Apply(content, "[broken", "changed", true, DefaultLimits(len(content)))
	if err == nil {
		t.Fatal("invalid regex must report an execution error")
	}
	if got != content {
		t.Fatalf("invalid regex changed content: %q", got)
	}
}

func TestApplyMatchesJavaScriptNamedCaptureTokens(t *testing.T) {
	got, err := Apply(
		"ab ax",
		`(?<left>a)(?<right>b)?`,
		"$<right>-$<missing>-$<left>",
		true,
		DefaultLimits(len("ab ax")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b--a --ax" {
		t.Fatalf("named JavaScript replacement tokens diverged: %q", got)
	}

	got, err = Apply("ab", `(a)(b)`, "$<left>", true, DefaultLimits(len("ab")))
	if err != nil {
		t.Fatal(err)
	}
	if got != "$<left>" {
		t.Fatalf("a named token without named captures must remain literal: %q", got)
	}
}

func TestApplyPreservesZeroLengthAndAnchorSemantics(t *testing.T) {
	got, err := Apply(
		"你a",
		`(?:)`,
		"-",
		true,
		DefaultLimits(len("你a")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "-你-a-" {
		t.Fatalf("zero-length semantics diverged: %q", got)
	}

	got, err = Apply("ab", "^", "-", true, DefaultLimits(len("ab")))
	if err != nil {
		t.Fatal(err)
	}
	if got != "-ab" {
		t.Fatalf("start anchor must not be reinterpreted at later offsets: %q", got)
	}
}

func TestApplyFailsClosedAtMatchAndOutputLimits(t *testing.T) {
	const content = "aaaa"
	got, err := Apply(content, "a", "b", true, Limits{
		MaxMatches:     2,
		MaxOutputBytes: 64,
	})
	if !errors.Is(err, ErrMatchLimit) {
		t.Fatalf("expected match limit, got output %q error %v", got, err)
	}
	if got != content {
		t.Fatalf("match-limit failure must preserve the input, got %q", got)
	}

	got, err = Apply(content, "a", "$`$`", true, Limits{
		MaxMatches:     100,
		MaxOutputBytes: 5,
	})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("expected output limit, got output %q error %v", got, err)
	}
	if got != content {
		t.Fatalf("output-limit failure must preserve the input, got %q", got)
	}
}

func TestCompileRejectsExcessiveCaptureGroups(t *testing.T) {
	pattern := strings.Repeat("(a)", MaxCaptureGroups+1)
	if _, err := Compile(pattern); !errors.Is(err, ErrCaptureLimit) {
		t.Fatalf("expected capture limit, got %v", err)
	}
}
