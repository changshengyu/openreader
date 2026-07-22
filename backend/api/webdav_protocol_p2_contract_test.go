package api

import (
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"openreader/backend/models"
)

const webDAVAllowContract = "OPTIONS, DELETE, GET, PUT, PROPFIND, MKCOL, MOVE, COPY, LOCK, UNLOCK"

type davMultiStatusContract struct {
	XMLName   xml.Name              `xml:"multistatus"`
	Responses []davResponseContract `xml:"response"`
}

type davResponseContract struct {
	Href     string              `xml:"href"`
	PropStat davPropStatContract `xml:"propstat"`
}

type davPropStatContract struct {
	Status string          `xml:"status"`
	Prop   davPropContract `xml:"prop"`
}

type davPropContract struct {
	DisplayName   string                  `xml:"displayname"`
	LastModified  string                  `xml:"getlastmodified"`
	ContentLength int64                   `xml:"getcontentlength"`
	ContentType   string                  `xml:"getcontenttype"`
	ResourceType  davResourceTypeContract `xml:"resourcetype"`
}

type davResourceTypeContract struct {
	Collection *struct{} `xml:"collection"`
}

func webDAVBasic(username, password string) string {
	credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return "Basic " + credentials
}

func webDAVProtocolRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	authorization string,
	body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertWebDAVDiscoveryHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("DAV") != "1,2" {
		t.Fatalf("DAV discovery header = %q, want 1,2", response.Header().Get("DAV"))
	}
	if response.Header().Get("Allow") != webDAVAllowContract {
		t.Fatalf("Allow header = %q, want %q", response.Header().Get("Allow"), webDAVAllowContract)
	}
	if response.Header().Get("MS-Author-Via") != "DAV" {
		t.Fatalf("MS-Author-Via = %q, want DAV", response.Header().Get("MS-Author-Via"))
	}
}

