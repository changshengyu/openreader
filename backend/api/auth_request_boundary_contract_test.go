package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"openreader/backend/models"
)

const authRequestBoundaryBytes = 16 << 10

func authBoundaryRequest(router http.Handler, path, body string, chunked bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertAuthBoundaryError(t *testing.T, response *httptest.ResponseRecorder, status int, message string, secret string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d, want %d: %s", response.Code, status, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v: %s", err, response.Body.String())
	}
	if len(payload) != 1 || payload["error"] != message {
		t.Fatalf("error payload=%v, want only %q", payload, message)
	}
	if secret != "" && strings.Contains(response.Body.String(), secret) {
		t.Fatalf("error response leaked submitted secret")
	}
}

func createAuthBoundaryUser(t *testing.T, server *Server, username, password string, lastActiveAt time.Time) models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	user := models.User{
		Username:       username,
		PasswordHash:   string(hash),
		Role:           "user",
		CanEditSources: true,
		CanAccessStore: true,
		LastActiveAt:   lastActiveAt,
	}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatalf("create auth fixture user: %v", err)
	}
	return user
}

func TestPublicAuthRejectsDeclaredAndChunkedOversizedBodiesBeforeMutation(t *testing.T) {
	for _, endpoint := range []string{"/api/auth/register", "/api/auth/login"} {
		for _, chunked := range []bool{false, true} {
			name := strings.TrimPrefix(endpoint, "/api/auth/")
			if chunked {
				name += "-chunked"
			} else {
				name += "-declared"
			}
			t.Run(name, func(t *testing.T) {
				router, server := setupTestServer(t)
				const password = "boundary-secret-password8"
				previous := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
				var user models.User
				username := "oversizedregister"
				if endpoint == "/api/auth/login" {
					username = "oversizedlogin"
					user = createAuthBoundaryUser(t, server, username, password, previous)
				}
				body := `{"username":"` + username + `","password":"` + password + `","padding":"` +
					strings.Repeat("x", authRequestBoundaryBytes) + `"}`
				response := authBoundaryRequest(router, endpoint, body, chunked)
				assertAuthBoundaryError(t, response, http.StatusRequestEntityTooLarge, "request body too large", password)

				if endpoint == "/api/auth/register" {
					var count int64
					if err := server.db.Model(&models.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Fatalf("oversized registration created %d user rows", count)
					}
					return
				}

				var stored models.User
				if err := server.db.First(&stored, user.ID).Error; err != nil {
					t.Fatal(err)
				}
				if !stored.LastActiveAt.Equal(previous) {
					t.Fatalf("oversized login updated last_active_at: got %s want %s", stored.LastActiveAt, previous)
				}
			})
		}
	}
}

func TestPublicAuthAcceptsTheExactBodyLimit(t *testing.T) {
	router, _ := setupTestServer(t)
	prefix := `{"username":"missinguser","password":"password8","padding":"`
	suffix := `"}`
	body := prefix + strings.Repeat("x", authRequestBoundaryBytes-len(prefix)-len(suffix)) + suffix
	if len(body) != authRequestBoundaryBytes {
		t.Fatalf("fixture size=%d, want %d", len(body), authRequestBoundaryBytes)
	}

	response := authBoundaryRequest(router, "/api/auth/login", body, false)
	assertAuthBoundaryError(t, response, http.StatusUnauthorized, "invalid username or password", "password8")
}

