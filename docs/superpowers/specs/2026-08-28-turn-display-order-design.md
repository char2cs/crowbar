# Turn display ordering: reserve at dispatch, not at persist

## Problem

Chat messages sort by `ActivityTurn.Seq`, a per-chat counter advanced when a
command is *processed* by the `ChatActivity` aggregate
(`internal/commands/commands.go`'s `advance()`). For an assistant's reply,
that happens in `CloseTurn.EmitEvent` — when the turn actually *finishes*,
not when it was dispatched.

"Interrupting" a turn is a graceful stop request, not a kill
(`runner/lifecycle.go`'s `StopChat` → `interruptTurn`, which falls through to
`retire()` → `TerminateGraceful` for wire type `"prompt"` — today's behavior
for every provider). A stopped CLI can keep producing output and complete its
own turn on its own schedule.

Combined: an earlier-dispatched turn (e.g. Claude, asked to stop) that
finishes later than a later-dispatched turn (e.g. Codex, after a provider
switch) gets a *higher* `Seq` than the turn sent after it, and sorts after it
in the transcript. Reproduced and confirmed by the user.

## Non-goal

`ChatActivity.Turn` is a single pointer, not a map — the aggregate assumes at
most one assistant turn open at a time. `OpenTurn`/`CloseTurn` both wipe
`Tools`/`Subagents`/`Interruptions`/`Choices` on every open/close, so a second
concurrent turn opening silently discards the first turn's in-flight
tool/subagent state. This is real and separate from the ordering bug, but
fixing it means changing every tool/subagent/choice command's "one ambient
current turn" assumption — a materially larger change. **Not fixed here,
left as a known, separate issue.**

## Design

Add two new fields to `ActivityTurn`, decoupled from `Seq`:

```go
DisplayOrder int64 // reserved once, at true dispatch time
ItemIndex    int    // this message's position within its turn (0 unless
                     // the turn produced several — Codex only)
```

`Seq` is untouched everywhere: still freshly advanced by every command, still
the row's unique identity and the pagination cursor (`Turns`/`TurnsBefore`
still filter/order by `seq` in SQL — cursors answer "have I seen everything
up to here", which doesn't need to reflect display order). `DisplayOrder`
is purely how an already-fetched page gets arranged on screen.

**Where each command gets its value:**
- `OpenTurn.EmitEvent`: `DisplayOrder: next.Seq` — this already runs at the
  moment `openAssistantTurn` fires, right after the user's own prompt is
  recorded (`turn.go`'s `openTurnFromPrompt`) — genuine dispatch time.
- `CloseTurn.EmitEvent`: inherit `DisplayOrder` from the currently-open
  placeholder via `inheritOpenTurn` (which already copies `StartedAt`/
  `ProviderID`/etc. the same way), falling back to a fresh `next.Seq` only
  when there's no open turn to inherit from (mirrors current behavior for
  that edge case — never worse than today).
- `AppendTurn.EmitEvent` (the user's own prompt, harness/notice turns): one
  synchronous record, no open/close cycle — `DisplayOrder: next.Seq` is
  already correct as-is.
- `ItemIndex`: threaded in from the usecase layer. `stream.Streams` (the
  per-chat message-assembly tracker) already tracks arrival order in
  `order[chatID]`; add `IndexOf(chatID, messageID) int` returning a message's
  position there, called from `recordAssistantMessage` before building
  `TurnInput`. Covers both call sites that record a message (the delta path
  when a single item finishes on its own, and `closeAssistantTurn`'s
  end-of-turn sweep for stragglers) without threading the index through
  every caller by hand.

**Full chain touched** (`ActivityTurn` → `TurnRow` (+2 columns, GORM
auto-migrates) → back to `ActivityTurn` on read → `LedgerMessage` → JSON
`AgentMessageDTO` (`api/v0/dto/agent.go`, already has explicit `json` tags
distinct from the tagless domain structs) → frontend `AgentChatMessage`).

**Frontend**: `displayOrder`/`itemIndex` added as *optional* fields on
`AgentChatMessage`, with `mergeMessages`' sort falling back to `sequence`/`0`
when absent — real API responses always have them once this ships; the
fallback means no sweep through unrelated test fixtures. The merge-map's
identity key stays `sequence` (unchanged, still unique) — only the final sort
comparator changes to `(displayOrder ?? sequence, itemIndex ?? 0)`.
Interruption-divider placement (`agent-chat-view.tsx`'s `interruptedBefore`)
is a per-message lookup keyed by `sequence` value, not array position — it
is unaffected by changing sort order and needs no change.

## Verification plan

- Go: a regression test in `commands_test.go` proving the exact bug — open
  turn A, open turn B (simulating a provider switch while A is still
  finishing), close B, then close A late; assert A's `DisplayOrder` is still
  lower than B's despite A closing last, and that A's `Seq` is higher (proof
  the two are genuinely decoupled).
- Frontend: a `mergeMessages` test asserting sort-by-`displayOrder` wins over
  arrival/fetch order, and a fallback test for entries missing the field.
- Full Go test suite and full web test suite green; `tsc` clean.