func TestWebDAVProtocolDiscoveryAndDualAuthentication(t *testing.T) {
	t.Run("anonymous OPTIONS advertises the upstream protocol", func(t *testing.T) {
		router, _ := setupTestServer(t)
		response := webDAVProtocolRequest(t, router, http.MethodOptions, "/reader3/webdav/", "", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("anonymous OPTIONS = %d: %s", response.Code, response.Body.String())
		}
		assertWebDAVDiscoveryHeaders(t, response)
	})

	t.Run("missing and invalid credentials challenge without filesystem access", func(t *testing.T) {
		router, server := setupTestServer(t)
		_ = authHeader(t, router)
		root := server.webdavDir()

		missing := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", "", "", nil)
		if missing.Code != http.StatusUnauthorized {
			t.Fatalf("missing credentials = %d: %s", missing.Code, missing.Body.String())
		}
		if missing.Header().Get("WWW-Authenticate") != `Basic realm="OpenReader WebDAV"` {
			t.Fatalf("missing Basic challenge: %q", missing.Header().Get("WWW-Authenticate"))
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("unauthenticated PROPFIND touched WebDAV root: %v", err)
		}

		invalid := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", webDAVBasic("testuser", "wrong-pass"), "", nil)
		if invalid.Code != http.StatusUnauthorized {
			t.Fatalf("invalid Basic credentials = %d: %s", invalid.Code, invalid.Body.String())
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("invalid Basic credentials touched WebDAV root: %v", err)
		}
	})

	t.Run("Basic and Bearer resolve the same user and permission", func(t *testing.T) {
		router, server := setupTestServer(t)
		bearer := authHeader(t, router)

		for _, request := range []struct {
			name string
			path string
			auth string
		}{
			{name: "upstream Basic", path: "/reader3/webdav/", auth: webDAVBasic("testuser", "test1234")},
			{name: "current Basic", path: "/webdav/", auth: webDAVBasic("testuser", "test1234")},
			{name: "current Bearer", path: "/webdav/", auth: bearer},
			{name: "upstream Bearer", path: "/reader3/webdav/", auth: bearer},
		} {
			t.Run(request.name, func(t *testing.T) {
				response := webDAVProtocolRequest(t, router, "PROPFIND", request.path, request.auth, "", map[string]string{"Depth": "0"})
				if response.Code != http.StatusMultiStatus {
					t.Fatalf("PROPFIND = %d: %s", response.Code, response.Body.String())
				}
				assertWebDAVDiscoveryHeaders(t, response)
			})
		}

		if err := server.db.Model(&struct {
			ID uint
		}{}).Table("users").Where("username = ?", "testuser").Update("can_access_webdav", false).Error; err != nil {
			t.Fatalf("disable WebDAV: %v", err)
		}
		denied := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", webDAVBasic("testuser", "test1234"), "", nil)
		if denied.Code != http.StatusForbidden {
			t.Fatalf("valid Basic without permission = %d: %s", denied.Code, denied.Body.String())
		}
	})

	t.Run("password reset immediately invalidates old Basic credentials", func(t *testing.T) {
		router, server := setupTestServer(t)
		_ = authHeader(t, router)
		newHash, err := bcrypt.GenerateFromPassword([]byte("changed123"), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		if err := server.db.Table("users").Where("username = ?", "testuser").Update("password_hash", string(newHash)).Error; err != nil {
			t.Fatal(err)
		}

		oldPassword := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", webDAVBasic("testuser", "test1234"), "", nil)
		if oldPassword.Code != http.StatusUnauthorized {
			t.Fatalf("old Basic password = %d: %s", oldPassword.Code, oldPassword.Body.String())
		}
		newPassword := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", webDAVBasic("testuser", "changed123"), "", nil)
		if newPassword.Code != http.StatusMultiStatus {
			t.Fatalf("new Basic password = %d: %s", newPassword.Code, newPassword.Body.String())
		}
	})
}

func TestWebDAVProtocolRejectsInvalidAuthorizationAndDeletedUsersBeforeStorage(t *testing.T) {
	router, server := setupTestServer(t)
	bearer := authHeader(t, router)
	root := server.webdavDir()

	for _, request := range []struct {
		name   string
		method string
		auth   string
	}{
		{name: "invalid bearer", method: "PROPFIND", auth: "Bearer invalid-token"},
		{name: "invalid Basic on OPTIONS", method: http.MethodOptions, auth: webDAVBasic("testuser", "wrong-pass")},
		{name: "unknown scheme", method: "PROPFIND", auth: "Digest abc"},
	} {
		t.Run(request.name, func(t *testing.T) {
			response := webDAVProtocolRequest(t, router, request.method, "/reader3/webdav/", request.auth, "", nil)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("invalid authorization = %d: %s", response.Code, response.Body.String())
			}
			assertWebDAVDiscoveryHeaders(t, response)
			if response.Header().Get("WWW-Authenticate") != `Basic realm="OpenReader WebDAV"` {
				t.Fatalf("invalid authorization lost Basic challenge: %q", response.Header().Get("WWW-Authenticate"))
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("invalid authorization touched storage: %v", err)
			}
		})
	}

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Delete(&models.User{}, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, authorization := range []string{bearer, webDAVBasic("testuser", "test1234")} {
		response := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", authorization, "", nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("deleted user authorization = %d: %s", response.Code, response.Body.String())
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("deleted user request touched storage: %v", err)
		}
	}
}

