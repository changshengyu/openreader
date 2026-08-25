package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"openreader/backend/models"
)

const websocketContractReadWait = 750 * time.Millisecond

func syncWebSocketURL(serverURL, token string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/ws/sync?token=" + url.QueryEscape(token)
}

func bearerToken(authorization string) string {
	return strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
}

func dialSyncWebSocket(serverURL, token, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	return websocket.DefaultDialer.Dial(syncWebSocketURL(serverURL, token), header)
}

func requireHandshakeStatus(t *testing.T, response *http.Response, err error, status int) {
	t.Helper()
	if err == nil {
		t.Fatalf("handshake unexpectedly succeeded; want HTTP %d", status)
	}
	if response == nil || response.StatusCode != status {
		got := 0
		if response != nil {
			got = response.StatusCode
		}
		t.Fatalf("handshake status = %d, want %d: %v", got, status, err)
	}
}

func TestSyncWebSocketHandshakeRequiresSameOriginAndExistingUser(t *testing.T) {
	router, server := setupTestServer(t)
	authorization := authHeader(t, router)
	token := bearerToken(authorization)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	t.Run("same origin", func(t *testing.T) {
		connection, response, err := dialSyncWebSocket(httpServer.URL, token, httpServer.URL)
		if err != nil {
			t.Fatalf("same-origin handshake: status=%v err=%v", responseStatus(response), err)
		}
		_ = connection.Close()
	})

	t.Run("origin-less diagnostic client", func(t *testing.T) {
		connection, response, err := dialSyncWebSocket(httpServer.URL, token, "")
		if err != nil {
			t.Fatalf("origin-less handshake: status=%v err=%v", responseStatus(response), err)
		}
		_ = connection.Close()
	})

	t.Run("cross origin", func(t *testing.T) {
		connection, response, err := dialSyncWebSocket(httpServer.URL, token, "https://attacker.invalid")
		if connection != nil {
			_ = connection.Close()
		}
		requireHandshakeStatus(t, response, err, http.StatusForbidden)
	})

	t.Run("invalid token", func(t *testing.T) {
		connection, response, err := dialSyncWebSocket(httpServer.URL, "not-a-token", "")
		if connection != nil {
			_ = connection.Close()
		}
		requireHandshakeStatus(t, response, err, http.StatusUnauthorized)
	})

	t.Run("missing token", func(t *testing.T) {
		connection, response, err := dialSyncWebSocket(httpServer.URL, "", "")
		if connection != nil {
			_ = connection.Close()
		}
		requireHandshakeStatus(t, response, err, http.StatusUnauthorized)
	})

	t.Run("deleted user", func(t *testing.T) {
		deletedAuthorization := registerStorageTestUser(t, router, "wsdeleteduser")
		var deleted models.User
		if err := server.db.Where("username = ?", "wsdeleteduser").First(&deleted).Error; err != nil {
			t.Fatalf("load deleted-user fixture: %v", err)
		}
		if err := server.db.Delete(&deleted).Error; err != nil {
			t.Fatalf("delete user fixture: %v", err)
		}
		connection, response, err := dialSyncWebSocket(httpServer.URL, bearerToken(deletedAuthorization), "")
		if connection != nil {
			_ = connection.Close()
		}
		requireHandshakeStatus(t, response, err, http.StatusUnauthorized)
	})
}

func responseStatus(response *http.Response) any {
	if response == nil {
		return nil
	}
	return response.StatusCode
}

type websocketReadResult struct {
	payload []byte
	err     error
}

func readWebSocket(connection *websocket.Conn) <-chan websocketReadResult {
	result := make(chan websocketReadResult, 1)
	go func() {
		_, payload, err := connection.ReadMessage()
		result <- websocketReadResult{payload: payload, err: err}
	}()
	return result
}

