package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

func TestJapaneseAndCyrillicTXTImportAndRead(t *testing.T) {
	tests := []struct {
		name     string
		encoding encoding.Encoding
		text     string
	}{
		{
			name:     "Shift_JIS",
			encoding: japanese.ShiftJIS,
			text:     "今日はとても良い天気ですね。私は朝から公園を散歩して、友達と一緒に昼ご飯を食べました。午後は図書館で本を読みながら、明日の予定について考えていました。\n",
		},
		{
			name:     "windows-1251",
			encoding: charmap.Windows1251,
			text:     "Сегодня утром была прекрасная погода. Я гулял по парку и обедал с другом. Днём я читал книгу в библиотеке и думал о своих планах на завтра.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := strings.Repeat(tt.text, 12)
			data, err := tt.encoding.NewEncoder().Bytes([]byte(text))
			if err != nil {
				t.Fatal(err)
			}
			router, _ := setupTestServer(t)
			auth := authHeader(t, router)
			previewW := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", "book.txt", data, nil)
			if previewW.Code != http.StatusOK {
				t.Fatalf("preview: expected 200, got %d: %s", previewW.Code, previewW.Body.String())
			}
			var preview struct {
				ChapterCount int    `json:"chapterCount"`
				ImportToken  string `json:"importToken"`
			}
			if err := json.Unmarshal(previewW.Body.Bytes(), &preview); err != nil {
				t.Fatal(err)
			}
			if preview.ChapterCount != 1 || !validLocalImportToken(preview.ImportToken) {
				t.Fatalf("preview = %+v, want one staged chapter", preview)
			}
			importW := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "book.txt", nil, map[string]string{
				"importToken": preview.ImportToken,
			})
			if importW.Code != http.StatusCreated {
				t.Fatalf("stage import: expected 201, got %d: %s", importW.Code, importW.Body.String())
			}
			var imported bookListItem
			if err := json.Unmarshal(importW.Body.Bytes(), &imported); err != nil {
				t.Fatal(err)
			}
			contentReq := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(imported.ID), 10)+"/chapters/0/content", nil)
			contentReq.Header.Set("Authorization", auth)
			contentW := httptest.NewRecorder()
			router.ServeHTTP(contentW, contentReq)
			if contentW.Code != http.StatusOK {
				t.Fatalf("chapter content: expected 200, got %d: %s", contentW.Code, contentW.Body.String())
			}
			var content struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(contentW.Body.Bytes(), &content); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(content.Content); got != strings.TrimSpace(text) {
				head := []rune(got)
				t.Fatalf("chapter content decoded as %q...", string(head[:min(24, len(head))]))
			}
		})
	}
}
