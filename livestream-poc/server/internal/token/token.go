// Package token mints LiveKit access tokens for hosts (publishers) and
// viewers (subscribers) joining a room named after the video_id.
package token

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/livekit/protocol/auth"
)

const tokenTTL = 6 * time.Hour

// Handler mints a JWT via LIVEKIT_API_KEY/LIVEKIT_API_SECRET. Accepts both
// GET and POST since the token spec calls it "POST /token" but the mint
// itself has no side effects — it only reads ?room=&role=.
func Handler(apiKey, apiSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		room := r.URL.Query().Get("room")
		role := r.URL.Query().Get("role")

		if room == "" {
			http.Error(w, "room is required", http.StatusBadRequest)
			return
		}
		if role != "publisher" && role != "subscriber" {
			http.Error(w, `role must be "publisher" or "subscriber"`, http.StatusBadRequest)
			return
		}

		isHost := role == "publisher"
		canPublish := isHost
		canSubscribe := true

		at := auth.NewAccessToken(apiKey, apiSecret).
			SetIdentity(uuid.NewString()).
			SetValidFor(tokenTTL).
			AddGrant(&auth.VideoGrant{
				RoomJoin:     true,
				Room:         room,
				CanPublish:   &canPublish,
				CanSubscribe: &canSubscribe,
			})

		jwt, err := at.ToJWT()
		if err != nil {
			http.Error(w, "failed to mint token", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(jwt))
	}
}
