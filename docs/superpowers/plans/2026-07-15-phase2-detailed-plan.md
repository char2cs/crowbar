# Phase 2 — Detailed Plan: unify agent-chat hosting (mounted-hidden keep-alive)

**Key finding (verified 3 ways):** the `TerminalHost`/`XtermPortal`/`terminal-slots-store` "portal host" is DEAD CODE — its registrar `TerminalSlot` was deleted in `69aac329`, nothing calls `register()`, so it renders nothing. Shell tabs actually keep-alive via `visibility:hidden` in `pane-container.tsx:610-633`. **DO NOT revive the portal** (rejected: dead code, its purge signal never fires for chats since `closeBuffer` doesn't remove a chat's session, sessionId-keying conflicts with provider-switch, and making shell tabs portal would regress them). Leave the dead host untouched.

**Unification = give agent chats the same `visibility:hidden` keep-alive shell tabs have, using the same `XtermTerminal` core.** All new behavior is gated on `attachOnly` so shell tabs are byte-for-byte unchanged. Chat chrome (provider switcher, title status line, x-axis inset column, `gridSlack`, empty-space focus, runner-follow, attach/revive/switch/gone lifecycle, title→buffer mirror, `flush`) all STAYS in `AgentChatPane`.

**Global rules:** commit locally per task; `git add` ONLY that task's files (never `git add -A`); no push/PR. Unit tests per task (mirror path in `web/src/__tests__/`, `@/` imports). Run `~/.bun/bin/bun run tsc --noEmit` + touched test suites + eslint on changed files — all green — before each commit. Do NOT live-verify (the orchestrator does that on the shared dev app). Commit trailer: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## Task 1 — Mount agent chats `visibility:hidden` (keep-alive) + gate auto-revive on visibility [SMALLEST DE-RISKER; DO FIRST]
Fixes tab-switch remount churn (acceptance d). This is the structural change and the cheapest proof the approach works.

**Files:** `web/src/features/panes/components/pane-container.tsx`, `web/src/features/agent/components/agent-chat-pane.tsx`

**Changes:**
1. `pane-container.tsx`: add an always-mounted block for `type==='agentChat'` mirroring the terminal block at 610-633 — each chat buffer in an `absolute inset-0` div, `visibility:hidden` unless it's the active tab, EACH wrapped in its OWN `Suspense fallback={null}` (AgentChatPane is lazy; a chunk load must not unmount sibling terminals — same reasoning as the comment at 605-609). Pass `isActivePane` and a new `isVisible={isActive}` (active TAB within the pane).
2. `pane-container.tsx:635-637`: exclude agentChat from the active-only Suspense (`activeBuffer.type !== 'terminal' && activeBuffer.type !== 'agentChat'`); remove the `case 'agentChat'` from `renderActiveBuffer` (511-525).
3. `agent-chat-pane.tsx`: add `isVisible: boolean` to props (70-79). Thread: `isActive={isActivePane && isVisible}` and `isVisible={isVisible}` on `<XtermTerminal>` (was `isActive={isActivePane}`, 535-538). **Gate auto-revive on `isVisible`** in the attach effect (372-392): a hidden DORMANT chat (`sessionId===''`) must NOT spawn a CLI — only revive when `isVisible`; a hidden dormant chat sits `idle`/`pending`. Already-attached chats stay attached while hidden (keep-alive). This preserves "opening a dormant chat revives it" (switching to its tab makes it visible) while preventing N hidden dormant tabs from each spawning a CLI on workspace load.

**Unit tests:** inactive agentChat buffer is mounted with `visibility:hidden`; `isVisible=false` + dormant → revive NOT called; `isVisible` false→true on a dormant chat → exactly one revive (respect `attemptedRef` budget). Mock `agent-api`.

**Live acceptance (orchestrator):** two chats A/B in one pane; switch tabs A→B→A; A's terminal DOM node persists (no remount — verify via a mount counter exposed on `window` and incremented in `initializeTerminal`, must not increase across the round-trip) and A is not blank. Workspace with 3+ dormant chat tabs, one active → only the active tab's CLI spawns (check `ipc_get_backend_state`/daemon).

---

## Task 2 — FORBID duplicating a chat into a second pane (reveal the existing view)
Fixes duplication-blank (acceptance b) at the source.

**Files:** `web/src/features/workspace/stores/slices/buffer-slice.ts`

