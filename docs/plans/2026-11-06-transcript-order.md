# Ordering a hooks-only provider's text and calls by its transcript

## Problem

On a hooks-only provider (claude), text and tool calls reach Crowbar through
different hooks, and nothing in those hooks says how they interleave:

- `PreToolUse` / `PostToolUse` carry `tool_use_id`, not the message they
  belong to.
- `MessageDisplay` carries `message_id` (a uuid), `index`, `final` and the
  text. It names no tool call.

The ledger therefore records rows in arrival order, and arrival order is not
the order the model spoke in. A turn captured on 2026-11-06 ("Say A, call
`echo one` and `echo two` in parallel, say B, call `echo three`, say C") ran
like this:

```
19:00:44.594 PreToolUse  echo one
19:00:44.639 PreToolUse  echo two
19:00:45.960 MessageDisplay "A"        <- said before both calls, arrives 1.3 s after
19:00:46.842 PostToolUse echo two
19:00:46.861 PostToolUse echo one
19:00:49.334 PreToolUse  echo three
19:00:49.914 MessageDisplay "B"        <- said before echo three
19:00:50.103 PostToolUse echo three
19:00:52.782 MessageDisplay "C"
```

Today the web compensates in `FeedFold.readsBelow`: text that lands while
calls of its turn are still running is placed above those calls. That holds
only while the calls are running. A call whose `PostToolUse` beats the text
(any call faster than the gap above) is placed above text the model said
before it, and parallel calls split by one fast post are only partly fixed.

A second gap has the same root: a call that fails claude's own validation (an
unknown tool, an invalid input, a call the harness blocks) fires no hook at
all. The TUI shows it; Crowbar never learns of it.

## Who owns the order

The provider's session transcript does: claude appends every assistant text
block, `tool_use` and `tool_result` (with `is_error`) to the jsonl at
`transcript_path`. Every hook payload names that path. Each entry has
`uuid` and `parentUuid`, so the jsonl is the conversation in the order it
was said, failed calls included.

Ordering from it means the ledger takes a fact from a second channel, which
is exactly where CLAUDE.md asks for a plan rather than a fix.

## Plan

1. **No second copy of the conversation.** The ledger stays the record. The
   transcript is read only to answer one question per row: *what does this
   row follow?*
2. **An ordering anchor on rows.** `assistant_text` gains
   `follows: <toolId|"">`, the call the text was said after ("" means the
   turn's start). `tool_call` pre-rows gain the same, naming the text or call
   before them. The anchor is set once, when the row is written, and is never
   rewritten, so the ledger stays append-only.
3. **Descriptor-driven, not provider-named.** A `transcript:` block in the
   descriptor says where the path is (`transcript_path`) and how to read an
   entry: the uuid, parent, role, the text block and the `tool_use` id paths,
   and the `tool_result.is_error` path. Go code knows only the block, never
   "claude".
4. **Ingress reads incrementally.** It keeps a byte offset per (runner,
   transcript), which is runner state like the live-delta buffer, not ledger
   state. On a `MessageDisplay` or `PreToolUse` it reads the lines appended
   since that offset and resolves the anchor for the row it is about to
   write. A row whose entry has not been written yet keeps `follows` empty
   and falls back to arrival order. That is today's behaviour, and nothing
   waits or polls.
5. **Failed validation becomes rows.** A `tool_use` whose `tool_result` has
   `is_error` and no hook ever announced it is recorded as a `tool_call` with
   phase `fail`, anchored the same way.
6. **The web places by anchor.** `FeedFold.place()` puts a row directly below
   its `follows` item when that item is placed. `readsBelow`'s running-call
   rule is deleted. The pending-prompt rule and the end-of-turn rule stay,
   because they order across turns, which the transcript does not answer.

## Tests

- Go: ingress tests feed a recorded jsonl fixture (scrubbed, like
  `testdata/fixtures/claude`) together with the hook sequence above, and
  assert the `follows` anchors, including a fast call whose post precedes
  the text, and a validation-failed call.
- Web: `fold-rows.test.ts` asserts anchor placement for the same sequence;
  the running-call heuristic test becomes an anchor test.

## Open questions

- **Verifying it touches the provider's home.** Live verification means the
  dev daemon reading real transcripts under `~/.claude/projects/...`. Today's
  test rules forbid touching provider homes, so this needs an explicit yes,
  or a fixture-only verification.
- **Transcript format stability.** The jsonl is not a published interface.
  The descriptor block keeps the paths in one place, and a format change
  degrades to arrival order, never to a wrong order.
