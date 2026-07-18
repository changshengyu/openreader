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
	if strings.HasPrefix(requestPath, "/ws/sync?") {
		return "/ws/sync?<redacted>"
	}
	for _, prefix := range capabilityResourceLogPrefixes {
		index := strings.Index(requestPath, prefix)
		if index < 0 {
			continue
		}
		capabilityStart := index + len(prefix)
		remainder := requestPath[capabilityStart:]
		slash := strings.IndexByte(remainder, '/')
		if slash < 0 {
			return requestPath[:capabilityStart] + "<redacted>"
		}
		return requestPath[:capabilityStart] + "<redacted>" + remainder[slash:]
	}
	return requestPath
}
