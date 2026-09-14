package comments

import "time"

// Comment is one chat message posted against a room (video_id).
type Comment struct {
	ID     int64     `json:"id"`
	Author string    `json:"author"`
	Text   string    `json:"text"`
	Ts     time.Time `json:"ts"`
}

// Event is one named SSE payload ("comment" or "count") fanned out to every
// subscriber of a room.
type Event struct {
	Name string
	Data []byte
}
