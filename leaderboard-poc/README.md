# Leaderboard POC (Go + RabbitMQ + Redis + MySQL)

A local proof-of-concept of a distributed game leaderboard: four independent
Go processes that talk to each other **only** through RabbitMQ, Redis, and
MySQL — never directly. That's deliberate: on a laptop this is as close as
you can get to demonstrating a fully distributed write path, since any one
of these processes could just as easily be a different service in a
different region.

```
                 POST /score/update
                        │
                        ▼
                 ┌─────────────┐        ZINCRBY / INCRBY
                 │   cmd/api   │───────────────────────────▶ Redis (ZSETs)
                 └──────┬──────┘
                        │ publish (1 msg per period type)
                        ▼
              ┌───────────────────┐
              │  score-events     │  (durable fanout exchange)
              └───┬───────┬───────┘
       ┌───────────────┬───────────┴─────────┐
       ▼               ▼                     ▼
 aggregator-queue  historywriter-queue   notifier-queue
       │               │                     │
       ▼               ▼                     ▼
 cmd/aggregator   cmd/historywriter      cmd/notifier
 (merges Redis    (inserts into MySQL    (ZREVRANK, then
  regional ZSETs   score_events —         PUBLISHes to Redis
  into lb:global   the durable,           Pub/Sub channel
  on a 5s ticker)  replayable source      notifications:*)
                    of truth)                   │
                                                 ▼
                                     cmd/api subscribes + forwards
                                     to matching WebSocket clients
```

## Stack

- Go 1.24+
- `github.com/redis/go-redis/v9`
- `github.com/go-sql-driver/mysql` + `database/sql` (no ORM)
- `github.com/rabbitmq/amqp091-go`
- `github.com/gorilla/websocket`
- `github.com/go-chi/chi/v5`

## Prerequisites

- Redis running locally (`redis-server`), reachable at `localhost:6379`, no auth.
- MySQL running in Docker, reachable at `localhost:3306`, with a `leaderboard` database.
- RabbitMQ running in Docker, AMQP on `localhost:5672`, management UI on `localhost:15672` (`guest`/`guest`).

## Setup

### 1. Apply the schema

```bash
# adjust the container name if yours differs
docker exec -i <mysql-container-name> mysql -uroot -p leaderboard < schema.sql
```

Or, if you have a `mysql` client locally:

```bash
mysql -h127.0.0.1 -P3306 -uroot -p leaderboard < schema.sql
```

### 2. Configure env vars

```bash
cp .env.example .env
# edit MYSQL_DSN's credentials to match your container
export $(grep -v '^#' .env | xargs)
```

Every binary calls the same `config.Load()` — it fails fast with a clear
error if `MYSQL_DSN` or `RABBITMQ_URL` isn't set.

### 3. Start all four services (separate terminals, any order)

```bash
make run-api            # HTTP :8080 + WebSocket
make run-aggregator     # merges regional -> global every AGGREGATION_INTERVAL_SECONDS
make run-historywriter  # writes score_events into MySQL
make run-notifier       # watches rank changes, publishes to Redis Pub/Sub
```

Each service idempotently declares the `score-events` fanout exchange and
its own queue on startup — whichever one you start first creates the
topology, the rest just bind alongside it. There's no separate "create the
topic" step.

### 4. Seed some data

```bash
make seed
# equivalent to: go run ./cmd/seed --users 20 --events 200 --api http://localhost:8080
```

This creates 20 users spread across `us-east` / `eu-west` / `apac`, a
random 5-15-friend graph per user, then posts 200 random score updates
through `cmd/api`'s real `/score/update` endpoint — the same write path a
real client would use.

## Manual test walkthrough

Run these after `make seed`, or run them one at a time to watch the system
react to a single event.

### Post a score update and check Redis directly

```bash
curl -s -X POST localhost:8080/score/update \
  -H 'Content-Type: application/json' \
  -d '{"user_id": 1, "region": "us-east", "score_delta": 50}' | jq
```

```bash
redis-cli ZREVRANGE lb:regional:us-east:daily:$(date -u +%F) 0 9 WITHSCORES
```

### Check the same leaderboard via the API

```bash
curl -s "localhost:8080/leaderboard/top?scope=regional&region=us-east&period_type=daily&period_key=$(date -u +%F)&n=10" | jq
```

```bash
curl -s "localhost:8080/leaderboard/rank?scope=regional&region=us-east&period_type=daily&period_key=$(date -u +%F)&user_id=1" | jq
```

```bash
curl -s "localhost:8080/leaderboard/around?scope=regional&region=us-east&period_type=daily&period_key=$(date -u +%F)&user_id=1&count=3" | jq
```

Friends scope (needs a user with friends from `make seed`):

```bash
curl -s "localhost:8080/leaderboard/top?scope=friends&period_type=daily&period_key=$(date -u +%F)&user_id=1" | jq
```

### Watch cmd/aggregator merge it into global within ~5s

Watch the `cmd/aggregator` terminal — within `AGGREGATION_INTERVAL_SECONDS`
(default 5) you should see a line like:

```
aggregator: daily/2026-09-07 merged 3 regions -> global top3: [{1 950} {7 910} {3 880}]
```

Then confirm it landed in Redis and via the API:

