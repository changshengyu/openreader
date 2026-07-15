package epubreader

import (
	"strings"
	"testing"
)

func TestSanitizeAndInjectDocumentBoundsFragmentSlice(t *testing.T) {
	data := []byte(`<!doctype html><html><head><script>window.authorScript = true</script></head><body>
<p id="before">切片前正文</p><section id="part-a"><p>片段一正文</p></section>
<section id="part-b"><p>片段二正文</p></section><p id="after">切片后正文</p>
</body></html>`)

	rendered, err := sanitizeAndInjectDocument(data, "part-a", "part-b")
	if err != nil {
		t.Fatal(err)
	}
	content := string(rendered)
	for _, want := range []string{"片段一正文", "openreader-epub-bridge", `notify("navigate"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("bounded EPUB document missing %q: %s", want, content)
		}
	}
	for _, unwanted := range []string{"切片前正文", "片段二正文", "切片后正文", "authorScript"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("bounded EPUB document retained %q: %s", unwanted, content)
		}
	}
}

func TestSanitizeAndInjectDocumentKeepsReadableDocumentWhenFragmentIsMissing(t *testing.T) {
	data := []byte(`<html><body><p id="part-a">片段一正文</p><p id="part-b">片段二正文</p></body></html>`)

	rendered, err := sanitizeAndInjectDocument(data, "missing", "later")
	if err != nil {
		t.Fatal(err)
	}
	content := string(rendered)
	if !strings.Contains(content, "片段一正文") || !strings.Contains(content, "片段二正文") {
		t.Fatalf("missing fragment must preserve readable document: %s", content)
	}
}
