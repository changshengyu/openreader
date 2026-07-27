package contentsearch

import "testing"

func TestFindMatchesFixedUpstreamExactOverlapAndUTF16Indexes(t *testing.T) {
	matches, truncated := Find("aaaa\n😀123TARGET尾", "aa", 20)
	if truncated || len(matches) != 3 {
		t.Fatalf("overlapping matches = %+v, truncated=%v", matches, truncated)
	}
	for index, expectedOffset := range []int{0, 1, 2} {
		if matches[index].ByteOffset != expectedOffset {
			t.Fatalf("match %d byte offset = %d, want %d", index, matches[index].ByteOffset, expectedOffset)
		}
	}

	matches, truncated = Find("😀123TARGET尾", "TARGET", 20)
	if truncated || len(matches) != 1 {
		t.Fatalf("UTF-16 fixture matches = %+v, truncated=%v", matches, truncated)
	}
	if matches[0].QueryIndexInChapter != 5 {
		t.Fatalf("queryIndexInChapter = %d, want fixed-upstream UTF-16 index 5", matches[0].QueryIndexInChapter)
	}
	if matches[0].QueryIndexInResult != 5 || matches[0].Excerpt != "😀123TARGET尾" {
		t.Fatalf("UTF-16 excerpt metadata = %+v", matches[0])
	}
}

func TestFindIsCaseAndPunctuationSensitive(t *testing.T) {
	for _, query := range []string{"target", "目 标"} {
		matches, truncated := Find("TARGET\n目标", query, 20)
		if truncated || len(matches) != 0 {
			t.Fatalf("query %q fabricated matches: %+v, truncated=%v", query, matches, truncated)
		}
	}
}
