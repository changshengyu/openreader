package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/models"
)

type replaceRuleRequestBoundaryFixture struct {
	method           string
	path             string
	limit            int64
	validBody        []byte
	wantExactStatus  int
	malformedMessage string
	assertUntouched  func(*testing.T)
}

func newReplaceRuleRequestBoundaryFixture(
	t *testing.T,
	router http.Handler,
	server *Server,
	route string,
) replaceRuleRequestBoundaryFixture {
	t.Helper()
	user := replaceRuleContractUser(t, server)
	assertNoRules := func(t *testing.T) {
		t.Helper()
		var count int64
		if err := server.db.Model(&models.ReplaceRule{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rejected request persisted %d replace rules", count)
		}
	}

	switch route {
	case "create":
		return replaceRuleRequestBoundaryFixture{
			method:           http.MethodPost,
			path:             "/api/replace-rules",
			limit:            maxReplaceRuleRequestBytes,
			validBody:        []byte(`{"name":"boundary-create","pattern":"a","scope":"*"}`),
			wantExactStatus:  http.StatusCreated,
			malformedMessage: "pattern is required",
			assertUntouched:  assertNoRules,
		}
	case "update":
		plain := false
		rule := models.ReplaceRule{
			UserID: user.ID, Name: "boundary-update", Pattern: "before", Scope: "*",
			IsRegex: &plain, Enabled: true,
		}
		if err := server.db.Create(&rule).Error; err != nil {
			t.Fatal(err)
		}
		return replaceRuleRequestBoundaryFixture{
			method:           http.MethodPut,
			path:             "/api/replace-rules/" + strconv.FormatUint(uint64(rule.ID), 10),
			limit:            maxReplaceRuleRequestBytes,
			validBody:        []byte(`{"name":"boundary-update","pattern":"after","scope":"*"}`),
			wantExactStatus:  http.StatusOK,
			malformedMessage: "invalid replace rule",
			assertUntouched: func(t *testing.T) {
				t.Helper()
				var stored models.ReplaceRule
				if err := server.db.First(&stored, rule.ID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.Pattern != "before" {
					t.Fatalf("rejected request updated the rule pattern to %q", stored.Pattern)
				}
			},
		}
	case "batch":
		return replaceRuleRequestBoundaryFixture{
			method:           http.MethodPost,
			path:             "/api/replace-rules/batch",
			limit:            maxReplaceRuleBatchRequestBytes,
			validBody:        []byte(`[{"name":"boundary-batch","pattern":"a","scope":"*"}]`),
			wantExactStatus:  http.StatusOK,
			malformedMessage: "invalid replace rules payload",
			assertUntouched:  assertNoRules,
		}
	case "batch-delete":
		plain := false
		rule := models.ReplaceRule{
			UserID: user.ID, Name: "boundary-delete", Pattern: "a", Scope: "*",
			IsRegex: &plain, Enabled: true,
		}
		if err := server.db.Create(&rule).Error; err != nil {
			t.Fatal(err)
		}
		return replaceRuleRequestBoundaryFixture{
			method:           http.MethodPost,
			path:             "/api/replace-rules/batch-delete",
			limit:            maxReplaceRuleDeleteRequestBytes,
			validBody:        []byte(`{"ids":[` + strconv.FormatUint(uint64(rule.ID), 10) + `]}`),
			wantExactStatus:  http.StatusOK,
			malformedMessage: "invalid replace rule ids",
			assertUntouched: func(t *testing.T) {
				t.Helper()
				var count int64
				if err := server.db.Model(&models.ReplaceRule{}).Where("id = ?", rule.ID).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatal("rejected request deleted the durable replace rule")
				}
			},
		}
	case "test":
		return replaceRuleRequestBoundaryFixture{
			method:           http.MethodPost,
			path:             "/api/replace-rules/test",
			limit:            maxReplaceRuleTestRequestBytes,
			validBody:        []byte(`{"pattern":"a","replacement":"b","text":"a"}`),
			wantExactStatus:  http.StatusOK,
			malformedMessage: "pattern and text are required",
			assertUntouched:  assertNoRules,
		}
	default:
		t.Fatalf("unknown replace-rule boundary route %q", route)
		return replaceRuleRequestBoundaryFixture{}
	}
}

func replaceRuleRequestBoundaryRequest(
	router http.Handler,
	fixture replaceRuleRequestBoundaryFixture,
	body []byte,
	token string,
	contentLength int64,
	requestContext context.Context,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(fixture.method, fixture.path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", token)
	request.ContentLength = contentLength
	if requestContext != nil {
		request = request.WithContext(requestContext)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func padReplaceRuleRequestBoundaryBody(t *testing.T, body []byte, size int64) []byte {
	t.Helper()
	if size < int64(len(body)) {
		t.Fatalf("cannot pad %d-byte body to %d bytes", len(body), size)
	}
	padded := make([]byte, int(size))
	copy(padded, body)
	for index := len(body); index < len(padded); index++ {
		padded[index] = ' '
	}
	return padded
}

func assertReplaceRuleRequestBoundaryError(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantStatus int,
	wantMessage string,
) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v: %s", err, response.Body.String())
	}
	if payload.Error != wantMessage {
		t.Fatalf("error = %q, want %q", payload.Error, wantMessage)
	}
}

func assertReplaceRuleRequestBoundaryNoEvent(t *testing.T, clientSend <-chan []byte) {
	t.Helper()
	select {
	case payload := <-clientSend:
		t.Fatalf("rejected request broadcast an update: %s", payload)
	default:
	}
}

func TestReplaceRuleRequestBoundaryEnforcesActualReadLimits(t *testing.T) {
	routes := []string{"create", "update", "batch", "batch-delete", "test"}
	for _, route := range routes {
		for _, mode := range []string{"declared", "chunked"} {
			t.Run(route+"/"+mode+"/exact", func(t *testing.T) {
				router, server := setupTestServer(t)
				token := authHeader(t, router)
				fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, route)
				body := padReplaceRuleRequestBoundaryBody(t, fixture.validBody, fixture.limit)
				contentLength := int64(len(body))
				if mode == "chunked" {
					contentLength = -1
				}
				response := replaceRuleRequestBoundaryRequest(
					router, fixture, body, token, contentLength, nil,
				)
				if response.Code != fixture.wantExactStatus {
					t.Fatalf("exact-limit request = %d, want %d: %s", response.Code, fixture.wantExactStatus, response.Body.String())
				}
			})

			t.Run(route+"/"+mode+"/overflow", func(t *testing.T) {
				router, server := setupTestServer(t)
				token := authHeader(t, router)
				fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, route)
				user := replaceRuleContractUser(t, server)
				client := server.hub.AddClient(user.ID, nil)
				defer server.hub.RemoveClient(client)
				body := padReplaceRuleRequestBoundaryBody(t, fixture.validBody, fixture.limit+1)
				contentLength := int64(len(body))
				if mode == "chunked" {
					contentLength = -1
				}
				response := replaceRuleRequestBoundaryRequest(
					router, fixture, body, token, contentLength, nil,
				)
				assertReplaceRuleRequestBoundaryError(
					t, response, http.StatusRequestEntityTooLarge, "request body too large",
				)
				fixture.assertUntouched(t)
				assertReplaceRuleRequestBoundaryNoEvent(t, client.Send)
			})
		}
	}
}

