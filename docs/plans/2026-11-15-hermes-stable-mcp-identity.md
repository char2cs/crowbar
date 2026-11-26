# Hermes: a Stable MCP Identity per Session — Plan

**Status:** Implemented (§3, daemon side) · **Date:** 2026-11-15

Live checks (§6.2) and the §7 inferences are still open. Deviations from the plan:

- **Relay lookup.** A relay id resolves through `chat_ledger_relays` (relay id → chat), a derived
  index beside `chat_ledger_sessions`. The chat's ledger `State` stays the authority: the index
  names a chat, and `State.RelayConversation` must still hold that id on a bound conversation of
  `current`. A lost, cleared or moved-away binding therefore resolves to nothing.
- **Route.** `POST /v0/relays/:relay/mcp`, a flat route with no project or repo in the path.
  `crowbar mcp --relay <id>` replaces `--segment/--project/--repo/--workspace`.
- **Who mints.** The descriptor decides: `Agent.SessionPinsRelay()` is true when a session call
  names `{relay_id}`. Only then does a fresh launch mint an id, so claude and codex write no relay
  id and keep `{segid}` = runner id unchanged.
- **Token file.** `<home>/chats/<chat>/relay/<relay>.token`, keyed by the chat's directory as
  planned. A session moved to another chat would change that path in a pinned block; api providers
  do not move sessions between chats today, so this stays a documented limit.
- **Per-request token.** The relay re-reads its token file on every message; a read error answers
  that message with a transport error. There is no retry loop.
- **Old bindings.** Sessions bound before this change have no relay id, are not resumed, and end at
  the `session.refused` stopgap. No migration.

## 1. The defect

A Hermes chat is unusable once the runner that created its session ends: a kill, a model
switch, a daemon or app restart, or a lost connection. Every later prompt fails. Each replacement
runner's `session.resume` is refused with JSON-RPC `4001 acp_mcp_policy_conflict` (dev daemon log,
three of three investigation chats). Ledger of `8dda75aa…`: `runner_started 6a4de594…`, then
`runner_exited stopped`, then two `runner_exited spawn_failed "acp_mcp_policy_conflict"`, each with
`prompt_settled delivery_failed`.

The stopgap (`session.refused: { rpc_code: 4001, needles: [acp_mcp_policy_conflict] }` in
`hermes.yaml`) makes this a lost session ("start a new conversation") instead of a chat that fails
on every prompt. The conversation is still gone. This plan is for the real fix: a resume that the
gateway accepts.

## 2. Root cause

`events.prompt.fresh` (`session.create`) and `events.prompt.resume` (`session.resume`) in
`hermes.yaml` each send an `editor.mcp_servers` block that starts Crowbar's relay:

```
crowbar mcp --segment {segid} --token-file {tmp}/mcp.token --project {project_id}
  --workspace {workspace_id} --repo {repo_id} --host {daemon_host}   env CROWBAR_HOME={crowbar_home}
```

The gateway pins this block to the session (`source: acp` is the only per-session MCP path) and
refuses a resume that sends a different one. The block changes on every runner:

| Placeholder | Owner today | Lifetime | Verified |
|---|---|---|---|
| `{segid}` | `renderSpawnContext`, `Segid: in.runnerID` (`runner/spawnplan.go`), passed on by `establishAPISession` (`runner/apiconn.go`) | one runner | code |
| `{tmp}` | `worktreepath.RunnerDir(chatsDir, runnerID, provider)` = `<chats>/runners/<runner>-<provider>` (`spawnplan.go`), reaped when the runner ends | one runner, and `chatsDir` comes from `AgentChatsDir(cwdWorkspaceID)`, so it also depends on the workspace | code |
| `{workspace_id}`, `{repo_id}`, `{project_id}` | the chat's placement at spawn | until the chat moves | code |
| `{crowbar}` | `selfExecutable()` (`engine/container.go`) | until the app is updated or moved | code; not checked whether an update changes the path |
| `{daemon_host}` | `DaemonAddress` | one daemon config | code |
| `{crowbar_home}` | the home | stable | code |

The runner id changes on every spawn and `{tmp}` changes with it. So no resume after the first
runner can match the pinned block. The prefix in the tool names (`mcp__editor_crowbar_<hash>__…`) is
a hash of the block, so it changes too. That part is inferred from the observed names, not read from
the gateway.

Today the token is `HMAC(runnerID)` with a secret the daemon makes at each boot
(`shared/tools/tokens.go`). `Resolver.Resolve` (`shared/tools/scope.go`) accepts a call only if
`(segid, token)` verifies and `segid` is a live runner. The relay reads its token file once, when it
starts (`cmd/crowbar/mcp.go`, `readRunnerToken`), and posts to
`/v0/projects/<p>/repos/<r>/chats/runners/<segid>/mcp`. `--workspace` is accepted and ignored
(`cmd/crowbar/scope.go`).

Claude and Codex render the same relay argv (`mcp_injection` in `claude.yaml` / `codex.yaml`). There
it is fine: each runner is a new CLI process that reads its MCP config fresh, and no server pins it.
Hermes is the only provider whose server keeps the block for the life of a session.

## 3. Proposal

Give the relay an identity whose lifetime is the session's binding, not the runner's. The resume
block is then the same bytes every time.

1. **A relay id for each binding.** Crowbar makes a random relay id just before `session.create`
   and records it on the binding: a new field on `ledger.SessionBound`, written by the row that
   binds the fresh session. Every resume reads it from the chat's current `Conversation`. The
   binding already owns the session, so this is a field on it, not a second copy of some other
   state. The provider's session id cannot be used: `session.create` has to carry the block before
   that id exists.
