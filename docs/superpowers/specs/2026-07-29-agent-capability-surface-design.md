# Agent Capability Surface — Design

**Date:** 2026-07-29
**Status:** Approved, ready for implementation planning
**Scope:** Give the agent running inside a Crowbar chat first-class access to Crowbar's own capabilities — read and answer the user's code-review threads, see the workspaces and chats it is allowed to see, and title its own chat — through a Model Context Protocol server hosted by the daemon and injected into every managed vendor CLI session.

---

## 1. Problem

Crowbar spawns real vendor CLIs (claude, codex) in managed PTYs and observes them through hooks. What it has never done is let them **reach back**. Exactly one capability is exposed today — `crowbar chat rename` — delivered as prose in the injected system prompt telling the model to retype a shell command carrying three UUIDs:

```
{crowbar} chat rename {scope_flags} --segment {segid} "<title>"
```

That is the whole agent-facing surface, and it is unreliable in practice: agents routinely ignore it.

Meanwhile the daemon already owns everything an agent would want. Review threads are a complete CRUD surface with a WebSocket topic. The conversation ledger renders clean per-turn logs. Workspaces already carry a `ParentID` lineage. None of it is reachable from inside a chat.

The concrete gap: **an agent asked to "review this branch" will reach for `gh` or write prose into the chat, because it has no idea Crowbar's review surface exists** — and the user's review comments, sitting right there in the UI, are invisible to it.

## 2. Guiding decisions (locked with the user)

1. **MCP is the verb surface.** It is the only channel that is session-scoped, symmetric across both providers, and writes nothing into the user's home or repo. Both mechanisms were verified live (§3).
2. **Crowbar never touches a provider's home directory.** Not `~/.codex`, not `~/.claude`, not even additive namespaced writes. This is why per-workspace `chat/` state exists. It is the constraint that eliminated every skills-based approach on codex.
3. **No skills, on either provider.** claude has a clean session-scoped channel (`--plugin-dir`, verified); codex has none. A capability that works on one of two providers cannot be designed around, and a second authored artifact describing the tools would drift the first time a tool is renamed. Revisit only if instrumentation shows a claude/codex compliance gap.
4. **The MCP server lives in the daemon.** `crowbar mcp` is a thin relay. See §5 for why.
5. **Scope is never an argument.** No tool accepts a `workspaceId`. Authority is derived daemon-side from the caller's runner.
6. **Nothing may break if the agent ignores the tools.** Every capability stays reachable by the user in the UI. Non-compliance degrades output quality; it never wedges a workflow.
7. **Compliance is instrumented from day one.** `set_chat_title` ships first specifically because its current failure rate is known — it is the control experiment.
8. **`create_workspace` is cut from v1.** Highest cost, weakest trigger story, and it is what makes the authorization model load-bearing.

## 3. Research findings

Everything in this section was verified by running the real binaries (**claude 2.1.220**, **codex 0.139.0**) on 2026-07-28, not read from documentation.

### 3.1 Injection channels

| channel | claude 2.1.220 | codex 0.139.0 |
|---|---|---|
| MCP, session-scoped, no file written | ✅ `--mcp-config '<json string>'` | ✅ `-c mcp_servers.<name>.command=…` |
| Skills, session-scoped | ✅ `--plugin-dir <dir>` | ❌ none exists |
| Skill roots | `~/.claude/skills`, `<root>/.claude/skills`, plugins | `$CODEX_HOME/skills`, `<root>/.agents/skills`, `<root>/.codex/skills` |
| Reads `<root>/.agents/skills` | ❌ **verified not read** | ✅ verified |

Evidence:

- `claude -p --strict-mcp-config --mcp-config '{"mcpServers":{"crowbarprobe":{…}}}'` → the model reported `mcp__crowbarprobe__codex, mcp__crowbarprobe__codex-reply`. **claude defers MCP tool schemas** ("currently deferred (schemas not loaded)"), so only the tool *name and one-line description* are in context until the model searches.
- `codex mcp list -c 'mcp_servers.crowbarprobe.command="/bin/echo"' -c 'mcp_servers.crowbarprobe.args=["hi"]'` → server listed as `enabled`, nothing written to disk.
- A throwaway plugin (`.claude-plugin/plugin.json` + `skills/*/SKILL.md`) passed via `--plugin-dir` surfaced as `crowbar:crowbar-probe` alongside the user's own skills.
- codex skill discovery, probed via `codex debug prompt-input`: finds `<root>/.agents/skills` and `<root>/.codex/skills`; works in a **linked worktree** (`.git` is a file) and in a **non-git cwd**; follows a `.agents` **symlink** pointing outside the repo; does **not** search parent directories above the workspace root.
- No external skill-root override was found. `skills.roots`, `skills.paths`, `skills.extra_roots`, `skills.additional_roots`, `skills.directories` and `skills.config=[{path=…,enabled=true}]` were each probed and none added a root. **This is absence of evidence, not proof** — codex's real skills config schema was never located, and the plugin-marketplace route (`marketplace_path` appears in the binary) was never tested. Decision 3 makes the answer moot.

