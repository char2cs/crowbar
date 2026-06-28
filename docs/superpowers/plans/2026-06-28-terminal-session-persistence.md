# Terminal Session Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make terminal sessions survive workspace switches (PTY kept alive), and — in later phases — survive app close/crash/machine restart (scrollback + CWD restored), implemented entirely in the Go daemon with no external dependency.

**Architecture:** The PTY, ring buffer, multi-client fan-out, and snapshot-replay-on-attach already exist in the daemon (`api/internal/engine/terminal/`). A PTY already survives a WebSocket drop. The work is: (Phase 1) stop the frontend from killing sessions on workspace switch and instead detach the WS + re-attach with replay on re-entry; (Phase 2) add an `active|detached|suspended|exited` lifecycle, idle detection, suspend/restore to disk, limits, and the durable store + DI; (Phase 3) reconcile-on-open + graceful-shutdown flush for restart survival.

**Tech Stack:** Go daemon (gin, gorilla/websocket, GORM + pure-Go SQLite `glebarez/sqlite`, `CGO_ENABLED=0`), React + Zustand + xterm.js frontend, Tauri sidecar, vitest (frontend) + Go `testing` with an `integration` build tag.

**Source spec:** `docs/superpowers/specs/2026-06-27-terminal-session-persistence-design.md` (cured over 3 review rounds).

## Global Constraints

- **Test location (frontend):** all tests live in `web/src/__tests__/` mirroring `web/src/`; use `@/` imports, never relative `../../`. A test for `web/src/lib/crowbar-bridge.ts` goes in `web/src/__tests__/lib/crowbar-bridge.test.ts`.
- **Component file naming:** kebab-case files; exported React component names stay PascalCase.
- **Store rule:** `useXxxStore((s) => s.field)` narrow selectors in render; `useXxxStore.getState()` only in handlers/effects. Stores must not import from `components/`.
- **No legacy migration:** pre-production, no users — never write migration/cleanup for stale persisted state; a schema/version bump clears dev caches instead.
- **Tokens/components:** always `@/components/ui/*` + CSS variable tokens; never hardcode colors.
- **Backend build posture:** `CGO_ENABLED=0` is hardcoded in `docker/Dockerfile` and the release workflows; **no `import "C"`** anywhere — it breaks the cross-compile release matrix (linux/{amd64,arm64} + darwin/{amd64,arm64} + windows/amd64).
- **Backend regression tests** live in `api/tests/integration/` behind the `integration` build tag as `TestRegression_*` — this suite is the v0 contract.
- **Verify-in-Tauri rule:** no change to terminal behavior may be called "working" until sampled live in the running Tauri app (automated tests/tsc/review do not substitute).
- **Two id-spaces (critical):** the **tab/buffer `sessionId`** (frontend, e.g. `terminal-tab-<n>`) is NOT the **daemon `connectionId`** (returned by `terminalCreate`, stored as `Terminal.connectionId` in `useTerminalStore`). The `crowbar-bridge.ts` maps (`terminals`, `tauriTerminals`, `sessionBases`) are all keyed by the **daemon `connectionId`**. Always resolve tab→connectionId via the store before calling a bridge fn.

---

# Phase 1 — Survive Workspace Switch (ship first)

**Outcome:** Opening a terminal in a pane, switching workspaces, and returning leaves the PTY alive and its scrollback intact; a running foreground process keeps running. No disk persistence, no GORM table, no IndexedDB, no launchd.

**Why these changes:** Today `destroyWorkspaceStore` calls `killTerminalSession` for every terminal buffer, which `DELETE`s the PTY. Pane-terminal xterms are destroyed by React on unmount (unlike bottom-panel terminals, which `TerminalHost` parks offscreen with the xterm intact). So for pane terminals we must: detach the WS on switch (close it — backend records the per-client detach, PTY keeps running), and on re-entry open a fresh WS (backend replays the ring snapshot into the new empty xterm → scrollback restored). The connectionId mapping must survive the switch (in-memory `useTerminalStore` is the primary holder; a localStorage mirror is the backstop for a webview reload while the daemon stays up).

## Phase 1 File Structure

- `web/src/features/terminal/utils/osc-parser.ts` — MODIFY: global regex, last match.
- `web/src/lib/crowbar-bridge.ts` — MODIFY: add `terminalDetach(connectionId)` and `terminalAttach(connectionId, base)`.
- `web/src/features/terminal/lib/terminal-reconnect-map.ts` — CREATE: localStorage `{ tabSessionId → connectionId }` per workspace (backstop mapping).
- `web/src/features/terminal/lib/detach-terminal-session.ts` — CREATE: the switch-time detach (mirror of `kill-terminal-session.ts`, but no DELETE).
- `web/src/features/workspace/stores/workspace-store-registry.ts` — MODIFY: `destroyWorkspaceStore` calls detach (not kill) for pane terminals.
- `web/src/features/terminal/components/terminal.tsx` — MODIFY: reconnect path — resolve an existing connectionId (store ∪ localStorage ∪ daemon list) and `terminalAttach` before falling to `createTerminal`.
- Tests mirror under `web/src/__tests__/...`.

