# Case Study: Building a Real-Time Leaderboard That Doesn't Fall Over

**A proof-of-concept in distributed systems design — how we keep rankings
fast, live, and durable under concurrent writes.**

---

## TL;DR

We set out to answer a deceptively simple question: *how do you update a
leaderboard in real time, for many concurrent players, without losing data
or coupling every part of the system together?*

The answer we built: four independent services that never call each other
directly — they only ever talk through Redis, RabbitMQ, and MySQL. Each
one can be restarted, scaled, or replaced without the others noticing.

Read on for the full breakdown, or jump to [How It Works](#how-it-works)
for the architecture.

---

## The Challenge

Leaderboards look simple on the surface — store some scores, sort them,
show the top N. In practice, a leaderboard that has to work at real
product scale has to answer several hard questions at once:

- **Latency** — when a score changes, how fast can we tell the player
  their new rank? Milliseconds matter; nobody wants to refresh to find out
  they moved up.
- **Fan-out** — one score update usually needs to trigger *several*
  independent things: merging into a global view, recording history for
  audit/analytics, and notifying anyone who just got overtaken. Should the
  API that accepted the write be responsible for all of that, synchronously,
  in the request path?
- **Durability** — what happens to a score update if a downstream service
  is down, restarting, or slow? Can we guarantee it's never silently lost?
- **Scope** — most real leaderboards aren't one flat list. Players expect
  regional rankings, a global view, and a friends-only view, often across
  daily/weekly/monthly periods, all at once.

Bolting all of this onto one service tends to produce a system where every
feature is coupled to every other feature, and a slow or failing
notification path can degrade the write path itself.

## The Approach

Instead of one service doing everything, we designed the system as four
independent Go processes that communicate **only** through shared
infrastructure — Redis, RabbitMQ, and MySQL — never through direct
service-to-service calls:

| Service | Responsibility |
|---|---|
| `api` | Accepts score updates over HTTP, writes to Redis, publishes one event, and serves reads (leaderboards, ranks, WebSocket subscriptions) |
| `aggregator` | Merges every region's leaderboard into a global one on a rolling interval |
| `historywriter` | Persists every score event to MySQL — the durable, replayable source of truth |
| `notifier` | Detects when a player's rank crosses a threshold and pushes a live update |

This isn't an arbitrary split. Each service is doing exactly one job, and
none of them know the others exist — they only know about the
infrastructure between them.

## How It Works

**1. A score update lands.**
A client posts to `POST /score/update` with a user, region, and score
delta. The API writes it directly into a Redis sorted set — this is what
makes rank lookups sub-millisecond, even under heavy write volume.

**2. One event, fanned out three ways.**
The API publishes a single message to a RabbitMQ **fanout exchange**
(`score-events`). RabbitMQ delivers an independent copy of that message to
three separate queues — one per downstream consumer — with zero coupling
between them. If the API had to call all three services directly and wait
for each one, one slow consumer would slow down every score update. With
fanout, the write path is done the moment the event is published.

**3. Three consumers do their jobs in parallel.**

- **Aggregator** treats each message purely as an activity signal ("this
  region/period is live"). It always *recomputes* the global leaderboard
  from Redis on its own ticker (every 5 seconds by default), so it stays
  correct even if a message is dropped, delayed, or arrives out of order.
- **History writer** inserts the event into MySQL's `score_events` table.
  This table — not the queue, not Redis — is the actual durable, replayable
  source of truth. RabbitMQ queues aren't a log; once a message is acked,
  it's gone. MySQL is what survives.
- **Notifier** computes the player's new rank (`ZREVRANK` in Redis) and, if
  it crossed a configurable threshold, publishes to a Redis Pub/Sub
  channel. Notably, the notifier never touches a WebSocket connection — it
  doesn't even know one exists.

**4. The update reaches the player live.**
The API is the only process that owns WebSocket connections. It subscribes
to the Redis Pub/Sub channel and forwards matching notifications straight
to connected clients — no polling, no refresh.

See [`architecture-diagram.drawio`](./architecture-diagram.drawio) for the
full visual, or the ASCII version in the project README.

## Data Model

Three tables carry the whole system:

- **`users`** / **`friends`** — identity and a lightweight social graph,
  which is what powers the friends-scope leaderboard.
- **`score_events`** — an append-only, replayable log of every score
  change, indexed by `(user_id, period_type, period_key)`.
- **`leaderboard_snapshots`** — point-in-time top-N captures per scope
  (global/regional/friends), so "what did the leaderboard look like at
  time X" is always answerable after the fact.

Redis carries the hot path: one sorted set per `region/period` and one for
`global/period`, plus a plain per-user score mirror so the friends-scope
endpoint can batch-fetch scores instead of hitting a sorted set per friend.

## API Surface

| Method | Endpoint | Purpose |
|---|---|---|
| `POST` | `/score/update` | Submit a score delta for a user |
| `GET` | `/leaderboard/top` | Top-N for a scope (global / regional / friends) |
| `GET` | `/leaderboard/rank` | A single user's current rank |
| `GET` | `/leaderboard/around` | The players ranked just above/below a user |
| `GET` | `/leaderboard/history` | Read back a saved snapshot |
| `POST` | `/admin/snapshot` | Trigger a manual leaderboard snapshot |
| `GET` | `/ws/leaderboard` | WebSocket stream of live rank-change notifications |

## What We Validated

Running this end to end on a single machine, we could directly observe the
properties the architecture was designed for:

- **Independent failure** — stopping the notifier mid-stream doesn't slow
  or block score writes, the aggregator, or history persistence. Restart
  it, and it just resumes consuming from its own queue.
- **Correct under disorder** — because the aggregator always recomputes
  from Redis rather than trusting message order, out-of-order or delayed
  events never produce a wrong global leaderboard.
- **Real fan-out, visible in real time** — watching RabbitMQ's management
  UI while posting score updates, you can see one publish turn into three
  independent queue deliveries, ticking up in lockstep.
- **Live client updates** — a connected WebSocket client receives a
  push the moment a score update crosses it into the top ranks, without a
  single poll.

## Why This Matters Beyond a POC

The pattern here — decoupling through infrastructure instead of direct
calls — is the same one a real multi-region production system would use;
this POC just demonstrates it on a laptop, where every "service" happens
to be a process instead of a fleet in a different region. Nothing about
the design changes if `aggregator`, `historywriter`, and `notifier` each
became their own deployment in their own region tomorrow.

It applies anywhere "who's #1 right now" has to update in real time, at
scale, without losing data: gaming leaderboards, learning-streak rankings,
sales contest boards, fitness challenges, and similar live-ranking
features.

## Tech Stack

Go 1.24+ &middot; Redis (sorted sets + Pub/Sub) &middot; RabbitMQ (fanout
exchange) &middot; MySQL (`database/sql`, no ORM) &middot; WebSockets
(`gorilla/websocket`) &middot; `chi` router

## Key Takeaways

1. **Decouple through infrastructure, not direct calls.** A message queue
   between services means a slow or failing consumer never blocks the
   write path.
2. **Pick one source of truth, deliberately.** A queue is a transport, not
   a log — durability has to live somewhere that's actually replayable.
3. **Design consumers to tolerate disorder.** Treating messages as
   "wake-up signals" rather than authoritative state made the aggregator
   correct by construction, not by careful sequencing.
4. **Keep notification delivery ignorant of transport.** The service that
   *detects* a rank change doesn't need to know anything about WebSockets
   — that separation kept both pieces simple.

---

*Want the full technical walkthrough, including how to run this yourself
end to end? [Link to README / GitHub repo].*