func TestSyncWebSocketRejectsClientApplicationEventsWithoutRelaying(t *testing.T) {
	router, server := setupTestServer(t)
	ownerAuthorization := authHeader(t, router)
	otherAuthorization := registerStorageTestUser(t, router, "wsotheruser")
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatalf("load owner: %v", err)
	}

	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	ownerA, _, err := dialSyncWebSocket(httpServer.URL, bearerToken(ownerAuthorization), "")
	if err != nil {
		t.Fatalf("connect owner A: %v", err)
	}
	defer ownerA.Close()
	ownerB, _, err := dialSyncWebSocket(httpServer.URL, bearerToken(ownerAuthorization), "")
	if err != nil {
		t.Fatalf("connect owner B: %v", err)
	}
	defer ownerB.Close()
	other, _, err := dialSyncWebSocket(httpServer.URL, bearerToken(otherAuthorization), "")
	if err != nil {
		t.Fatalf("connect other user: %v", err)
	}
	defer other.Close()

	ownerBRead := readWebSocket(ownerB)
	otherRead := readWebSocket(other)
	if err := ownerA.WriteJSON(map[string]any{
		"type":    "bookshelf_delete",
		"payload": map[string]any{"id": 999999},
	}); err != nil {
		t.Fatalf("write forged client event: %v", err)
	}

	select {
	case result := <-ownerBRead:
		if result.err == nil {
			t.Fatalf("client application event was relayed to peer: %s", result.payload)
		}
		t.Fatalf("healthy peer closed while rejecting another client: %v", result.err)
	case <-time.After(200 * time.Millisecond):
	}

	legitimate := map[string]any{
		"type":    "bookshelf_update",
		"payload": map[string]any{"id": 42},
	}
	if err := server.hub.Broadcast(owner.ID, nil, legitimate); err != nil {
		t.Fatalf("broadcast legitimate server event: %v", err)
	}
	select {
	case result := <-ownerBRead:
		if result.err != nil {
			t.Fatalf("owner peer did not receive server event: %v", result.err)
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(result.payload, &message); err != nil || message.Type != "bookshelf_update" {
			t.Fatalf("owner peer event = %s, err=%v", result.payload, err)
		}
	case <-time.After(websocketContractReadWait):
		t.Fatal("owner peer timed out waiting for legitimate server event")
	}

	select {
	case result := <-otherRead:
		if result.err == nil {
			t.Fatalf("other user received owner event: %s", result.payload)
		}
		t.Fatalf("other user's healthy connection closed: %v", result.err)
	case <-time.After(200 * time.Millisecond):
	}

	_ = ownerA.SetReadDeadline(time.Now().Add(websocketContractReadWait))
	_, _, err = ownerA.ReadMessage()
	var closeError *websocket.CloseError
	if !strings.Contains(errorText(err), "policy violation") &&
		!(errors.As(err, &closeError) && closeError.Code == websocket.ClosePolicyViolation) {
		t.Fatalf("forged sender close = %v, want policy violation", err)
	}
}

func TestSyncWebSocketBoundsOversizedClientApplicationMessage(t *testing.T) {
	router, _ := setupTestServer(t)
	authorization := authHeader(t, router)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	connection, _, err := dialSyncWebSocket(httpServer.URL, bearerToken(authorization), "")
	if err != nil {
		t.Fatalf("connect websocket: %v", err)
	}
	defer connection.Close()
	if err := connection.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 2048))); err != nil {
		t.Fatalf("write oversized client message: %v", err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(websocketContractReadWait))
	_, _, err = connection.ReadMessage()
	var closeError *websocket.CloseError
	if !errors.As(err, &closeError) || closeError.Code != websocket.CloseMessageTooBig {
		t.Fatalf("oversized sender close = %v, want message-too-big", err)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.ToLower(err.Error())
}

type usersUpdateMessage struct {
	Type    string `json:"type"`
	Payload struct {
		Kind    string `json:"kind"`
		UserIDs []uint `json:"userIds"`
	} `json:"payload"`
}

func readUsersUpdate(t *testing.T, connection *websocket.Conn) usersUpdateMessage {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(websocketContractReadWait))
	var message usersUpdateMessage
	if err := connection.ReadJSON(&message); err != nil {
		t.Fatalf("read users_update: %v", err)
	}
	if message.Type != "users_update" {
		t.Fatalf("event type = %q, want users_update", message.Type)
	}
	sort.Slice(message.Payload.UserIDs, func(i, j int) bool {
		return message.Payload.UserIDs[i] < message.Payload.UserIDs[j]
	})
	return message
}