### 3.2 Protocol version

Both CLIs top out at MCP **`2025-11-25`** (verified by inspecting protocol-version constants in both binaries). The **`2026-07-28`** revision — published the day before this spec — is a large breaking change: the protocol goes stateless, the `initialize`/`initialized` handshake and `Mcp-Session-Id` are removed, server-initiated requests are replaced by Multi Round-Trip Requests, and Roots/Sampling/Logging are deprecated on a twelve-month minimum window. SDKs are in beta for it.

**We target `2025-11-25`.** §12 covers the migration.

### 3.3 Consequence for event-driven notification

A server cannot make an idle agent act. Under `2025-11-25` the server→client mechanisms (`sampling/createMessage`, `elicitation/create`, `notifications/*`) are connection-scoped and none starts a turn; under `2026-07-28` server-initiated requests are removed outright. Independently, neither CLI exposes a "inject a turn" surface. Notification-driven capability is therefore out of scope, and the pull model (the user tells the agent there are threads to act on) is the design, not a fallback — with multiple chats open, push has no correct answer to "*which* chat?".

## 4. Architecture

```
vendor CLI (claude | codex)
  │  stdio, MCP 2025-11-25
  ▼
crowbar mcp            ← thin relay, ~50 lines: stdin/stdout ⇄ unix socket
  │  unix socket (existing ipc transport)
  ▼
daemon
  ├── MCP protocol handler   ← initialize / tools/list / tools/call / ping
  ├── agent scope resolver   ← segment+token → runner → chat → workspace → subtree
  └── usecases               ← branchreview, threads, agent, workspace (all existing)
```

The vendor CLI spawns `crowbar mcp` as an ordinary stdio MCP server. It shovels bytes to the daemon and back. Every decision — protocol handling, tool dispatch, authorization — happens in the daemon, next to the usecases.

## 5. Why the server lives in the daemon

The obvious shape is for `crowbar mcp` to implement the protocol and call the daemon's HTTP API. It is rejected for two reasons:

1. **Two API surfaces over the same usecases drift.** Every tool would couple to an HTTP route shape, and the agent-facing contract would have to be versioned and kept in sync with the REST contract.
2. **Version skew.** A `crowbar` binary that implements tools can disagree with the daemon it is talking to. A relay is immune by construction — it has nothing to be stale about.

A third benefit: the `2026-07-28` migration is localised to one component sitting beside the scope resolver, rather than spread across a process boundary.

## 6. Injection

Pure `config_injection` in each descriptor. **No Go changes to the injection engine, no new descriptor field.**

`descriptors/claude.yaml`:

```yaml
config_injection:
  # …existing hook settings…
  - pass_arg:
      arg: "--mcp-config"
      value: '{"mcpServers":{"crowbar":{"command":"{crowbar}","args":["mcp","--segment","{segid}","--token","{runner_token}","--project","{project_id}","--workspace","{workspace_id}","--repo","{repo_id}"]}}}'
```

`descriptors/codex.yaml`:

```yaml
config_injection:
  # …existing hook overrides…
  - pass_arg: { arg: "-c", value: 'mcp_servers.crowbar.command="{crowbar}"' }
  - pass_arg:
      arg: "-c"
      value: 'mcp_servers.crowbar.args=["mcp","--segment","{segid}","--token","{runner_token}","--project","{project_id}","--workspace","{workspace_id}","--repo","{repo_id}"]'
```

Two notes:

- **Scope is passed as discrete array elements, never `{scope_flags}`.** The JSON/TOML array form hands each token to the process separately, which sidesteps the empty-`--repo` shell-collapsing bug documented on `TemplateCtx.ScopeFlags` — a project-home workspace's empty repo id arrives as a distinct empty argument rather than swallowing its neighbour.
- `config_injection` re-applies on **every** spawn including resume, so the tools survive a resumed session on both providers — unlike `context_inject`, which a resumed codex ignores entirely.

One new template variable is required: `{runner_token}` (§7).

## 7. Identity and authorization

### 7.1 Runner token

The segment id alone is a bearer token protected by nothing but unguessability. The agent controls the process that holds it and can read its own argv; nothing prevents `crowbar mcp --segment <other>` once another id is known.

Therefore: **the daemon mints a per-runner token at spawn**, bound to that runner id, stored with the runner record, and revoked when the runner exits. It is exposed to the descriptor as `{runner_token}` on `TemplateCtx` and travels in the relay's argv beside `--segment`. Every tool call validates `(segment, token)` against a live runner, and a call naming a dead or mismatched runner is rejected.

