package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestTXTWithSparseMalformedBytesImportsAndReads(t *testing.T) {
	line := "前浪推后浪，他们在雨里说了很多话。彭向明站在窗前，看着楼下来来往往的人群，心里想着明天的事情。\n"
	marker := "坏字节之后的正文仍然可读。"
	gbkHead, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(strings.Repeat(line, 10)))
	if err != nil {
		t.Fatal(err)
	}
	gbkTail, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(strings.Repeat(line, 10) + marker + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "GBK", data: append(append(gbkHead, 0xFF), gbkTail...)},
		{name: "UTF-8", data: []byte(strings.Repeat(line, 10) + "\xff" + strings.Repeat(line, 10) + marker + "\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestServer(t)
			auth := authHeader(t, router)
			previewW := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", "malformed.txt", tt.data, nil)
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
			importW := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "malformed.txt", nil, map[string]string{
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
			want := strings.Repeat(line, 10) + "�" + strings.Repeat(line, 10) + marker
			if got := strings.TrimSpace(content.Content); got != want {
				t.Fatalf("chapter content differs apart from the stray byte: got %d bytes ending %q", len(got), got[max(0, len(got)-60):])
			}
		})
	}
}