```bash
redis-cli ZREVRANGE lb:global:daily:$(date -u +%F) 0 9 WITHSCORES
curl -s "localhost:8080/leaderboard/top?scope=global&period_type=daily&period_key=$(date -u +%F)&n=10" | jq
```

### Trigger a manual snapshot and read it back

```bash
curl -s -X POST localhost:8080/admin/snapshot \
  -H 'Content-Type: application/json' \
  -d "{\"scope\":\"regional\",\"region\":\"us-east\",\"period_type\":\"daily\",\"period_key\":\"$(date -u +%F)\",\"top_n\":10}" | jq
```

```bash
curl -s "localhost:8080/leaderboard/history?scope=regional&region=us-east&period_type=daily&period_key=$(date -u +%F)&limit=10" | jq
```

### Watch a WebSocket push arrive when a score crosses into the top 10

Connect first, then post an update that should push someone into the
top 10 (a large `score_delta` on a low-ranked or brand-new user works
well). Any of these connect the same way:

**websocat:**

```bash
websocat "ws://localhost:8080/ws/leaderboard?scope=regional&region=us-east&period_type=daily&period_key=$(date -u +%F)"
```

**Two-line Python:**

```python
import websocket
ws = websocket.WebSocket()
ws.connect("ws://localhost:8080/ws/leaderboard?scope=regional&region=us-east&period_type=daily&period_key=" + __import__("datetime").datetime.utcnow().strftime("%Y-%m-%d"))
print(ws.recv())
```

**Two-line Go** (`go run` this from a scratch file with
`go get github.com/gorilla/websocket` available):

```go
c, _, _ := websocket.DefaultDialer.Dial("ws://localhost:8080/ws/leaderboard?scope=regional&region=us-east&period_type=daily&period_key=2026-09-07", nil)
_, msg, _ := c.ReadMessage(); fmt.Println(string(msg))
```

With that connected, in another terminal:

```bash
curl -s -X POST localhost:8080/score/update \
  -H 'Content-Type: application/json' \
  -d '{"user_id": 999, "region": "us-east", "score_delta": 100000}'
```

Watch the `cmd/notifier` terminal log the rank change, and the WebSocket
client print the JSON notification it received via Redis Pub/Sub.

## Watching the fanout in RabbitMQ

Open `http://localhost:15672` and watch the message rates on
`aggregator-queue`, `historywriter-queue`, and `notifier-queue` tick up in
real time as you post score updates — one publish to the `score-events`
exchange fans out into three independent deliveries, one per queue.

For a closer look at the raw JSON, bind one more throwaway queue to the
exchange and peek at it without permanently consuming — same instinct as
`curl -v` for tracing a TCP handshake, just watching messages fan out
across a broker instead of packets across a wire:

```bash
rabbitmqadmin declare queue name=debug-peek durable=false
rabbitmqadmin declare binding source=score-events destination=debug-peek
rabbitmqadmin get queue=debug-peek ackmode=ack_requeue_true
```

`ack_requeue_true` puts the message back after showing it to you, so it
doesn't steal deliveries from `aggregator-queue` / `historywriter-queue` /
`notifier-queue` — fanout means `debug-peek` gets its own independent copy
of every message regardless.

## Env vars

| Var | Default | Notes |
|---|---|---|
| `REDIS_ADDR` | `localhost:6379` | |
| `MYSQL_DSN` | *(required)* | e.g. `root:password@tcp(localhost:3306)/leaderboard?parseTime=true` |
| `RABBITMQ_URL` | *(required)* | e.g. `amqp://guest:guest@localhost:5672/` |
| `RABBITMQ_EXCHANGE` | `score-events` | |
| `HTTP_PORT` | `8080` | `cmd/api` only |
| `AGGREGATION_INTERVAL_SECONDS` | `5` | `cmd/aggregator` only |
| `RANK_CROSS_THRESHOLD` | `3` | `cmd/notifier` only |

## Project layout

```
leaderboard-poc/
├── schema.sql
├── Makefile
├── .env.example
├── internal/
│   ├── config/          # env var loading
│   ├── models/          # shared structs + period-key helpers
│   ├── redisclient/     # key scheme + ZSET/Pub-Sub helpers
│   ├── mysqlclient/     # explicit SQL, no ORM
│   └── rabbitmqclient/  # exchange/queue declare, publish, consume
└── cmd/
    ├── api/             # HTTP + WebSocket
    ├── aggregator/      # regional -> global merge
    ├── historywriter/   # durable score_events writer
    ├── notifier/        # rank-change detector
    └── seed/            # synthetic data + load generator
```

## Notes on the design

- RabbitMQ queues aren't a replayable log — once a message is acked and
  removed, it's gone. That's fine here because `historywriter`'s MySQL
  table is the actual durable, replayable source of truth; the queue was
  only ever the transport.
- `cmd/aggregator` treats incoming messages purely as an activity signal
  ("this region/period combo is live"). The merge itself always
  recomputes from Redis on the ticker, so it stays correct even if a
  message was dropped, delayed, or arrived out of order.
- `cmd/notifier` never touches a WebSocket connection — it doesn't even
  know one exists. It publishes to a Redis Pub/Sub channel; `cmd/api` is
  the only process that owns WebSocket connections and forwards whatever
  arrives on the matching channel.
