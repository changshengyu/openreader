package middleware

import "testing"

func TestRedactAccessPathHidesEPUBCapability(t *testing.T) {
	token := "secret.payload.signature"
	path := "/api/epub-resource/" + token + "/OPS/one.xhtml"
	got := RedactAccessPath(path)
	if got != "/api/epub-resource/<redacted>/OPS/one.xhtml" {
		t.Fatalf("redacted path = %q", got)
	}
	cases := map[string]string{
		"/api/cbz-resource/" + token + "/pages/001.jpg":    "/api/cbz-resource/<redacted>/pages/001.jpg",
		"/api/audio-resource/" + token + "/tracks/001.mp3": "/api/audio-resource/<redacted>/tracks/001.mp3",
	}
	for input, want := range cases {
		if got := RedactAccessPath(input); got != want {
			t.Fatalf("redacted path = %q, want %q", got, want)
		}
	}
	if RedactAccessPath("/api/books/1") != "/api/books/1" {
		t.Fatal("ordinary API path should remain unchanged")
	}
}
