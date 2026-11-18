# Chat open: what a cold open still costs

Status: open. Detached feeds are kept in memory (shipped); the items below are not built.

## Where the time goes

A chat scope opens on a window of its newest rows (`OPEN_ROWS`, 1500) and publishes the
feed once, at `synced`. Until then the transcript shows "Loading messages…".
Measured on the dev daemon over loopback with a 1500-turn seeded chat (21,000 rows):

- the open sends 1,513 frames, ~520 KB, 7% of the ledger (not the whole history);
- sub to live is ~55 ms, one chat-view mount ~150 ms after a reload.

Loopback hides what a phone pays: one webview message and one socket frame per row, and
nothing paints until the last. A heavy chat's rows are larger than the fixture's.
The phone's 4 s "Loading messages…" (recording of 2026-11-18) was not reproducible
without a remote host; the per-frame count and bytes are the knobs.

## Shipped

A feed whose last view detaches is kept (newest 8, live only) and its next attach shows it
at once and resumes from its `seq`: a re-attach is 3 frames, not 562 (40-turn chat), and
paints 280 items at attach. It is the same feed, folded by the same code; a socket
reconnect already resumed it this way.

## Shipped: persisted feed (IndexedDB `chat-feeds`, v15)

`stores/lib/feed-cache.ts`, keyed `[hostId, chatId]`, value `{schema, rows, cursor {seq, from, epoch}, savedAt}`.
`rows` is the folded feed (items, order, facts) without live text; the newest 500 items are kept,
the 100 most recently saved chats. A schema mismatch drops the entry on read.

- Open: an attach with no feed in memory reads the entry; a hit shows at once and resubscribes
  with `since = cursor.seq`. A frame that beat the read keeps the screen.
- Reconcile: another run's epoch, a head or resync that disagrees with the cursor (daemon holds
  fewer rows) forgets the entry and replays the newest window behind the shown copy, replacing
  it at ready in one write (the `widening` path).
- Write: through on settle and per folded row, queued behind a microtask and one transaction at a
  time; hidden/pagehide flush queued rows first.
- Forget: chat deletion, host removal, a live epoch change. Any IndexedDB failure is a miss.

## Remaining

1. ~~Persist the folded feed~~ shipped, below.
2. **Batch a replay into one message** (plan 2026-11-06-long-chat-open-window item 1):
   one socket message and one webview channel send per page, not per row.
3. **Window by bytes, not rows**, and open smaller (a few turns) with older turns paged
   in as the reader scrolls up (`before=<from>`), so the first paint waits for one
   screen, not 1500 rows.
4. **Measure on the real path**: a remote host with latency, on the booted simulator,
   frame count and bytes per open, before changing the window.
