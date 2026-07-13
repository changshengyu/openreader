package api

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
)

type sourceErrorDetails struct {
	Code  string
	Stage string
}

// sourceErrorDetailsFor turns parser and remote-fetch failures into a small,
// stable client contract. The cause remains available to server-side failure
// handling, but never needs to cross the API boundary with source URLs,
// headers, credentials, variables, or response text.
func sourceErrorDetailsFor(err error, stage string) sourceErrorDetails {
	if err == nil {
		return sourceErrorDetails{}
	}
	code := "content_unavailable"
	switch {
	case errors.Is(err, engine.ErrUnsupportedSourceRule):
		code = "source_rule_unsupported"
	case errors.Is(err, engine.ErrInvalidSourceRule):
		code = "source_rule_invalid"
	case engine.IsSourceRequestError(err):
		code = "source_request_failed"
	}
	return sourceErrorDetails{Code: code, Stage: stage}
}

func sourceErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errTimeout) {
		return errTimeout.Error()
	}
	if strings.Contains(err.Error(), "has no search URL") {
		// The engine currently embeds the configured source name in this local
		// validation error. Preserve the old actionable wording without echoing
		// that arbitrary value or other parser detail to the client.
		return "no search URL"
	}
	switch sourceErrorDetailsFor(err, "").Code {
	case "source_rule_unsupported":
		return "book source rule is unsupported"
	case "source_rule_invalid":
		return "book source rule is invalid"
	case "source_request_failed":
		return "failed to request book source"
	default:
		return "book source content is unavailable"
	}
}

func sourceErrorPayload(message string, err error, stage string) gin.H {
	payload := gin.H{"error": message}
	details := sourceErrorDetailsFor(err, stage)
	if details.Code != "" {
		payload["code"] = details.Code
		payload["stage"] = details.Stage
	}
	return payload
}

func writeSourceError(c *gin.Context, status int, message string, err error, stage string) {
	c.JSON(status, sourceErrorPayload(message, err, stage))
}

func sourceDebugPayload(payload gin.H, err error, stage string) gin.H {
	payload["error"] = sourceErrorMessage(err)
	details := sourceErrorDetailsFor(err, stage)
	if details.Code != "" {
		payload["code"] = details.Code
		payload["stage"] = details.Stage
	}
	return payload
}
