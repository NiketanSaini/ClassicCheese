package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"leaderboard-poc/internal/models"
	"leaderboard-poc/internal/mysqlclient"
	"leaderboard-poc/internal/rabbitmqclient"
	"leaderboard-poc/internal/redisclient"
)

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// --- POST /score/update ---

type scoreUpdateRequest struct {
	UserID     int    `json:"user_id"`
	Region     string `json:"region"`
	ScoreDelta int    `json:"score_delta"`
}

func (s *server) handleScoreUpdate(w http.ResponseWriter, r *http.Request) {
	var req scoreUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.UserID == 0 || req.Region == "" || req.ScoreDelta == 0 {
		writeError(w, http.StatusBadRequest, errRequiredFields)
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()
	scores := make(map[string]float64, len(models.PeriodTypes))

	for _, periodType := range models.PeriodTypes {
		periodKey := models.PeriodKey(periodType, now)

		newScore, err := redisclient.IncrementScore(ctx, s.rdb, req.Region, req.UserID, periodType, periodKey, req.ScoreDelta)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		scores[periodType] = newScore

		event := models.ScoreEvent{
			UserID:     req.UserID,
			Region:     req.Region,
			PeriodType: periodType,
			PeriodKey:  periodKey,
			ScoreDelta: req.ScoreDelta,
			Timestamp:  now,
		}
		s.publishScoreEvent(event)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": req.UserID,
		"region":  req.Region,
		"scores":  scores,
	})
}

// publishScoreEvent is fire-and-forget: the Redis write already succeeded
// and is what the request's correctness depends on, so a publish failure
// here is only logged, never surfaced to the caller.
func (s *server) publishScoreEvent(event models.ScoreEvent) {
	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("marshal score event for user %d: %v", event.UserID, err)
		return
	}
	if err := rabbitmqclient.Publish(s.amqpCh, s.cfg.RabbitMQExchange, body); err != nil {
		log.Printf("publish score event for user %d (%s): %v", event.UserID, event.PeriodType, err)
	}
}

// --- shared scope helpers ---

// rankedEntry is the common shape returned by /leaderboard/top, /rank and
// /around regardless of which scope produced it.
type rankedEntry struct {
	Rank   int     `json:"rank"`
	UserID int     `json:"user_id"`
	Score  float64 `json:"score"`
}

func zToEntries(z []redis.Z, startRank int) ([]rankedEntry, error) {
	entries := make([]rankedEntry, 0, len(z))
	for i, member := range z {
		userID, err := strconv.Atoi(member.Member.(string))
		if err != nil {
			return nil, err
		}
		entries = append(entries, rankedEntry{Rank: startRank + i, UserID: userID, Score: member.Score})
	}
	return entries, nil
}

// friendsLeaderboard builds a ranked list for a user's friends group (plus
// the user themself) by MGET-ing the user_score mirrors instead of
// maintaining a dedicated ZSET per friend group.
func (s *server) friendsLeaderboard(r *http.Request, userID int, periodType, periodKey string) ([]rankedEntry, error) {
	friendIDs, err := mysqlclient.FriendIDs(r.Context(), s.db, userID)
	if err != nil {
		return nil, err
	}
	ids := append([]int{userID}, friendIDs...)

	raw, err := redisclient.MGetUserScores(r.Context(), s.rdb, ids, periodType, periodKey)
	if err != nil {
		return nil, err
	}

	entries := make([]rankedEntry, 0, len(ids))
	for i, v := range raw {
		if v == nil {
			entries = append(entries, rankedEntry{UserID: ids[i], Score: 0})
			continue
		}
		score, _ := strconv.ParseFloat(v.(string), 64)
		entries = append(entries, rankedEntry{UserID: ids[i], Score: score})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Score > entries[j].Score })
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return entries, nil
}

// --- GET /leaderboard/top ---

func (s *server) handleLeaderboardTop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := q.Get("scope")
	periodType := q.Get("period_type")
	periodKey := q.Get("period_key")
	n := parseIntDefault(q.Get("n"), 10)

	switch scope {
	case "global":
		z, err := redisclient.TopN(r.Context(), s.rdb, redisclient.GlobalKey(periodType, periodKey), int64(n))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		entries, err := zToEntries(z, 1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, entries)

	case "regional":
		region := q.Get("region")
		if region == "" {
			writeError(w, http.StatusBadRequest, errMissingRegion)
			return
		}
		z, err := redisclient.TopN(r.Context(), s.rdb, redisclient.RegionalKey(region, periodType, periodKey), int64(n))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		entries, err := zToEntries(z, 1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, entries)

	case "friends":
		userID := parseIntDefault(q.Get("user_id"), 0)
		if userID == 0 {
			writeError(w, http.StatusBadRequest, errMissingUserID)
			return
		}
		entries, err := s.friendsLeaderboard(r, userID, periodType, periodKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if n < len(entries) {
			entries = entries[:n]
		}
		writeJSON(w, http.StatusOK, entries)

	default:
		writeError(w, http.StatusBadRequest, errUnknownScope)
	}
}

// --- GET /leaderboard/rank ---

