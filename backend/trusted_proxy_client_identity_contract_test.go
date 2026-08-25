package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
)

func TestTrustedProxyDefaultIgnoresForwardedClientHeadersForRateLimit(t *testing.T) {
	router := trustedProxyContractRouter(t, "")

	first := trustedProxyContractRequest(router, "127.0.0.1:31001", "198.51.100.10", "")
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("first request = %d, want 401", first.Code)
	}
	second := trustedProxyContractRequest(router, "127.0.0.1:31002", "203.0.113.11", "192.0.2.12")
	assertTrustedProxyRateLimited(t, second)
}

func TestTrustedProxyConfigurationSeparatesForwardedClients(t *testing.T) {
	router := trustedProxyContractRouter(t, " 127.0.0.1/32 , ::1 ")

	first := trustedProxyContractRequest(router, "127.0.0.1:32001", "198.51.100.10", "")
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("first forwarded client = %d, want 401", first.Code)
	}
	secondClient := trustedProxyContractRequest(router, "127.0.0.1:32002", "203.0.113.11", "")
	if secondClient.Code != http.StatusUnauthorized {
		t.Fatalf("second forwarded client = %d, want separate 401 bucket", secondClient.Code)
	}
	repeated := trustedProxyContractRequest(router, "127.0.0.1:32003", "198.51.100.10", "")
	assertTrustedProxyRateLimited(t, repeated)
}

func TestTrustedProxyConfigurationRejectsUntrustedPeerHeaders(t *testing.T) {
	router := trustedProxyContractRouter(t, "10.0.0.0/8")

	first := trustedProxyContractRequest(router, "127.0.0.1:33001", "198.51.100.10", "")
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("first untrusted-peer request = %d, want 401", first.Code)
	}
	second := trustedProxyContractRequest(router, "127.0.0.1:33002", "203.0.113.11", "")
	assertTrustedProxyRateLimited(t, second)
}

func TestTrustedProxyConfigurationUsesFirstUntrustedHopFromRight(t *testing.T) {
	router := trustedProxyContractRouter(t, "127.0.0.1/32,10.0.0.0/8")

	first := trustedProxyContractRequest(router, "127.0.0.1:34001", "198.51.100.10, 10.0.0.5", "")
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("first proxy-chain request = %d, want 401", first.Code)
	}
	repeated := trustedProxyContractRequest(router, "127.0.0.1:34002", "198.51.100.10, 10.0.0.6", "")
	assertTrustedProxyRateLimited(t, repeated)

	different := trustedProxyContractRequest(router, "127.0.0.1:34003", "203.0.113.11, 10.0.0.5", "")
	if different.Code != http.StatusUnauthorized {
		t.Fatalf("different proxy-chain client = %d, want separate 401 bucket", different.Code)
	}
}

func TestTrustedProxyConfigurationRejectsMalformedLists(t *testing.T) {
	for _, value := range []string{
		"127.0.0.1,,10.0.0.0/8",
		"not-an-ip",
		"10.0.0.0/99",
		"*",
	} {
		t.Run(value, func(t *testing.T) {
			router := gin.New()
			if err := configureTrustedProxies(router, value); err == nil {
				t.Fatalf("configureTrustedProxies(%q) succeeded, want startup error", value)
			}
		})
	}
}

func TestTrustedProxyIdentityAndAccessLogUseTheSameValidatedAddress(t *testing.T) {
	previousWriter := gin.DefaultWriter
	var logs bytes.Buffer
	gin.DefaultWriter = &logs
	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
	})

	router := gin.New()
	if err := configureTrustedProxies(router, ""); err != nil {
		t.Fatalf("configure default trusted proxies: %v", err)
	}
	router.Use(middleware.AccessLogger(), middleware.NewRateLimiter(1, time.Minute).Middleware())
	router.GET("/api/private", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	response := trustedProxyContractRequest(router, "127.0.0.1:35001", "198.51.100.10", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("logged request = %d, want 401", response.Code)
	}
	entry := logs.String()
	if !strings.Contains(entry, "127.0.0.1") {
		t.Fatalf("access log missing peer identity: %q", entry)
	}
	if strings.Contains(entry, "198.51.100.10") {
		t.Fatalf("access log trusted a direct client header: %q", entry)
	}

	logs.Reset()
	trusted := gin.New()
	if err := configureTrustedProxies(trusted, "127.0.0.1"); err != nil {
		t.Fatalf("configure loopback proxy: %v", err)
	}
	trusted.Use(middleware.AccessLogger(), middleware.NewRateLimiter(1, time.Minute).Middleware())
	trusted.GET("/api/private", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	response = trustedProxyContractRequest(trusted, "127.0.0.1:35002", "198.51.100.10", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("trusted logged request = %d, want 401", response.Code)
	}
	entry = logs.String()
	if !strings.Contains(entry, "198.51.100.10") {
		t.Fatalf("access log missing validated forwarded identity: %q", entry)
	}
}

func trustedProxyContractRouter(t *testing.T, configured string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := configureTrustedProxies(router, configured); err != nil {
		t.Fatalf("configure trusted proxies %q: %v", configured, err)
	}
	router.Use(middleware.NewRateLimiter(1, time.Minute).Middleware())
	router.GET("/api/private", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	return router
}

func trustedProxyContractRequest(router http.Handler, remoteAddress, forwardedFor, realIP string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	request.RemoteAddr = remoteAddress
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	if realIP != "" {
		request.Header.Set("X-Real-IP", realIP)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertTrustedProxyRateLimited(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited request = %d body=%q, want 429", response.Code, response.Body.String())
	}
	want := `{"error":{"code":"RATE_LIMITED","message":"too many requests, try again later"}}`
	if strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("rate-limit body = %q, want %q", response.Body.String(), want)
	}
}
