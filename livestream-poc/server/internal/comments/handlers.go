package comments

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const keepAliveInterval = 15 * time.Second

type postCommentRequest struct {
	VideoID string `json:"video_id"`
	Author  string `json:"author"`
	Text    string `json:"text"`
}

// PostComment handles POST /comments: { video_id, author, text }.
func PostComment(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req postCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		comment, count, err := svc.AddComment(req.VideoID, req.Author, req.Text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Comment
			Count int64 `json:"count"`
		}{comment, count})
	}
}

// Stream handles GET /stream?room=: an SSE feed of "comment" and "count"
// events for that room, plus a periodic keep-alive comment line.
func Stream(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		room := r.URL.Query().Get("room")
		if room == "" {
			http.Error(w, "room is required", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // don't let nginx buffer the stream
		w.WriteHeader(http.StatusOK)

		sub := svc.Subscribe(room)
		defer svc.Unsubscribe(room, sub)

		// Prime the client with the room's current count so a fresh page
		// load doesn't sit at 0 until the next comment arrives.
		writeEvent(w, Event{Name: "count", Data: []byte(strconv.FormatInt(svc.Count(room), 10))})
		flusher.Flush()

		ticker := time.NewTicker(keepAliveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-sub.kill:
				return
			case ev := <-sub.events:
				writeEvent(w, ev)
				flusher.Flush()
			case <-ticker.C:
				fmt.Fprint(w, ": keep-alive\n\n")
				flusher.Flush()
			}
		}
	}
}

func writeEvent(w http.ResponseWriter, ev Event) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Name, ev.Data)
}