---

### Task 1: `parseOSC7` returns the last (newest) directory

**Files:**
- Modify: `web/src/features/terminal/utils/osc-parser.ts`
- Test: `web/src/__tests__/features/terminal/utils/osc-parser.test.ts`

**Interfaces:**
- Produces: `parseOSC7(data: string): string | null` (unchanged signature; now returns the directory from the **last** OSC 7 match in `data`, not the first).

- [ ] **Step 1: Write the failing test**

```typescript
// web/src/__tests__/features/terminal/utils/osc-parser.test.ts
import { describe, it, expect } from 'vitest'
import { parseOSC7 } from '@/features/terminal/utils/osc-parser'

const osc7 = (path: string) => `\x1b]7;file://host${path}\x07`

describe('parseOSC7', () => {
  it('returns the only directory', () => {
    expect(parseOSC7(osc7('/a/b'))).toBe('/a/b')
  })

  it('returns the LAST directory when a burst contains several (replay case)', () => {
    const burst = `${osc7('/old/dir')}some output${osc7('/new/dir')}`
    expect(parseOSC7(burst)).toBe('/new/dir')
  })

  it('decodes percent-encoding', () => {
    expect(parseOSC7(osc7('/a%20b'))).toBe('/a b')
  })

  it('returns null when no OSC 7 present', () => {
    expect(parseOSC7('plain text')).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/terminal/utils/osc-parser.test.ts`
Expected: the "returns the LAST directory" case FAILS (current impl returns `/old/dir`).

- [ ] **Step 3: Implement — global regex, iterate to last match**

```typescript
// web/src/features/terminal/utils/osc-parser.ts
/**
 * Parse OSC 7 sequence for working directory tracking.
 * OSC 7 format: ESC]7;file://hostname/pathBEL
 * Returns the directory from the LAST match in `data` — important for the
 * reconnect replay burst, where the whole ring (many prompts) arrives at once
 * and the newest cwd must win.
 */
export function parseOSC7(data: string): string | null {
  const ESC = String.fromCharCode(0x1b)
  const BEL = String.fromCharCode(0x07)
  const osc7Regex = new RegExp(`${ESC}\\]7;file://[^/]*([^${BEL}]+)${BEL}`, 'g')

  let lastPath: string | null = null
  for (const match of data.matchAll(osc7Regex)) {
    if (match[1]) lastPath = match[1]
  }
  if (lastPath === null) return null
  try {
    return decodeURIComponent(lastPath)
  } catch {
    return lastPath
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/features/terminal/utils/osc-parser.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/terminal/utils/osc-parser.ts web/src/__tests__/features/terminal/utils/osc-parser.test.ts
git commit -m "fix(terminal): parseOSC7 returns the newest cwd (last match) for replay bursts"
```

---

### Task 2: `terminalDetach(connectionId)` — close the WS without DELETE

**Files:**
- Modify: `web/src/lib/crowbar-bridge.ts`
- Test: `web/src/__tests__/lib/crowbar-bridge-detach.test.ts`

**Interfaces:**
- Consumes: the module-level `terminals: Map<connectionId, TerminalConnection>`, `tauriTerminals`, `sessionBases`, `isTauri()`, `tauriInvoke`.
- Produces: `terminalDetach(connectionId: string): Promise<void>` — closes only the PTY WebSocket / Tauri channel and removes the transport map entry; does **not** issue `DELETE`; **keeps** `sessionBases.get(connectionId)` so re-attach can re-dial.

- [ ] **Step 1: Write the failing test**

```typescript
// web/src/__tests__/lib/crowbar-bridge-detach.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

// Force the browser (non-Tauri) path.
vi.mock('@/lib/tauri', () => ({ isTauri: () => false, tauriInvoke: vi.fn() }))

import { terminalCreate, terminalDetach, __getBridgeInternals } from '@/lib/crowbar-bridge'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  readyState = 0
  closed = false
  constructor(public url: string) { FakeWebSocket.instances.push(this); queueMicrotask(() => this.onopen?.()) }
  send = vi.fn()
  close = vi.fn(() => { this.closed = true })
}

beforeEach(() => {
  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
  // apiFetch returns {sessionId} for the POST create
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ sessionId: 'conn-1' }), { status: 201 })))
})