func TestReplaceRuleRequestBoundaryRejectsMalformedSingleDocuments(t *testing.T) {
	routes := []string{"create", "update", "batch", "batch-delete", "test"}
	for _, route := range routes {
		for _, malformed := range []string{"second-json", "trailing-garbage", "invalid-utf8", "null", "wrong-shape"} {
			t.Run(route+"/"+malformed, func(t *testing.T) {
				router, server := setupTestServer(t)
				token := authHeader(t, router)
				fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, route)
				user := replaceRuleContractUser(t, server)
				client := server.hub.AddClient(user.ID, nil)
				defer server.hub.RemoveClient(client)

				var body []byte
				switch malformed {
				case "second-json":
					body = append(append([]byte{}, fixture.validBody...), []byte(` {}`)...)
				case "trailing-garbage":
					body = append(append([]byte{}, fixture.validBody...), []byte(` trailing`)...)
				case "invalid-utf8":
					if route == "batch" {
						body = []byte(`[{"name":"boundary-`)
						body = append(body, 0xff)
						body = append(body, []byte(`","pattern":"a","scope":"*"}]`)...)
					} else if route == "batch-delete" {
						body = append([]byte(`{"ids":[1],"ignored":"`), 0xff)
						body = append(body, []byte(`"}`)...)
					} else if route == "test" {
						body = append([]byte(`{"pattern":"a","text":"`), 0xff)
						body = append(body, []byte(`"}`)...)
					} else {
						body = append([]byte(`{"name":"boundary-`), 0xff)
						body = append(body, []byte(`","pattern":"a","scope":"*"}`)...)
					}
				case "null":
					body = []byte(`null`)
				case "wrong-shape":
					if route == "batch" {
						body = []byte(`{"name":"wrong-shape"}`)
					} else {
						body = []byte(`[]`)
					}
				}

				response := replaceRuleRequestBoundaryRequest(
					router, fixture, body, token, int64(len(body)), nil,
				)
				assertReplaceRuleRequestBoundaryError(
					t, response, http.StatusBadRequest, fixture.malformedMessage,
				)
				fixture.assertUntouched(t)
				assertReplaceRuleRequestBoundaryNoEvent(t, client.Send)
			})
		}
	}
}

