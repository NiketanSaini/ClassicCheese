# LinkedIn — Company Page (detailed, A-to-Z walkthrough)

🏆 How do you keep a leaderboard fast, accurate, and "alive" when thousands
of scores update every second — without it ever falling over?

We built a proof-of-concept to find out, end to end.

𝗧𝗵𝗲 𝗽𝗿𝗼𝗯𝗹𝗲𝗺
Real-time leaderboards (gaming, learning streaks, sales gamification,
fitness challenges) look simple until you ask three questions at once: How
fast can a rank update? How do you notify the right users the instant
they're overtaken? And how do you never lose a single score event, even if
a service crashes mid-update?

𝗧𝗵𝗲 𝗮𝗽𝗽𝗿𝗼𝗮𝗰𝗵
Instead of one service doing everything, we split the write path into
independent processes that only ever talk through infrastructure — never
directly to each other:

1️⃣ A score update hits our API and lands in Redis sorted sets instantly —
sub-millisecond rank reads, per region and globally.
2️⃣ That same event is published once to a fanout exchange, and delivered
to three consumers in parallel, with zero coupling between them.
3️⃣ An aggregator merges every region's leaderboard into a global one on a
rolling interval.
4️⃣ A history writer persists every single event to a durable, replayable
log — the real source of truth, not just a cache.
5️⃣ A notifier detects rank changes and pushes updates straight to the
client over WebSockets the moment a player is overtaken — no polling, no
refresh button.

𝗪𝗵𝘆 𝗶𝘁 𝗺𝗮𝘁𝘁𝗲𝗿𝘀
Because every piece is decoupled, we can scale, restart, or even swap out
any one service without the others noticing. Lose a consumer mid-message?
The queue redelivers it. Need another region? Add it without touching
existing code. That's the difference between a leaderboard that "mostly
works" and one that holds up under real load.

𝗪𝗵𝗲𝗿𝗲 𝘁𝗵𝗶𝘀 𝗮𝗽𝗽𝗹𝗶𝗲𝘀
Gaming leaderboards, learning-streak rankings, sales contest boards,
fitness challenges — anywhere "who's #1 right now" needs to update in real
time, at scale, without ever losing data.

𝗦𝘁𝗮𝗰𝗸: Go · Redis · RabbitMQ · MySQL · WebSockets

More architecture breakdowns like this coming from our engineering team —
follow along 👇

---
Notes for you (not part of the post):
- ~280 words — comfortably detailed for LinkedIn (limit is ~3,000 chars), still skimmable via bold section headers.
- Replace "We" with your company name / handle where it reads naturally once posted from the page (no name needed if posted from the branded account itself).
- Bold section labels above use Unicode bold characters (𝗧𝗵𝗲…) — this renders as bold on LinkedIn without needing markdown, since LinkedIn strips markdown formatting. Paste as-is.
- Swap the closing CTA line for a link to a blog post / GitHub repo / careers page if you have one ready.
