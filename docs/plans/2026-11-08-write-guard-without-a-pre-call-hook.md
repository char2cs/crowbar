# Write Guard Without a Pre-Call Hook — Plan

**Status:** Option 1 implemented (descriptor schema 4) · **Date:** 2026-11-08

## 1. The gap

A chat that moved folders must not write into the folder it left
(`ingress/guard.go`). Crowbar refuses such a write in one of two places:

- **A call's own pre-call hook.** The hooks channel has one: claude at every
  level, and a codex TUI.
- **An ask about the call.** A sandbox that asks before it writes outside its
  workspace sends one. That covers codex app-server at `trusted`
  (workspace-write + on-request), and it is answered before any card (40ee8e16).

Codex app-server at `full-auto` (danger-full-access + never) has neither. It
asks nothing, and the api channel reports a call only once it has started. A
moved chat at that level writes into the folder it left. This was verified live
on 0.164.0: `touch <left folder>/guard-full.txt` succeeded.

## 2. What was tried, live, on 0.164.0

| Mechanism | Result |
|---|---|
| `-c hooks.PreToolUse=[…]` on `codex app-server` | `hooks/list` reports the hook `trustStatus: untrusted` and it never runs. `--dangerously-bypass-hook-trust` exists for the TUI only; app-server rejects it. |
| `-c hooks.state."<key>".trusted_hash="<currentHash>"` (also `trusted_content_hash`, inline table) | Still `untrusted`. Hook trust is read only from the persisted config in CODEX_HOME, which Crowbar must not write. |
| A permission profile: `permissions.<id>.filesystem={":root"="write","<left>"="deny"}` plus `default_permissions` | `thread/start` fails: `bwrap: Can't mkdir /.git: Permission denied`. A writable root of `/` cannot be sandboxed. |

## 3. Options

1. **Narrow full-auto for a moved chat.** Launch it as workspace-write with
   `network_access=true`, `writable_roots` = the bound worktree plus the
   attachments dir, and approval `never`. A write outside then fails in the
   sandbox. This changes what full-auto means after a move: every outside
   write is refused, not only the left folder. It belongs in the descriptor's
   `permission_levels` as a level variant chosen when `LeftDirs()` is non-empty.
   It is not a Go branch.
2. **Ask upstream for a session-scoped hook trust in app-server.** That would be
   the equivalent of the TUI's `--dangerously-bypass-hook-trust`. With it, the
   descriptor declares a `guard_injection` (a pre-call hook only, applied to the
   serve process), and ingress treats a hooks-channel `tool_pre` from an api
   runner as guard-only: refuse, or record nothing. The api channel already
   records the call.
3. **Accept and say so.** The move dialog, and the level picker on a moved chat
   at full-auto, state that the old folder is not protected at this level.

Decision: option 1, declared per level as `vars_after_leave` (descriptor
schema 4). A chat that has left a folder launches with those values over the
level's `vars`; codex's full-auto narrows to `sandbox: workspace-write`, so the
new worktree (and /tmp) stays writable and the folder it left does not. The
first launch after the move says so in the chat. Verified live on 0.164.0: in
a moved chat at full-auto, `touch <left folder>/guard-full2.txt` failed with
"Read-only file system" and `touch inside-ok.txt` in the worktree succeeded.

Option 2 stays the better end state, since it keeps full-auto's reach
everywhere except the left folder. Take it once app-server accepts a
session-scoped hook trust.