**Change:** in `openContent`'s dedup hit (~113-116), for `agentChat` AND `terminal` (same latent bug), reveal the existing buffer in the pane that already holds it instead of `addBufferToPane(activePaneId, …)`:
```
if (existing) {
  if (existing.type === 'agentChat' || existing.type === 'terminal') {
    const pane = get().paneActions.getPaneByBufferId(existing.id)
    if (pane) { get().paneActions.setActivePane(pane.id); get().paneActions.activatePaneBuffer(pane.id, existing.id); return existing.id }
  }
  get().paneActions.addBufferToPane(get().activePaneId, existing.id, true) // editors etc. unchanged
  return existing.id
}
```
The drag-to-split path (`moveBufferToPane`, pane-slice 267-275) already removes from source — no duplication there, no change. `openContent`→`addBufferToPane` was the only duplication vector; this closes it.

**Unit tests:** `openContent({type:'agentChat',chatId:X})` when a buffer for X exists in pane P, active pane Q → active becomes P, no second pane holds it, returns existing id. Editor dedup unchanged.

**Live acceptance:** chat in pane A; split so B active; open the same chat from the sidebar → focus jumps to A; B gets no copy; A not blank. Exactly one `[data-terminal-session-id]` node for that session.

---

## Task 3 — Reference-counted `attachOnly` detach (one unmount can't kill a live co-view's transport)
Defense-in-depth for the shared-transport teardown; also removes the detach/attach gap during a pane move.

**Files:** new `web/src/features/terminal/lib/attach-refcount.ts` (framework-free), `web/src/features/terminal/components/terminal.tsx`

**Change:** `attach-refcount.ts`: module-level `Map<connectionId, number>` with `retain(connId)` / `release(connId): boolean` (true when count hits 0 → caller should detach); unknown-id release returns true (safe default). In `terminal.tsx`, **only for `attachOnly`**: after a successful resolve yielding a `connectionId` (init 639-661 and `doReconnect` 218) call `retain(connId)`; in the detach-on-unmount effect (886-892) and the reconciler stale-detach (261-264) replace bare `terminalDetach` with `if (release(connId)) terminalDetach(connId)`. Keyed by **connectionId** (shared PTY handle), not sessionId. Non-`attachOnly` (shell) path untouched.

**Unit tests (`attach-refcount.test.ts`):** retain×2 → release false then true; unknown-id release → true.

**Live acceptance:** rapid drag-to-split of a chat → destination redraws (no blank), source teardown doesn't blank a still-live view; daemon shows exactly one detach.

---

## Task 4 — Single PTY-size owner: only the visible view pushes PTY resize
Belt-and-suspenders (Task 2 removes co-views).

**Files:** `web/src/features/terminal/components/terminal.tsx`

**Change:** thread `isVisible` into the fit path so that when `attachOnly && !isVisible` the terminal fits xterm locally but does NOT push `terminalResize` (pass a no-op `resize` into `refitAndSyncPty` at 374-383 when hidden, so `lastSyncedRef` isn't advanced). Gate strictly on `attachOnly && !isVisible`; shell-tab behavior unchanged. On becoming visible, the active-catch-up fit (1072-1107) re-pushes the correct size.

**Unit test:** extend refit coverage — no-op resize → no push, `lastSyncedRef` not advanced; visible path pushes as today.

**Live acceptance:** two chats (A visible, B hidden); resize pane → only A's connection gets resize IPC; switch to B → B resizes + redraws correctly.

---

## Shell-tab non-regression (explicit — do NOT touch)
- `terminal-tab.tsx`, `terminal-pane.tsx` — untouched.
- `pane-container.tsx:610-633` terminal block — untouched; the agentChat block is ADDED ALONGSIDE it.
- `XtermTerminal` non-`attachOnly` path — every new gate (Tasks 3, 4) conditioned on `attachOnly`; shell init/resolve/reuse/fit/transport-drop byte-for-byte unchanged.
- The dead portal host (`terminal-host.tsx`, `terminal-slots-store.ts`, `XtermPortal`, `ide-shell.tsx:224`) — left exactly as-is.

## Risks + guards
1. Hidden already-attached chats hold a live WS while hidden (cheap); dormant hidden chats spawn nothing (Task 1 gate). 2. Split still remounts the MOVED chat (by design, same as shell tabs); transient blank risk if attachOnly re-attach + snapshot is slow — `refit.ts` (landed) pushes size on dims-differ; if a blank persists it's a snapshot/refit-ordering tweak, contained (Task 5). 3. `gridSlack` under `visibility:hidden` — box still has layout, `getBoundingClientRect` valid; test recompute on hide→show. 4. `isVisible` vs `isActivePane` are distinct — pass `isActive={isActivePane && isVisible}`; test a hidden tab in the active pane doesn't steal focus. 5. Provider-switch under keep-alive — pane mounted throughout; existing `switchingRef`/imperative-reattach guards behave as today; regression-test it. 6. Refcount unknown-id → detach (no leak). 7. Reconnect map keyed by sessionId — unaffected. 8. `onTerminalRef` — survives keep-alive (instance not torn down on tab-switch); verify empty-space focus after a tab round-trip.
