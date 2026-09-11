// Command seed populates MySQL with synthetic users and a friends graph,
// then drives score updates through cmd/api's real HTTP write path (never
// writing to Redis/MySQL directly for the score events themselves) so a
// demo has data to look at across all three scopes.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"leaderboard-poc/internal/config"
	"leaderboard-poc/internal/mysqlclient"
)

var regions = []string{"us-east", "eu-west", "apac"}

func main() {
	users := flag.Int("users", 20, "number of synthetic users to create")
	events := flag.Int("events", 100, "number of score events to post")
	apiURL := flag.String("api", "http://localhost:8080", "cmd/api base URL")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := mysqlclient.New(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	userIDs, userRegions, err := createUsers(ctx, db, *users)
	if err != nil {
		log.Fatalf("creating users: %v", err)
	}
	log.Printf("seed: created %d users across %v", len(userIDs), regions)

	if err := createFriendGraph(ctx, db, userIDs); err != nil {
		log.Fatalf("creating friend graph: %v", err)
	}
	log.Printf("seed: created friend graph")

	if err := postScoreEvents(*apiURL, userIDs, userRegions, *events); err != nil {
		log.Fatalf("posting score events: %v", err)
	}
	log.Printf("seed: posted %d score events to %s", *events, *apiURL)
}

func createUsers(ctx context.Context, db *sql.DB, n int) ([]int, map[int]string, error) {
	ids := make([]int, 0, n)
	regionOf := make(map[int]string, n)

	for i := 0; i < n; i++ {
		region := regions[i%len(regions)]
		name := fmt.Sprintf("player-%03d", i+1)

		res, err := db.ExecContext(ctx, `INSERT INTO users (name, region) VALUES (?, ?)`, name, region)
		if err != nil {
			return nil, nil, fmt.Errorf("inserting user %s: %w", name, err)
		}
		id64, err := res.LastInsertId()
		if err != nil {
			return nil, nil, err
		}
		id := int(id64)
		ids = append(ids, id)
		regionOf[id] = region
	}
	return ids, regionOf, nil
}

func createFriendGraph(ctx context.Context, db *sql.DB, userIDs []int) error {
	if len(userIDs) < 2 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT IGNORE INTO friends (user_id, friend_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, userID := range userIDs {
		friendCount := 5 + rand.Intn(11) // 5-15 friends
		if friendCount > len(userIDs)-1 {
			friendCount = len(userIDs) - 1
		}

		picked := make(map[int]struct{}, friendCount)
		for len(picked) < friendCount {
			candidate := userIDs[rand.Intn(len(userIDs))]
			if candidate == userID {
				continue
			}
			picked[candidate] = struct{}{}
		}

		for friendID := range picked {
			if _, err := stmt.ExecContext(ctx, userID, friendID); err != nil {
				return fmt.Errorf("linking %d -> %d: %w", userID, friendID, err)
			}
		}
	}
	return tx.Commit()
}

type scoreUpdateBody struct {
	UserID     int    `json:"user_id"`
	Region     string `json:"region"`
	ScoreDelta int    `json:"score_delta"`
}

func postScoreEvents(apiURL string, userIDs []int, userRegions map[int]string, count int) error {
	client := &http.Client{Timeout: 5 * time.Second}
	endpoint := apiURL + "/score/update"

	for i := 0; i < count; i++ {
		userID := userIDs[rand.Intn(len(userIDs))]
		body := scoreUpdateBody{
			UserID:     userID,
			Region:     userRegions[userID],
			ScoreDelta: 1 + rand.Intn(100),
		}
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}

		resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("posting score event %d/%d: %w", i+1, count, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("score event %d/%d got status %d", i+1, count, resp.StatusCode)
		}
	}
	return nil
}