func TestReplaceRuleRequestBoundaryPreservesAuthAndTargetPriority(t *testing.T) {
	t.Run("authentication before body", func(t *testing.T) {
		router, server := setupTestServer(t)
		_ = authHeader(t, router)
		fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, "create")
		body := padReplaceRuleRequestBoundaryBody(t, fixture.validBody, fixture.limit+1)
		response := replaceRuleRequestBoundaryRequest(router, fixture, body, "", int64(len(body)), nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated overflow = %d, want 401: %s", response.Code, response.Body.String())
		}
	})

	t.Run("missing update target before body", func(t *testing.T) {
		router, server := setupTestServer(t)
		token := authHeader(t, router)
		fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, "update")
		fixture.path = "/api/replace-rules/99999999"
		body := padReplaceRuleRequestBoundaryBody(t, fixture.validBody, fixture.limit+1)
		response := replaceRuleRequestBoundaryRequest(
			router, fixture, body, token, int64(len(body)), nil,
		)
		if response.Code != http.StatusNotFound {
			t.Fatalf("missing target with overflow = %d, want 404: %s", response.Code, response.Body.String())
		}
	})
}

func TestReplaceRuleRequestBoundaryEnforcesRawCardinality(t *testing.T) {
	t.Run("batch rows", func(t *testing.T) {
		router, server := setupTestServer(t)
		token := authHeader(t, router)
		fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, "batch")
		rows := make([]replaceRuleRequest, maxReplaceRuleBatchItems)
		body, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		response := replaceRuleRequestBoundaryRequest(router, fixture, body, token, int64(len(body)), nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"skipped":2000`) {
			t.Fatalf("exact raw batch cardinality = %d: %s", response.Code, response.Body.String())
		}

		rows = append(rows, replaceRuleRequest{})
		body, err = json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		response = replaceRuleRequestBoundaryRequest(router, fixture, body, token, int64(len(body)), nil)
		assertReplaceRuleRequestBoundaryError(t, response, http.StatusBadRequest, fixture.malformedMessage)
		fixture.assertUntouched(t)
	})

	t.Run("batch delete ids before dedupe", func(t *testing.T) {
		router, server := setupTestServer(t)
		token := authHeader(t, router)
		fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, "batch-delete")
		var request replaceRuleBatchDeleteRequest
		if err := json.Unmarshal(fixture.validBody, &request); err != nil {
			t.Fatal(err)
		}
		targetID := request.IDs[0]
		ids := make([]uint, maxReplaceRuleBatchItems)
		for index := range ids {
			ids[index] = targetID
		}
		body, err := json.Marshal(replaceRuleBatchDeleteRequest{IDs: ids})
		if err != nil {
			t.Fatal(err)
		}
		response := replaceRuleRequestBoundaryRequest(router, fixture, body, token, int64(len(body)), nil)
		if response.Code != http.StatusOK {
			t.Fatalf("exact raw delete cardinality = %d: %s", response.Code, response.Body.String())
		}

		fixture = newReplaceRuleRequestBoundaryFixture(t, router, server, "batch-delete")
		if err := json.Unmarshal(fixture.validBody, &request); err != nil {
			t.Fatal(err)
		}
		targetID = request.IDs[0]
		ids = make([]uint, maxReplaceRuleBatchItems+1)
		for index := range ids {
			ids[index] = targetID
		}
		body, err = json.Marshal(replaceRuleBatchDeleteRequest{IDs: ids})
		if err != nil {
			t.Fatal(err)
		}
		response = replaceRuleRequestBoundaryRequest(router, fixture, body, token, int64(len(body)), nil)
		assertReplaceRuleRequestBoundaryError(t, response, http.StatusBadRequest, "too many replace rule ids")
		fixture.assertUntouched(t)
	})
}

func TestReplaceRuleRequestBoundaryStopsCancelledWork(t *testing.T) {
	for _, route := range []string{"create", "update", "batch", "batch-delete", "test"} {
		t.Run(route, func(t *testing.T) {
			router, server := setupTestServer(t)
			token := authHeader(t, router)
			fixture := newReplaceRuleRequestBoundaryFixture(t, router, server, route)
			user := replaceRuleContractUser(t, server)
			client := server.hub.AddClient(user.ID, nil)
			defer server.hub.RemoveClient(client)
			requestContext, cancel := context.WithCancel(context.Background())
			cancel()

			response := replaceRuleRequestBoundaryRequest(
				router, fixture, fixture.validBody, token, int64(len(fixture.validBody)), requestContext,
			)
			fixture.assertUntouched(t)
			assertReplaceRuleRequestBoundaryNoEvent(t, client.Send)
			if route == "test" && strings.Contains(response.Body.String(), `"output"`) {
				t.Fatalf("cancelled test request executed the replace engine: %s", response.Body.String())
			}
		})
	}
}