This is not optional hardening deferred to later. It must land before any mutating tool ships.

### 7.2 Scope resolution

Derived server-side, on every call, from the validated runner:

```
segment + token → runner → its CURRENT chat → workspace
```

Resolving through the runner's *current* chat (not a baked chat id) is the same property `RenameByRunner` already relies on: an agent that `/clear`s moves to a different chat, and the runner id stays stable across that move.

From the workspace, visibility is:

| the chat's workspace | may see and act on |
|---|---|
| a git workspace | itself and its descendants, via `Workspace.ParentID` |
| the repo's `IsDefault` workspace | every workspace in that repo |
| a `Kind == home` workspace | every workspace in that project |

Downward only, never upward. Enforced in the daemon; the relay never self-authorizes; **no tool accepts a workspace or scope argument.**

## 8. Tool surface

Tools surface to the model as `mcp__crowbar__<name>`, so names do not repeat "crowbar". Because claude defers schemas, **the one-line description is the entire trigger budget** and must say *when to call*, not what the tool does. Bare `review` is avoided — it collides with the model's `gh pr review` prior.

### 8.1 v1

| tool | description |
|---|---|
| `list_review_threads` | List the code-review threads the user left on this branch. Call when asked to address, answer, or check review comments. |
| `get_review_scope` | What this branch review covers: base ref and changed files. Call before reviewing so findings target the right diff. |
| `post_review_comment` | Post a review finding as a thread anchored to a file and line range, visible in Crowbar's review UI. Use this instead of writing findings in chat. |
| `reply_to_review_thread` | Reply to an existing review thread. |
| `resolve_review_thread` | Mark a review thread resolved. |
| `set_chat_title` | Set this chat's title. Call once, early. |

`get_review_scope` is not padding. Without it the agent reviews whatever range it guesses (`HEAD~1`, `main`) instead of the fork-point range Crowbar's review actually displays; a wrong range anchors every finding wrong.

### 8.2 Phase 2

| tool | description |
|---|---|
| `list_workspaces` | List the workspaces this chat can see — itself and its children, or the whole repo/project depending on where it runs — each with its chats. |
| `get_chat_log` | Read the conversation of another chat you can see. |

Chats are folded into `list_workspaces` rather than given their own listing tool: it saves a schema slot (codex does **not** defer tool schemas, so every tool costs context on every turn there) and matches how the surface is actually navigated.

### 8.3 Held

`create_workspace` — create a child workspace on a new branch. Not in v1 (decision 8). Requires §7.1 and a creation cap.

### 8.4 Schema requirements

- `post_review_comment` takes the same anchor shape as `ThreadDTO` (`filePath`, `startLine`, `endLine`, `side`) and is **rejected** when the anchor does not land in a hunk of the current review. Geometry comes from the existing `/review/outline` and `/review/patch`.
- `post_review_comment` accepts an **idempotency key**. An agent that posts three comments, fails on the fourth and retries must not duplicate the first three.
- Agent-authored threads set `ThreadDTO.IsAgent` — the field already exists; agent authorship is not a new concept.
- Every mutation broadcasts the resulting `ThreadDTO` on the existing WebSocket topic, so the UI updates live while the agent works.

## 9. Response format

Results are returned as MCP **text content**. `outputSchema` is deliberately **not** declared: under `2025-11-25` a tool declaring it must return `structuredContent` *and* should also serialise the same data into `content`, which sends the payload twice.

- **Lists** are line-oriented with stable columns — keys appear once, not per row:

  ```
  #3  src/auth.go:41-47  right  unresolved  2 replies
      "This retry loop can spin forever if the token is revoked."
  ```

- **Prose and code** — comment bodies, chat logs — go in their own block, never inlined in a row, because they contain the delimiters.
- Only fields the agent needs are returned. `ThreadDTO` has fifteen; roughly six matter. Replies are capped and lists paginate.
- Not YAML: review bodies and chat logs are user-authored markdown, and colons, leading dashes, code fences and newlines make naive YAML emission a corruption risk. The saving over JSON is ~10–25%; emitting keys once saves far more.

Structured output becomes worth its cost only if Crowbar's frontend ever renders tool results itself. Then there is a real machine consumer.

## 10. Triggering

Discovery is not the problem; **preference** is. Told "review this branch", a model reaches for `gh` or writes prose because that is its prior. Tool descriptions alone do not override a prior.

1. **A policy line in the injected context.** A new `capabilities_instruction` key in `config/default.yaml`, expanded through the existing `Expand` and composed into the single `{context}` document beside `title_instruction` — the same composition `spawnRunner` already performs. Directive, not capability: *code review happens in Crowbar; post findings with the crowbar tools; not `gh`, not chat prose.*
2. **Tool names and descriptions** per §8 — the only signal claude has before it loads a schema.
3. **`--disallowed-tools 'Bash(gh pr review:*)'`** if "never GitHub" must be a guarantee rather than a preference. Deterministic where prompting is not. Not in v1; available if instrumentation shows it is needed.

