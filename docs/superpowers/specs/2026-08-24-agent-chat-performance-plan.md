# The agent chat, made fast

Status: plan. Nothing here is built.
Measured 2026-08-24 in the real Tauri webview (WebKit), not jsdom, not a browser.

## 1. What we are aiming at

"As fast as native code on a GPU" is not literally reachable — this is a DOM
document and text layout is the irreducible cost. But the target it points at is
precise, and we are a long way from it:

| | target | today |
| --- | --- | --- |
| Open a chat, any length, to first paint | < 16 ms | 430 ms @ 100 loaded |
| Scroll, sustained | 120 fps (8.3 ms/frame) | hitches past ~200 loaded |
| Cost of one streamed token | O(1) | O(conversation) |
| Memory for a 1M-turn chat | same as a 100-turn one | same (paging saves us) |
| Work done by an idle chat | zero | zero (already true) |

The last row is already right and worth protecting: polling only runs while a
turn is open.

## 2. The measurements everything below rests on

One typical message — two sentences, inline code, bold — through
`MarkdownMessage`:

| loaded | mount | re-render, props unchanged | DOM nodes |
| --- | --- | --- | --- |
| 50 | 259 ms | — | 30/msg |
| 200 | 864 ms | 64 ms | 30/msg |
| 800 | 4,839 ms | 225 ms | 30/msg |

≈4.3–6 ms to mount each, ≈0.3 ms to re-render each. The markdown parse is
**0.17 ms** of that. The rest is Plate building an editor and React committing
30 nodes.

Two numbers set the ceiling. A viewport holds 10–20 messages, so filling one
costs 43–86 ms — five frames, which is why fast scrolling will hitch no matter
what else we fix. And 225 ms of re-render lands **every second** while a turn
streams, which is 13 dropped frames per second at exactly the moment someone is
watching.

`PlateStatic` over `getStaticPlugins(chatComposerPlugins)` renders the same
message at **1.74 ms** — but that call returns only **1** plugin, so marks and
block types render as nothing. Static rendering is a real ~3× win and it needs a
component set built for it. It is not a free 10×.

## 3. Where the time actually goes

Four layers, and only one of them is the transcript.

**The transcript renders everything it has loaded.** No virtualization —
`messages.map()`. The saving grace is that it never loads the whole chat: page
size 100, server cap 200, cursor-paged on an indexed `(chat_id, seq)`. So a
1M-turn chat *opens* exactly as fast as a 100-turn one. The ceiling is how much
you have paged in, not how long the conversation is.

**Every poll re-renders every row.** Three multipliers stack. `MessageRow` is
not memoized. `applyMessages` runs even on an *empty* page, rebuilding a Map
over all messages and handing React a fresh array. And `AgentTurnTools` filters
and sorts the **entire** `toolCalls` array once per assistant row — measured at
52 ms for 500 rows × 5,000 calls, quadratic in conversation length. A fourth,
smaller: `previousAssistantProvider` walks backward per row and is called twice
per row.

**The streaming bubble re-parses and rebuilds an editor per token.** It goes
through `MessageRow` → `MarkdownMessage`, whose `usePlateEditor(…, [value])`
rebuilds when the text changes. That is ~4 ms per token, on top of the
whole-transcript re-render.

**The daemon has four unbounded reads and a growing aggregate.** `OpenWork`
loads every tool call in the chat to ask whether any is `running` — and it sits
on the terminal-settle path, so it runs hot during a turn. `ChatTurns` (behind
`get_chat_log`) and `renderConversation` (behind handoff assembly, which is what
a provider switch does) load every turn and build one string.
`Subagents`/`Interruptions`/`Choices` are unbounded scans on every activity poll.
Underneath, `ChatActivity` keeps `Tools`, `Subagents`, `Interruptions` and
`Choices` as maps that only ever grow, and asynx writes RFC-6902 full-state
diffs — so appending tool call *n* diffs a document holding all *n−1*.
Snapshots bound the read path, not that.

**One of these is a correctness bug, not a slow path.** The client calls
`listChatActivity` with no cursor; the server clamps to 500; the query is
`seq > 0 ORDER BY seq ASC LIMIT 500` — the **oldest** 500. Past 500 tool calls
the tool rows under recent replies silently stop appearing.

## 4. The plan

Six phases. The ordering is deliberate: the cheap wins are also the ones felt
during streaming, virtualization needs the per-message cost down first to be
worth having, and the aggregate is last because it is the only one with a real
design question in it.

### Phase 0 — a harness that can fail

Nothing below is provable by hand. Before any of it: a dev-only seeded chat of
N turns and M tool calls, and `performance.mark` instrumentation for
open-to-first-paint, scroll frame times, and per-token streaming cost — recorded
as budgets that **fail** when a number regresses, not as a report someone reads.

