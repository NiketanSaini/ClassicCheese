// Command aggregator consumes aggregator-queue purely as an activity
// signal — which (region, period_type, period_key) combos are "live" right
// now — and on a fixed ticker recomputes each live combo's global
// leaderboard straight from Redis, so the merge is always correct even if
// a message was missed or arrived out of order.
package main

import (
	"context"
	"encoding/json"
	"log"
	"sort"
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
	queueName   = "aggregator-queue"
	perRegionN  = 50 // how many entries to pull from each regional ZSET before merging
	globalTopN  = 50 // how many merged entries to write into the global ZSET
	logTopN     = 3  // how many to print in the per-cycle log line
	consumePref = 10
)

// comboKey identifies one leaderboard "instance": a period type + key, e.g.
// (daily, 2026-09-07). Regions are tracked per combo since that's what
// needs merging into a single global ZSET.
type comboKey struct {
	PeriodType string
	PeriodKey  string
}

type activityTracker struct {
	mu     sync.Mutex
	combos map[comboKey]map[string]struct{}
}

func newActivityTracker() *activityTracker {
	return &activityTracker{combos: make(map[comboKey]map[string]struct{})}
}

func (a *activityTracker) markActive(periodType, periodKey, region string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := comboKey{periodType, periodKey}
	if a.combos[key] == nil {
		a.combos[key] = make(map[string]struct{})
	}
	a.combos[key][region] = struct{}{}
}

// snapshot returns a deep copy so the merge loop can iterate without
// holding the lock across Redis calls.
func (a *activityTracker) snapshot() map[comboKey][]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[comboKey][]string, len(a.combos))
	for k, regions := range a.combos {
		list := make([]string, 0, len(regions))
		for r := range regions {
			list = append(list, r)
		}
		out[k] = list
	}
	return out
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

	tracker := newActivityTracker()

	go func() {
		for d := range deliveries {
			var event models.ScoreEvent
			if err := json.Unmarshal(d.Body, &event); err != nil {
				log.Printf("aggregator: dropping unparseable message: %v", err)
				d.Nack(false, false)
				continue
			}
			tracker.markActive(event.PeriodType, event.PeriodKey, event.Region)
			d.Ack(false)
		}
	}()

	interval := time.Duration(cfg.AggregationIntervalSecond) * time.Second
	log.Printf("aggregator: merging every %s", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx := context.Background()
	for range ticker.C {
		runCycle(ctx, rdb, tracker)
	}
}

func runCycle(ctx context.Context, rdb *redis.Client, tracker *activityTracker) {
	for combo, regions := range tracker.snapshot() {
		merged := make(map[int]float64)

		for _, region := range regions {
			key := redisclient.RegionalKey(region, combo.PeriodType, combo.PeriodKey)
			entries, err := redisclient.TopN(ctx, rdb, key, perRegionN)
			if err != nil {
				log.Printf("aggregator: reading %s: %v", key, err)
				continue
			}
			for _, e := range entries {
				userID, err := strconv.Atoi(e.Member.(string))
				if err != nil {
					continue
				}
				if existing, ok := merged[userID]; !ok || e.Score > existing {
					merged[userID] = e.Score
				}
			}
		}

		type userScore struct {
			UserID int
			Score  float64
		}
		ranked := make([]userScore, 0, len(merged))
		for userID, score := range merged {
			ranked = append(ranked, userScore{userID, score})
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
		if len(ranked) > globalTopN {
			ranked = ranked[:globalTopN]
		}

		members := make([]redis.Z, len(ranked))
		for i, r := range ranked {
			members[i] = redis.Z{Score: r.Score, Member: strconv.Itoa(r.UserID)}
		}

		globalKey := redisclient.GlobalKey(combo.PeriodType, combo.PeriodKey)
		if err := redisclient.ReplaceZSet(ctx, rdb, globalKey, members); err != nil {
			log.Printf("aggregator: writing %s: %v", globalKey, err)
			continue
		}

		top := ranked
		if len(top) > logTopN {
			top = top[:logTopN]
		}
		log.Printf("aggregator: %s/%s merged %d regions -> global top%d: %v",
			combo.PeriodType, combo.PeriodKey, len(regions), len(top), top)
	}
}