func TestPublicAuthRejectsMultipleJSONValuesWithoutMutation(t *testing.T) {
	t.Run("register", func(t *testing.T) {
		router, server := setupTestServer(t)
		const password = "multiple-secret-password8"
		body := `{"username":"multiuser","password":"` + password + `"}` +
			` {"username":"ignored","password":"ignored-password"}`
		response := authBoundaryRequest(router, "/api/auth/register", body, false)
		assertAuthBoundaryError(t, response, http.StatusBadRequest, "username and password are required", password)

		var count int64
		if err := server.db.Model(&models.User{}).Where("username IN ?", []string{"multiuser", "ignored"}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("multi-value registration created %d user rows", count)
		}
	})

	t.Run("login", func(t *testing.T) {
		router, server := setupTestServer(t)
		const password = "multiple-login-password8"
		previous := time.Date(2021, time.February, 3, 4, 5, 6, 0, time.UTC)
		user := createAuthBoundaryUser(t, server, "multilogin", password, previous)
		body := `{"username":"multilogin","password":"` + password + `"}` +
			` {"username":"ignored","password":"ignored-password"}`
		response := authBoundaryRequest(router, "/api/auth/login", body, false)
		assertAuthBoundaryError(t, response, http.StatusBadRequest, "username and password are required", password)

		var stored models.User
		if err := server.db.First(&stored, user.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !stored.LastActiveAt.Equal(previous) {
			t.Fatalf("multi-value login updated last_active_at: got %s want %s", stored.LastActiveAt, previous)
		}
	})
}

func TestPublicAuthAllowsTrailingWhitespace(t *testing.T) {
	router, _ := setupTestServer(t)
	body := "{\"username\":\"spaceuser\",\"password\":\"password8\"}\r\n\t  "
	response := authBoundaryRequest(router, "/api/auth/register", body, false)
	if response.Code != http.StatusOK {
		t.Fatalf("trailing JSON whitespace: status=%d: %s", response.Code, response.Body.String())
	}
}

func TestPublicAuthRejectsTrailingGarbageWithoutMutation(t *testing.T) {
	router, server := setupTestServer(t)
	const password = "garbage-secret-password8"
	response := authBoundaryRequest(
		router,
		"/api/auth/register",
		`{"username":"garbageuser","password":"`+password+`"} trailing`,
		false,
	)
	assertAuthBoundaryError(t, response, http.StatusBadRequest, "username and password are required", password)

	var count int64
	if err := server.db.Model(&models.User{}).Where("username = ?", "garbageuser").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("trailing garbage created %d user rows", count)
	}
}

func TestPublicAuthEnforcesBcryptPasswordBoundaryWithoutCredentialTruncation(t *testing.T) {
	t.Run("register-73-bytes", func(t *testing.T) {
		router, server := setupTestServer(t)
		password := strings.Repeat("p", 73)
		response := authBoundaryRequest(
			router,
			"/api/auth/register",
			`{"username":"longpassword","password":"`+password+`"}`,
			false,
		)
		assertAuthBoundaryError(t, response, http.StatusBadRequest, "password must be at most 72 bytes", password)

		var count int64
		if err := server.db.Model(&models.User{}).Where("username = ?", "longpassword").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("overlong bcrypt password created %d user rows", count)
		}
	})

	t.Run("register-and-login-72-bytes", func(t *testing.T) {
		router, _ := setupTestServer(t)
		password := strings.Repeat("p", 72)
		register := authBoundaryRequest(
			router,
			"/api/auth/register",
			`{"username":"maxpassword","password":"`+password+`"}`,
			false,
		)
		if register.Code != http.StatusOK {
			t.Fatalf("72-byte registration: status=%d: %s", register.Code, register.Body.String())
		}
		login := authBoundaryRequest(
			router,
			"/api/auth/login",
			`{"username":"maxpassword","password":"`+password+`"}`,
			false,
		)
		if login.Code != http.StatusOK {
			t.Fatalf("72-byte login: status=%d: %s", login.Code, login.Body.String())
		}
	})

	t.Run("login-does-not-truncate-73-bytes", func(t *testing.T) {
		router, server := setupTestServer(t)
		password := strings.Repeat("p", 72)
		previous := time.Date(2022, time.March, 4, 5, 6, 7, 0, time.UTC)
		user := createAuthBoundaryUser(t, server, "truncatecheck", password, previous)
		submitted := password + "x"
		response := authBoundaryRequest(
			router,
			"/api/auth/login",
			`{"username":"truncatecheck","password":"`+submitted+`"}`,
			false,
		)
		assertAuthBoundaryError(t, response, http.StatusUnauthorized, "invalid username or password", submitted)

		var stored models.User
		if err := server.db.First(&stored, user.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !stored.LastActiveAt.Equal(previous) {
			t.Fatalf("overlong login updated last_active_at: got %s want %s", stored.LastActiveAt, previous)
		}
	})
}
