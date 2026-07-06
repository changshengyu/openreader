package middleware

import "testing"

func TestRedactAccessPathHidesEPUBCapability(t *testing.T) {
	token := "secret.payload.signature"
	path := "/api/epub-resource/" + token + "/OPS/one.xhtml"
	got := RedactAccessPath(path)
	if got != "/api/epub-resource/<redacted>/OPS/one.xhtml" {
		t.Fatalf("redacted path = %q", got)
	}
	if RedactAccessPath("/api/books/1") != "/api/books/1" {
		t.Fatal("ordinary API path should remain unchanged")
	}
}
