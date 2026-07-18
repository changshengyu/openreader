package sync

import (
	"encoding/json"
	stdsync "sync"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu      stdsync.RWMutex
	clients map[uint]map[*Client]struct{}
}

type Client struct {
	UserID uint
	Conn   *websocket.Conn
	Send   chan []byte
	hub    *Hub
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[uint]map[*Client]struct{}),
	}
}

func (h *Hub) AddClient(userID uint, conn *websocket.Conn) *Client {
	client := &Client{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 16),
		hub:    h,
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*Client]struct{})
	}
	h.clients[userID][client] = struct{}{}
	return client
}

func (h *Hub) RemoveClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	userClients := h.clients[client.UserID]
	if userClients == nil {
		return
	}
	if _, exists := userClients[client]; exists {
		delete(userClients, client)
		close(client.Send)
	}
	if len(userClients) == 0 {
		delete(h.clients, client.UserID)
	}
}

func (h *Hub) Broadcast(userID uint, except *Client, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	backpressured := make([]*Client, 0)
	for client := range h.clients[userID] {
		if client == except {
			continue
		}
		select {
		case client.Send <- payload:
		default:
			backpressured = append(backpressured, client)
		}
	}
	h.mu.RUnlock()
	h.evictBackpressured(backpressured)
	return nil
}

func (h *Hub) BroadcastAll(except *Client, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	backpressured := make([]*Client, 0)
	for _, userClients := range h.clients {
		for client := range userClients {
			if client == except {
				continue
			}
			select {
			case client.Send <- payload:
			default:
				backpressured = append(backpressured, client)
			}
		}
	}
	h.mu.RUnlock()
	h.evictBackpressured(backpressured)
	return nil
}

func (h *Hub) evictBackpressured(clients []*Client) {
	for _, client := range clients {
		h.RemoveClient(client)
		if client.Conn != nil {
			_ = client.Conn.Close()
		}
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.RemoveClient(c)
		_ = c.Conn.Close()
	}()

	for {
		_, payload, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}

		var event struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(payload, &event); err != nil || event.Type == "" {
			continue
		}
		_ = c.hub.Broadcast(c.UserID, c, event)
	}
}

func (c *Client) WritePump() {
	for payload := range c.Send {
		if err := c.Conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}
