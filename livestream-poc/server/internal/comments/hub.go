package comments

import (
	"sync"

	"github.com/google/uuid"
)

// subscriberBufferSize caps how many un-flushed events an SSE connection can
// queue before it's considered slow and dropped.
const subscriberBufferSize = 32

// subscriber is one open SSE connection's mailbox.
type subscriber struct {
	id     string
	events chan Event
	kill   chan struct{}
	once   sync.Once
}

// drop signals the owning SSE handler to close the connection. Safe to call
// concurrently and more than once.
func (s *subscriber) drop() {
	s.once.Do(func() { close(s.kill) })
}

// Hub is the in-memory registry of every room's live SSE subscribers:
// map[video_id]map[connID]*subscriber, guarded by a RWMutex.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[string]*subscriber
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[string]map[string]*subscriber)}
}

// Subscribe registers a new connection for room and returns its mailbox.
// Call Unsubscribe with the returned id when the connection closes.
func (h *Hub) Subscribe(room string) *subscriber {
	s := &subscriber{
		id:     uuid.NewString(),
		events: make(chan Event, subscriberBufferSize),
		kill:   make(chan struct{}),
	}

	h.mu.Lock()
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[string]*subscriber)
	}
	h.rooms[room][s.id] = s
	h.mu.Unlock()

	return s
}

func (h *Hub) Unsubscribe(room string, s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.rooms[room]; ok {
		delete(conns, s.id)
		if len(conns) == 0 {
			delete(h.rooms, room)
		}
	}
}

// Publish fans ev out to every subscriber of room. A subscriber whose
// buffer is full is dropped rather than blocking the broadcaster — its SSE
// handler notices the kill signal and the client reconnects.
func (h *Hub) Publish(room string, ev Event) {
	h.mu.RLock()
	conns := h.rooms[room]
	targets := make([]*subscriber, 0, len(conns))
	for _, s := range conns {
		targets = append(targets, s)
	}
	h.mu.RUnlock()

	for _, s := range targets {
		select {
		case s.events <- ev:
		default:
			s.drop()
		}
	}
}
