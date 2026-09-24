# codex fixtures

`<method with / replaced by _>.json` is a LIVE CAPTURE off a real codex
app-server connection.

`<same>.<variant>.json` is a SUM-TYPE VARIANT of the same wire method. A wire
method that serves several canonical events (codex's `item/started` and
`item/completed` are both `ThreadItem` sum types) needs one fixture per variant,
or the replay harness silently checks only whichever variant happened to be
captured — which is exactly how `tool_result: item.content` survived: the only
`item/started` recording was a `userMessage`, so `tool_pre`/`tool_post` were
never replayed against anything.

Files whose variant name ends in `-schema` are **derived from codex's own
generated protocol schema**, not captured live:

    codex-rs/app-server-protocol/schema/json/ServerNotification.json  (ThreadItem)

They are exact against that schema's field names and enum values, but they are
NOT proof of what a live codex emits — this repo's history already records four
paths written from a published schema that turned out wrong against real
traffic. Re-capture them live and drop the `-schema` suffix when a codex CLI is
at hand.

## hooks-channel fixtures

`SessionStart.json`, `UserPromptSubmit.json`, and `Stop.json` are the ONE
recorded exception to "one file per wire method, api-shaped" above: they are
the **hooks-transport** payload (no JSON-RPC envelope — the POSTed body IS the
document) for session_start/user_prompt/turn_stop's own `hooks:` channel
block, not their `api:` one. They are VERBATIM captures off a real codex
0.146.0 process with `[features] memories = true`, transcribed from
`api/tests/agent_internal_session_test.go`'s `realStart`/`realPrompt` (session_
start/user_prompt) and `memoryStop` (turn_stop — no separate non-memory Stop
capture exists; it is a genuine codex Stop hook payload all the same, from the
CLI's own internal memory-consolidation session). Only the paths were already
shortened, by that test file's own author.

No recorded hooks-transport payload exists for PreToolUse, PostToolUse,
PreCompact, or PostCompact — tool_pre/tool_post/compact_pre/compact_post's own
`hooks:` blocks in codex.yaml are UNVERIFIED against real traffic; their field
names are carried over unchanged from the pre-channel-split `||` fallback
terms. Capturing them needs a real `codex` CLI driven through an actual tool
call/compaction with hooks configured to dump stdin, not just `app-server` —
see docs/plans/2026-09-22-descriptor-channel-split.md P3's own note on this.

Same gap for PermissionRequest — permission's own `hooks:` block is UNVERIFIED
against real traffic for the same reason (P2b of that plan).

permission's `api:` block now HAS `fixtures:` — `item_commandExecution_
requestApproval.json` and `item_fileChange_requestApproval.json`, the same
captures verbatim (codex-cli 0.149.1) formerly held only in dispatch_test.go's
commandExecutionApproval/fileChangeApproval consts, now written to files and
wrapped in the JSON-RPC envelope. They prove session_id/message (threadId/
reason) resolve — and F1 of docs/plans/2026-09-22-descriptor-channel-
split.md is now fixed: codex's real approval payload carries no `tool` or
`params` field on EITHER wire method, because the tool identity is namespaced
by the WIRE METHOD itself, not any payload field. codex.yaml's permission.api
block now declares `by_wire:`, a generic (zero codex-vocabulary-in-Go)
mechanism that derives a literal field from whichever wire name actually
matched — apidriver.translateLoop merges it into the payload before map:
resolves, and TestV3Descriptors_ResolveAgainstRecordedTraffic's own
loadFixturesFor applies the identical overlay (keyed off each fixture's own
recorded "method") so the fix is proven against the real captures here, not
skipped. See spec.ChannelBlock.ByWire's doc comment, and apidriver_test.go's
TestStart_ByWireOverlayNamesTheToolOnThePermissionCard for the live,
over-the-wire proof.