func TestWebDAVProtocolAliasesShareOnlyTheAuthenticatedPrivateRoot(t *testing.T) {
	router, server := setupTestServer(t)
	adminBearer := authHeader(t, router)
	adminBasic := webDAVBasic("testuser", "test1234")
	memberBearer := registerStorageTestUser(t, router, "davmember")
	memberBasic := webDAVBasic("davmember", "secret123")

	for _, operation := range []struct {
		method string
		path   string
		auth   string
		body   string
	}{
		{method: "MKCOL", path: "/reader3/webdav/admin", auth: adminBasic},
		{method: http.MethodPut, path: "/reader3/webdav/admin/only.txt", auth: adminBasic, body: "administrator"},
		{method: "MKCOL", path: "/reader3/webdav/member", auth: memberBasic},
		{method: http.MethodPut, path: "/reader3/webdav/member/only.txt", auth: memberBasic, body: "member"},
	} {
		response := webDAVProtocolRequest(t, router, operation.method, operation.path, operation.auth, operation.body, nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("%s %s = %d: %s", operation.method, operation.path, response.Code, response.Body.String())
		}
	}

	memberAlias := webDAVProtocolRequest(t, router, http.MethodGet, "/webdav/member/only.txt", memberBearer, "", nil)
	if memberAlias.Code != http.StatusOK || memberAlias.Body.String() != "member" {
		t.Fatalf("member alias read = %d: %q", memberAlias.Code, memberAlias.Body.String())
	}
	adminAlias := webDAVProtocolRequest(t, router, http.MethodGet, "/webdav/admin/only.txt", adminBearer, "", nil)
	if adminAlias.Code != http.StatusOK || adminAlias.Body.String() != "administrator" {
		t.Fatalf("administrator alias read = %d: %q", adminAlias.Code, adminAlias.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, http.MethodGet, "/reader3/webdav/admin/only.txt", memberBasic, "", nil); response.Code != http.StatusNotFound {
		t.Fatalf("member read administrator root = %d: %s", response.Code, response.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, http.MethodGet, "/reader3/webdav/member/only.txt", adminBasic, "", nil); response.Code != http.StatusNotFound {
		t.Fatalf("administrator read member root = %d: %s", response.Code, response.Body.String())
	}

	copyResponse := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/member/only.txt", memberBasic, "", map[string]string{
		"Destination": "/webdav/member/copied.txt",
	})
	if copyResponse.Code != http.StatusCreated {
		t.Fatalf("cross-prefix COPY = %d: %s", copyResponse.Code, copyResponse.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, http.MethodGet, "/reader3/webdav/member/copied.txt", adminBasic, "", nil); response.Code != http.StatusNotFound {
		t.Fatalf("cross-prefix COPY escaped caller root = %d: %s", response.Code, response.Body.String())
	}

	if content, err := os.ReadFile(filepath.Join(server.webdavDir(), "admin", "only.txt")); err != nil || string(content) != "administrator" {
		t.Fatalf("administrator physical root changed: content=%q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(server.webdavDir(), "users", "davmember", "member", "copied.txt")); err != nil || string(content) != "member" {
		t.Fatalf("member private physical root changed: content=%q err=%v", content, err)
	}
}

func TestWebDAVProtocolPropfindContract(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth := webDAVBasic("testuser", "test1234")
	root := server.webdavDir()
	if err := os.MkdirAll(filepath.Join(root, "目录"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	depthOne := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", auth, "", map[string]string{"Depth": "1"})
	if depthOne.Code != http.StatusMultiStatus {
		t.Fatalf("Depth 1 PROPFIND = %d: %s", depthOne.Code, depthOne.Body.String())
	}
	if !strings.Contains(depthOne.Body.String(), `DAV:`) {
		t.Fatalf("PROPFIND is not DAV namespaced: %s", depthOne.Body.String())
	}
	var listing davMultiStatusContract
	if err := xml.Unmarshal(depthOne.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode PROPFIND: %v\n%s", err, depthOne.Body.String())
	}
	if listing.XMLName.Space != "DAV:" || len(listing.Responses) != 3 {
		t.Fatalf("PROPFIND root/children contract: namespace=%q responses=%+v", listing.XMLName.Space, listing.Responses)
	}
	if listing.Responses[0].Href != "/reader3/webdav/" {
		t.Fatalf("root href = %q", listing.Responses[0].Href)
	}
	if listing.Responses[1].PropStat.Prop.DisplayName != "alpha.txt" || listing.Responses[1].PropStat.Prop.ContentLength != 5 {
		t.Fatalf("stable file response = %+v", listing.Responses[1])
	}
	if listing.Responses[2].PropStat.Prop.DisplayName != "目录" || listing.Responses[2].PropStat.Prop.ResourceType.Collection == nil {
		t.Fatalf("stable directory response = %+v", listing.Responses[2])
	}

	depthZero := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", auth, "", map[string]string{"Depth": "0"})
	var targetOnly davMultiStatusContract
	if err := xml.Unmarshal(depthZero.Body.Bytes(), &targetOnly); err != nil || len(targetOnly.Responses) != 1 {
		t.Fatalf("Depth 0 target-only contract: err=%v body=%s", err, depthZero.Body.String())
	}

	missingPath := filepath.Join(root, "missing")
	missing := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/missing", auth, "", map[string]string{"Depth": "1"})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing PROPFIND = %d: %s", missing.Code, missing.Body.String())
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("missing PROPFIND created a path: %v", err)
	}
}

func TestWebDAVProtocolPropfindEncodesHrefsAndClampsDepth(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth := webDAVBasic("testuser", "test1234")
	root := server.webdavDir()
	if err := os.MkdirAll(filepath.Join(root, "目录"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "空 格.txt"), []byte("encoded"), 0o644); err != nil {
		t.Fatal(err)
	}

	response := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/", auth, "", map[string]string{"Depth": "infinity"})
	if response.Code != http.StatusMultiStatus {
		t.Fatalf("Depth infinity PROPFIND = %d: %s", response.Code, response.Body.String())
	}
	var listing davMultiStatusContract
	if err := xml.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing: %v\n%s", err, response.Body.String())
	}
	if len(listing.Responses) != 3 {
		t.Fatalf("Depth infinity was not clamped to one level: %+v", listing.Responses)
	}
	byName := make(map[string]davResponseContract, len(listing.Responses))
	for _, item := range listing.Responses {
		byName[item.PropStat.Prop.DisplayName] = item
	}
	file := byName["空 格.txt"]
	if file.Href != "/reader3/webdav/%E7%A9%BA%20%E6%A0%BC.txt" {
		t.Fatalf("encoded file href = %q", file.Href)
	}
	if file.PropStat.Status != "HTTP/1.1 200 OK" || file.PropStat.Prop.LastModified == "" || !strings.HasPrefix(file.PropStat.Prop.ContentType, "text/plain") {
		t.Fatalf("file DAV properties incomplete: %+v", file)
	}
	directory := byName["目录"]
	if directory.Href != "/reader3/webdav/%E7%9B%AE%E5%BD%95/" || directory.PropStat.Prop.ResourceType.Collection == nil {
		t.Fatalf("encoded directory properties = %+v", directory)
	}

	fileResponse := webDAVProtocolRequest(t, router, "PROPFIND", "/reader3/webdav/%E7%A9%BA%20%E6%A0%BC.txt", auth, "", map[string]string{"Depth": "infinity"})
	var targetOnly davMultiStatusContract
	if fileResponse.Code != http.StatusMultiStatus || xml.Unmarshal(fileResponse.Body.Bytes(), &targetOnly) != nil || len(targetOnly.Responses) != 1 {
		t.Fatalf("file target PROPFIND = %d: %s", fileResponse.Code, fileResponse.Body.String())
	}
}

func TestWebDAVProtocolMutationAndLockContract(t *testing.T) {
	router, _ := setupTestServer(t)
	bearer := authHeader(t, router)
	auth := webDAVBasic("testuser", "test1234")

	mkdir := webDAVProtocolRequest(t, router, "MKCOL", "/reader3/webdav/books", auth, "", nil)
	if mkdir.Code != http.StatusCreated {
		t.Fatalf("MKCOL = %d: %s", mkdir.Code, mkdir.Body.String())
	}
	mkdirAgain := webDAVProtocolRequest(t, router, "MKCOL", "/reader3/webdav/books", auth, "", nil)
	if mkdirAgain.Code != http.StatusCreated {
		t.Fatalf("idempotent MKCOL = %d: %s", mkdirAgain.Code, mkdirAgain.Body.String())
	}

	missingParent := webDAVProtocolRequest(t, router, http.MethodPut, "/reader3/webdav/missing/a.txt", auth, "no parent", nil)
	if missingParent.Code != http.StatusConflict {
		t.Fatalf("PUT without parent = %d: %s", missingParent.Code, missingParent.Body.String())
	}
	put := webDAVProtocolRequest(t, router, http.MethodPut, "/reader3/webdav/books/a.txt", auth, "hello", nil)
	if put.Code != http.StatusCreated {
		t.Fatalf("PUT = %d: %s", put.Code, put.Body.String())
	}

	copyResponse := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/books/a.txt", auth, "", map[string]string{
		"Destination": "/webdav/books/b.txt",
	})
	if copyResponse.Code != http.StatusCreated {
		t.Fatalf("COPY = %d: %s", copyResponse.Code, copyResponse.Body.String())
	}
	copyWithoutOverwrite := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/books/a.txt", auth, "", map[string]string{
		"Destination": "/reader3/webdav/books/b.txt",
	})
	if copyWithoutOverwrite.Code != http.StatusPreconditionFailed {
		t.Fatalf("COPY without overwrite = %d: %s", copyWithoutOverwrite.Code, copyWithoutOverwrite.Body.String())
	}
	copyWithOverwrite := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/books/a.txt", auth, "", map[string]string{
		"Destination": "/reader3/webdav/books/b.txt",
		"Overwrite":   "T",
	})
	if copyWithOverwrite.Code != http.StatusCreated {
		t.Fatalf("COPY with overwrite = %d: %s", copyWithOverwrite.Code, copyWithOverwrite.Body.String())
	}

	move := webDAVProtocolRequest(t, router, "MOVE", "/reader3/webdav/books/b.txt", auth, "", map[string]string{
		"Destination": "/reader3/webdav/books/c.txt",
	})
	if move.Code != http.StatusCreated {
		t.Fatalf("MOVE = %d: %s", move.Code, move.Body.String())
	}
	get := webDAVProtocolRequest(t, router, http.MethodGet, "/reader3/webdav/books/c.txt", auth, "", nil)
	if get.Code != http.StatusOK || get.Body.String() != "hello" {
		t.Fatalf("GET copied/moved file = %d: %q", get.Code, get.Body.String())
	}

	lock := webDAVProtocolRequest(t, router, "LOCK", "/reader3/webdav/books/c.txt", auth, "", map[string]string{"Timeout": "Second-120"})
	if lock.Code != http.StatusOK || !strings.HasPrefix(lock.Header().Get("Lock-Token"), "urn:uuid:") {
		t.Fatalf("LOCK = %d token=%q body=%s", lock.Code, lock.Header().Get("Lock-Token"), lock.Body.String())
	}
	if !strings.Contains(lock.Body.String(), "lockdiscovery") || !strings.Contains(lock.Body.String(), "Second-120") {
		t.Fatalf("LOCK body = %s", lock.Body.String())
	}
	unlockMissing := webDAVProtocolRequest(t, router, "UNLOCK", "/reader3/webdav/books/c.txt", auth, "", nil)
	if unlockMissing.Code != http.StatusBadRequest {
		t.Fatalf("UNLOCK without token = %d: %s", unlockMissing.Code, unlockMissing.Body.String())
	}
	unlock := webDAVProtocolRequest(t, router, "UNLOCK", "/reader3/webdav/books/c.txt", auth, "", map[string]string{"Lock-Token": lock.Header().Get("Lock-Token")})
	if unlock.Code != http.StatusNoContent {
		t.Fatalf("UNLOCK = %d: %s", unlock.Code, unlock.Body.String())
	}

	deleted := webDAVProtocolRequest(t, router, http.MethodDelete, "/reader3/webdav/books/c.txt", auth, "", nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("upstream DELETE = %d: %s", deleted.Code, deleted.Body.String())
	}
	missingDelete := webDAVProtocolRequest(t, router, http.MethodDelete, "/reader3/webdav/books/c.txt", auth, "", nil)
	if missingDelete.Code != http.StatusNotFound {
		t.Fatalf("missing DELETE = %d: %s", missingDelete.Code, missingDelete.Body.String())
	}

	currentDelete := webDAVProtocolRequest(t, router, http.MethodDelete, "/webdav/books/a.txt", bearer, "", nil)
	if currentDelete.Code != http.StatusNoContent {
		t.Fatalf("current DELETE compatibility = %d: %s", currentDelete.Code, currentDelete.Body.String())
	}
}

