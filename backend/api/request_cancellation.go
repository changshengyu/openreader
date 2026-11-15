package api

import "github.com/gin-gonic/gin"

// requestContextEnded reports whether the request's context was cancelled or
// passed its deadline, meaning the client has gone away. A handler then
// returns without writing a response, so the access log shows Gin's default
// 200 (no 499 is introduced). It checks the request context itself, not an
// error chain: GORM's joined rollback error hides context.Canceled, and the
// timeout of a derived context is a real failure that keeps its response.
// See docs/compat/request-cancellation-remaining-handlers-p2-contract.md.
func requestContextEnded(c *gin.Context) bool {
	return c.Request.Context().Err() != nil
}
