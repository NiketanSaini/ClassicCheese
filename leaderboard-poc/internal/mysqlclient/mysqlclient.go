// Package mysqlclient holds the explicit SQL this POC needs against the
// leaderboard schema. No ORM — every query is spelled out so the mapping
// from "what the app does" to "what SQL runs" stays obvious.
package mysqlclient

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"leaderboard-poc/internal/models"
)

func New(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening mysql connection: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging mysql at startup: %w", err)
	}
	return db, nil
}

// InsertScoreEvent records one durable copy of a score update. This is the
// system's actual replayable source of truth — RabbitMQ is only transport.
func InsertScoreEvent(ctx context.Context, db *sql.DB, ev models.ScoreEvent) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO score_events (user_id, region, period_type, period_key, score_delta, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, ev.UserID, ev.Region, ev.PeriodType, ev.PeriodKey, ev.ScoreDelta, ev.Timestamp)
	if err != nil {
		return fmt.Errorf("inserting score_event for user %d: %w", ev.UserID, err)
	}
	return nil
}

// InsertSnapshot writes one leaderboard_snapshots row per ranked entry,
// all stamped with the same snapshotAt so they read back together as one
// snapshot.
func InsertSnapshot(ctx context.Context, db *sql.DB, scope string, region *string, periodType, periodKey string, rows []models.SnapshotRow, snapshotAt time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning snapshot transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO leaderboard_snapshots (scope, region, period_type, period_key, rank_position, user_id, score, snapshot_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing snapshot insert: %w", err)
	}
	defer stmt.Close()

	for _, row := range rows {
		if _, err := stmt.ExecContext(ctx, scope, region, periodType, periodKey, row.RankPosition, row.UserID, row.Score, snapshotAt); err != nil {
			return fmt.Errorf("inserting snapshot row for user %d: %w", row.UserID, err)
		}
	}
	return tx.Commit()
}

// LatestSnapshot returns the most recent snapshot's rows for a given
// (scope, region, period_type, period_key), ordered by rank.
func LatestSnapshot(ctx context.Context, db *sql.DB, scope string, region *string, periodType, periodKey string, limit int) ([]models.SnapshotRow, error) {
	if limit <= 0 {
		limit = 10
	}

	var maxSnapshotAt sql.NullTime
	var err error
	if region == nil {
		err = db.QueryRowContext(ctx, `
			SELECT MAX(snapshot_at) FROM leaderboard_snapshots
			WHERE scope = ? AND region IS NULL AND period_type = ? AND period_key = ?
		`, scope, periodType, periodKey).Scan(&maxSnapshotAt)
	} else {
		err = db.QueryRowContext(ctx, `
			SELECT MAX(snapshot_at) FROM leaderboard_snapshots
			WHERE scope = ? AND region = ? AND period_type = ? AND period_key = ?
		`, scope, *region, periodType, periodKey).Scan(&maxSnapshotAt)
	}
	if err != nil {
		return nil, fmt.Errorf("finding latest snapshot timestamp: %w", err)
	}
	if !maxSnapshotAt.Valid {
		return nil, nil
	}

	var rows *sql.Rows
	if region == nil {
		rows, err = db.QueryContext(ctx, `
			SELECT rank_position, user_id, score, snapshot_at FROM leaderboard_snapshots
			WHERE scope = ? AND region IS NULL AND period_type = ? AND period_key = ? AND snapshot_at = ?
			ORDER BY rank_position ASC LIMIT ?
		`, scope, periodType, periodKey, maxSnapshotAt.Time, limit)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT rank_position, user_id, score, snapshot_at FROM leaderboard_snapshots
			WHERE scope = ? AND region = ? AND period_type = ? AND period_key = ? AND snapshot_at = ?
			ORDER BY rank_position ASC LIMIT ?
		`, scope, *region, periodType, periodKey, maxSnapshotAt.Time, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("querying snapshot rows: %w", err)
	}
	defer rows.Close()

	var result []models.SnapshotRow
	for rows.Next() {
		var row models.SnapshotRow
		if err := rows.Scan(&row.RankPosition, &row.UserID, &row.Score, &row.SnapshotAt); err != nil {
			return nil, fmt.Errorf("scanning snapshot row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// FriendIDs returns the friend_id list for a user, used by the
// friends-scope leaderboard endpoints.
func FriendIDs(ctx context.Context, db *sql.DB, userID int) ([]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT friend_id FROM friends WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying friends for user %d: %w", userID, err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning friend id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
