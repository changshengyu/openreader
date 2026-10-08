package api

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// Hide *os.File fast paths: every ServeContent seek/read observes the original
// request, while metadata and all response bytes still come from one handle.
type storeReadSeeker struct {
	ctx  context.Context
	file io.ReadSeeker
}

type storeReadResponseWriter struct {
	http.ResponseWriter
	ctx context.Context
}

func (w storeReadResponseWriter) WriteHeader(status int) {
	if w.ctx.Err() == nil {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w storeReadResponseWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.ResponseWriter.Write(data)
}

func (r storeReadSeeker) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.Read(buffer)
}

func (r storeReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.Seek(offset, whence)
}

func serveOpenedStoreFile(c *gin.Context, file *os.File, info os.FileInfo) {
	if c.Request.Context().Err() != nil {
		return
	}
	ctx := c.Request.Context()
	http.ServeContent(storeReadResponseWriter{ResponseWriter: c.Writer, ctx: ctx}, c.Request, info.Name(), info.ModTime(), storeReadSeeker{ctx: ctx, file: file})
}
