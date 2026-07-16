# Agent-Chat Terminal Unification — Implementation Plan

> **For agentic workers:** execute with superpowers:subagent-driven-development. Each phase ships and is **live-verified in the running dev-desktop app** before the next begins. No task is "done" on green tests alone — only on on-screen proof.

**Goal:** Make an agent chat a **chat chrome composed around one shared terminal core**, not a second terminal implementation — and fix the two shared-path bugs that unification exposes.

**Architecture:** A chat = an outer **chat layer** (provider switching, title, x-axis padding/inset, turn/event aggregation) wrapping an inner **terminal core** (`XtermTerminal` + the keep-alive/portal host + transport that shell tabs already use). The chat delegates terminal hosting to the shared host and feeds its chrome off the terminal's stream. Shell tabs are unchanged.

**Why:** Every agent-chat terminal bug (pane-split refit-blank, same-chat duplication-blank, remount churn, dead scroll, scrollback/scroll loss) is a **derived-state desync** caused by the chat pane **forking** terminal hosting instead of composing it, plus two bugs that live in the shared path. Fixing the fork + the two shared bugs collapses ~6 of 7 known bug classes.

## Global Constraints
- **Shell terminal tabs must not regress.** They keep the screen-model + keep-alive + portal exactly as today.
- **Every chat feature preserved:** provider switching (Claude/Codex), title, x-axis padding/inset, turn/event aggregation.
- **Live verification is mandatory** at each phase via the Tauri MCP, using RELIABLE methods only: DOM measurement (`.xterm-viewport` scrollHeight/clientHeight), screenshots, and **atomic** single-call xterm-instance probes (grab-by-max-rows + attach onData + inject `WheelEvent` with clientX/Y + read, all in ONE `webview_execute_js`). Do NOT: hold a grabbed xterm instance across calls (it goes stale on remount), return a Promise from `webview_execute_js`, or use the keyboard-shortcut MCP (both time out).
- Commit locally per task; **no push, no PR** unless the user asks. `git add` only the task's files (never `git add -A`); verify `git status --short` before commit.
- Keep the already-landed clean fixes: stale-base diff, `wheel-routing.ts` intent model, `refit.ts` PTY sync.

---

## Phase 0 — Transport liveness (SHIP FIRST; fixes the dead scroll; low-risk, independent)

**Problem (confirmed via live instrumentation):** the terminal WS goes **half-open** — REST re-dials work and the daemon lists the session live, but the persistent socket delivers neither input nor output. The client believes it is connected because liveness is **presence-only**, so no reconnect fires. The enabling defect: the Rust writer task swallows send failures without retiring the session or emitting a drop; only the *reader* surfaces drops, and only on a daemon-initiated close. User-visible symptom: "dead scroll" (actually total silent I/O death).

**Files:**
- `desktop/src-tauri/src/terminal.rs` (~197–204 writer task; ~229–252 reader-drop path)
- `web/src/lib/crowbar-bridge.ts` (~214–223 terminalWrite; ~360–362 terminalHasTransport)
- `web/src/features/terminal/components/resolve-terminal-connection.ts` (~65–92 reuse-on-hasTransport)
- (verify) daemon WS ping/pong: `api/internal/.../ws.go` (~20)

**Changes:**
1. **Rust writer retires + surfaces drops symmetrically.** On a writer-side `send`/socket failure, retire the session (same cleanup the reader path does) and emit `terminal:transport-dropped` so the FE's existing transport-drop → re-attach chain fires. Consider a write/ping timeout so a half-open socket (writes that never error but never arrive) is also caught — if the daemon ping/pong exists, treat a missed pong as a drop.
2. **Liveness is verified, not assumed.** `terminalHasTransport` / the resolver must not reuse a socket by mere map presence; a session whose transport is not provably alive must force a fresh attach (which the FE already knows how to do on `transport-dropped`).