func TestWebDAVProtocolMutationFailuresAndRecursiveCopyAreTransactional(t *testing.T) {
	router, _ := setupTestServer(t)
	_ = authHeader(t, router)
	auth := webDAVBasic("testuser", "test1234")

	for _, path := range []string{"/reader3/webdav/tree", "/reader3/webdav/tree/sub", "/reader3/webdav/target"} {
		response := webDAVProtocolRequest(t, router, "MKCOL", path, auth, "", nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("MKCOL %s = %d: %s", path, response.Code, response.Body.String())
		}
	}
	if response := webDAVProtocolRequest(t, router, http.MethodPut, "/reader3/webdav/tree/sub/value.txt", auth, "recursive", nil); response.Code != http.StatusCreated {
		t.Fatalf("seed recursive tree = %d: %s", response.Code, response.Body.String())
	}
	copyResponse := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/tree", auth, "", map[string]string{"Destination": "/reader3/webdav/copied"})
	if copyResponse.Code != http.StatusCreated {
		t.Fatalf("recursive COPY = %d: %s", copyResponse.Code, copyResponse.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, http.MethodGet, "/reader3/webdav/copied/sub/value.txt", auth, "", nil); response.Code != http.StatusOK || response.Body.String() != "recursive" {
		t.Fatalf("recursive COPY content = %d: %q", response.Code, response.Body.String())
	}

	for _, request := range []struct {
		name        string
		method      string
		path        string
		destination string
		overwrite   string
		want        int
	}{
		{name: "missing source", method: "COPY", path: "/reader3/webdav/missing", destination: "/reader3/webdav/target/missing", want: http.StatusPreconditionFailed},
		{name: "missing destination", method: "MOVE", path: "/reader3/webdav/tree/sub/value.txt", want: http.StatusBadRequest},
		{name: "missing destination parent", method: "COPY", path: "/reader3/webdav/tree/sub/value.txt", destination: "/reader3/webdav/no-parent/value.txt", want: http.StatusConflict},
		{name: "explicit no overwrite", method: "COPY", path: "/reader3/webdav/tree/sub/value.txt", destination: "/reader3/webdav/copied/sub/value.txt", overwrite: "F", want: http.StatusPreconditionFailed},
		{name: "copy onto self", method: "COPY", path: "/reader3/webdav/tree", destination: "/reader3/webdav/tree", overwrite: "T", want: http.StatusForbidden},
		{name: "move into descendant", method: "MOVE", path: "/reader3/webdav/tree", destination: "/reader3/webdav/tree/sub/moved", want: http.StatusForbidden},
	} {
		t.Run(request.name, func(t *testing.T) {
			headers := map[string]string{}
			if request.destination != "" {
				headers["Destination"] = request.destination
			}
			if request.overwrite != "" {
				headers["Overwrite"] = request.overwrite
			}
			response := webDAVProtocolRequest(t, router, request.method, request.path, auth, "", headers)
			if response.Code != request.want {
				t.Fatalf("response = %d: %s, want %d", response.Code, response.Body.String(), request.want)
			}
		})
	}

	if response := webDAVProtocolRequest(t, router, http.MethodPut, "/reader3/webdav/tree", auth, "directory target", nil); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT directory target = %d: %s", response.Code, response.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, http.MethodPut, "/reader3/webdav/tree/sub/value.txt", auth, "overwritten", nil); response.Code != http.StatusCreated {
		t.Fatalf("overwrite file = %d: %s", response.Code, response.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, http.MethodGet, "/reader3/webdav/tree/sub/value.txt", auth, "", nil); response.Body.String() != "overwritten" {
		t.Fatalf("overwritten content = %d: %q", response.Code, response.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, "MKCOL", "/reader3/webdav/tree/sub/value.txt/child", auth, "", nil); response.Code != http.StatusConflict {
		t.Fatalf("MKCOL below file = %d: %s", response.Code, response.Body.String())
	}
}

func TestWebDAVProtocolRejectsPortableTraversalAndRootMutation(t *testing.T) {
	router, _ := setupTestServer(t)
	_ = authHeader(t, router)
	auth := webDAVBasic("testuser", "test1234")
	if response := webDAVProtocolRequest(t, router, "MKCOL", "/reader3/webdav/safe", auth, "", nil); response.Code != http.StatusCreated {
		t.Fatalf("seed safe directory = %d: %s", response.Code, response.Body.String())
	}
	if response := webDAVProtocolRequest(t, router, http.MethodPut, "/reader3/webdav/safe/source.txt", auth, "safe", nil); response.Code != http.StatusCreated {
		t.Fatalf("seed safe file = %d: %s", response.Code, response.Body.String())
	}

	for _, path := range []string{
		"/reader3/webdav/%2e%2e/outside.txt",
		"/reader3/webdav/C:/Windows/system.ini",
		"/reader3/webdav/C:%5CWindows%5Csystem.ini",
		"/reader3/webdav/safe/%00outside.txt",
	} {
		response := webDAVProtocolRequest(t, router, http.MethodPut, path, auth, "blocked", nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("unsafe PUT %q = %d: %s", path, response.Code, response.Body.String())
		}
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete, "MKCOL", "MOVE", "COPY"} {
		headers := map[string]string{}
		if method == "MOVE" || method == "COPY" {
			headers["Destination"] = "/reader3/webdav/safe/root-target"
		}
		response := webDAVProtocolRequest(t, router, method, "/reader3/webdav/", auth, "blocked", headers)
		if response.Code != http.StatusForbidden {
			t.Fatalf("root %s = %d: %s", method, response.Code, response.Body.String())
		}
	}
	for _, destination := range []string{
		"/outside.txt",
		"/reader3/webdav/../outside.txt",
		"/reader3/webdav/C:/outside.txt",
	} {
		response := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/safe/source.txt", auth, "", map[string]string{"Destination": destination})
		if response.Code != http.StatusBadRequest && response.Code != http.StatusForbidden {
			t.Fatalf("unsafe Destination %q = %d: %s", destination, response.Code, response.Body.String())
		}
	}
}

func TestWebDAVProtocolLockDefaultIsEphemeral(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth := webDAVBasic("testuser", "test1234")
	root := server.webdavDir()

	lock := webDAVProtocolRequest(t, router, "LOCK", "/reader3/webdav/missing.txt", auth, "", nil)
	if lock.Code != http.StatusOK || !strings.Contains(lock.Body.String(), "Second-3600") {
		t.Fatalf("default LOCK = %d: %s", lock.Code, lock.Body.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("ephemeral LOCK created persistent storage: %v", err)
	}
	unlock := webDAVProtocolRequest(t, router, "UNLOCK", "/reader3/webdav/missing.txt", auth, "", map[string]string{"Lock-Token": lock.Header().Get("Lock-Token")})
	if unlock.Code != http.StatusNoContent {
		t.Fatalf("ephemeral UNLOCK = %d: %s", unlock.Code, unlock.Body.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("ephemeral UNLOCK created persistent storage: %v", err)
	}
}

func TestWebDAVProtocolRejectsSymlinksAndDescendantCopies(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth := webDAVBasic("testuser", "test1234")
	root := server.webdavDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	for _, request := range []struct {
		name    string
		method  string
		path    string
		body    string
		headers map[string]string
	}{
		{name: "GET source", method: http.MethodGet, path: "/reader3/webdav/linked/secret.txt"},
		{name: "PROPFIND source", method: "PROPFIND", path: "/reader3/webdav/linked", headers: map[string]string{"Depth": "1"}},
		{name: "PUT destination", method: http.MethodPut, path: "/reader3/webdav/linked/new.txt", body: "blocked"},
		{name: "COPY source", method: "COPY", path: "/reader3/webdav/linked", headers: map[string]string{"Destination": "/reader3/webdav/copied"}},
	} {
		t.Run(request.name, func(t *testing.T) {
			response := webDAVProtocolRequest(t, router, request.method, request.path, auth, request.body, request.headers)
			if response.Code != http.StatusForbidden {
				t.Fatalf("unsafe symlink operation = %d: %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), root) || strings.Contains(response.Body.String(), outside) {
				t.Fatalf("unsafe response leaked a host path: %s", response.Body.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("PUT followed symlink outside root: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "tree"), 0o755); err != nil {
		t.Fatal(err)
	}
	descendant := webDAVProtocolRequest(t, router, "COPY", "/reader3/webdav/tree", auth, "", map[string]string{
		"Destination": "/reader3/webdav/tree/child",
	})
	if descendant.Code != http.StatusForbidden {
		t.Fatalf("directory COPY into descendant = %d: %s", descendant.Code, descendant.Body.String())
	}
}

func TestWebDAVProtocolRejectsPrivateRootParentSymlink(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	memberBearer := registerStorageTestUser(t, router, "davsymlink")
	base := server.webdavDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "users")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	response := webDAVProtocolRequest(t, router, http.MethodPut, "/reader3/webdav/private.txt", memberBearer, "blocked", nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("private root parent symlink PUT = %d: %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "davsymlink", "private.txt")); !os.IsNotExist(err) {
		t.Fatalf("private root symlink escaped WebDAV base: %v", err)
	}
}
