package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

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
		"/api/chapter-image/" + token:                      "/api/chapter-image/<redacted>",
		"/api/cover/" + token:                              "/api/cover/<redacted>",
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

func TestRedactAccessPathHidesWebSocketLoginToken(t *testing.T) {
	input := "/ws/sync?token=secret.login.jwt&clientId=reader"
	if got := RedactAccessPath(input); got != "/ws/sync?<redacted>" {
		t.Fatalf("redacted websocket path = %q", got)
	}
	if got := RedactAccessPath("/ws/sync"); got != "/ws/sync" {
		t.Fatalf("websocket path without query = %q", got)
	}
}

func TestRedactAccessPathHidesEveryQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ordinary", input: "/api/health?q=private-reading-phrase", want: "/api/health?<redacted>"},
		{name: "modern search", input: "/api/search/content?q=chapter%20secret", want: "/api/search/content?<redacted>"},
		{name: "legacy search", input: "/reader3/searchBook?url=https%3A%2F%2Fsource.example&bookUrl=private&keyword=phrase", want: "/reader3/searchBook?<redacted>"},
		{name: "local store", input: "/api/local-store?path=%2FUsers%2Freader%2Fprivate.txt", want: "/api/local-store?<redacted>"},
		{name: "explore", input: "/api/explore?exploreUrl=https%3A%2F%2Fuser%3Apassword%40source.example", want: "/api/explore?<redacted>"},
		{name: "empty query", input: "/api/health?", want: "/api/health?<redacted>"},
		{name: "empty value", input: "/api/health?q=", want: "/api/health?<redacted>"},
		{name: "repeated", input: "/api/health?q=one&q=two", want: "/api/health?<redacted>"},
		{name: "malformed encoding", input: "/api/health?q=%zz", want: "/api/health?<redacted>"},
		{name: "unicode", input: "/api/health?q=私密正文", want: "/api/health?<redacted>"},
		{name: "capability query", input: "/api/epub-resource/secret-token/OPS/one.xhtml?download=private", want: "/api/epub-resource/<redacted>/OPS/one.xhtml?<redacted>"},
		{name: "capability prefix only in query", input: "/api/health?next=/api/epub-resource/secret-token/OPS/one.xhtml", want: "/api/health?<redacted>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RedactAccessPath(test.input); got != test.want {
				t.Fatalf("redacted path = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRedactAccessPathBoundsLargeQueryOutput(t *testing.T) {
	query := strings.Repeat("x", 256<<10)
	got := RedactAccessPath("/api/health?q=" + query)
	if got != "/api/health?<redacted>" {
		t.Fatalf("redacted path length = %d, want fixed marker %q", len(got), "/api/health?<redacted>")
	}
}

func TestAccessLoggerRedactsQueryWithoutChangingRequest(t *testing.T) {
	previousWriter := gin.DefaultWriter
	var logs bytes.Buffer
	gin.DefaultWriter = &logs
	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
	})

	router := gin.New()
	router.Use(AccessLogger())
	var receivedQuery string
	router.GET("/ok", func(c *gin.Context) {
		receivedQuery = c.Request.URL.RawQuery
		c.Status(http.StatusOK)
	})
	router.GET("/private", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	tests := []struct {
		name       string
		target     string
		wantStatus int
		secrets    []string
	}{
		{
			name:       "success",
			target:     "/ok?q=private-reading-phrase&url=https%3A%2F%2Fuser%3Apassword%40source.example%2Fbook%3Ftoken%3Dsecret",
			wantStatus: http.StatusOK,
			secrets:    []string{"private-reading-phrase", "user%3Apassword", "user:password", "token%3Dsecret", "token=secret"},
		},
		{name: "unauthorized", target: "/private?token=unauthorized-secret", wantStatus: http.StatusUnauthorized, secrets: []string{"unauthorized-secret"}},
		{name: "not found", target: "/missing?path=not-found-secret", wantStatus: http.StatusNotFound, secrets: []string{"not-found-secret"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := logs.Len()
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}

			entry := logs.String()[before:]
			path := strings.SplitN(test.target, "?", 2)[0]
			if !strings.Contains(entry, path+"?<redacted>") {
				t.Fatalf("access log does not contain fixed redaction marker: %q", entry)
			}
			for _, secret := range test.secrets {
				if strings.Contains(entry, secret) {
					t.Fatalf("access log contains query secret %q: %q", secret, entry)
				}
			}
		})
	}

	wantQuery := "q=private-reading-phrase&url=https%3A%2F%2Fuser%3Apassword%40source.example%2Fbook%3Ftoken%3Dsecret"
	if receivedQuery != wantQuery {
		t.Fatalf("handler raw query = %q, want unchanged query", receivedQuery)
	}
}

func TestAccessLoggerBoundsLargeQueryWithoutChangingRequest(t *testing.T) {
	previousWriter := gin.DefaultWriter
	var logs bytes.Buffer
	gin.DefaultWriter = &logs
	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
	})

	query := strings.Repeat("x", 256<<10)
	var receivedLength int
	router := gin.New()
	router.Use(AccessLogger())
	router.GET("/large", func(c *gin.Context) {
		receivedLength = len(c.Request.URL.RawQuery)
		c.Status(http.StatusNoContent)
	})

	before := logs.Len()
	request := httptest.NewRequest(http.MethodGet, "/large?q="+query, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if receivedLength != len("q=")+len(query) {
		t.Fatalf("handler raw query length = %d, want %d", receivedLength, len("q=")+len(query))
	}
	entry := logs.String()[before:]
	if !strings.Contains(entry, "/large?<redacted>") || len(entry) > 512 {
		t.Fatalf("access log is not fixed and bounded: length=%d prefix=%q", len(entry), entry[:min(len(entry), 160)])
	}
}
