# claude fixtures

claude is hooks-only and never dual-shape (see claude.yaml's own top-of-file
comment), so unlike `codex/` there is no api:/hooks: channel split here — one
file per hook, named exactly as the descriptor's own `in:` value, is the
whole convention.

## Provenance

All six files are VERBATIM captures off a real `claude` **2.1.278** process on
2026-09-22, using a throwaway `--settings` file whose hooks dumped raw stdin
to disk (no `-p`/headless mode — an interactive session with hooks
configured the normal way). One session: a user prompt asking for a single
`Read` tool call on a scratch file, then a plain-text reply. That session
produced exactly these hook firings in order: SessionStart, UserPromptSubmit,
PreToolUse, PostToolUse, Stop, SessionEnd.

## Scrubbing

`transcript_path`, `cwd`, `scratchpad_dir`, `tool_input.file_path`, and
`tool_response.file.filePath` carried a real machine's home directory and a
long worktree-specific path. Each is replaced with a stable placeholder in
the same shape, following the precedent `../codex/SessionStart.json` already
set (`cwd` -> `/ws`, `transcript_path` -> a shortened `/h/.claude/...`
path):

- `cwd` -> `/ws`
- `scratchpad_dir` -> `/ws/.scratchpad`
- `transcript_path` -> `/h/.claude/projects/-ws/<session_id>.jsonl`
- `tool_input.file_path` / `tool_response.file.filePath` -> `/ws/note.txt`

`session_id`, `prompt_id`, and `tool_use_id` are left verbatim — they are
random UUIDs/opaque tokens, not machine-identifying, and keeping them
consistent across the six files preserves the real shape of one session's
traffic (the same `session_id` recurs in every file, the same `tool_use_id`
ties PreToolUse to PostToolUse). `last_assistant_message` ("DONE"),
`prompt` (the literal test instruction), and `tool_response.file.content`
("alpha\n") are the CLI's and the test's own synthetic values, not
machine-specific, and are kept verbatim. No field the descriptor actually
maps was altered in value shape (still a string/object/array of the same
kind), only path fields' contents.

## Coverage

These six files verify the six hook events claude.yaml wires `fixtures:` to:
`session_start` (SessionStart.json), `user_prompt` (UserPromptSubmit.json),
`turn_stop` (Stop.json), `tool_pre` (PreToolUse.json), `tool_post`
(PostToolUse.json), `session_end` (SessionEnd.json).

Every other event claude.yaml declares remains UNVERIFIED against real
traffic — no fixture exists for any of them, and none should be invented:

- `turn_failed` (StopFailure) — needs a captured run that errors out.
- `message_delta` (MessageDisplay) — needs a captured streaming turn.
- `tool_fail` (PostToolUseFailure) — needs a captured failing tool call.
- `subagent_pre` / `subagent_post` (SubagentStart/SubagentStop) — needs a
  captured Task-tool subagent run.
- `notification` (Notification) — needs a captured idle/permission notice.
- `compact_pre` / `compact_post` (PreCompact/PostCompact) — needs a captured
  `/compact`.
- `permission` (PermissionRequest, `ask:`) — needs a captured tool that
  actually asks (this session ran under `acceptEdits`, so nothing asked).
- `elicitation` (Elicitation, `ask:`) — needs a captured MCP elicitation.
- `compact_start` has no inbound payload at all (`out: prompt` — Crowbar
  sends `/compact`, claude never hooks it) and cannot have a fixture.

## A known-bad mapping this capture surfaced (see docs/plans/2026-09-22-
descriptor-channel-split.md, F1/F2 class)

`user_prompt`'s `map:` is `{ message: prompt }` only — it does not map
`session_id`, even though `UserPromptSubmit.json` here demonstrably carries
one (`session_id` is the FIRST key in the real payload). claude.yaml's own
comment above `user_prompt` calls this deliberate-carried-over, not
newly introduced by this phase, but it is now a PROVEN gap against real
traffic rather than a theoretical one. Not fixed here — see the phase's own
report for why (field-semantics change, owned by a later phase).
