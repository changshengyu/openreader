package engine

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseCBZBuildsSortedImageChaptersAndComicInfo(t *testing.T) {
	data := makeCBZFixture(t, map[string]string{
		"ComicInfo.xml":    `<ComicInfo><Title>漫画标题</Title><Writer>作者名</Writer></ComicInfo>`,
		"pages/010.jpg":    "ten",
		"pages/002.png":    "two",
		"pages/readme.txt": "ignored",
		"cover.webp":       "cover",
	})

	book, err := ParseCBZ(data)
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "漫画标题" || book.Author != "作者名" {
		t.Fatalf("comic info not parsed: %+v", book)
	}
	got := make([]string, 0, len(book.Chapters))
	for _, chapter := range book.Chapters {
		got = append(got, chapter.ResourcePath)
	}
	want := []string{"cover.webp", "pages/002.png", "pages/010.jpg"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("chapters = %#v, want %#v", got, want)
	}
	for index, chapter := range book.Chapters {
		if chapter.Index != index || chapter.Title != want[index] || chapter.Content != "" {
			t.Fatalf("unexpected chapter %d: %+v", index, chapter)
		}
	}
}

func TestParseCBZRejectsUnsafeArchivePaths(t *testing.T) {
	for _, name := range []string{
		"../escape.jpg",
		"/absolute.jpg",
		`dir\windows.jpg`,
		"C:/drive.jpg",
		"safe/../escape.jpg",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseCBZ(makeCBZFixture(t, map[string]string{name: "x"}))
			if err == nil {
				t.Fatalf("expected unsafe path %q to fail", name)
			}
		})
	}
}

func TestParseCBZRejectsDuplicateNormalizedPaths(t *testing.T) {
	_, err := ParseCBZ(makeCBZFixture(t, map[string]string{
		"Page.JPG": "one",
		"page.jpg": "two",
	}))
	if err == nil {
		t.Fatal("expected duplicate normalized path to fail")
	}
}

func makeCBZFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
