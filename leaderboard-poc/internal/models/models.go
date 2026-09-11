// Package models holds the wire/data types shared by every service, plus
// the period-key helpers so "what's today's daily/weekly/monthly key" is
// computed identically everywhere it matters.
package models

import (
	"fmt"
	"time"
)

// PeriodTypes are the only three period types this POC supports, in the
// order cmd/api fans update messages out for.
var PeriodTypes = []string{"daily", "weekly", "monthly"}

// ScoreEvent is both the RabbitMQ message payload and the row shape for the
// MySQL score_events table.
type ScoreEvent struct {
	UserID     int       `json:"user_id"`
	Region     string    `json:"region"`
	PeriodType string    `json:"period_type"`
	PeriodKey  string    `json:"period_key"`
	ScoreDelta int       `json:"score_delta"`
	Timestamp  time.Time `json:"timestamp"`
}

// Notification is published to a Redis Pub/Sub channel by cmd/notifier and
// forwarded verbatim to WebSocket clients by cmd/api.
type Notification struct {
	UserID       int       `json:"user_id"`
	Scope        string    `json:"scope"`
	Region       string    `json:"region"`
	PeriodType   string    `json:"period_type"`
	PeriodKey    string    `json:"period_key"`
	Rank         int       `json:"rank"`          // 1-indexed
	PreviousRank int       `json:"previous_rank"` // 1-indexed, 0 = unranked/unknown
	Score        float64   `json:"score"`
	Reason       string    `json:"reason"` // "top10" or "rank_jump"
	Timestamp    time.Time `json:"timestamp"`
}

// SnapshotRow mirrors one row of leaderboard_snapshots.
type SnapshotRow struct {
	RankPosition int       `json:"rank_position"`
	UserID       int       `json:"user_id"`
	Score        int       `json:"score"`
	SnapshotAt   time.Time `json:"snapshot_at"`
}

// DailyKey returns the period key for the daily leaderboard containing t,
// e.g. "2026-09-07".
func DailyKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// WeeklyKey returns the ISO-8601 week key containing t, e.g. "2026-W36".
func WeeklyKey(t time.Time) string {
	year, week := t.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

// MonthlyKey returns the period key for the month containing t, e.g. "2026-09".
func MonthlyKey(t time.Time) string {
	return t.UTC().Format("2006-01")
}

// PeriodKey dispatches to the right *Key helper for a given period type.
func PeriodKey(periodType string, t time.Time) string {
	switch periodType {
	case "daily":
		return DailyKey(t)
	case "weekly":
		return WeeklyKey(t)
	case "monthly":
		return MonthlyKey(t)
	default:
		return ""
	}
}
