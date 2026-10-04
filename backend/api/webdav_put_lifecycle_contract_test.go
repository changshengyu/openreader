package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type webDAVPutMutationBody struct {
	mutate func()
	done   bool
}

func (r *webDAVPutMutationBody) Read(data []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	r.mutate()
	return copy(data, "uploaded"), io.EOF
}

func TestWebDAVPutLifecycleRetainsDualRouteStatusAndOriginalFiles(t *testing.T) {
	for _, prefix := range []string{"/reader3/webdav", "/webdav"} {
		t.Run(prefix, func(t *testing.T) {
			router, server := setupTestServer(t)
			_ = authHeader(t, router)
			auth := webDAVBasic("testuser", "test1234")
			server.cfg.MaxImportBytes = 4
			for _, payload := range []string{"1234", "new", ""} {
				response := webDAVProtocolRequest(t, router, http.MethodPut, prefix+"/target", auth, payload, nil)
				if response.Code != http.StatusCreated || response.Body.Len() != 0 {
					t.Fatalf("PUT %q = %d: %q", payload, response.Code, response.Body.String())
				}
				get := webDAVProtocolRequest(t, router, http.MethodGet, prefix+"/target", auth, "", nil)
				if get.Code != http.StatusOK || get.Body.String() != payload {
					t.Fatalf("GET uploaded file = %d: %q", get.Code, get.Body.String())
				}
			}
			overLimit := webDAVProtocolRequest(t, router, http.MethodPut, prefix+"/target", auth, "12345", nil)
			if overLimit.Code != http.StatusRequestEntityTooLarge || overLimit.Body.Len() != 0 {
				t.Fatalf("over-limit PUT = %d: %q", overLimit.Code, overLimit.Body.String())
			}
			missing := webDAVProtocolRequest(t, router, http.MethodPut, prefix+"/missing/target", auth, "x", nil)
			if missing.Code != http.StatusConflict || missing.Body.Len() != 0 {
				t.Fatalf("missing parent PUT = %d: %q", missing.Code, missing.Body.String())
			}
			server.cfg.MaxImportBytes = 1024
			target := filepath.Join(server.webdavDir(), "target")
			body := &webDAVPutMutationBody{mutate: func() {
				if err := os.Rename(target, target+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("newcomer"), 0o600); err != nil {
					t.Fatal(err)
				}
			}}
			request := httptest.NewRequest(http.MethodPut, prefix+"/target", body)
			request.Header.Set("Authorization", auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || response.Body.Len() != 0 {
				t.Fatalf("replaced target PUT = %d: %q", response.Code, response.Body.String())
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "newcomer" {
				t.Fatalf("concurrent target = %q: %v", data, err)
			}
			data, err = os.ReadFile(target + "-original")
			if err != nil || len(data) != 0 {
				t.Fatalf("original target = %q: %v", data, err)
			}
		})
	}
}
