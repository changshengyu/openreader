package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/config"
)

func TestCORSDefersWebDAVDiscoveryOptionsToTheProtocolRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors(config.Config{CORSOrigin: "https://reader.example"}))
	router.Handle(http.MethodOptions, "/reader3/webdav/*path", func(c *gin.Context) {
		c.Header("DAV", "1,2")
		c.Header("Allow", "OPTIONS, DELETE, GET, PUT, PROPFIND, MKCOL, MOVE, COPY, LOCK, UNLOCK")
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodOptions, "/reader3/webdav/", nil)
	request.Header.Set("Origin", "https://reader.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("WebDAV OPTIONS = %d, want 200", response.Code)
	}
	if response.Header().Get("DAV") != "1,2" {
		t.Fatalf("WebDAV OPTIONS lost DAV discovery header: %q", response.Header().Get("DAV"))
	}
	if methods := response.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "PROPFIND") || !strings.Contains(methods, "COPY") {
		t.Fatalf("CORS methods do not advertise WebDAV methods: %q", methods)
	}
}

func TestCORSStillCompletesOrdinaryPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors(config.Config{CORSOrigin: "https://reader.example"}))
	router.Handle(http.MethodOptions, "/api/example", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodOptions, "/api/example", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("ordinary CORS preflight = %d, want 204", response.Code)
	}
}

func TestCORSWildcardNeverAdvertisesCredentialedAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors(config.Config{}))
	router.GET("/api/example", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/example", nil))
	if response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("fallback CORS origin = %q, want *", response.Header().Get("Access-Control-Allow-Origin"))
	}
	if response.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("wildcard CORS must not allow credentials: %q", response.Header().Get("Access-Control-Allow-Credentials"))
	}
}
