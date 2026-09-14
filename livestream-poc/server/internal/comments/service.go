package comments

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxAuthorLen = 64
	maxTextLen   = 500
)

// Service owns per-room comment id sequences and live counters on top of a
// Hub, and validates/normalizes input before it ever reaches a subscriber.
type Service struct {
	hub *Hub

	mu       sync.Mutex
	seqs     map[string]*atomic.Int64
	counters map[string]*atomic.Int64
}

func NewService(hub *Hub) *Service {
	return &Service{
		hub:      hub,
		seqs:     make(map[string]*atomic.Int64),
		counters: make(map[string]*atomic.Int64),
	}
}

func (s *Service) counterFor(m map[string]*atomic.Int64, room string) *atomic.Int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := m[room]
	if !ok {
		c = &atomic.Int64{}
		m[room] = c
	}
	return c
}

// AddComment validates author/text, assigns the next id for room, then
// broadcasts both a "comment" event and the room's updated "count".
func (s *Service) AddComment(room, author, text string) (Comment, int64, error) {
	room = strings.TrimSpace(room)
	author = strings.TrimSpace(author)
	text = strings.TrimSpace(text)

	if room == "" {
		return Comment{}, 0, fmt.Errorf("video_id is required")
	}
	if author == "" {
		return Comment{}, 0, fmt.Errorf("author is required")
	}
	if text == "" {
		return Comment{}, 0, fmt.Errorf("text is required")
	}
	if len(author) > maxAuthorLen {
		return Comment{}, 0, fmt.Errorf("author must be at most %d characters", maxAuthorLen)
	}
	if len(text) > maxTextLen {
		return Comment{}, 0, fmt.Errorf("text must be at most %d characters", maxTextLen)
	}

	id := s.counterFor(s.seqs, room).Add(1)
	c := Comment{ID: id, Author: author, Text: text, Ts: time.Now().UTC()}

	if data, err := json.Marshal(c); err == nil {
		s.hub.Publish(room, Event{Name: "comment", Data: data})
	}

	count := s.counterFor(s.counters, room).Add(1)
	s.hub.Publish(room, Event{Name: "count", Data: []byte(strconv.FormatInt(count, 10))})

	return c, count, nil
}

// Count returns the current comment count for room without incrementing it,
// e.g. so a newly-connected SSE client's first "count" event isn't 0.
func (s *Service) Count(room string) int64 {
	return s.counterFor(s.counters, room).Load()
}

func (s *Service) Subscribe(room string) *subscriber {
	return s.hub.Subscribe(room)
}

func (s *Service) Unsubscribe(room string, sub *subscriber) {
	s.hub.Unsubscribe(room, sub)
}
