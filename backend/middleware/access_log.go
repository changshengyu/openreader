package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

const epubResourceLogPrefix = "/api/epub-resource/"

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
	index := strings.Index(requestPath, epubResourceLogPrefix)
	if index < 0 {
		return requestPath
	}
	capabilityStart := index + len(epubResourceLogPrefix)
	remainder := requestPath[capabilityStart:]
	slash := strings.IndexByte(remainder, '/')
	if slash < 0 {
		return requestPath[:capabilityStart] + "<redacted>"
	}
	return requestPath[:capabilityStart] + "<redacted>" + remainder[slash:]
}
