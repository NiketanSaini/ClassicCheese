# LinkedIn — Personal Profile (short, hook-driven, no company name)

Ever wondered what actually happens between tapping "submit score" and
watching your name jump up a leaderboard — in real time, for thousands of
players at once?

I got curious enough to build it myself.

Not a toy demo — a real distributed write path:
→ Redis sorted sets for sub-millisecond rank lookups
→ One event, fanned out to 3 independent services via a message queue
→ A durable, replayable event log as the actual source of truth (not just cache)
→ Live rank-change pushes over WebSockets — zero polling

The part I'm proudest of: kill any one of those services mid-flight and
nothing else even notices. That resilience isn't luck — it's the entire
point of talking through a queue instead of calling each other directly.

Built purely to understand how leaderboards for games, streak trackers, and
sales dashboards stay fast and correct at scale.

Happy to nerd out on the design tradeoffs in the comments 👇

---
Notes for you (not part of the post):
- ~150 words, LinkedIn-native short line breaks, curiosity-gap hook first line.
- Zero company/employer references by design.
- CTA is soft (invites comments) rather than a hard ask — keeps it feeling personal, not promotional.
- Swap "sales dashboards" for whatever domain resonates most with your network if you want to tune the hook.
