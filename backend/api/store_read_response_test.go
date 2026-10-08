package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type observedStoreReader struct {
	reads, seeks int
}

func (r *observedStoreReader) Read([]byte) (int, error)       { r.reads++; return 0, io.EOF }
func (r *observedStoreReader) Seek(int64, int) (int64, error) { r.seeks++; return 0, nil }

func TestStoreReadSeekerCancelledNeverTouchesSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := &observedStoreReader{}
	reader := storeReadSeeker{ctx: ctx, file: source}
	if n, err := reader.Read(make([]byte, 8)); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Read: n=%d err=%v", n, err)
	}
	if _, err := reader.Seek(0, io.SeekEnd); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Seek: %v", err)
	}
	if source.reads != 0 || source.seeks != 0 {
		t.Fatalf("cancelled response touched source: %+v", source)
	}
}

func TestStoreReadResponseCancelledSuppressesServeContentErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/file.txt", nil).WithContext(ctx)
	reader := storeReadSeeker{ctx: ctx, file: strings.NewReader("original")}
	http.ServeContent(storeReadResponseWriter{ctx: ctx, ResponseWriter: response}, request, "file.txt", time.Time{}, reader)
	if response.Body.Len() != 0 || response.Code == 500 {
		t.Fatalf("cancelled ServeContent produced error/success bytes: status=%d body=%q", response.Code, response.Body.String())
	}
}
