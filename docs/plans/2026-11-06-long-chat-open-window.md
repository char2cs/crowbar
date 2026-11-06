# Long chats: open on a window of the ledger

## Problem

A chat's feed replays its whole ledger on every attach (`attachFeed` reads rows
`since` to the head; the web attaches with `since=0` after any detach or
reload). For a heavy daily chat that is the dominant cost of opening it.

Measured on a seeded chat of 1,520 turns, 42,836 rows (tool calls with real
sized diffs and outputs by ref, 227 subagent launches, long markdown replies), on a
shared 4-core host under load:

| | |
|---|---|
| bytes on the socket per open | 56 MB, 42,836 frames |
| daemon CPU per open | ~1.3 s (40% JSON re-compaction of row bodies, 30% SQLite reads) |
| web CPU to fold the replay | ~0.4–0.9 s (`applyFeedFrame`, socket `onmessage`) |
| `chat.open` (attach → ready) | 5.3 / 6.7 / 10.0 s (three cold opens) |

The 1.5 s cold-open budget cannot be met by making the fold faster: the
replay itself is linear in the chat's whole life, and so are reload,
reconnect after a daemon restart, and a second tab.

## Who owns what

- The **chat ledger** (daemon) owns the rows. It is append-only and already
  folds a `ledger.State` per chat (head, plan, open asks, compaction, live
  runs). That state is derived from rows, not a second copy.
- The **chat feed** (web `chatStore.feeds[chatId]`, folded by `FeedFold`) owns
  what the transcript draws. Its `facts` are also derived from rows.

A window must keep both derived: the web may not invent facts the rows it
holds do not imply, and the daemon may not keep a per-client copy.

## Plan

1. **Turn-aligned window on attach.** The chat scope takes an optional
   `window` (turns). `attachFeed` finds the seq of the first row of the
   `window`-th newest turn (an index on `(chat_id, kind, seq)` for
   `turn_started`/`user_message` rows, or a `turn_starts` side table the
   ledger writer maintains) and replays from there. The first frame says
   where history starts: `{t:"window", from: <seq>, olderTurns: <n>}`.
2. **Carried facts come from the daemon's fold.** What the web needs from
   before the window is exactly what `ledger.State` already folds (plan of
   the main scope, open asks, compaction, live subagents, running turn). The
   window frame carries `State`'s projection for `from - 1`, so the web seeds
   `facts` from it instead of from rows it does not hold. No new state.
3. **Older turns on demand.** `GET /v0/.../chats/{id}/rows?before=<seq>&turns=<k>`
   returns whole turns. `FeedFold` gains a `prepend(rows)` that folds them
   into a fresh draft and splices `order` in front; items are keyed by id, so
   `ItemMap` already shares every untouched bucket. The transcript asks for
   a page when the reader nears the top (the virtualizer's first index), and
   keeps its anchor through the prepend the way it does through history
   loading today (`loadingHistory`).
4. **Prompt history and search** read the loaded window; recall already
   says "never reaches past what has been paged in". Jump-to-turn asks the
   rows endpoint for the turn's page first.
5. **Cheaper replay frames.** Row bodies are stored compact; send them as
   `json.RawMessage` that skips `encoding/json`'s re-compaction (about 40% of
   the daemon's replay CPU), and batch replay rows into one socket message per
   page instead of one per row.

## Tests

- Go: `attachFeed` with `window=k` replays from the first row of the k-th
  newest turn, and its window frame's facts equal `State` folded to `from-1`.
- Web: a feed opened on a window then prepended with the older page equals
  the feed folded from all rows (order, items, facts), and a long chat's open
  folds a number of rows independent of its length.
