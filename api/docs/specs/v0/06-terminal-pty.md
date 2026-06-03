# Crowbar Backend — Terminal / PTY

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `02-api-surface.md`,
> `03-realtime-websockets.md`
> **Scope:** PTY terminal sessions and terminal profiles. Covers UX spec §12,
> §29, and the backend portion of §30 (external editor bridge).

---

## 1. Nature of This Subsystem

Terminals are the one **purely in-memory, process-bound** resource:

- A session exists only while its shell process is alive. It is **not persisted**
  and is **lost on server restart**.
- It is the only **bidirectional** WebSocket — every other topic is server→client
  push; the terminal also carries client→server input.
- Its lifecycle is **not** ref-counted by WS subscription (unlike the file
  watcher / LSP). The PTY is created by an explicit REST call and lives
  independently of whether any WS is attached.

```
internal/engine/terminal/
  terminal.go         TerminalEngine interface — Create, Resize, Write, Kill, Attach
  internal/
    session/          PTY session: spawn shell via creack/pty, io pumps, ring buffer
    registry/         in-memory map sessionId → *Session (mutex-guarded)
    profile/          resolve TerminalProfile → shell + cwd + startup commands
```

---

## 2. Session Lifecycle — UX §12

```
1. POST /v0/workspaces/:wsId/terminals { profileId? }
     → resolve profile (or default shell)
     → creack/pty.Start spawns the shell in the workspace repo dir
     → register session, return { sessionId }

2. WS /v0/ws/terminals/:sessionId   (bidirectional)
     → attach the WS to the session's PTY
     → read pump:  PTY stdout → WS → xterm
     → write pump: WS → PTY stdin

3. Pane resized → { type: 'resize', cols, rows } → SIGWINCH to the PTY

4. DELETE /v0/terminals/:sessionId   OR   process exits   → kill + deregister
```

Creating a session and attaching a WS are **separate steps**. The process
survives WS disconnects; the WS can re-attach later (§4).

---

## 3. Wire Protocol — UX §12

```
Client → server (input):   { data: string }
Client → server (resize):  { type: 'resize', cols, rows }
Server → client (output):  { sessionId, data, isInput: false }
```

The server→client direction is the `PTYFrame` broadcaster type
(`03-realtime-websockets.md`). The WS handler's read side decodes the two client
message shapes (input vs resize).

Client-side-only interactions (no backend involvement): zoom, find-in-buffer,
select+copy, hyperlink clicks, large-paste confirmation. Rendering settings
(font, cursor, theme, scrollback size) are client-only too.

---

## 4. Reconnect & Ring Buffer (Q1, Q2)

### Ring buffer (Q1)

Each session keeps a **server-side ring buffer** of recent output (last N KB).
When a WS attaches — including a **re-attach** after an accidental disconnect —
the backend first **replays the ring buffer** so the xterm shows recent history,
then streams live output. This survives transient disconnects without losing
context.

### Multiple attachments (Q2)

**Multiple WebSockets may attach to the same `sessionId` simultaneously**
(mirrored / observed terminals). The session **fans output out** to all attached
WS clients. Input from any attached client is written to the single PTY stdin.
Resize is last-writer-wins on the shared PTY.

This makes the Terminal topic a small fan-out broadcaster keyed by `sessionId`,
with the session's ring buffer as the replay source on each new attach.

---

## 5. Terminal Profiles — UX §29

Profiles are the **one piece of settings the backend persists** (GORM, see
`00-architecture-and-domain.md`) — because the PTY layer needs shell / cwd /
startup-commands server-side at spawn time. All other settings stay client-side.

```
TerminalProfile {
  id               uuid
  name             string
  shell            string?     // e.g. "/bin/zsh"; falls back to system default
  startupDirectory string?     // falls back to the workspace repo root
  startupCommands  []string    // run into the PTY immediately after spawn
  icon             string?
  color            string?
}
```

- CRUD at `/v0/settings/terminal/profiles`.
- On session create, `profile/` resolves shell + cwd and writes
  `startupCommands` into the PTY right after spawn.
- The profile **picker** (shown when >1 profile exists) is client-side; the
  backend only receives the chosen `profileId`.

---

## 6. External Editor Bridge — UX §30

`externalEditor` (Vim / Helix / Neovim in a PTY) is **not a separate
subsystem** — it is a normal terminal session running an editor process. Saving
inside the editor writes to disk, which the file watcher (§27 /
`05-filesystem-and-watcher.md`) already picks up and broadcasts.

The only extra is a `connectionId` linking the terminal to a buffer so the tab
title / dirty state stay in sync — and that is **client-side bookkeeping** per
the UX spec. **No backend work beyond §12.**

---

## 7. Not Our Terminal — Agent `run_command` (Q3)

The agent's `run_command` tool (UX §28) streams output "into a terminal buffer,"
but **agents run their own shell** — entirely separate from this subsystem.
Agent command execution is an **Agentic Bridge** concern
(`11-agentic-bridge-spike.md`) and reuses **none** of this PTY machinery. This
subsystem serves only **user-opened** terminals.

---

## 8. Persistence & Recovery

None. Sessions are in-memory only and intentionally **do not survive restart**
(`00-architecture-and-domain.md`). On restart the frontend's persisted terminal
tabs reconnect, find the session gone, and open fresh ones (the frontend already
stores only `{ sessionId, workingDirectory }` in memory per UX §18).

---

## 9. Out of Scope

- Agent shells / `run_command` (Agentic Bridge, §7 above).
- Persisting sessions across restart (intentional).
- Client-only rendering/settings (font, theme, scrollback, zoom, find).
```
