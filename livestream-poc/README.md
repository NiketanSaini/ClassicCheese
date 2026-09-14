# Live Stream + Comments MVP

A minimal live-streaming app: one host goes live from a browser (camera +
mic), any number of viewers watch, and anyone in the room — including the
host — can post text comments that show up in real time for everyone. A
live comment count is visible on the stream.

**Definition of done:** a host clicks "Go live" in one browser, a viewer
opens a link in another browser/device, sees the host's video, posts a
comment, and it appears in real time for everyone watching, including the
host. Verified end-to-end with two independent browser sessions — see
"Manual test walkthrough" below.

## Architecture

One identifier ties video and comments together: **the LiveKit room name is
the `video_id`**. Everything else keys off it.

```
                 ┌─────────────┐   POST /token?room=&role=   ┌──────────────┐
   host/viewer ─▶│   Go server │◀───────────────────────────▶│ LiveKit (SFU)│
    (browser)     │  (:8090)   │   mints JWT (devkey/secret)  │  (:7880/7881)│
                 └──────┬──────┘                               └──────┬───────┘
                        │                                             │
              POST /comments                                room.connect(token)
              GET  /stream (SSE)                             publish/subscribe
                        │                                     video+audio tracks
                        ▼
              in-memory Hub: map[video_id]map[connID]chan Event
              fans out "comment" and "count" SSE events to every
              open /stream connection for that room
```

- **Video/audio** never touches the Go server — the browser talks WebRTC
  directly to LiveKit once it has a token.
- **Comments** never touch LiveKit — they're plain HTTP POST + SSE against
  the Go server, fanned out by an in-memory pub/sub registry (`internal/comments`).

## Stack

- **Backend:** Go (`go-chi/chi`, `livekit/protocol/auth` for JWTs)
- **Frontend:** React + Vite (`livekit-client`, `react-router-dom`)
- **Video/WebRTC:** LiveKit, self-hosted via Docker Compose
- **Real-time comments:** Server-Sent Events, in-memory fan-out (no Redis yet)

## Prerequisites

- Docker + Docker Compose
- Go 1.23+
- Node 20+

## Setup

### 1. Start LiveKit

```bash
docker compose up -d
```

This runs `livekit-server` locally using `livekit.yaml` (dev key pair
`devkey` / a generated 48-char secret — LiveKit rejects secrets shorter
than 32 characters, so this isn't the `devkey`/`secret` pair you'll see in
older LiveKit docs). HTTP/WS signaling is on `:7880`, RTC-over-TCP fallback
on `:7881`, ICE/UDP on `51000-51100` (shifted up from LiveKit's usual
`50000-50100` default to avoid clashing with another service already using
that range on this machine — change it in both `livekit.yaml` and
`docker-compose.yml` if you hit a conflict of your own).

### 2. Start the Go backend

```bash
cd server
cp .env.example .env   # already points at the docker-compose LiveKit above
make run                # go run ., listens on :8090
```

`HTTP_PORT` defaults to `8090` (not `8080`) purely to avoid clashing with
other local services — change it freely in `.env`.

### 3. Start the React frontend

```bash
cd web
npm install
cp .env.example .env.local
npm run dev             # listens on :5173
```

### 4. Try it

Open `http://localhost:5173`, type a room name, click **Go live (host)** in
one browser/tab (grant camera/mic access), then open the same room name at
`/watch/:room` in another tab, browser, or device on the same network and
click **Watch**. Post a comment from either side — it appears on both in
real time, and the live count updates.

## Manual test walkthrough

This mirrors how it was verified during development (Playwright driving two
separate `google-chrome` instances with `--use-fake-device-for-media-stream`
so no real camera is needed):

1. Host page (`/host/:room`) connects to LiveKit as a `publisher`, calls
   `getUserMedia`, and publishes both tracks — status flips to "You are
   live".
2. Viewer page (`/watch/:room`) connects as a `subscriber`, listens for
   `RoomEvent.TrackSubscribed`, and attaches the incoming video track to a
   `<video>` element — status flips to "Live" once the video track arrives,
   and `video.videoWidth/videoHeight` are non-zero (real frames, not a
   blank element).
3. Either side posts a comment via the shared `CommentFeed` component
   (`POST /comments`) — both sides receive it over their own `GET /stream`
   SSE connection within the same event loop tick, and the live count
   (`X comments`) increments on both.

To do this by hand: run the app (above), open two browser windows side by
side on the same room, and repeat steps 1-3 visually.

## Env vars

| Var | Where | Default | Notes |
|---|---|---|---|
| `LIVEKIT_API_KEY` | `server/.env` | `devkey` | must match `livekit.yaml` |
| `LIVEKIT_API_SECRET` | `server/.env` | generated 48-char hex | must match `livekit.yaml`; LiveKit rejects <32 chars |
| `HTTP_PORT` | `server/.env` | `8090` | |
| `CORS_ALLOW_ORIGIN` | `server/.env` | `http://localhost:5173` | the Vite dev origin |
| `VITE_API_BASE_URL` | `web/.env.local` | `http://localhost:8090` | Go backend |
| `VITE_LIVEKIT_URL` | `web/.env.local` | `ws://localhost:7880` | LiveKit signaling |

## Project layout

```
livestream-poc/
├── docker-compose.yml   → LiveKit server for local dev
├── livekit.yaml         → LiveKit dev config (keys, ports)
├── server/              → Go backend
│   ├── main.go          → router wiring, CORS
│   └── internal/
│       ├── config/      → env var loading
│       ├── token/       → LiveKit JWT minting (POST /token)
│       └── comments/    → Hub (pub/sub registry) + Service (validation,
│                           id/count sequences) + HTTP handlers
│                           (POST /comments, GET /stream)
└── web/                 → React frontend (Vite)
    └── src/
        ├── pages/        → HomePage, HostPage, ViewerPage
        ├── components/   → CommentFeed (shared by host + viewer)
        ├── hooks/        → useCommentStream (EventSource), useDisplayName
        └── lib/api.js    → fetch wrappers for /token, /comments, /stream
```

## Notes on the design

- `POST /token` also accepts `GET` — minting a token has no side effects,
  so there's no reason to reject the simpler request the reference
  frontend snippet naturally makes.
- The SSE `Hub` drops a subscriber outright if its 32-message buffer fills
  rather than blocking the broadcaster goroutine; the client's `EventSource`
  reconnects on its own and gets a fresh `count` event immediately.
- Comment ids and counts are per-room monotonic counters
  (`sync/atomic.Int64`), not global — restarting the process resets both,
  which is an accepted MVP tradeoff (see "out of scope" below).

## Explicitly out of scope for this MVP

- Multi-region deployment, Kafka, cross-region replication
- Sharded/approximate counters
- Typing indicators
- Durable comment persistence / replay-on-reconnect (`Last-Event-ID`)
- High availability / failover — restarting the process is an acceptable
  failure mode for now
- Real auth — a typed display name is enough for v1
