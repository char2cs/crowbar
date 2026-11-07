# Long chats: what is left after the windowed open

## Where it stands

A chat opens on a window of its ledger: the scope's `since` is negative, the
daemon replays about the newest `-since` rows widened to whole turns (and to
every turn with open work: the running turn, a pending ask, a live subagent, a
held prompt), and the first row says the seq it follows (`from`). Scrolling to
the top replays a wider window behind the shown feed (`loadOlder`). Facts stay
derived from the rows the feed holds plus the snapshot.

On the seeded chat (3,200 turns, ~89k rows) a cold open sends ~2.3k frames
(~0.9 MB) instead of 43k–100k frames (56–80 MB); `chat.open` is 1.2–1.8 s and
first rows paint at 2.3–3.9 s on a loaded shared host.

## Remaining

1. **Cheaper replay frames.** Row bodies are stored compact; send them as
   `json.RawMessage` that skips `encoding/json`'s re-compaction (about 40% of
   the daemon's replay CPU), and batch a replay page into one socket message
   instead of one per row.
2. **A wider window re-replays what the feed holds.** `loadOlder` asks for the
   newest `rows + 3000` rows again. Once replay frames are batched, a page
   `before=<from>` folded into a fresh draft ahead of the held rows would cost
   only the older rows; the fold must then accept rows older than its head.
3. **Jump to an old turn** (none exists in the web yet) would ask for the
   window that holds it first.

## Tests

- Go: a page `before=<from>` returns whole turns ending at `from`.
- Web: a feed widened by a page equals the feed folded from all its rows
  (order, items, facts).
