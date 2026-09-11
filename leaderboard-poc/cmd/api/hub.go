package main

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// Hub fans Redis Pub/Sub notifications out to WebSocket clients grouped by
// the notification channel they registered for. Each channel is subscribed
// to Redis exactly once no matter how many WS clients are watching it.
type Hub struct {
	rdb *redis.Client

	mu          sync.Mutex
	conns       map[string]map[*websocket.Conn]struct{}
	subscribing map[string]bool
}

func NewHub(rdb *redis.Client) *Hub {
	return &Hub{
		rdb:         rdb,
		conns:       make(map[string]map[*websocket.Conn]struct{}),
		subscribing: make(map[string]bool),
	}
}

// Register adds conn to the set of clients watching channel and, the first
// time anyone watches this channel, starts the shared Redis subscription
// that feeds all of them.
func (h *Hub) Register(channel string, conn *websocket.Conn) {
	h.mu.Lock()
	if h.conns[channel] == nil {
		h.conns[channel] = make(map[*websocket.Conn]struct{})
	}
	h.conns[channel][conn] = struct{}{}
	needsSubscription := !h.subscribing[channel]
	if needsSubscription {
		h.subscribing[channel] = true
	}
	h.mu.Unlock()

	if needsSubscription {
		go h.subscribeLoop(channel)
	}
}

// Unregister removes conn from channel's watcher set.
func (h *Hub) Unregister(channel string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.conns[channel]; ok {
		delete(set, conn)
	}
}

func (h *Hub) subscribeLoop(channel string) {
	ctx := context.Background()
	sub := h.rdb.Subscribe(ctx, channel)
	defer sub.Close()

	log.Printf("hub: subscribed to redis channel %s", channel)
	for msg := range sub.Channel() {
		h.broadcast(channel, []byte(msg.Payload))
	}
}

func (h *Hub) broadcast(channel string, payload []byte) {
	h.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.conns[channel]))
	for c := range h.conns[channel] {
		conns = append(conns, c)
	}
	h.mu.Unlock()

	for _, c := range conns {
		if err := c.WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Printf("hub: dropping ws client on %s: %v", channel, err)
			h.Unregister(channel, c)
			c.Close()
		}
	}
}
