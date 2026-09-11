# Prompts + context to hand to other agents/tools

Copy the **Context Brief** into any agent first (or prepend it to a prompt),
then use the specific prompt for what you need. Keeping the brief separate
means you can reuse it across an image generator, a copywriting agent, a
diagramming tool, etc. without retyping the project explanation each time.

---

## Context Brief (paste this first, every time)

> Project: a proof-of-concept distributed, real-time leaderboard system
> built in Go. One HTTP endpoint accepts score updates. Redis sorted sets
> give instant regional/global/friends rank lookups. A RabbitMQ fanout
> exchange decouples three independent consumers: an aggregator (merges
> regional leaderboards into a global one every 5s), a history writer
> (persists every event to MySQL as the durable, replayable source of
> truth), and a notifier (detects rank crossings and pushes live updates
> to clients over WebSockets via Redis Pub/Sub). No service calls another
> service directly — everything communicates only through Redis, RabbitMQ,
> or MySQL. That's deliberate: it's the same pattern a real multi-region
> production system would use, just demonstrated on a laptop.
>
> Applicable domains: gaming leaderboards, learning-streak rankings, sales
> contest boards, fitness challenges — anywhere "who's #1 right now" needs
> to update in real time, at scale, without losing data.
>
> Stack: Go, Redis, RabbitMQ, MySQL, WebSockets, chi router.

---

## Prompt 1 — Personal LinkedIn post (copywriting agent)

```
[paste Context Brief above]

Write a short, punchy LinkedIn post for my PERSONAL profile about this
side project. Requirements:
- No company/employer name anywhere — this is posted as an individual, not on behalf of any org.
- Hook in the very first line: curiosity gap, no throat-clearing, no "Excited to share...".
- 120-160 words.
- Short line breaks, LinkedIn-native rhythm (not paragraph blocks).
- Tone: curious builder sharing something they figured out, technically credible, not braggy.
- End with a soft CTA that invites comments/conversation, not a hard ask.
```

## Prompt 2 — Company LinkedIn post (copywriting agent)

```
[paste Context Brief above]

Write a detailed LinkedIn post for OUR COMPANY PAGE explaining this
engineering project end to end. Structure:
1. Hook question (1 line)
2. The problem (2-3 sentences)
3. The approach, as a numbered A-to-Z walkthrough of the write path
4. Why the design matters (decoupling, resilience, scalability)
5. Where this pattern applies (real-world use cases)
6. One-line tech stack
7. CTA to follow the page for more engineering content
- 250-300 words, bold section labels, no marketing buzzwords, skimmable.
```

## Prompt 3 — Cover image / carousel graphic (image-generation agent)

```
[paste Context Brief above]

Design a LinkedIn cover graphic (or 3-slide carousel) to accompany a post
about this system. Show the flow left-to-right or top-to-bottom:
Client -> API -> Redis (instant rank write) -> RabbitMQ fanout exchange ->
three parallel workers (Aggregator / History Writer / Notifier) -> MySQL +
WebSocket push back to the client.

Style: clean, modern, dark-mode tech-brand palette, icon-based (not
screenshots or code), minimal text, one clear focal flow, high contrast,
readable as a thumbnail.
```

## Prompt 4 — Architecture diagram (diagramming agent, e.g. Mermaid/Eraser/another draw.io agent)

```
[paste Context Brief above]

Create an architecture diagram for this system with this exact flow:

Client --POST /score/update--> API service
API --ZINCRBY (writes ZSET)--> Redis
API --publish (1 event)--> RabbitMQ fanout exchange "score-events"
RabbitMQ fans out to 3 queues -> 3 consumers:
  - Aggregator: merges regional Redis ZSETs into a global ZSET every 5s
  - History Writer: inserts every event into MySQL "score_events" (durable source of truth)
  - Notifier: computes ZREVRANK, then PUBLISHes to a Redis Pub/Sub channel
Redis Pub/Sub --subscribe--> API --WebSocket push--> Client (live rank-change notification, no polling)

Style requirements: color-code by component type (API=blue, Redis=red,
RabbitMQ=orange, workers=purple, MySQL=teal, client=gray), clear
directional arrows with short labels, visually group the 3 workers to
show they run in parallel with zero coupling to each other.
```

---

Note: a ready-made version of the diagram in Prompt 4 is already at
`social/architecture-diagram.drawio` — open it in draw.io / diagrams.net
and tweak colors/labels directly instead of regenerating from scratch,
unless you want a different visual style.