describe('terminalDetach', () => {
  it('closes the WS and drops the transport entry but keeps the DELETE base', async () => {
    const connectionId = await terminalCreate('ws-1')
    const ws = FakeWebSocket.instances[0]

    await terminalDetach(connectionId)

    expect(ws.close).toHaveBeenCalledOnce()
    const internals = __getBridgeInternals()
    expect(internals.terminals.has(connectionId)).toBe(false)   // transport removed
    expect(internals.sessionBases.has(connectionId)).toBe(true) // base kept for re-attach
  })

  it('does NOT call DELETE', async () => {
    const connectionId = await terminalCreate('ws-1')
    const fetchSpy = globalThis.fetch as ReturnType<typeof vi.fn>
    fetchSpy.mockClear()
    await terminalDetach(connectionId)
    const deleteCalls = fetchSpy.mock.calls.filter(([, init]) => (init as RequestInit | undefined)?.method === 'DELETE')
    expect(deleteCalls).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/lib/crowbar-bridge-detach.test.ts`
Expected: FAIL — `terminalDetach` / `__getBridgeInternals` are not exported.

- [ ] **Step 3: Implement `terminalDetach` + test hook**

Add to `web/src/lib/crowbar-bridge.ts` (near `terminalClose`):

```typescript
// Detach the WS transport for a workspace switch: closes the socket (the daemon
// records a per-client detach and keeps the PTY running) WITHOUT issuing DELETE.
// `sessionBases` is intentionally retained so terminalAttach can re-dial later.
export async function terminalDetach(connectionId: string): Promise<void> {
  if (isTauri()) {
    if (tauriTerminals.delete(connectionId)) {
      await tauriInvoke('terminal_close', { sessionId: connectionId }).catch(() => {})
    }
    return
  }
  const conn = terminals.get(connectionId)
  if (conn) {
    conn.ws.close()
    terminals.delete(connectionId)
  }
}
```

And add a test-only accessor at the end of the file (guarded so it is tree-shaken/ignored in prod usage):

```typescript
// Test-only: expose internal maps for unit tests. Do not use in app code.
export function __getBridgeInternals() {
  return { terminals, tauriTerminals, sessionBases }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/lib/crowbar-bridge-detach.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/crowbar-bridge.ts web/src/__tests__/lib/crowbar-bridge-detach.test.ts
git commit -m "feat(terminal): terminalDetach — close PTY WS on switch without killing the daemon session"
```

---

### Task 3: `terminalAttach(connectionId, base)` — open a WS to an existing PTY without POST

**Files:**
- Modify: `web/src/lib/crowbar-bridge.ts`
- Test: `web/src/__tests__/lib/crowbar-bridge-attach.test.ts`

**Interfaces:**
- Consumes: `terminals` map, `sessionBases`, `wsUrl()`, `isTauri()`, `tauriInvoke`, the existing `TerminalConnection` shape + `ws.onopen/onmessage` wiring used by `terminalCreate`.
- Produces: `terminalAttach(connectionId: string, base: string): Promise<void>` — opens the WS to `${base}/${connectionId}/ws` (browser) or invokes Tauri `terminal_open` with that ws path, registers the transport entry so `terminalListen/Write/Resize` stop no-opping, and records `sessionBases.set(connectionId, base)`. Does **not** POST.

- [ ] **Step 1: Write the failing test**

```typescript
// web/src/__tests__/lib/crowbar-bridge-attach.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
vi.mock('@/lib/tauri', () => ({ isTauri: () => false, tauriInvoke: vi.fn() }))
import { terminalAttach, terminalListen, __getBridgeInternals } from '@/lib/crowbar-bridge'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  constructor(public url: string) { FakeWebSocket.instances.push(this); queueMicrotask(() => this.onopen?.()) }
  send = vi.fn()
  close = vi.fn()
}
beforeEach(() => { FakeWebSocket.instances = []; vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket) })

describe('terminalAttach', () => {
  it('opens a WS to the existing session path without POSTing and registers the transport', async () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    const base = '/v0/projects/p/repos/r/workspaces/w/terminals'

    await terminalAttach('conn-1', base)

    expect(fetchSpy).not.toHaveBeenCalled()                         // no POST
    expect(FakeWebSocket.instances[0].url).toContain('/conn-1/ws')  // dialed existing PTY
    expect(__getBridgeInternals().terminals.has('conn-1')).toBe(true)
    expect(__getBridgeInternals().sessionBases.get('conn-1')).toBe(base)
  })

  it('delivers the replay snapshot to a later terminalListen', async () => {
    await terminalAttach('conn-2', '/base')
    const received: string[] = []
    terminalListen('conn-2', (d) => received.push(d))
    FakeWebSocket.instances[0].onmessage?.({ data: JSON.stringify({ data: 'REPLAY' }) })
    expect(received).toContain('REPLAY')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/lib/crowbar-bridge-attach.test.ts`
Expected: FAIL — `terminalAttach` not exported.

- [ ] **Step 3: Implement `terminalAttach`**

Factor the WS-wiring out of `terminalCreate` into a shared helper and call it from both. Add to `crowbar-bridge.ts`:

```typescript
// Wire a browser WebSocket for a connectionId into the `terminals` map.
// (Extracted from terminalCreate so terminalAttach can reuse it.)
function openBrowserSocket(connectionId: string, base: string): void {
  const ws = new WebSocket(wsUrl(`${base}/${encodeURIComponent(connectionId)}/ws`))
  const conn: TerminalConnection = { ws, listener: null, outputBuffer: [], inputQueue: [], open: false }
  ws.onopen = () => { conn.open = true; for (const data of conn.inputQueue) ws.send(JSON.stringify({ data })) }
  ws.onmessage = (event) => {
    let data: string | undefined
    try { data = (JSON.parse(event.data as string) as { data?: string }).data } catch { return }
    if (typeof data !== 'string') return
    if (conn.listener) conn.listener(data)
    else conn.outputBuffer.push(data)
  }
  terminals.set(connectionId, conn)
}

// Attach to an EXISTING daemon PTY (after a workspace switch) without creating a
// new one. The daemon replays its ring snapshot on attach, restoring scrollback.
export async function terminalAttach(connectionId: string, base: string): Promise<void> {
  sessionBases.set(connectionId, base)
  if (isTauri()) {
    const wsPath = `${base}/${encodeURIComponent(connectionId)}/ws`
    tauriTerminals.set(connectionId, { listener: null, outputBuffer: [] })
    await tauriInvoke('terminal_open', { sessionId: connectionId, wsPath })
    return
  }
  openBrowserSocket(connectionId, base)
}
```

Refactor `terminalCreate`'s browser branch to call `openBrowserSocket(sessionId, base)` instead of its inline duplicate (keep behavior identical).

- [ ] **Step 4: Run tests (new + existing bridge tests) to verify pass + no regression**

Run: `cd web && npx vitest run src/__tests__/lib/crowbar-bridge-attach.test.ts src/__tests__/lib/crowbar-bridge-detach.test.ts`
Expected: PASS (4 tests), and the create path still works.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/crowbar-bridge.ts web/src/__tests__/lib/crowbar-bridge-attach.test.ts
git commit -m "feat(terminal): terminalAttach — reconnect to an existing PTY WS (replays ring scrollback)"
```

---

### Task 4: localStorage reconnect map `{ tabSessionId → connectionId }`

**Files:**
- Create: `web/src/features/terminal/lib/terminal-reconnect-map.ts`
- Test: `web/src/__tests__/features/terminal/lib/terminal-reconnect-map.test.ts`

**Interfaces:**
- Produces:
  - `saveReconnect(workspaceId: string, tabSessionId: string, connectionId: string): void`
  - `loadReconnect(workspaceId: string, tabSessionId: string): string | null`
  - `clearReconnect(workspaceId: string, tabSessionId: string): void`
  - storage key: `crowbar:terminal-reconnect:<workspaceId>` → JSON `Record<tabSessionId, connectionId>`.

- [ ] **Step 1: Write the failing test**

```typescript
// web/src/__tests__/features/terminal/lib/terminal-reconnect-map.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { saveReconnect, loadReconnect, clearReconnect } from '@/features/terminal/lib/terminal-reconnect-map'

beforeEach(() => localStorage.clear())

describe('terminal-reconnect-map', () => {
  it('round-trips a mapping', () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    expect(loadReconnect('ws-1', 'tab-1')).toBe('conn-1')
  })
  it('isolates by workspace and tab', () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    expect(loadReconnect('ws-2', 'tab-1')).toBeNull()
    expect(loadReconnect('ws-1', 'tab-2')).toBeNull()
  })
  it('clears a mapping', () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    clearReconnect('ws-1', 'tab-1')
    expect(loadReconnect('ws-1', 'tab-1')).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/terminal/lib/terminal-reconnect-map.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```typescript
// web/src/features/terminal/lib/terminal-reconnect-map.ts
// Backstop mapping of frontend tab id -> daemon connectionId, per workspace.
// In-memory useTerminalStore is the primary holder across a workspace switch;
// this localStorage mirror survives a webview reload while the daemon stays up.
const keyFor = (workspaceId: string) => `crowbar:terminal-reconnect:${workspaceId}`

function read(workspaceId: string): Record<string, string> {
  try {
    const raw = localStorage.getItem(keyFor(workspaceId))
    return raw ? (JSON.parse(raw) as Record<string, string>) : {}
  } catch {
    return {}
  }
}

function write(workspaceId: string, map: Record<string, string>): void {
  try {
    localStorage.setItem(keyFor(workspaceId), JSON.stringify(map))
  } catch {
    /* quota / private mode — best effort */
  }
}

export function saveReconnect(workspaceId: string, tabSessionId: string, connectionId: string): void {
  const map = read(workspaceId)
  map[tabSessionId] = connectionId
  write(workspaceId, map)
}

export function loadReconnect(workspaceId: string, tabSessionId: string): string | null {
  return read(workspaceId)[tabSessionId] ?? null
}

export function clearReconnect(workspaceId: string, tabSessionId: string): void {
  const map = read(workspaceId)
  if (tabSessionId in map) {
    delete map[tabSessionId]
    write(workspaceId, map)
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/features/terminal/lib/terminal-reconnect-map.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/terminal/lib/terminal-reconnect-map.ts web/src/__tests__/features/terminal/lib/terminal-reconnect-map.test.ts
git commit -m "feat(terminal): localStorage tab->connectionId reconnect map"
```

---

### Task 5: Detach (not kill) pane terminals on workspace switch

**Files:**
- Create: `web/src/features/terminal/lib/detach-terminal-session.ts`
- Modify: `web/src/features/workspace/stores/workspace-store-registry.ts:111-119`
- Test: `web/src/__tests__/features/terminal/lib/detach-terminal-session.test.ts`

**Interfaces:**
- Consumes: `useTerminalStore.getState().getSession(tabSessionId)?.connectionId`, `terminalDetach` (Task 2), `saveReconnect` (Task 4), `sessionBases` base lookup.
- Produces: `detachTerminalSession(workspaceId: string, tabSessionId: string): Promise<void>` — looks up the connectionId, persists the reconnect mapping, calls `terminalDetach(connectionId)`, and **keeps** the session in `useTerminalStore` (does NOT `removeSession`, so the in-memory connectionId survives the switch).

- [ ] **Step 1: Write the failing test**

```typescript
// web/src/__tests__/features/terminal/lib/detach-terminal-session.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

const detachSpy = vi.fn(async () => {})
vi.mock('@/lib/crowbar-bridge', () => ({ terminalDetach: detachSpy }))

import { detachTerminalSession } from '@/features/terminal/lib/detach-terminal-session'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { loadReconnect } from '@/features/terminal/lib/terminal-reconnect-map'

beforeEach(() => {
  detachSpy.mockClear()
  localStorage.clear()
  useTerminalStore.setState({ sessions: new Map() })
})

describe('detachTerminalSession', () => {
  it('detaches by connectionId, persists the mapping, and keeps the store entry', async () => {
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'conn-1' })

    await detachTerminalSession('ws-1', 'tab-1')

    expect(detachSpy).toHaveBeenCalledWith('conn-1')                  // by connectionId, not tab id
    expect(loadReconnect('ws-1', 'tab-1')).toBe('conn-1')            // mapping persisted
    expect(useTerminalStore.getState().getSession('tab-1')).toBeTruthy() // store entry kept
  })

  it('no-ops when there is no connectionId', async () => {
    await detachTerminalSession('ws-1', 'tab-unknown')
    expect(detachSpy).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/terminal/lib/detach-terminal-session.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the detach helper**

```typescript
// web/src/features/terminal/lib/detach-terminal-session.ts
import { terminalDetach } from '@/lib/crowbar-bridge'
import { useTerminalStore } from '../stores/terminal-store'
import { saveReconnect } from './terminal-reconnect-map'

// Workspace-switch teardown for a pane terminal: keep the daemon PTY alive,
// just close the WS transport. The in-memory store entry is kept so re-entry
// can reuse the connectionId; a localStorage mirror is the reload backstop.
export async function detachTerminalSession(workspaceId: string, tabSessionId: string): Promise<void> {
  const connectionId = useTerminalStore.getState().getSession(tabSessionId)?.connectionId
  if (!connectionId) return
  saveReconnect(workspaceId, tabSessionId, connectionId)
  await terminalDetach(connectionId)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/features/terminal/lib/detach-terminal-session.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Wire it into `destroyWorkspaceStore`**

In `web/src/features/workspace/stores/workspace-store-registry.ts`, replace the terminal-kill loop (lines 111-119) with detach:

```typescript
const terminalBuffers = buffers.filter((b) => b.type === 'terminal')
if (terminalBuffers.length > 0) {
  void import('@/features/terminal/lib/detach-terminal-session').then(({ detachTerminalSession }) => {
    for (const buf of terminalBuffers) {
      void detachTerminalSession(wsId, (buf as TerminalContent).sessionId).catch(() => {})
    }
  })
}
```

(`wsId` is the workspace id already in scope in `destroyWorkspaceStore`; confirm the parameter name and use it. `killTerminalSession` stays for real tab-close / `exit`.)

- [ ] **Step 6: Typecheck + run the workspace-registry tests**

Run: `cd web && npx tsc --noEmit && npx vitest run src/__tests__/features/workspace`
Expected: PASS / no type errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/terminal/lib/detach-terminal-session.ts web/src/features/workspace/stores/workspace-store-registry.ts web/src/__tests__/features/terminal/lib/detach-terminal-session.test.ts
git commit -m "feat(terminal): detach (not kill) pane terminals on workspace switch"
```

---

### Task 6: Reconnect on workspace re-entry (attach before xterm init)

**Files:**
- Modify: `web/src/features/terminal/components/terminal.tsx:298-327`
- Test: `web/src/__tests__/features/terminal/components/terminal-reconnect.test.tsx` (logic-level test of the resolve-connectionId helper extracted below)

**Interfaces:**
- Consumes: `getSession(tabSessionId)?.connectionId`, `loadReconnect` (Task 4), `terminalAttach` (Task 3), the workspace terminals `base` (`${workspaceBase(wsId)}/terminals`), the existing daemon `GET …/terminals` list (returns live connectionIds for the workspace), `createTerminal` (existing).
- Produces: `resolveTerminalConnection(args): Promise<{ connectionId: string; reused: boolean }>` — extracted pure-ish helper used by `terminal.tsx` before init. Reuse order: (1) `existingSession.connectionId` in store; (2) `loadReconnect(...)` validated against the daemon list; if found → `terminalAttach` + `reused: true`; else → `createTerminal` + `reused: false`.

- [ ] **Step 1: Write the failing test (resolve helper)**

```typescript
// web/src/__tests__/features/terminal/components/terminal-reconnect.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'

const attachSpy = vi.fn(async () => {})
const createSpy = vi.fn(async () => 'fresh-conn')
const listSpy = vi.fn(async () => ['conn-1'])  // daemon says conn-1 is alive
vi.mock('@/lib/crowbar-bridge', () => ({ terminalAttach: attachSpy }))

import { resolveTerminalConnection } from '@/features/terminal/components/resolve-terminal-connection'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'

beforeEach(() => { attachSpy.mockClear(); createSpy.mockClear(); listSpy.mockClear(); localStorage.clear() })

describe('resolveTerminalConnection', () => {
  it('reuses a store connectionId without attaching or creating', async () => {
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: 'conn-store',
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(r).toEqual({ connectionId: 'conn-store', reused: true })
    expect(attachSpy).not.toHaveBeenCalled()
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('attaches to a persisted connectionId that the daemon confirms is alive', async () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: undefined,
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(attachSpy).toHaveBeenCalledWith('conn-1', '/base')
    expect(r).toEqual({ connectionId: 'conn-1', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('creates fresh when the persisted connectionId is no longer alive', async () => {
    saveReconnect('ws-1', 'tab-1', 'dead-conn')
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: undefined,
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/terminal/components/terminal-reconnect.test.tsx`
Expected: FAIL — `resolve-terminal-connection` module not found.

- [ ] **Step 3: Implement the resolve helper**

```typescript
// web/src/features/terminal/components/resolve-terminal-connection.ts
import { terminalAttach } from '@/lib/crowbar-bridge'
import { loadReconnect, clearReconnect } from '../lib/terminal-reconnect-map'

interface ResolveArgs {
  workspaceId: string
  tabSessionId: string
  storeConnectionId: string | undefined
  base: string
  listLiveSessions: () => Promise<string[]>
  createTerminal: () => Promise<string>
}

// Decide whether to reuse, re-attach to, or freshly create the daemon PTY for a
// terminal tab. Reuse order: in-memory store > persisted+daemon-confirmed > fresh.
export async function resolveTerminalConnection(
  args: ResolveArgs,
): Promise<{ connectionId: string; reused: boolean }> {
  if (args.storeConnectionId) return { connectionId: args.storeConnectionId, reused: true }

  const persisted = loadReconnect(args.workspaceId, args.tabSessionId)
  if (persisted) {
    const live = await args.listLiveSessions().catch(() => [] as string[])
    if (live.includes(persisted)) {
      await terminalAttach(persisted, args.base)
      return { connectionId: persisted, reused: true }
    }
    clearReconnect(args.workspaceId, args.tabSessionId) // stale — daemon no longer has it
  }
  const fresh = await args.createTerminal()
  return { connectionId: fresh, reused: false }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/features/terminal/components/terminal-reconnect.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Wire into `terminal.tsx`**

Replace the create-vs-reuse block (`terminal.tsx:298-327`) so it calls `resolveTerminalConnection` BEFORE xterm init, passing the live-session lister (a thin wrapper over the existing `GET ${base}` discovery) and the existing `createTerminal` closure. Set `connectionId` in `useTerminalStore` (`updateSession`) and the local `activeConnectionId` from the result. When `reused` is true, set `reuseExistingConnection` so the initial command is not re-sent. Keep all other behavior. The ordering requirement (populate `connectionId` before `XtermTerminal` mounts) is satisfied because this block already runs before init in the existing effect.

- [ ] **Step 6: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/terminal/components/resolve-terminal-connection.ts web/src/features/terminal/components/terminal.tsx web/src/__tests__/features/terminal/components/terminal-reconnect.test.tsx
git commit -m "feat(terminal): reconnect to live PTY on workspace re-entry (replay scrollback)"
```

---

### Task 7: Live Tauri verification (Phase 1 acceptance)

**Files:** none (verification only).

- [ ] **Step 1: Rebuild the daemon sidecar and launch the app** (per the daemon dev-restart procedure: `go build -tags noEmbed` into `desktop/src-tauri/binaries/` + `target/debug`, restart on the fixed socket; FE auto-reconnects).
- [ ] **Step 2:** Open a pane terminal, run a long-lived process (e.g. `ping localhost` or `npm run dev`), note the scrollback.
- [ ] **Step 3:** Switch to another workspace, then switch back.
- [ ] **Step 4: Verify acceptance:** the terminal still shows prior scrollback, the process is still running/producing output, the prompt is responsive, and no second PTY was spawned (`/tmp/crowbar-daemon.log` shows no new create for that tab). Confirm no leaked WS (browser devtools / daemon connection count returns to baseline after switch).
- [ ] **Step 5:** Drive editor/terminal content via disk writes where MCP keystroke injection into WKWebView is unavailable (known harness limitation). Capture a screenshot for the record.
- [ ] **Step 6: Commit** any fixes found during live verification with `fix(terminal): …`.

---

# Phase 2 — Idle Suspend / Restore + Durable Store + DI

**Outcome:** long-lived detached idle sessions are suspended to disk (PTY killed, scrollback + CWD saved) under per-workspace soft limit / global hard ceiling, and transparently restored on re-entry; lifecycle states reach the UI. Introduces the durable `view.db` store and the engine DI required for suspend/restore.

**Ship gate:** Phase 1 verified in production first.

## Phase 2 File Structure & Tasks (task-level; expand to TDD steps at execution)

- **Task 2.1 — `TerminalSession` GORM model + store.** Create `domain.TerminalSession` (`SessionID` pk, `WorkspaceID`, `ProjectID`, `RepoID`, `CWD`, `Shell`, `ProfileID`, `State`, `CreatedAt`, `LastActiveAt`; `TableName() = "terminal_sessions"`), register it in the global `state/view.db` GORM stores (mirror `terminal_profiles` in `usecases/container.go` `GORMStores`). *Test:* store save/get/delete round-trip. *Supersedes D6 — update the D6 comments + the contract test `container_terminals_test.go`.*
- **Task 2.2 — `SessionMetaStore` port + engine DI.** Define `engineterminal.SessionMetaStore` interface (`Save(ctx, Meta) error`, `Delete(ctx, sessionID) error`) and `Meta` struct in the engine package. Change `engineterminal.New()` → `New(meta SessionMetaStore)`; thread it from `engine/container.go` (which must now receive it) and implement it in the terminal usecase over the GORM store. Resolve+thread the per-workspace storage path into `Create` via `worktreepath.StorageDir` using the `workspace_locations` index already reachable from the usecase. *Test:* engine calls `Save` on create/detach; `Delete` on Kill — assert via a fake store.
- **Task 2.3 — ring `Flush`/`Load` + resize.** Add `Flush(io.Writer) error` / `Load(io.Reader) error` to `ring.go`; bump `defaultRingSize` to 256KB (configurable). *Test:* extend `ring_test.go` with a flush→load round-trip including wrap-around.
- **Task 2.4 — session lifecycle fields + safety.** Add `state`, `IsLive()` (`ptmx != nil`), `NewPlaceholder(...)`, `AttachedCount()`, `isIdleLocked()`/`IsIdle()` (tcgetpgrp), `suspending` flag, `flushMu`, mutable `cwd`/`shell`/`profileId`. Make the pump hold `s.mu` across `ring.Write`+`fanOut` (fixes the replay/live duplication race). Make `Kill` take `s.mu` only around `ptmx.Close`/`Process.Kill` then release before `shutdown()`; short-circuit `Kill`/`shutdown`/`isIdleLocked` when `ptmx == nil`. Add OSC 7 scan in the pump updating `cwd`. *Tests (extend `session_test.go`, run under `-race`):* `IsIdle` with/without a foreground child; placeholder `Kill` no-panic; replay→live handoff produces no duplicate/lost bytes.
- **Task 2.5 — engine Suspend/Restore/auto-suspend/reap.** Refactor `Create` → private `spawn(id, …)`; make `Attach` restore-aware (`Restore` when `!IsLive`, guarded/idempotent, `SessionExists` counts placeholders so no 404); add `Suspend` (re-verify `clients==0 && !suspending && idle` under `s.mu`, set `suspending`, kill process, flush `.buf` + `metaStore.Save state=suspended`, suppress `ended`), `Restore` (re-register same id, `Load` `.buf`, reset state). Add the 10s auto-suspend sweep (soft limit per workspace + global ceiling: evict idle by `lastActiveAt`; force-suspend last-resort for the global ceiling only). Make `reapOnDone` honor `suspending`; on real exit/Kill do `metaStore.Delete` + `.buf` delete; capture `cmd.Wait()` exit code. *Tests (integration, `TestRegression_*`):* detach→re-attach replay; suspend→restore in saved CWD with no `ended` frame; running detached session never auto-suspended.
- **Task 2.6 — hardened atomic persistence helper.** A `persistence` helper writing `.buf` via `os.CreateTemp(dir, sessionId+".buf-*")` → write → `tmp.Sync()` → close → `os.Rename` → parent-dir `fsync`; serialized per session via `flushMu`. *Test:* concurrent flushers never corrupt; torn-write leaves prior good file.
- **Task 2.7 — cadence flush.** Per-session 10s ticker (stopped on `s.Done()`) that, when there is un-flushed output, writes `.buf` + `metaStore.Save` (cwd/state/lastActiveAt). *Test:* row + buf updated within the interval.
- **Task 2.8 — wire protocol + snapshot re-source.** Extend status to `active|detached|suspended|ended` (+ `exitCode?`) in `dto/terminal.go` and `web/src/lib/types.ts`; re-source `terminalsSnapshot` / `ListSessions` from store∪registry emitting real state (replace the three hardcoded `"active"` sites); push frames on detach transition (inside `Attach`), `Suspend`, `Restore`, reap. *Tests:* update the contract assertions that currently expect `"active"`.
- **Task 2.9 — limits + observability + UI states.** Global ceiling counters on the registry; settings "daemon status" row (counts + ring bytes); terminal-tab indicators `Live`/`Suspended`/`Dead` fed by status frames (use `@/components/ui/*` + tokens). *Tests:* component renders each state; ceiling eviction picks the oldest `lastActiveAt`.
- **Task 2.10 — live Tauri verification** of suspend/restore (idle shells suspend over the limit and restore on re-entry; a running detached process is never auto-suspended).

# Phase 3 — Persistence Across Daemon Restart

**Outcome:** after app quit/relaunch or machine reboot, sessions restore (fresh shell, saved CWD, replayed scrollback); graceful shutdown loses ≤ the flush interval. (launchd deferred.)

## Phase 3 Tasks (task-level)

- **Task 3.1 — `LoadPersistedSessions` + reconcile-on-open** in the terminal usecase: enumerate `TerminalSession` rows, resolve each storage dir via `worktreepath.StorageDir`, register PTY-less placeholders (`NewPlaceholder` with ring pre-loaded from `.buf`), drop rows whose `.buf` is missing/corrupt (log + skip, never block startup). Called on daemon start. *Test (integration):* a restart reloads suspended sessions; a re-attach restores; orphaned rows are dropped.
- **Task 3.2 — graceful shutdown flush.** `Terminal.Shutdown()` flushes every session's `.buf` + `metaStore.Save` BEFORE kill; no unconditional kill-all of detached/suspended; placeholders treated as already-suspended. *Test:* `Shutdown` persists then exits; a subsequent load restores.
- **Task 3.3 — Tauri graceful terminate.** Replace `child.kill()` on window close (`desktop/src-tauri/src/lib.rs`) with a SIGTERM + bounded await so `main.go`'s handler runs `Container.Close`. *Verify:* Cmd-Q then relaunch restores scrollback; live Tauri.
- **Task 3.4 — live Tauri verification** of quit/relaunch and reboot restore.

---

## Self-Review

- **Spec coverage:** Phase 1 covers the showstopper (stop-kill + detach + attach-with-replay + OSC7 + reconnect map). Phases 2–3 cover lifecycle/idle/suspend/restore, durable store + DI (`SessionMetaStore`), wire protocol, limits/observability, restart reconcile, graceful shutdown, Tauri SIGTERM — each spec section maps to a task. The deferred items (launchd, pure-Go `proc_info` CWD fallback) are documented in the spec's *What This Does NOT Solve* and intentionally not tasked.
- **Placeholder scan:** Phase 1 steps contain real code + real signatures grounded in the current source; Phases 2–3 are intentionally task-level (to be expanded to TDD steps against live code at execution, per the ship-gate) — not placeholders within an executing task.
- **Type consistency:** `terminalDetach(connectionId)`, `terminalAttach(connectionId, base)`, `resolveTerminalConnection({...})`, `detachTerminalSession(workspaceId, tabSessionId)`, and the reconnect-map signatures are used identically across tasks. The two id-spaces (tab `sessionId` vs daemon `connectionId`) are stated in Global Constraints and respected in every task.

## Execution

Per the cured spec's phasing and the verify-in-Tauri rule: execute **Phase 1 end-to-end first** (subagent-driven, one task per subagent, review between), verify live in Tauri, then gate Phases 2–3 on that verification.