A resumed codex ignores `context_inject` entirely, so the policy line is lost on resume there. The **tools** survive, because `config_injection` re-applies (§6).

## 11. Build order

| phase | contents | why here |
|---|---|---|
| 0 | `set_chat_title` only — relay, in-daemon server, injection, runner token, scope resolver | The control experiment. Its current failure rate is known, so this converts "will agents ignore the tools?" into a measurement for the price of one endpoint. |
| 1 | Review threads: `list_review_threads`, `get_review_scope`, `post_review_comment`, `reply_to_review_thread`, `resolve_review_thread` | Requested work, not proactive chore work — the class with the highest natural compliance. Near-total reuse of the existing `/threads` surface. |
| 2 | `list_workspaces`, `get_chat_log` | Read-only, off the existing ledger. |
| 3 | `create_workspace` | Held. Gated on phase 0/1 compliance numbers and on a creation cap. |

## 12. Testing

**Primary gate — Go integration, real binaries.** `api/tests/integration/agent/barriers_test.go` already drives a real vendor CLI by writing a prompt and `\r` into its PTY via `h.eng.Terminal.Write`. A test spawns a real claude, drives *"open a review thread on foo.go line 10 saying X"*, and asserts the thread comes back through the API. That exercises the whole chain — descriptor injection, MCP registration, tool dispatch, scope resolution, daemon write — against the real CLIs. Repeated for codex.

Also covered: a rejected out-of-scope call (a workspace outside the subtree), a rejected stale-token call, and a rejected off-hunk anchor.

**Black-box regression tests** in `api/tests` for every bug found, per project convention.

**Tauri manual test** — the one thing only the app proves:

1. `make dev-desktop`
2. Create a workspace with a diff; leave a review comment in the UI
3. Open an agent chat; type "answer the review threads"
4. The agent's reply appears **in the review pane** as an agent-authored reply on that thread — not as chat prose

Step 4 is the product thesis. The MCP bridge cannot inject xterm keystrokes, so a human drives this; that is a limitation of automation, not of the test.

**Instrumentation** — per-tool-call counters in the daemon from phase 0, so compliance is a number rather than an impression.

## 13. Rejected alternatives

| alternative | why rejected |
|---|---|
| **Skills as the primary surface** | No session-scoped channel on codex (§3.1). The only codex-native routes are the user's repo tree or `$CODEX_HOME`, and decision 2 forbids the latter absolutely. |
| **Install skills into `~/.codex/skills` / `~/.claude/skills`** | Violates decision 2 outright, and leaks Crowbar skills into every non-Crowbar session. |
| **Symlink `<worktree>/.agents` into the runner's tmp dir** | Verified working, but puts an untracked entry in the user's tree, must refuse to clobber a pre-existing `.agents`, needs a `Kind == home` guard, and is codex-only. |
| **CLI verbs bound by `set_env`** | Genuinely viable and was the leading candidate: zero injection surface, `set_env` already exists, and it fixes the retyped-UUID smell. Lost to MCP on typed arguments, structured dispatch, and because MCP needs no new inject verb at all. The CLI remains for humans. |
| **`crowbar mcp` implements the protocol over the HTTP API** | Two drifting API surfaces plus version skew (§5). |
| **Push/notify the agent when the user finishes reviewing** | Impossible in the protocol and getting more so (§3.3), and with several chats open there is no correct answer to which one to notify. |
| **`outputSchema` / structured results** | Duplicates the payload under `2025-11-25` (§9), with no machine consumer today. |

## 14. Known risks

- **Thread anchors carry no revision.** `ThreadDTO` anchors on `filePath`/`line`/`side` with no commit or diff identity, so a thread drifts when the branch moves under it. Pre-existing and equally true of human comments — but this design raises thread volume enough to make it bite sooner. Out of scope here; worth its own spec.
- **The `2026-07-28` migration is deferred, not avoided.** Targeting `2025-11-25` is correct today because that is what both CLIs speak, and the deprecation window is twelve months minimum. §5's placement keeps the rewrite to one component.
- **codex does not defer tool schemas.** Every tool costs context on every codex turn. The surface is capped at roughly eight; the first merge if it bites is `resolve_review_thread` into `reply_to_review_thread(threadId, body?, resolve?)`.
- **Compliance is unproven.** Agents already ignore `title_instruction`. Phase 0 exists to measure this before the rest is built, and decision 6 ensures a non-compliant agent degrades output rather than breaking a workflow.
- **codex's skills channel was never conclusively ruled out** (§3.1). Decision 3 makes it moot, but the claim should not be repeated as settled fact.