**Acceptance (LIVE, on-screen):** Reproduce/observe a chat whose I/O is dead (inject a wheel → art does NOT scroll; the atomic probe shows `onData` bytes generated but nothing reaches Claude). After the fix: the half-open socket is detected, the client re-attaches, and input + scroll + output resume — verified by injecting a wheel and seeing the art scroll, and by the daemon receiving input. Screenshot before/after.

---

## Phase 1 — Keyframe soft-apply (shared core; preserve scrollback + scroll position)

**Problem:** the client applies every daemon keyframe (7 triggers: alt-flip, scroll-region change, scrollback shrink, ring-rotation, resize, origin change, unprimed) with a full `terminal.reset()` + redraw, then `scrollToBottom()`. This **wipes client scrollback and yanks a scrolled-up user to the bottom** on any keyframe. Hits any alt-buffer TUI; chats run one constantly.

**Files:**
- `web/src/features/terminal/hooks/use-terminal-connection.ts` (~282–333 snapshot/keyframe handler; ~159–162 first-frame finalize scrollToBottom)

**Changes:** Replace `reset()`+redraw+unconditional-`scrollToBottom()` with a resync that preserves scroll state: capture the user's distance-from-bottom before applying; `scrollToBottom()` only if they were already at the bottom (tail-follow), else restore the same relative position; avoid the full `reset()` where the keyframe can be applied without discarding history. **Preserve the correctness the reset was added for** (replacing client reflow junk with daemon truth) — this is the delicate part. Extract the pure decision (was-at-bottom / restore offset) so it is unit-testable, and add regression tests.

**Acceptance (LIVE):** scroll up in a running TUI (or shell `less`), trigger a keyframe (alt-flip via opening/closing a pager, or a resize) → view stays put, scrollback intact, no yank to bottom. Confirmed on-screen.

> Risk: highest of the three. If it fights xterm's reset semantics, reduce scope to the `scrollToBottom` tail-follow guard first (kills the yank) and defer the reset-avoidance.

---

## Phase 2 — Re-layer agent chats onto the shared terminal host (the structural change)

**Problem:** `AgentChatPane` forks hosting — agent chats render only when active and **remount on split**, with detach keyed by the shared `sessionId`. Shell tabs instead stay mounted-hidden (keep-alive) and are portaled/reparented across panes. The fork causes refit-blank, duplication-blank, and remount churn.

**Files:**
- `web/src/features/agent/components/agent-chat-pane.tsx` (chat wrapper; renders `XtermTerminal` directly today)
- `web/src/features/terminal/components/terminal-host.tsx` (the keep-alive + portal host shell tabs use) and `web/src/features/panes/components/pane-container.tsx` (~610–636: shell keep-alive vs chat render-only-when-active)
- `web/src/features/terminal/components/terminal.tsx` (~886–892 detach-keyed-by-sessionId)

**Changes:**
1. Route agent chats through the **same keep-alive + portal host** as shell tabs, so splits/reparents/tab-switches do NOT remount or destroy them.
2. `AgentChatPane` becomes the **chat chrome only** — provider switcher, title, x-axis padding/inset, turn/event aggregation — wrapping the shared host and consuming its stream; it no longer owns terminal lifecycle.
3. **Single PTY-size owner:** the active/visible view owns PTY size (no last-resize-wins flap between co-views).
4. **Duplicate-into-two-panes policy:** decide one — either forbid duplicating a chat into a second pane (a chat is one live view; a second pane *mirrors*, does not re-attach), or make co-views coherent. Fix the detach-keyed-by-shared-`sessionId` teardown so one view's unmount can't kill another's transport.

**Acceptance (LIVE):** (a) two DIFFERENT chats split → both render, no blank, DOM mismatch 0; (b) duplicate one chat into two panes → defined behavior, no blank, no transport teardown of the other; (c) drag the sash to extremes with a TUI running → reflows to final size, no remount churn; (d) switch chat tabs → survives without remount. All on-screen.

---

## Execution order & sign-off
Phase 0 → live-verify → Phase 1 → live-verify → Phase 2 → live-verify → final whole-branch review. Phase 0 is independently shippable and fixes the user's dead scroll today.
