package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

var capabilityResourceLogPrefixes = []string{
	"/api/epub-resource/",
	"/api/cbz-resource/",
	"/api/audio-resource/",
	"/api/chapter-image/",
	"/api/cover/",
}

func AccessLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(params gin.LogFormatterParams) string {
		path := RedactAccessPath(params.Path)
		return fmt.Sprintf(
			"[GIN] %v |%3d| %13v | %15s |%-7s %s\n",
			params.TimeStamp.Format("2006/01/02 - 15:04:05"),
			params.StatusCode,
			params.Latency,
			params.ClientIP,
			params.Method,
			path,
		)
	})
}

func RedactAccessPath(requestPath string) string {
	path, _, hasQuery := strings.Cut(requestPath, "?")
	redactedPath := path
	for _, prefix := range capabilityResourceLogPrefixes {
		index := strings.Index(path, prefix)
		if index < 0 {
			continue
		}
		capabilityStart := index + len(prefix)
		remainder := path[capabilityStart:]
		slash := strings.IndexByte(remainder, '/')
		if slash < 0 {
			redactedPath = path[:capabilityStart] + "<redacted>"
		} else {
			redactedPath = path[:capabilityStart] + "<redacted>" + remainder[slash:]
		}
		break
	}
	if hasQuery {
		return redactedPath + "?<redacted>"
	}
	return redactedPath
}
