// Package redisclient wraps go-redis with the key scheme and access
// patterns this leaderboard POC relies on: regional/global ZSETs, a plain
// per-user score mirror for cheap MGETs, and the Pub/Sub notification
// channel used by cmd/notifier and cmd/api.
package redisclient

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

func New(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

// RegionalKey is the ZSET holding one region's cumulative scores for a
// period, e.g. lb:regional:us-east:daily:2026-09-07.
func RegionalKey(region, periodType, periodKey string) string {
	return fmt.Sprintf("lb:regional:%s:%s:%s", region, periodType, periodKey)
}

// GlobalKey is the ZSET cmd/aggregator merges regional top-N lists into. It
// carries no region segment.
func GlobalKey(periodType, periodKey string) string {
	return fmt.Sprintf("lb:global:%s:%s", periodType, periodKey)
}

// UserScoreKey is a plain string mirror of a user's score in one period, so
// the friends-scope endpoint can MGET a batch instead of touching a ZSET
// per friend.
func UserScoreKey(userID int, periodType, periodKey string) string {
	return fmt.Sprintf("user_score:%d:%s:%s", userID, periodType, periodKey)
}

// NotificationChannel is the Pub/Sub channel cmd/notifier publishes rank-
// change events to and cmd/api subscribes to on behalf of WebSocket
// clients watching the same (scope, region, period_type, period_key) tuple.
func NotificationChannel(scope, region, periodType, periodKey string) string {
	return fmt.Sprintf("notifications:%s:%s:%s:%s", scope, region, periodType, periodKey)
}

// IncrementScore applies a score delta to a region's ZSET and to the
// matching user_score mirror in one round trip, returning the user's new
// cumulative score for that period.
func IncrementScore(ctx context.Context, rdb *redis.Client, region string, userID int, periodType, periodKey string, delta int) (float64, error) {
	zsetKey := RegionalKey(region, periodType, periodKey)
	mirrorKey := UserScoreKey(userID, periodType, periodKey)

	pipe := rdb.Pipeline()
	zIncr := pipe.ZIncrBy(ctx, zsetKey, float64(delta), strconv.Itoa(userID))
	pipe.IncrBy(ctx, mirrorKey, int64(delta))
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("incrementing score for user %d in %s: %w", userID, zsetKey, err)
	}
	return zIncr.Val(), nil
}

// TopN returns the top n (member, score) pairs from a ZSET, highest first.
func TopN(ctx context.Context, rdb *redis.Client, key string, n int64) ([]redis.Z, error) {
	if n <= 0 {
		n = 10
	}
	return rdb.ZRevRangeWithScores(ctx, key, 0, n-1).Result()
}

// Rank returns the 1-indexed rank of member in key's ZSET (highest score is
// rank 1), or ok=false if the member isn't present.
func Rank(ctx context.Context, rdb *redis.Client, key, member string) (rank int, score float64, ok bool, err error) {
	zRank, err := rdb.ZRevRank(ctx, key, member).Result()
	if err == redis.Nil {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	zScore, err := rdb.ZScore(ctx, key, member).Result()
	if err != nil {
		return 0, 0, false, err
	}
	return int(zRank) + 1, zScore, true, nil
}

// Around returns up to 2*count+1 (member, score) pairs centered on member's
// rank in key's ZSET.
func Around(ctx context.Context, rdb *redis.Client, key, member string, count int64) ([]redis.Z, error) {
	rank, err := rdb.ZRevRank(ctx, key, member).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	start := rank - count
	if start < 0 {
		start = 0
	}
	stop := rank + count
	return rdb.ZRevRangeWithScores(ctx, key, start, stop).Result()
}

// ReplaceZSet atomically clears key and repopulates it with members, used by
// cmd/aggregator to rewrite the small global ZSET each cycle.
func ReplaceZSet(ctx context.Context, rdb *redis.Client, key string, members []redis.Z) error {
	pipe := rdb.Pipeline()
	pipe.Del(ctx, key)
	if len(members) > 0 {
		pipe.ZAdd(ctx, key, members...)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// MGetUserScores fetches the user_score mirror for a batch of users in one
// round trip. Missing users come back as nil in the returned slice.
func MGetUserScores(ctx context.Context, rdb *redis.Client, userIDs []int, periodType, periodKey string) ([]interface{}, error) {
	keys := make([]string, len(userIDs))
	for i, id := range userIDs {
		keys[i] = UserScoreKey(id, periodType, periodKey)
	}
	return rdb.MGet(ctx, keys...).Result()
}

// Publish sends a raw JSON payload to a Pub/Sub channel.
func Publish(ctx context.Context, rdb *redis.Client, channel string, payload []byte) error {
	return rdb.Publish(ctx, channel, payload).Err()
}