func TestSyncHubCloseIsIdempotentAndRejectsNewLifetime(t *testing.T) {
	router, server := setupTestServer(t)
	token := bearerToken(authHeader(t, router))
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	connection, response, err := dialSyncWebSocket(httpServer.URL, token, "")
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		t.Fatalf("connect websocket: %v", err)
	}
	defer connection.Close()

	server.hub.Close()
	server.hub.Close()
	_ = connection.SetReadDeadline(time.Now().Add(websocketContractReadWait))
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("hub close left an existing websocket connected")
	}

	late, response, err := dialSyncWebSocket(httpServer.URL, token, "")
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err == nil {
		defer late.Close()
		_ = late.SetReadDeadline(time.Now().Add(websocketContractReadWait))
		if _, _, err := late.ReadMessage(); err == nil {
			t.Fatal("closed hub accepted a new websocket lifetime")
		}
	}
}

func TestUsersUpdateTargetsAdministratorsAndAffectedUsersOnly(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuthorization := authHeader(t, router)
	targetAAuthorization := registerStorageTestUser(t, router, "wstargetalpha")
	targetBAuthorization := registerStorageTestUser(t, router, "wstargetbravo")
	unrelatedAuthorization := registerStorageTestUser(t, router, "wsunrelated")

	users := map[string]models.User{}
	for _, username := range []string{"testuser", "wstargetalpha", "wstargetbravo", "wsunrelated"} {
		var user models.User
		if err := server.db.Where("username = ?", username).First(&user).Error; err != nil {
			t.Fatalf("load %s: %v", username, err)
		}
		users[username] = user
	}

	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	connect := func(authorization string) *websocket.Conn {
		connection, _, err := dialSyncWebSocket(httpServer.URL, bearerToken(authorization), "")
		if err != nil {
			t.Fatalf("connect websocket: %v", err)
		}
		return connection
	}
	adminConnection := connect(adminAuthorization)
	defer adminConnection.Close()
	targetAConnection := connect(targetAAuthorization)
	defer targetAConnection.Close()
	targetBConnection := connect(targetBAuthorization)
	defer targetBConnection.Close()
	unrelatedConnection := connect(unrelatedAuthorization)
	defer unrelatedConnection.Close()

	targetAID := users["wstargetalpha"].ID
	targetBID := users["wstargetbravo"].ID
	body := `{"ids":[` + strconv.FormatUint(uint64(targetAID), 10) + `,` + strconv.FormatUint(uint64(targetBID), 10) + `]}`
	request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/admin/users/batch-delete", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create batch-delete request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", adminAuthorization)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("batch-delete users: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("batch-delete status = %d", response.StatusCode)
	}

	adminEvent := readUsersUpdate(t, adminConnection)
	if got, want := adminEvent.Payload.UserIDs, []uint{targetAID, targetBID}; !equalUserIDs(got, want) {
		t.Fatalf("admin userIds = %v, want %v", got, want)
	}
	targetAEvent := readUsersUpdate(t, targetAConnection)
	if got, want := targetAEvent.Payload.UserIDs, []uint{targetAID}; !equalUserIDs(got, want) {
		t.Fatalf("target A userIds = %v, want self-only %v", got, want)
	}
	targetBEvent := readUsersUpdate(t, targetBConnection)
	if got, want := targetBEvent.Payload.UserIDs, []uint{targetBID}; !equalUserIDs(got, want) {
		t.Fatalf("target B userIds = %v, want self-only %v", got, want)
	}

	_ = unrelatedConnection.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	if _, payload, err := unrelatedConnection.ReadMessage(); err == nil {
		t.Fatalf("unrelated user received users_update: %s", payload)
	}
}

func equalUserIDs(left, right []uint) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
