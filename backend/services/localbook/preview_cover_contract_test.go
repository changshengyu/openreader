package localbook

import (
	"archive/zip"
	"bytes"
	"testing"
)

// Contract: docs/compat/local-book-cover-fixed-baseline-second-audit-p1-contract.md (C3, A7)

// Only the import dialog's preview inlines the cover. Staging, the durable
// import and portable restore call Prepare/Preview and drop the preview, so
// they must not decode and encode cover images.
func TestPrepareAndPreviewDoNotComputeThePreviewCover(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, body := range map[string]string{
		"META-INF/container.xml": `<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`,
		"OPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>封面</dc:title><meta name="cover" content="cover"/></metadata><manifest>
  <item id="one" href="one.xhtml" media-type="application/xhtml+xml"/>
  <item id="cover" href="cover.jpg" media-type="image/jpeg"/>
</manifest><spine><itemref idref="one"/></spine></package>`,
		"OPS/one.xhtml": `<html><head><title>第一章</title></head><body><p>正文。</p></body></html>`,
		"OPS/cover.jpg": "\xff\xd8\xff\xe0cover",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{FileName: "封面.epub", Extension: ".epub", Data: archive.Bytes()}

	prepared, _, err := (Importer{}).Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.CoverURL != "" {
		t.Fatalf("Prepare computed a preview cover (%d bytes)", len(prepared.CoverURL))
	}
	preview, err := (Importer{}).Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CoverURL != "" {
		t.Fatalf("Preview computed a preview cover (%d bytes)", len(preview.CoverURL))
	}
}
