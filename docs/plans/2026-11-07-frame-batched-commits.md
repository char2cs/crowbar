# Frame-batched chat commits

Status: done.

## Problem

A streaming chat writes the chat store once per live ledger row. `useSyncExternalStore`
makes each write a synchronous React commit, and the transcript's scroll anchor reads
`scrollHeight` in each commit, forcing a layout of the growing stream row. Under load,
several rows land per frame and the frame pays for several renders and layouts.

Deltas are already coalesced into one write per animation frame (`chat-feed.ts`); rows
are not.

## Change

The chat feed attachment owns one ordered queue of the frames it would publish (live rows
and deltas). The queue is applied once per animation frame in one store write:

- Order is the arrival order; rows and deltas fold in sequence onto one feed and publish once.
- Replay rows (the windowed open, a wider window, a resync replay) fold into the draft as
  today and publish at synced: the first frames are not delayed.
- `synced`, `closed` and refusals flush the queue first, so the head check and the resync
  see every applied row.
- A hidden document applies frames as they come (no frame callback runs while hidden),
  and becoming hidden flushes what is queued.
- Detaching drops the queue with the feed; nothing is applied after teardown.

No second copy of state: the queue holds frames not yet folded, never a feed.

## Tests (`chat-feed.test.ts`)

- Many live rows and deltas in one frame give one store notification, in arrival order.
- A synced flushes queued rows before checking the head.
- Detaching drops the queue; a hidden document applies frames at once.
