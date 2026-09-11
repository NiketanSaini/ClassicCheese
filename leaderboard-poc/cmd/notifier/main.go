// Command notifier consumes notifier-queue, checks each user's new rank in
// their regional leaderboard, and publishes a Redis Pub/Sub notification
// when they enter the top 10 or jump up more than RANK_CROSS_THRESHOLD
// positions. It never touches a WebSocket connection itself — cmd/api owns
// those and forwards whatever notifier publishes.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"leaderboard-poc/internal/config"
	"leaderboard-poc/internal/models"
	"leaderboard-poc/internal/rabbitmqclient"
	"leaderboard-poc/internal/redisclient"
)

const (
	queueName     = "notifier-queue"
	consumePref   = 10
	topNThreshold = 10
)

// rankMemory tracks the last known rank per (user, region, period_type,
// period_key) in-process. A POC doesn't need this persisted: if the
// process restarts, the next event just starts from "unranked" again.
type rankMemory struct {
	mu    sync.Mutex
	ranks map[string]int
}

func newRankMemory() *rankMemory {
	return &rankMemory{ranks: make(map[string]int)}
}

func comboKey(userID int, region, periodType, periodKey string) string {
	return fmt.Sprintf("%d:%s:%s:%s", userID, region, periodType, periodKey)
}

func (m *rankMemory) previous(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ranks[key] // zero value = unknown/unranked
}

func (m *rankMemory) set(key string, rank int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ranks[key] = rank
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rdb := redisclient.New(cfg.RedisAddr)
	defer rdb.Close()

	conn, ch, err := rabbitmqclient.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	if err := rabbitmqclient.DeclareExchange(ch, cfg.RabbitMQExchange); err != nil {
		log.Fatalf("declaring exchange: %v", err)
	}
	if _, err := rabbitmqclient.DeclareAndBindQueue(ch, cfg.RabbitMQExchange, queueName); err != nil {
		log.Fatalf("declaring queue: %v", err)
	}

	deliveries, err := rabbitmqclient.Consume(ch, queueName, consumePref)
	if err != nil {
		log.Fatalf("consuming: %v", err)
	}

	memory := newRankMemory()
	ctx := context.Background()
	log.Printf("notifier: watching top %d and rank jumps > %d positions", topNThreshold, cfg.RankCrossThreshold)

	for d := range deliveries {
		var event models.ScoreEvent
		if err := json.Unmarshal(d.Body, &event); err != nil {
			log.Printf("notifier: dropping unparseable message: %v", err)
			d.Nack(false, false)
			continue
		}

		if err := handleEvent(ctx, rdb, memory, cfg.RankCrossThreshold, event); err != nil {
			log.Printf("notifier: handling event for user %d failed, requeueing: %v", event.UserID, err)
			d.Nack(false, true)
			continue
		}
		d.Ack(false)
	}
}

func handleEvent(ctx context.Context, rdb *redis.Client, memory *rankMemory, threshold int, event models.ScoreEvent) error {
	key := redisclient.RegionalKey(event.Region, event.PeriodType, event.PeriodKey)
	rank, score, ok, err := redisclient.Rank(ctx, rdb, key, strconv.Itoa(event.UserID))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	memKey := comboKey(event.UserID, event.Region, event.PeriodType, event.PeriodKey)
	previous := memory.previous(memKey)
	memory.set(memKey, rank)

	reason := ""
	switch {
	case rank <= topNThreshold && (previous == 0 || previous > topNThreshold):
		reason = "top10"
	case previous > 0 && previous-rank > threshold:
		reason = "rank_jump"
	}
	if reason == "" {
		return nil
	}

	notification := models.Notification{
		UserID:       event.UserID,
		Scope:        "regional",
		Region:       event.Region,
		PeriodType:   event.PeriodType,
		PeriodKey:    event.PeriodKey,
		Rank:         rank,
		PreviousRank: previous,
		Score:        score,
		Reason:       reason,
		Timestamp:    time.Now().UTC(),
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	channel := redisclient.NotificationChannel("regional", event.Region, event.PeriodType, event.PeriodKey)
	if err := redisclient.Publish(ctx, rdb, channel, payload); err != nil {
		return err
	}

	log.Printf("notifier: user=%d %s/%s rank %d->%d reason=%s", event.UserID, event.PeriodType, event.PeriodKey, previous, rank, reason)
	return nil
}