2. **`{segid}` becomes that relay id** for a session call's values (`establishAPISession`). A
   generic template variable, e.g. `{relay_id}`, is cleaner than redefining `{segid}`. PTY providers
   keep `{segid}` = runner id. No provider name enters Go; `hermes.yaml` names the variable.
3. **The token file lives at a stable path,** keyed by the relay id under the chat's own directory
   (`LedgerChatsDir(home)/<chat>/relay/<relay>.token`). That path depends on neither the workspace
   nor the runner. Each runner rewrites it at spawn through `mcp_injection.write_file`; a new
   template variable names the directory. `{tmp}` leaves the pinned block.
4. **Nothing that changes on a move goes in the pinned block.** Remove `--workspace` (ignored
   anyway). `--project`/`--repo` only build the URL. Either the relay posts to a route that needs no
   placement (`/v0/chats/relays/<relay>/mcp`, with scope taken from the resolved runner as it is
   today), or the block keeps them and a cross-repo move is handled as in §5. The first is better: it
   drops the whole class.
5. **Resolve a relay id to the chat's live runner.** `Resolve` maps `relay → binding → chat →` the
   chat's one live runner (`live.byChat`). With no live runner the call is refused, so a call made
   between runners fails as it does today. The token becomes `HMAC(relayID)`.
6. **The relay reads the token on every request, not once.** The daemon secret rotates at each boot,
   so after a daemon restart the gateway's long-lived relay would keep presenting a dead token. A
   re-read per request (or on a 401) picks up what the new runner wrote. This is the token file,
   which the runner owns. It is not a retry loop.

## 4. Security

- **A token that lives longer.** `HMAC(relayID)` is valid for a daemon boot, not one runner. It
  still grants nothing without a live runner on the binding (point 5). The file stays 0600 under the
  home, as today. The minter's own comment says it "is not a boundary against an agent with a shell".
  That threat model does not change.
- **A stale runner using the new identity.** The relay process belongs to the gateway's session, not
  to a runner. After a replacement the "old" relay and the new runner are the same session, so they
  are meant to share the identity. What must be refused is a *different* session or chat
  presenting it. Relay ids are random and one per binding, and `Resolve` checks the binding as well
  as the HMAC.
- **The new runner using a stale identity.** A cleared or lost binding (`session_lost`,
  `CauseCleared`) makes its relay id resolve to nothing. A new fresh session gets a new relay id.
- **Moved sessions.** `CauseMoved` hands a session to another chat. The relay id moves with the
  binding (it is recorded on the moved `SessionBound`), so the gateway's relay follows the session to
  the chat that now holds it. The chat that gave it up resolves nothing.

## 5. Cases to settle

- **Gateway restarted.** The descriptor's comment says the gateway keeps the `source: acp` MCP in
  memory, so a resume after a restart has to resend the block. With a stable block that just works.
  Not verified: whether a restarted gateway still pins the old block (from disk), or accepts any
  block for a session it reloaded. Either way the byte-identical block is accepted. Nothing changes
  unless the gateway also refuses a session it reloaded, which would be a separate needle.
- **Chat move within a repo.** With §3.4 nothing in the block changes. If `--project/--repo` stay in
  the block, a move *across* repos changes it. Then the move has to start a new session on purpose
  (lose the binding with a reason the chat words), not fail on the next prompt.
- **App update.** If an update moves the `crowbar` binary path, `{crowbar}` changes the block too.
  Check this first. If it does, render a stable launcher path (for example a symlink the daemon keeps
  under the home), not `selfExecutable()`.
- **Model switch.** Model and provider are create-only (`restart_tui`), so a switch is a new session
  with a new relay id. Nothing to resume.
- **Sessions bound before this ships.** They carry no relay id, and their pinned block is unknowable.
  They stay lost through the stopgap's refusal. Nothing is migrated.

## 6. Verification against real Hermes

1. Unit (Go, generic): two spawns on one binding render byte-identical `session.resume` params,
   compared as JSON. A fresh binding renders a new relay id. A relay id with no live runner, or from
   another binding, resolves `ErrUnauthorized`. A token file rewritten after a minter rotation is
   read on the next relay request.
2. Live, on a dev daemon only (never the prod socket), with a recorded Hermes version:
   - send one prompt, stop the runner, send again: resume accepted, history intact, and one
     `crowbar_*` tool call succeeds after the resume with the same tool-name prefix as before;
   - restart the dev daemon between prompts: the same, and the relay authenticates with the
     rewritten token;
   - restart the gateway between prompts: the resume is accepted;
   - move the chat to another folder in the same repo: the resume is accepted;
   - model switch: a new session, and the old one is not resumed.
3. Watch the ledger for `runner_exited spawn_failed` and `session_lost` rows and the daemon log for
   `4001`. None should appear except for sessions bound before the change.

## 7. What is verified and what is inferred

Verified in code: the owners of `{segid}`, `{tmp}`, `{workspace_id}`, `{repo_id}`, `{crowbar}` and
`{daemon_host}`; the per-boot HMAC token; that `Resolve` requires a live runner; the read-once token
file; the placement-scoped relay URL; the other providers' `mcp_injection`. Verified in data: the
dev ledger rows and daemon log lines in §1.

Inferred, to be checked live: that the gateway compares the whole block byte for byte, rather than
the server name or command; that the tool-name hash comes from the block; how a restarted gateway
treats a pinned block; whether the gateway keeps one relay process across Crowbar runners; whether
an app update changes the `crowbar` path.