func (s *server) handleLeaderboardRank(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := q.Get("scope")
	periodType := q.Get("period_type")
	periodKey := q.Get("period_key")
	userID := parseIntDefault(q.Get("user_id"), 0)
	if userID == 0 {
		writeError(w, http.StatusBadRequest, errMissingUserID)
		return
	}

	switch scope {
	case "global", "regional":
		var key string
		if scope == "global" {
			key = redisclient.GlobalKey(periodType, periodKey)
		} else {
			region := q.Get("region")
			if region == "" {
				writeError(w, http.StatusBadRequest, errMissingRegion)
				return
			}
			key = redisclient.RegionalKey(region, periodType, periodKey)
		}
		rank, score, ok, err := redisclient.Rank(r.Context(), s.rdb, key, strconv.Itoa(userID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"user_id": userID, "rank": rank, "score": score, "found": ok})

	case "friends":
		entries, err := s.friendsLeaderboard(r, userID, periodType, periodKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for _, e := range entries {
			if e.UserID == userID {
				writeJSON(w, http.StatusOK, map[string]interface{}{"user_id": userID, "rank": e.Rank, "score": e.Score, "found": true})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"user_id": userID, "found": false})

	default:
		writeError(w, http.StatusBadRequest, errUnknownScope)
	}
}

// --- GET /leaderboard/around ---

func (s *server) handleLeaderboardAround(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := q.Get("scope")
	periodType := q.Get("period_type")
	periodKey := q.Get("period_key")
	userID := parseIntDefault(q.Get("user_id"), 0)
	count := int64(parseIntDefault(q.Get("count"), 5))
	if userID == 0 {
		writeError(w, http.StatusBadRequest, errMissingUserID)
		return
	}

	switch scope {
	case "global", "regional":
		var key string
		if scope == "global" {
			key = redisclient.GlobalKey(periodType, periodKey)
		} else {
			region := q.Get("region")
			if region == "" {
				writeError(w, http.StatusBadRequest, errMissingRegion)
				return
			}
			key = redisclient.RegionalKey(region, periodType, periodKey)
		}
		z, err := redisclient.Around(r.Context(), s.rdb, key, strconv.Itoa(userID), count)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		rank, _, ok, err := redisclient.Rank(r.Context(), s.rdb, key, strconv.Itoa(userID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !ok {
			writeJSON(w, http.StatusOK, []rankedEntry{})
			return
		}
		startRank := rank - int(count)
		if startRank < 1 {
			startRank = 1
		}
		entries, err := zToEntries(z, startRank)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, entries)

	case "friends":
		entries, err := s.friendsLeaderboard(r, userID, periodType, periodKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		center := -1
		for i, e := range entries {
			if e.UserID == userID {
				center = i
				break
			}
		}
		if center == -1 {
			writeJSON(w, http.StatusOK, []rankedEntry{})
			return
		}
		start := center - int(count)
		if start < 0 {
			start = 0
		}
		end := center + int(count) + 1
		if end > len(entries) {
			end = len(entries)
		}
		writeJSON(w, http.StatusOK, entries[start:end])

	default:
		writeError(w, http.StatusBadRequest, errUnknownScope)
	}
}

// --- GET /leaderboard/history ---

func (s *server) handleLeaderboardHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := q.Get("scope")
	periodType := q.Get("period_type")
	periodKey := q.Get("period_key")
	limit := parseIntDefault(q.Get("limit"), 10)

	var region *string
	if v := q.Get("region"); v != "" {
		region = &v
	}

	rows, err := mysqlclient.LatestSnapshot(r.Context(), s.db, scope, region, periodType, periodKey, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// --- POST /admin/snapshot ---

type adminSnapshotRequest struct {
	Scope      string `json:"scope"`
	Region     string `json:"region"`
	PeriodType string `json:"period_type"`
	PeriodKey  string `json:"period_key"`
	TopN       int    `json:"top_n"`
}

func (s *server) handleAdminSnapshot(w http.ResponseWriter, r *http.Request) {
	var req adminSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.TopN <= 0 {
		req.TopN = 10
	}

	var key string
	var regionPtr *string
	switch req.Scope {
	case "global":
		key = redisclient.GlobalKey(req.PeriodType, req.PeriodKey)
	case "regional":
		if req.Region == "" {
			writeError(w, http.StatusBadRequest, errMissingRegion)
			return
		}
		key = redisclient.RegionalKey(req.Region, req.PeriodType, req.PeriodKey)
		regionPtr = &req.Region
	default:
		writeError(w, http.StatusBadRequest, errUnknownScope)
		return
	}

	z, err := redisclient.TopN(r.Context(), s.rdb, key, int64(req.TopN))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	rows := make([]models.SnapshotRow, 0, len(z))
	for i, member := range z {
		userID, err := strconv.Atoi(member.Member.(string))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		rows = append(rows, models.SnapshotRow{RankPosition: i + 1, UserID: userID, Score: int(member.Score)})
	}

	snapshotAt := time.Now().UTC()
	if err := mysqlclient.InsertSnapshot(r.Context(), s.db, req.Scope, regionPtr, req.PeriodType, req.PeriodKey, rows, snapshotAt); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"rows_written": len(rows), "snapshot_at": snapshotAt})
}

// --- GET /ws/leaderboard ---

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // POC only, no browser origin restrictions
}

func (s *server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := q.Get("scope")
	region := q.Get("region")
	periodType := q.Get("period_type")
	periodKey := q.Get("period_key")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	channel := redisclient.NotificationChannel(scope, region, periodType, periodKey)
	s.hub.Register(channel, conn)
	log.Printf("ws client connected on %s", channel)

	// Block on reads purely to detect client disconnects; this POC never
	// expects inbound WS messages.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			s.hub.Unregister(channel, conn)
			conn.Close()
			log.Printf("ws client disconnected from %s", channel)
			return
		}
	}
}

func parseIntDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}
