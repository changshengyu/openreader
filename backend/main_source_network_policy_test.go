package main

import (
	"strings"
	"testing"

	"openreader/backend/config"
)

func TestConfigureSourceRuntimeRejectsInvalidAllowlistBeforeServerSetup(t *testing.T) {
	err := configureSourceRuntime(config.Config{
		SourceRequestTimeoutSeconds: 15,
		MaxSourceResponseBytes:      16 * 1024 * 1024,
		MaxSourceRedirects:          5,
		MaxSourceRetries:            3,
		SourceNetworkAllowlist:      "http://private-secret.internal",
	})
	if err == nil {
		t.Fatal("invalid source network allowlist was accepted")
	}
	if strings.Contains(err.Error(), "private-secret") {
		t.Fatalf("startup policy error leaked allowlist value: %v", err)
	}
}
