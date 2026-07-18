package sync

import (
	"testing"
	"time"
)

func addBufferedTestClient(hub *Hub, userID uint, capacity int) *Client {
	client := &Client{
		UserID: userID,
		Send:   make(chan []byte, capacity),
		hub:    hub,
	}
	hub.mu.Lock()
	if hub.clients[userID] == nil {
		hub.clients[userID] = make(map[*Client]struct{})
	}
	hub.clients[userID][client] = struct{}{}
	hub.mu.Unlock()
	return client
}

func hubContainsClient(hub *Hub, client *Client) bool {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	_, ok := hub.clients[client.UserID][client]
	return ok
}

func TestBroadcastEvictsBackpressuredClientWithoutStarvingHealthyPeer(t *testing.T) {
	hub := NewHub()
	slow := addBufferedTestClient(hub, 1, 1)
	healthy := addBufferedTestClient(hub, 1, 2)
	otherUser := addBufferedTestClient(hub, 2, 1)
	slow.Send <- []byte(`{"type":"queued"}`)

	if err := hub.Broadcast(1, nil, map[string]any{"type": "bookshelf_update"}); err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if hubContainsClient(hub, slow) {
		t.Fatal("slow client remained connected after a state event could not be queued")
	}
	select {
	case <-healthy.Send:
	case <-time.After(time.Second):
		t.Fatal("healthy same-user client did not receive the event")
	}
	select {
	case <-otherUser.Send:
		t.Fatal("another user's client received a scoped event")
	default:
	}

	<-slow.Send // drain the event that filled the queue before eviction
	if _, open := <-slow.Send; open {
		t.Fatal("evicted slow client's send queue remained open")
	}
}

func TestBroadcastAllEvictsOnlyBackpressuredClients(t *testing.T) {
	hub := NewHub()
	slow := addBufferedTestClient(hub, 1, 1)
	healthy := addBufferedTestClient(hub, 2, 1)
	slow.Send <- []byte(`{"type":"queued"}`)

	if err := hub.BroadcastAll(nil, map[string]any{"type": "users_update"}); err != nil {
		t.Fatalf("broadcast all: %v", err)
	}
	if hubContainsClient(hub, slow) {
		t.Fatal("slow client remained connected after BroadcastAll backpressure")
	}
	if !hubContainsClient(hub, healthy) {
		t.Fatal("healthy client was evicted")
	}
	select {
	case <-healthy.Send:
	case <-time.After(time.Second):
		t.Fatal("healthy client did not receive BroadcastAll event")
	}
}