Frame times, not CPU percentages: a stalled frame and a busy one look identical
in Activity Monitor.

Built alongside Phase 1, because Phase 1 is what proves the harness works.

### Phase 1 — stop the per-second storm

No architecture change; a day's work.

1. Activity cursor — ask for the newest window, or have the server return
   newest-N. Fixes the missing-tool-rows bug and the wasted payload together.
2. Build `Map<turnId, calls[]>` once per activity change; `AgentTurnTools` looks
   its turn up. Kills the quadratic scan.
3. `memo(MessageRow)`, skip `applyMessages` on an empty page, stable `providers`
   identity.
4. `previousAssistantProvider` → one forward pass producing the set of sequences
   that should carry a provider label.

Expected: the 225 ms per-second cost at 800 loaded goes to near zero.

### Phase 2 — make one message cheap

Static rendering, plus caching what cannot change.

`PlateStatic` needs a static component set. The thing that makes this
tractable rather than the "second definition of what a heading looks like" the
current comment fears: **the appearance already lives in `transcript.css`**,
keyed on `.slate-*` classes and semantic tags. The static components are a thin
tag-and-class mapping, and the gate writes itself — render every node type
through both engines and assert identical HTML. If that test is green, the two
cannot drift.

Then cache the parsed value and the rendered element per message id. A recorded
message is immutable, so scrolling back over one should cost nothing.

The streaming bubble gets the same treatment and benefits most: rebuilding a
*static* editor per token is cheap in a way that rebuilding a real one is not.

Expected: 4.3–6 ms → ~2 ms per newly-mounted message; re-entering a seen message
→ ~0.

### Phase 3 — render only what is on screen

The load-bearing change, and the one where the bugs are.

A windowed list with per-row measured heights, a ResizeObserver per rendered
row, an estimate for unmeasured rows, and overscan so rows mount before they are
needed rather than at the viewport edge.

Three existing constraints it has to keep, all of which have already broken
once:

- Bottom-anchored via `margin-top: auto` — `justify-content: flex-end` makes
  start-direction overflow unreachable, which cost 3,565 px of unreachable
  history the first time.
- The `useTranscriptAnchor` contract: stick to the bottom, release the moment
  the reader scrolls up, re-arm when they come back. It observes **both** the
  content and the scroller, because the viewport shrinking when the composer
  grows is the same event as the content growing.
- Prepending an older page must not jump — hold distance from the **bottom**,
  not `scrollTop`.

Re-expressing all three against a virtualizer's own scroll offset is the actual
work. Budget for it accordingly; this is not a library drop-in.

Expected: filling a viewport becomes 1–2 mounts per frame, inside the 8.3 ms
ProMotion budget, at any history length.

### Phase 4 — push instead of poll

`${workspaceBase(wsId)}/chats/ws` already exists and already carries lifecycle
frames, consumed by `use-workspace-agent-chats-stream.ts`. Add a
message-appended frame carrying the message, and the client appends one row
instead of re-fetching 100 and re-merging.

That deletes the 1 s cadence and `mergeMessages`' O(n log n) per tick outright.
The REST paging path stays exactly as it is — it is what a cold open and
"load earlier" use, and it is already correct.

### Phase 5 — bound the daemon

Cheap, server-side, no design question.

- `OpenWork` → `WHERE status = 'running' LIMIT 1`. It is a table scan wearing an
  existence check, on a hot path.
- `ChatTurns` and `renderConversation` → bounded or streamed. Handoff assembly
  is feeding a context window, so a cap is not a compromise, it is the actual
  requirement.
- `Subagents`/`Interruptions`/`Choices` → cursor and limit, like tool calls
  already have.

### Phase 6 — bound the aggregate

The only phase with a real question in it, which is why it is last.

`ChatActivity.Turn` already holds only the **open** turn — finished turns live
in the projection. The four maps should follow the same rule: evict an entry
once it is resolved and projected, so the aggregate describes open work and
nothing else. That makes each RFC-6902 diff proportional to what is in flight
rather than to everything that ever happened.

The question it raises is replay: old events must still fold. That needs a
schema-version and upcast story before a line is written.

## 5. What this does not fix

Text layout. At the end of all six phases a message still costs what WebKit
charges to lay out its text, and a viewport of prose is a viewport of prose.
The plan makes us stop paying that cost for messages nobody is looking at,
repeatedly, every second. It does not make the visible ones free.

If that is not enough, the next step is not more tuning — it is not rendering
markdown as a DOM tree at all. That is a different project and it should be
argued on its own.
