# First prompt launches the provider CLI once

## Problem

For a provider with no api connection, the first prompt of a new chat spawns
two CLI processes; the first is killed ~110 ms after the second starts. Two
`agent: runner launched ... rung=fresh` lines (supervisor.go:232) appear.

Evidence (api/internal/app/usecases/chat/):
- Chat creation launches an idle CLI: `SpawnChat` (internal/runner/spawn.go:22-38),
  `StartRunner` (spawn.go:41-58), called from internal/tree/chats.go:47,
  own_worktree.go:59, imported_worktree.go:52.
- The prompt path requires that idle CLI to exist: `promptTarget` returns
  `ErrPromptSessionUnavailable` when there is no live runner (prompts.go:252-259).
- With no api connection, the prompt is delivered only through argv:
  `submitPromptLocked` -> `displaceForPrompt` -> `quitOutgoingCLI`
  (prompts.go:278-295, processes.go:63-79) -> `spawnRunner(..., text)`
  (prompts.go:120-126). The first CLI never saw a turn.
- `resolvePromptDelivery` already special-cases this: "a dormant chat's revive
  spawn ... never given a chance to answer before being displaced"
  (promptdelivery.go, `liveTurned`). Delivery code knows the first launch is
  disposable; the owner of launch (chat creation) does not.

Owner of the state: "a live runner exists" is created at chat creation but is
only consumed once a prompt, attach, or resize needs a PTY. Creation and first
prompt each assume they own the first launch.

## Options

A. Lazy spawn. Creation persists the chat dormant (no runner, no process). The
   first prompt on a chat with no live runner spawns once with the prompt in
   argv (the existing dormant/revive path, `quitOutgoingCLI` already returns nil
   when dormant, processes.go:65). Terminal attach / explicit start spawns
   without a prompt. `promptTarget` stops treating "no live runner" as an error
   for chats that have never run.
   - Pros: one launch per first prompt; reuses the dormant path that already
     exists; no new state.
   - Cons: first attach to an empty chat now launches on demand (needs the same
     `StartRunner` entry, already exists); the trust-prompt modal (empty ledger in
     a fresh worktree) appears on first prompt/attach instead of at creation;
     UI that reads `liveRunnerId`/`terminalSessionId` right after creation sees
     dormant and must not wire a pane to ''.
B. Start the first CLI with the prompt (creation takes an optional first
   prompt). Cons: changes the create-chat contract in four callers and the web;
   the prompt can no longer be submitted separately from creation; draft/queue
   semantics break. Rejected.
C. Skip the displace when the live runner never turned and nothing else changed
   (deliver into the idle CLI via PTY). Cons: provider-specific stdin injection;
   it is what argv delivery replaced. Rejected.

## Recommendation

Option A. Creation callers stop spawning for CLI-only providers; spawn moves to
the first of: prompt, terminal attach, explicit start. Phase `dormant` is the
honest state of a chat that has never run (ChatPhase already has it).
Providers with an api connection are unchanged. Before implementing, confirm
with the web owner (web/src/features/workspace/components/workspace-view.tsx,
web/src/features/panes/stores/slices/pane-slice.ts) that a dormant new chat
renders its composer and does not require a PTY id; if it does, that gap is
fixed in the web, not papered over with a placeholder runner.

## Test plan

Black-box, in the chat usecase package, using the fake provider that records
spawns:
- `TestRegression_FirstPromptLaunchesCLIOnce`: create chat, submit first prompt;
  assert exactly one spawn, whose argv carries the prompt, and zero displaced
  runners.
- `TestRegression_NewChatIsDormantUntilUsed`: after create, no live runner and
  zero spawns.
- `TestRegression_AttachToNewChatSpawnsOnceWithoutPrompt`, then a prompt on it
  displaces once (the existing path, now counted as 2 launches total, 1 displace).
- Resume: second prompt on a chat with a native session still resumes (no extra
  launch beyond today's one displace-per-prompt).
- Web: a test that a freshly created chat with `liveRunnerId === ''` mounts the
  composer and sends the first prompt without a start call.

## Risks

- Trust prompt timing: the empty-ledger trust modal moves to first use; verify
  it still blocks the prompt delivery rather than swallowing it.
- Handoff content: `resolvePromptDelivery` assembles conversation when
  `!liveTurned`; a dormant first prompt must produce the same (empty) handoff.
- Workspace/chat creation flows that return `runnerID` to the caller
  (tree/chats.go, own_worktree.go, imported_worktree.go) need their response
  contract changed; check every consumer, not only the first prompt.
- Latency: the first prompt no longer finds a warm CLI, but today's warm CLI is
  already discarded, so no regression.
