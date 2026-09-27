# Remote Instances: Web/Desktop UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the desktop app hold N daemon connections (local + paired remotes) at once, show every connected host's projects in one flat sidebar list, and add/manage hosts through Settings — using the daemon auth surface and Tauri bridge commands this plan's two sibling plans already build.

**Architecture:** A new `web/src/features/hosts/` module owns a persisted config list (`{id,name,endpoint,pinnedFingerprint}`, local implicit) and a per-host runtime registry (status, mirroring `workspace-store-registry.ts`'s Map-keyed pattern). `apiFetch`/`wsUrl`/the Tauri WS bridge gain an explicit `hostId` parameter (default `'local'`, zero behavior change for every existing call site) instead of reading a single module-level `API_BASE`. `useProjectDataStore` fetches every live host's `/v0/projects` and merges into one tagged list; a project's `hostId` is recorded on its workspace store at creation and is the ONLY source a project-scoped call ever resolves its host from — never an ambient "currently viewed" fallback. Three UI surfaces: the project row (title/subtitle/dot), Create Space's daemon picker, and a Coss UI tree-based remote file picker, all approved via the visual-companion mockups in `.superpowers/brainstorm/`.

**Tech Stack:** React, Zustand, TypeScript, Tailwind, Base UI (`@base-ui/react`), Coss UI (shadcn-CLI-sourced).

**Spec:** `docs/superpowers/specs/2026-09-27-remote-instances-design.md` (§2 component boundaries, §4 project aggregation & routing, §5 UI). This plan's Tauri command calls (`registerRemoteHost`, `storeHostToken`, etc.) are built in `docs/superpowers/plans/2026-09-27-remote-instances-tauri-bridge.md` (Task 7) — confirm that plan's Task 7 is done (or do it as a prerequisite) before this plan's Task 2.

## Global Constraints

- CI checks: `bunx eslint . --max-warnings 0`, `bun run knip`, `bunx tsc --noEmit`, `bunx prettier --check .`, `bun run test:coverage`, `bun run lint:doctor`.
- Test files live in `web/src/__tests__/` mirroring `web/src/`'s structure (e.g. `features/hosts/stores/host-config-store.ts` → `web/src/__tests__/features/hosts/stores/host-config-store.test.ts`); use `@/` imports in tests, never relative `../../`.
- Component files: kebab-case filenames (`host-switcher.tsx`), PascalCase exported component names.
- `useXxxStore((state) => state.field)` narrow selectors only — never a bare `useXxxStore()`. `useXxxStore.getState()` only inside event handlers/`useEffect`, never in render.
- Stores never import from `components/`; side effects (toasts, DOM) live in components watching store state via `useEffect`.
- Global app state → `features/window/stores/` or `features/settings/`; per-workspace state → `features/workspace/stores/`; `lib/store/` is for server-state-adjacent structures (this is why `useProjectDataStore` stays in `lib/store/projects.ts` — this plan extends it there, not a new location).
- No second library for a job already covered: icons `@phosphor-icons/react`, DnD `dnd-kit`, headless UI `@base-ui/react`, highlighting `shiki`. Coss UI is sourced ON TOP of `@base-ui/react` (already the house headless library), not a competing one.
- `≤3 vi.mock` per file; no asserting Tailwind class names or reading rendered text against `src/` literals; no sleeps in tests (`waitFor`, fake timers, or real signals only).

## Review Focus

- A project-scoped action (open a workspace, run git, start a chat) must resolve its host from that project's OWN recorded `hostId` — never from whichever host happens to be visually active/ambient elsewhere in the window. This is the exact bug class already fixed once in this codebase for workspace ids (`project_ambient_wsid_fallback_bug_class`); a new instance of it here is the single most likely way this feature ships a real data-corruption bug.
- A host that goes from up→down while its projects are already listed must not make those rows disappear — they stay, and an action against them fails fast with a clear error instead of hanging.
- The remote file picker and the daemon dropdown must both treat "no remote hosts are reachable" as a real, handled state (empty list / all-disabled), not an empty screen or a crash.
- Adding a host with a fingerprint the user does NOT confirm, or confirms against the wrong value, must not silently proceed — the redeem step must not fire until the fingerprint has been explicitly acknowledged.
- None of this feature's UI (Settings Hosts tab, daemon dropdown, remote file picker) should be reachable in a non-Tauri browser context — remote hosts require the Rust-side pinned-TLS proxy (`docs/superpowers/plans/2026-09-27-remote-instances-tauri-bridge.md`), which does not exist in a plain browser.

---

### Task 1: host config store + runtime registry

**Files:**
- Create: `web/src/features/hosts/types.ts`
- Create: `web/src/features/hosts/stores/host-config-store.ts`
- Create: `web/src/features/hosts/stores/host-runtime-registry.ts`
- Test: `web/src/__tests__/features/hosts/stores/host-config-store.test.ts`
- Test: `web/src/__tests__/features/hosts/stores/host-runtime-registry.test.ts`

**Interfaces:**
- Produces: `Host{id, name, endpoint, pinnedFingerprint}`, `HostStatus = 'unknown' | 'probing' | 'up' | 'down'`, `useHostConfigStore` (persisted Zustand: `hosts: Host[]`, `addHost(h)`, `removeHost(id)`; local's implicit `{id:'local', name:'Local', endpoint:'', pinnedFingerprint:''}` is NEVER stored in this list — it's a constant merged in at read time, so it can't be accidentally removed), `getOrCreateHostRuntime(hostId)`, `getHostRuntime(hostId)`, `destroyHostRuntime(hostId)`, `subscribeHostRuntimeRegistry(cb)`.

- [ ] **Step 1: Write `types.ts`**

```ts
export interface Host {
  id: string
  name: string
  endpoint: string
  pinnedFingerprint: string
}

export type HostStatus = 'unknown' | 'probing' | 'up' | 'down'

/** The implicit local daemon — never persisted, never removable. */
export const LOCAL_HOST: Host = { id: 'local', name: 'Local', endpoint: '', pinnedFingerprint: '' }
```

- [ ] **Step 2: Write the failing config-store test**

```ts
import { beforeEach, describe, expect, it } from 'vitest'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

describe('useHostConfigStore', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
  })

  it('starts with no saved hosts', () => {
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })

  it('addHost appends a host', () => {
    useHostConfigStore.getState().addHost({ id: 'h1', name: 'homelab', endpoint: '1.2.3.4:3737', pinnedFingerprint: 'SHA256:aaaa' })
    expect(useHostConfigStore.getState().hosts).toHaveLength(1)
    expect(useHostConfigStore.getState().hosts[0].id).toBe('h1')
  })

  it('removeHost drops it', () => {
    useHostConfigStore.getState().addHost({ id: 'h1', name: 'homelab', endpoint: 'x', pinnedFingerprint: 'x' })
    useHostConfigStore.getState().removeHost('h1')
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })

  it('addHost never lets id "local" be added — it is implicit, not stored', () => {
    useHostConfigStore.getState().addHost({ id: 'local', name: 'sneaky', endpoint: 'x', pinnedFingerprint: 'x' })
    expect(useHostConfigStore.getState().hosts.find((h) => h.id === 'local')).toBeUndefined()
  })
})
```

- [ ] **Step 3: Run to verify failure, then implement**

```ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Host } from '@/features/hosts/types'

interface HostConfigState {
  hosts: Host[]
  addHost: (host: Host) => void
  removeHost: (id: string) => void
}

export const useHostConfigStore = create<HostConfigState>()(
  persist(
    (set) => ({
      hosts: [],
      addHost: (host) =>
        set((s) => (host.id === 'local' ? s : { hosts: [...s.hosts.filter((h) => h.id !== host.id), host] })),
      removeHost: (id) => set((s) => ({ hosts: s.hosts.filter((h) => h.id !== id) })),
    }),
    { name: 'crowbar.hosts', partialize: (s) => ({ hosts: s.hosts }) },
  ),
)
```

Run: `cd web && bunx vitest run src/__tests__/features/hosts/stores/host-config-store.test.ts`
Expected: PASS.

- [ ] **Step 4: Write the failing runtime-registry test**

Mirror `workspace-store-registry.ts`'s own test file if one exists (check `web/src/__tests__/features/workspace/stores/workspace-store-registry.test.ts`) for the established assertion style on a registry; otherwise use this:

```ts
import { describe, expect, it } from 'vitest'
import {
  getOrCreateHostRuntime,
  getHostRuntime,
  destroyHostRuntime,
  subscribeHostRuntimeRegistry,
} from '@/features/hosts/stores/host-runtime-registry'

describe('host runtime registry', () => {
  it('getOrCreateHostRuntime mints a store with status "unknown"', () => {
    const store = getOrCreateHostRuntime('h1')
    expect(store.getState().status).toBe('unknown')
    destroyHostRuntime('h1')
  })

  it('getOrCreateHostRuntime returns the SAME store on a second call', () => {
    const a = getOrCreateHostRuntime('h1')
    const b = getOrCreateHostRuntime('h1')
    expect(a).toBe(b)
    destroyHostRuntime('h1')
  })

  it('getHostRuntime returns undefined for an unregistered id, without minting one', () => {
    expect(getHostRuntime('never-created')).toBeUndefined()
  })

  it('destroyHostRuntime removes it and notifies subscribers', () => {
    getOrCreateHostRuntime('h1')
    let notified: string | undefined
    const unsubscribe = subscribeHostRuntimeRegistry((change) => {
      notified = change
    })
    destroyHostRuntime('h1')
    expect(notified).toBe('destroyed')
    expect(getHostRuntime('h1')).toBeUndefined()
    unsubscribe()
  })
})
```

- [ ] **Step 5: Run to verify failure, then implement**

```ts
import { create, type StoreApi, type UseBoundStore } from 'zustand'

export type HostStatus = 'unknown' | 'probing' | 'up' | 'down'

interface HostRuntimeState {
  status: HostStatus
  latencyMs: number | null
  setStatus: (status: HostStatus, latencyMs?: number | null) => void
}

export type HostRuntimeStore = UseBoundStore<StoreApi<HostRuntimeState>>

function createHostRuntimeStore(): HostRuntimeStore {
  return create<HostRuntimeState>((set) => ({
    status: 'unknown',
    latencyMs: null,
    setStatus: (status, latencyMs = null) => set({ status, latencyMs }),
  }))
}

const registry = new Map<string, HostRuntimeStore>()

export type HostRuntimeRegistryChange = 'registered' | 'destroyed'
const listeners = new Set<(change: HostRuntimeRegistryChange) => void>()
function notify(change: HostRuntimeRegistryChange): void {
  for (const listener of listeners) listener(change)
}

export function subscribeHostRuntimeRegistry(callback: (change: HostRuntimeRegistryChange) => void): () => void {
  listeners.add(callback)
  return () => {
    listeners.delete(callback)
  }
}

export function getOrCreateHostRuntime(hostId: string): HostRuntimeStore {
  if (!registry.has(hostId)) {
    registry.set(hostId, createHostRuntimeStore())
    notify('registered')
  }
  return registry.get(hostId)!
}

export function getHostRuntime(hostId: string): HostRuntimeStore | undefined {
  return registry.get(hostId)
}

export function destroyHostRuntime(hostId: string): void {
  if (registry.delete(hostId)) notify('destroyed')
}

export function getAllRegisteredHostIds(): string[] {
  return Array.from(registry.keys())
}
```

Run: `cd web && bunx vitest run src/__tests__/features/hosts/stores/host-runtime-registry.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/hosts/types.ts web/src/features/hosts/stores/ web/src/__tests__/features/hosts/stores/
git commit -m "feat(web): host config store and a per-host runtime status registry"
```

---

### Task 2: `apiFetch`/`wsUrl`/Tauri bridge — explicit `hostId`

**Files:**
- Modify: `web/src/lib/api.ts` (read in full first)
- Modify: `web/src/lib/ws/url.ts` (already quoted in full in this plan's research above; re-read before editing)
- Modify: `web/src/lib/crowbar-bridge.ts` (read in full first — it is large; focus on `openSocket`, `openTerminal`, and wherever WS paths are opened)
- Modify: `web/src/lib/ws/tauri-transport.ts` (read in full first)
- Test: `web/src/__tests__/lib/api.test.ts` (extend if present, else create), `web/src/__tests__/lib/ws/url.test.ts` (extend if present, else create)

**Interfaces:**
- Produces: `apiFetch<T>(path: string, init?: RequestInit, hostId?: string): Promise<T>` (default `'local'`), `apiBaseFor(hostId: string): string`, `wsUrl(path: string, hostId?: string): string` (default `'local'`), `TauriWebSocket`'s constructor gains an optional `hostId` threaded down to the `ws_open` invoke call (Tauri bridge plan Task 6's `host_id` param).
- Consumes: `useHostConfigStore` (Task 1), `Host` (Task 1).

- [ ] **Step 1: Write the failing `apiBaseFor`/`apiFetch` tests**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiBaseFor } from '@/lib/api'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

describe('apiBaseFor', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
  })

  it('returns the existing API_BASE for "local"', () => {
    // API_BASE is whatever this test env already resolves it to (window.__CROWBAR__
    // or VITE_API_URL or '') — assert apiBaseFor('local') equals the exported
    // API_BASE constant, not a specific literal string.
    expect(apiBaseFor('local')).toBe(apiBaseFor('local'))
  })

  it('returns a crowbar://<hostId> base for a registered remote host', () => {
    // Only meaningful under Tauri — this test mocks isTauri() to true; see
    // an existing test in this repo that already mocks '@/lib/crowbar-bridge'
    // isTauri and copy its exact mocking shape rather than inventing a new one.
  })

  it('throws for an unknown remote hostId', () => {
    expect(() => apiBaseFor('no-such-host')).toThrow()
  })
})
```

The second test case is deliberately left for you to fill in against this repo's real `isTauri()` mocking convention (search `web/src/__tests__/` for an existing `vi.mock('@/lib/crowbar-bridge'` — there should be at least one, since `isTauri()` gates behavior all over this codebase) — do not invent a new mocking mechanism if one is already established.

- [ ] **Step 2: Run to verify failure**

Run: `cd web && bunx vitest run src/__tests__/lib/api.test.ts`
Expected: FAIL — `apiBaseFor` undefined.

- [ ] **Step 3: Implement in `api.ts`**

Read the file in full first (its head is quoted in this plan's research above — `API_BASE`, `assetURL`, `ApiError`, `apiFetch`/`apiFetchRaw`). Add, near `API_BASE`:

```ts
import { isTauri } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

/**
 * Resolves the API base for a given host. 'local' (the default every
 * existing call site keeps using unchanged) returns the SAME API_BASE this
 * file has always exported — zero behavior change. Any other id must be a
 * registered remote host; resolving one requires the desktop app, since only
 * the Tauri bridge (crowbar://<hostId>) can complete the pinned-TLS dial —
 * see docs/superpowers/plans/2026-09-27-remote-instances-tauri-bridge.md.
 */
export function apiBaseFor(hostId: string): string {
  if (hostId === 'local') return API_BASE
  if (!isTauri()) {
    throw new Error(`remote host "${hostId}" is only reachable from the desktop app`)
  }
  const known = useHostConfigStore.getState().hosts.some((h) => h.id === hostId)
  if (!known) {
    throw new Error(`unknown host "${hostId}"`)
  }
  return `crowbar://${hostId}`
}
```

Find every existing `apiFetch`/`apiFetchRaw` definition in this file and add a trailing `hostId = 'local'` parameter, replacing their internal use of the bare `API_BASE` constant with `apiBaseFor(hostId)`. Do this as a pure signature extension — do not change any existing call site in this task (there are none yet; every current caller omits the new param and keeps working identically, since it defaults to `'local'`).

- [ ] **Step 4: Implement in `ws/url.ts`**

```ts
import { isTauri } from '@/lib/crowbar-bridge'
import { apiBaseFor } from '@/lib/api'

export function wsUrl(path: string, hostId = 'local'): string {
  if (/^wss?:\/\//i.test(path)) return path
  const base = hostId === 'local'
    ? crowbar?.api || import.meta.env.VITE_API_URL || window.location.origin
    : apiBaseFor(hostId)
  const resolved = new URL(path, base)
  if (resolved.protocol === 'https:') resolved.protocol = 'wss:'
  else if (resolved.protocol === 'http:') resolved.protocol = 'ws:'
  return resolved.toString()
}
```

Keep the existing `isWebSocketCapable()` untouched — it answers a question about the LOCAL transport's capability, unrelated to this task.

- [ ] **Step 5: Thread `hostId` through the Tauri WS bridge**

Read `crowbar-bridge.ts`'s `openSocket`/`openTerminal` and `ws/tauri-transport.ts`'s `TauriWebSocket` class in full. Add an optional `hostId` parameter to `openSocket(path: string, hostId = 'local')` and to `TauriWebSocket`'s constructor, threading it into whatever `invoke('ws_open', {...})` call `TauriWebSocket` makes today — add `hostId: hostId === 'local' ? undefined : hostId` to that invoke's argument object, matching the Tauri Bridge plan's Task 6 `ws_open` signature (`host_id: Option<String>`).

- [ ] **Step 6: Run the full existing api/ws test suite plus the new tests**

Run: `cd web && bunx vitest run src/__tests__/lib/api.test.ts src/__tests__/lib/ws/url.test.ts src/__tests__/lib/crowbar-bridge.test.ts`
Expected: every pre-existing test still passes (this is Review Focus's implicit local-path-unaffected check for the web side, mirroring the Tauri plan's own), plus the new ones.

- [ ] **Step 7: `tsc` and lint**

Run: `cd web && bunx tsc --noEmit && bunx eslint src/lib/api.ts src/lib/ws/url.ts src/lib/crowbar-bridge.ts src/lib/ws/tauri-transport.ts --max-warnings 0`

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/api.ts web/src/lib/ws/url.ts web/src/lib/crowbar-bridge.ts web/src/lib/ws/tauri-transport.ts web/src/__tests__/lib/
git commit -m "feat(web): thread an explicit hostId through apiFetch/wsUrl/the Tauri WS bridge"
```

---

### Task 3: health probing (up/down/unreachable)

**Files:**
- Create: `web/src/features/hosts/lib/host-probe.ts`
- Test: `web/src/__tests__/features/hosts/lib/host-probe.test.ts`

**Interfaces:**
- Consumes: `apiFetch` with `hostId` (Task 2), `getOrCreateHostRuntime` (Task 1).
- Produces: `probeHost(hostId: string): Promise<void>` (fetches `/v0/health` for that host, sets its runtime status to `'up'` or `'down'`), `startHostProbing(hostId: string, intervalMs?: number): () => void` (returns a stop function).

- [ ] **Step 1: Write the failing tests**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { probeHost } from '@/features/hosts/lib/host-probe'
import { getOrCreateHostRuntime, destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))
import { apiFetch } from '@/lib/api'

describe('probeHost', () => {
  beforeEach(() => {
    destroyHostRuntime('h1')
    vi.mocked(apiFetch).mockReset()
  })

  it('sets status "up" when the health call succeeds', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce(undefined)
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('up')
  })

  it('sets status "down" when the health call rejects', async () => {
    vi.mocked(apiFetch).mockRejectedValueOnce(new Error('unreachable'))
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('down')
  })

  it('sets status "probing" while the call is in flight', async () => {
    let resolve!: () => void
    vi.mocked(apiFetch).mockReturnValueOnce(new Promise((r) => { resolve = () => r(undefined) }))
    getOrCreateHostRuntime('h1')
    const promise = probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('probing')
    resolve()
    await promise
  })
})
```

- [ ] **Step 2: Run to verify failure, then implement**

```ts
import { apiFetch } from '@/lib/api'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

/** Probes one host's /v0/health and records the result on its runtime store. */
export async function probeHost(hostId: string): Promise<void> {
  const runtime = getOrCreateHostRuntime(hostId)
  runtime.getState().setStatus('probing')
  const started = performance.now()
  try {
    await apiFetch('/v0/health', undefined, hostId)
    runtime.getState().setStatus('up', performance.now() - started)
  } catch {
    runtime.getState().setStatus('down')
  }
}

/** Probes hostId immediately and on an interval; returns a stop function. */
export function startHostProbing(hostId: string, intervalMs = 15_000): () => void {
  void probeHost(hostId)
  const timer = setInterval(() => void probeHost(hostId), intervalMs)
  return () => clearInterval(timer)
}
```

Run: `cd web && bunx vitest run src/__tests__/features/hosts/lib/host-probe.test.ts`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/hosts/lib/host-probe.ts web/src/__tests__/features/hosts/lib/host-probe.test.ts
git commit -m "feat(web): per-host health probing (up/down/probing)"
```

---

### Task 4: pairing dialog (two-stage: address + fingerprint confirm, then code)

**Files:**
- Create: `web/src/features/hosts/lib/pairing.ts`
- Create: `web/src/features/hosts/components/add-host-dialog.tsx`
- Test: `web/src/__tests__/features/hosts/lib/pairing.test.ts`
- Test: `web/src/__tests__/features/hosts/components/add-host-dialog.test.tsx`

**Interfaces:**
- Consumes: `apiFetch` (Task 2, called against a NOT-YET-saved host — see below), `registerRemoteHost`/`storeHostToken` (Tauri bridge plan, `crowbar-bridge.ts`), `useHostConfigStore.addHost` (Task 1), `Dialog`/`DialogContent`/etc. from `@/components/ui/dialog`.
- Produces: `redeemPairingCode(hostId: string, endpoint: string, code: string, name: string): Promise<void>` (registers the host with the bridge, redeems, stores the token — see Step 1's ordering note), `AddHostDialog` component.

- [ ] **Step 1: Write the failing `redeemPairingCode` test**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/crowbar-bridge', () => ({
  registerRemoteHost: vi.fn(),
  storeHostToken: vi.fn(),
  unregisterRemoteHost: vi.fn(),
}))

import { apiFetch } from '@/lib/api'
import { registerRemoteHost, storeHostToken, unregisterRemoteHost } from '@/lib/crowbar-bridge'
import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

describe('redeemPairingCode', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
    vi.mocked(apiFetch).mockReset()
    vi.mocked(registerRemoteHost).mockReset()
    vi.mocked(storeHostToken).mockReset()
    vi.mocked(unregisterRemoteHost).mockReset()
  })

  it('registers with the bridge BEFORE redeeming, so the redeem call can dial the host at all', async () => {
    const order: string[] = []
    vi.mocked(registerRemoteHost).mockImplementation(async () => { order.push('register') })
    vi.mocked(apiFetch).mockImplementation(async () => { order.push('redeem'); return { token: 'tok', clientId: 'c1' } })
    await redeemPairingCode('h1', '1.2.3.4:3737', 'SHA256:aaaa', '123456', 'my-device')
    expect(order).toEqual(['register', 'redeem'])
  })

  it('on success: stores the token and saves the host config', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ token: 'tok', clientId: 'c1' })
    await redeemPairingCode('h1', '1.2.3.4:3737', 'SHA256:aaaa', '123456', 'my-device')
    expect(storeHostToken).toHaveBeenCalledWith('h1', 'tok')
    expect(useHostConfigStore.getState().hosts.find((h) => h.id === 'h1')).toBeTruthy()
  })

  it('on redeem failure: unregisters the host and does NOT save it or store a token', async () => {
    vi.mocked(apiFetch).mockRejectedValueOnce(new Error('bad code'))
    await expect(redeemPairingCode('h1', '1.2.3.4:3737', 'SHA256:aaaa', '000000', 'my-device')).rejects.toThrow()
    expect(unregisterRemoteHost).toHaveBeenCalledWith('h1')
    expect(storeHostToken).not.toHaveBeenCalled()
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })
})
```

- [ ] **Step 2: Run to verify failure, then implement `pairing.ts`**

```ts
import { apiFetch } from '@/lib/api'
import { registerRemoteHost, storeHostToken, unregisterRemoteHost } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

interface PairResponse {
  token: string
  clientId: string
}

/**
 * Redeems a pairing code against a host the user has NOT yet saved. Order
 * matters: the Rust bridge must know this host's endpoint+fingerprint
 * BEFORE the redeem's apiFetch(hostId) call can resolve a crowbar://<hostId>
 * base at all (see docs/.../remote-instances-tauri-bridge.md Task 5). A
 * failed redeem tears the registration back down rather than leaving an
 * unpaired, tokenless host registered in the Rust layer.
 */
export async function redeemPairingCode(
  hostId: string,
  endpoint: string,
  pinnedFingerprint: string,
  code: string,
  name: string,
): Promise<void> {
  await registerRemoteHost(hostId, endpoint, pinnedFingerprint)
  try {
    const { token } = await apiFetch<PairResponse>('/v0/pair', {
      method: 'POST',
      body: JSON.stringify({ code, name }),
      headers: { 'Content-Type': 'application/json' },
    }, hostId)
    await storeHostToken(hostId, token)
    useHostConfigStore.getState().addHost({ id: hostId, name, endpoint, pinnedFingerprint })
  } catch (err) {
    await unregisterRemoteHost(hostId)
    throw err
  }
}
```

Run: `cd web && bunx vitest run src/__tests__/features/hosts/lib/pairing.test.ts`
Expected: PASS.

- [ ] **Step 3: Write the failing `AddHostDialog` test**

Read `@/components/ui/dialog.tsx` in full first (already summarized above — `Dialog`/`DialogContent`/`DialogHeader`/`DialogFooter`/`AppDialog`) and pick whichever of `AppDialog` vs. the composable primitives this repo's OTHER feature dialogs already use for a similar two-stage flow (search for an existing multi-step dialog under `web/src/features/` for precedent before choosing).

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'

vi.mock('@/features/hosts/lib/pairing', () => ({ redeemPairingCode: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))

import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { apiFetch } from '@/lib/api'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'

describe('AddHostDialog', () => {
  it('stage 1: entering a name and address and confirming the fingerprint advances to the code stage', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ fingerprint: 'SHA256:aaaa bbbb cccc dddd eeee ffff' })
    render(<AddHostDialog open onOpenChange={() => {}} />)

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
    fireEvent.click(screen.getByRole('button', { name: /continue|next/i }))

    await waitFor(() => expect(screen.getByText(/SHA256:aaaa bbbb cccc dddd eeee ffff/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    expect(await screen.findByLabelText(/code/i)).toBeInTheDocument()
  })

  it('does NOT call redeemPairingCode until the fingerprint has been confirmed', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ fingerprint: 'SHA256:aaaa' })
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
    fireEvent.click(screen.getByRole('button', { name: /continue|next/i }))
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  it('stage 2: submitting a code calls redeemPairingCode with the confirmed fingerprint', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ fingerprint: 'SHA256:aaaa' })
    vi.mocked(redeemPairingCode).mockResolvedValueOnce(undefined)
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
    fireEvent.click(screen.getByRole('button', { name: /continue|next/i }))
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    fireEvent.change(await screen.findByLabelText(/code/i), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: /pair|connect/i }))

    await waitFor(() => expect(redeemPairingCode).toHaveBeenCalledWith(
      expect.any(String), '1.2.3.4:3737', 'SHA256:aaaa', '123456', 'homelab',
    ))
  })
})
```

- [ ] **Step 4: Run to verify failure, then implement `AddHostDialog`**

Build a two-stage form component: stage 1 collects `name`/`endpoint`, on submit calls `apiFetch<{fingerprint: string}>('/v0/pair/fingerprint-probe-or-similar', ..., newHostId)` — **note:** there is no dedicated "probe fingerprint" daemon endpoint in the daemon plan; the fingerprint is obtained by completing the TLS handshake itself (Task 2's `apiBaseFor` + a bare fetch triggers it), not a JSON field any endpoint returns. Before implementing this component, re-read the daemon plan's §3 and the Tauri bridge plan's Task 5 `forward_remote` to confirm exactly HOW a fingerprint becomes visible to the web layer on first connect — if no mechanism exists yet, add a minimal one as part of THIS step rather than guessing: the simplest correct shape is a new Tauri command `probe_remote_fingerprint(endpoint: string) -> Result<String, String>` that dials the address directly (bypassing the registry, since the host isn't registered yet) and returns whatever fingerprint the presented certificate has, for the UI to display — add this command in `desktop/src-tauri/src/api_proxy.rs` or `tls_pin.rs` (small, additive, its own tiny task-within-a-task with a `#[tauri::command]` and a unit test dialing a local self-signed test server) before wiring the dialog to it. Then stage 2 collects the 6-digit code and calls `redeemPairingCode`.

- [ ] **Step 5: Run to verify the dialog tests pass**

Run: `cd web && bunx vitest run src/__tests__/features/hosts/components/add-host-dialog.test.tsx`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/hosts/lib/pairing.ts web/src/features/hosts/components/add-host-dialog.tsx web/src/__tests__/features/hosts/
git commit -m "feat(web): two-stage add-host dialog — address+fingerprint confirm, then pairing code"
```

---

### Task 5: Settings → Hosts tab

**Files:**
- Modify: `web/src/features/window/stores/ui-state-store.ts` (add `'hosts'` to the `SettingsTab` union)
- Modify: `web/src/features/settings/components/settings-tab-items.ts` (add the tab entry)
- Create: `web/src/features/settings/components/tabs/hosts-settings.tsx`
- Test: `web/src/__tests__/features/settings/components/tabs/hosts-settings.test.tsx`

**Interfaces:**
- Consumes: `useHostConfigStore` (Task 1), `useHostRuntime`/`getOrCreateHostRuntime` (Task 1), `startHostProbing` (Task 3), `AddHostDialog` (Task 4), `unregisterRemoteHost`/`forgetHostToken` (Tauri bridge).

- [ ] **Step 1: Add the tab**

In `ui-state-store.ts`: add `| 'hosts'` to the `SettingsTab` union. In `settings-tab-items.ts`: add `{ id: 'hosts', label: 'Hosts', icon: <pick an appropriate phosphor icon, e.g. HardDrives or Cloud> }` to `SETTINGS_TAB_ITEMS`, gated behind `isTauri()` the same way `developer` is gated behind `import.meta.env.DEV` — per this plan's Review Focus, this whole surface must not appear in a non-Tauri context:

```ts
  ...(isTauri() ? [{ id: 'hosts' as SettingsTabItem['id'], label: 'Hosts', icon: HardDrives }] : []),
```

Import `isTauri` from `@/lib/crowbar-bridge` and the icon from `@phosphor-icons/react`.

- [ ] **Step 2: Write the failing test**

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: () => true, unregisterRemoteHost: vi.fn(), forgetHostToken: vi.fn() }))

import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { HostsSettings } from '@/features/settings/components/tabs/hosts-settings'

describe('HostsSettings', () => {
  it('lists Local plus every saved host', () => {
    useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab', endpoint: '1.2.3.4:3737', pinnedFingerprint: 'SHA256:aaaa' }] })
    render(<HostsSettings />)
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(screen.getByText('homelab')).toBeInTheDocument()
  })

  it('does not offer to revoke or remove the Local entry', () => {
    useHostConfigStore.setState({ hosts: [] })
    render(<HostsSettings />)
    const localRow = screen.getByText('Local').closest('[data-testid="host-row"]')
    expect(localRow?.querySelector('[aria-label*="revoke" i], [aria-label*="remove" i]')).toBeNull()
  })

  it('clicking Add host opens the add-host dialog', () => {
    useHostConfigStore.setState({ hosts: [] })
    render(<HostsSettings />)
    fireEvent.click(screen.getByRole('button', { name: /add.*host/i }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('clicking revoke on a saved host removes it from the config store', () => {
    useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pinnedFingerprint: 'x' }] })
    render(<HostsSettings />)
    const row = screen.getByText('homelab').closest('[data-testid="host-row"]')!
    fireEvent.click(row.querySelector('[aria-label*="revoke" i]')!)
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })
})
```

- [ ] **Step 3: Run to verify failure, then implement `HostsSettings`**

Render a list: `LOCAL_HOST` first (no revoke control), then `useHostConfigStore((s) => s.hosts)` (narrow selector), each row showing name/endpoint and a status dot sourced via `useEffect` + `getOrCreateHostRuntime(host.id)`/`startHostProbing(host.id)` (subscribe in an effect, clean up the interval on unmount — side effects belong in the component per this plan's Global Constraints, not the store). An "Add host" button opens `AddHostDialog` (local `useState` for open/closed, per `AppDialog`'s own convention if that's what Task 4 used). Each non-local row's revoke button calls `unregisterRemoteHost(host.id)`, `forgetHostToken(host.id)`, then `useHostConfigStore.getState().removeHost(host.id)` — in that order, from an event handler (not render).

- [ ] **Step 4: Run to verify the tests pass**

Run: `cd web && bunx vitest run src/__tests__/features/settings/components/tabs/hosts-settings.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/window/stores/ui-state-store.ts web/src/features/settings/components/ web/src/__tests__/features/settings/
git commit -m "feat(web): Settings → Hosts tab — list, add, revoke, live status"
```

---

### Task 6: project row — heading font, host subtitle, status dot

**Files:**
- Modify: whichever component currently renders the project row shown in the approved mockup (`.superpowers/brainstorm/*/content/project-row-v2.html`) — this plan's earlier research identified `web/src/components/sidebar/space-header.tsx` as the project/space header row; confirm by reading it in full (already partially quoted above) and locating the exact `<span>`/label element that renders `project.name` today.
- Test: the existing test file for that component under `web/src/__tests__/components/sidebar/` (extend it).

**Interfaces:**
- Consumes: `Host` (Task 1), `useHostConfigStore` (Task 1), `useHostRuntime`-style read of Task 1's runtime registry for the status dot. Requires `Project.hostId: string` to exist — this plan's Task 8 adds that field; if Task 8 has not landed yet when this task starts, add `hostId?: string` to the `Project`/`ProjectDTO` types here as a minimal forward-declare and let Task 8 make it non-optional and actually populated, OR reorder execution so Task 8 runs before this task. Prefer reordering: do Task 8 before Task 6 if working sequentially.

- [ ] **Step 1: Read the approved mockup and the live component**

Open `.superpowers/brainstorm/*/content/project-row-v2.html` (the last-approved version) for the exact target shape: title in `var(--font-heading)`, a subtitle line below with a status dot (`--success` green / muted grey) + the host's name, "Local" for the local host. Read the live `space-header.tsx` row markup in full.

- [ ] **Step 2: Write the failing test**

```tsx
it('renders the project title in the heading font and a host subtitle with a status dot', () => {
  useHostConfigStore.setState({ hosts: [] })
  const project = { id: 'p1', name: 'Rabbyte Labs', path: '/x', lastActivity: new Date(), hostId: 'local' }
  render(<SpaceHeader project={project} /* ...other required props, copy from an existing test in this file */ />)
  const title = screen.getByText('Rabbyte Labs')
  expect(title).toHaveClass('font-heading') // or whatever Tailwind token class this repo's --font-heading maps to — confirm via an existing font-heading usage (e.g. markdown-editor.css's comment about heading-node.tsx's font-heading Tailwind class) before asserting a specific class name
  expect(screen.getByText('Local')).toBeInTheDocument()
})

it('shows the remote host\'s name and a down-status dot when its runtime is "down"', () => {
  useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab-prox', endpoint: 'x', pinnedFingerprint: 'x' }] })
  getOrCreateHostRuntime('h1').getState().setStatus('down')
  const project = { id: 'p1', name: 'Quiver Sync', path: '/x', lastActivity: new Date(), hostId: 'h1' }
  render(<SpaceHeader project={project} /* ... */ />)
  expect(screen.getByText('homelab-prox')).toBeInTheDocument()
})
```

Fill in the omitted required props by copying an existing `SpaceHeader` render call from this file's current tests.

- [ ] **Step 3: Run to verify failure, then implement**

Change the row's title element to use the heading font token (mirror whatever class/style an EXISTING `--font-heading` consumer in this codebase already uses — e.g. `markdown-editor.css`'s heading rule or a Tailwind `font-heading` utility class, confirmed by grep before picking one) and add a subtitle line below it: a `<span>` with a small colored dot (`bg-success`/token-based, not a raw hex) and the resolving host's name, reading `useHostConfigStore((s) => (project.hostId === 'local' ? LOCAL_HOST : s.hosts.find((h) => h.id === project.hostId)))` (narrow selector) and the runtime status via Task 1's registry (subscribe via `useSyncExternalStore` or a small hook — check whether `workspace-store-registry.ts`'s consumers already have an established hook pattern for subscribing to a registry-housed store from a component, and mirror it rather than inventing a new subscription mechanism).

- [ ] **Step 4: Run to verify the tests pass**

Run: `cd web && bunx vitest run <the extended test file>`

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/space-header.tsx web/src/__tests__/components/sidebar/
git commit -m "feat(web): project row shows its owning host — heading title, status-dot subtitle"
```

---

### Task 7: Create Space — daemon picker

**Files:**
- Modify: the Create-a-Space panel component (the mockup's source — locate via the approved mockup `.superpowers/brainstorm/*/content/create-space-daemon.html` and `web/src/components/projects/` or wherever `create-space-panel`/`add-repository-modal.tsx` lives per this plan's earlier research; read in full first).
- Test: that component's existing test file (extend).

**Interfaces:**
- Consumes: `useHostConfigStore` (Task 1), host runtime status (Task 1), `@/components/ui/select` (Base UI `Select`, per the established wrapping convention read in this plan's research).

- [ ] **Step 1: Read the approved mockup and the live component**

`.superpowers/brainstorm/*/content/create-space-daemon.html` is the target: a "Daemon" field row between Space Name and Folder, a dropdown listing Local (default) + every saved host with its status dot, unreachable hosts listed-but-disabled, plus a trailing "+ Add a host…" entry opening Task 4's `AddHostDialog`.

- [ ] **Step 2: Write the failing test**

```tsx
it('defaults the daemon selection to Local', () => {
  useHostConfigStore.setState({ hosts: [] })
  render(<CreateSpacePanel /* ...existing required props */ />)
  expect(screen.getByRole('combobox', { name: /daemon/i })).toHaveTextContent('Local')
})

it('lists a saved, reachable host as selectable', () => {
  useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab-prox', endpoint: 'x', pinnedFingerprint: 'x' }] })
  getOrCreateHostRuntime('h1').getState().setStatus('up')
  render(<CreateSpacePanel /* ... */ />)
  fireEvent.click(screen.getByRole('combobox', { name: /daemon/i }))
  const option = screen.getByRole('option', { name: /homelab-prox/i })
  expect(option).not.toHaveAttribute('aria-disabled', 'true')
})

it('lists an unreachable host but disabled, not hidden', () => {
  useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'office-vm', endpoint: 'x', pinnedFingerprint: 'x' }] })
  getOrCreateHostRuntime('h1').getState().setStatus('down')
  render(<CreateSpacePanel /* ... */ />)
  fireEvent.click(screen.getByRole('combobox', { name: /daemon/i }))
  expect(screen.getByRole('option', { name: /office-vm/i })).toHaveAttribute('aria-disabled', 'true')
})

it('creating a space passes the selected daemon\'s hostId to the create call', async () => {
  // Copy this repo's existing assertion style for "creating a space calls X
  // with these args" from this file's current tests, adding the hostId
  // expectation to it.
})
```

- [ ] **Step 3: Run to verify failure, then implement**

Add a `Select`/`SelectTrigger`/`SelectContent`/`SelectItem` (from `@/components/ui/select`, Base UI-backed) row between the Space Name field and the Folder field, options = `[LOCAL_HOST, ...useHostConfigStore((s) => s.hosts)]` each rendered with its status dot (reuse Task 6's dot styling if it was extracted to a shared small component; otherwise inline it identically), `disabled` on any option whose runtime status is `'down'`, a trailing item that opens `AddHostDialog`. Thread the selected `hostId` (component-local `useState`, defaulting to `'local'`) into whatever create-project call this panel already makes.

- [ ] **Step 4: Run to verify the tests pass, then `tsc`/lint**

Run: `cd web && bunx vitest run <the extended test file> && bunx tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add <the modified component and its test>
git commit -m "feat(web): Create Space gains a daemon picker (Local + saved hosts)"
```

---

### Task 8: multi-host project aggregation + workspace hostId binding

**Files:**
- Modify: `web/src/lib/types.ts` (add `hostId: string` to `Project` and `ProjectDTO`)
- Modify: `web/src/lib/api.ts` (`fetchProjects` — see below)
- Modify: `web/src/lib/store/projects.ts` (already quoted in full above; re-read before editing)
- Modify: `web/src/features/workspace/stores/workspace-store.ts` and/or wherever a workspace store is FIRST constructed with its owning project's data (read `getOrCreateWorkspaceStore`'s call sites to find where a workspace learns which project it belongs to)
- Test: `web/src/__tests__/lib/store/projects.test.ts` (extend or create)

**Interfaces:**
- Produces: `Project.hostId: string`, `fetchProjects(): Promise<Project[]>` becomes `fetchProjectsForHost(hostId: string): Promise<Project[]>` (fetches that one host's `/v0/projects` via `apiFetch(path, init, hostId)`), a new `fetchAllProjects(): Promise<Project[]>` that calls `fetchProjectsForHost` for `'local'` plus every `up` saved host and concatenates, tagging each result with its `hostId`. `useProjectDataStore`'s `fetcher` becomes `fetchAllProjects`. Every workspace store gains a `hostId` field, set ONCE at construction from its owning project's `hostId` and never re-derived.

- [ ] **Step 1: Write the failing test for multi-host fetch**

```ts
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiFetch: vi.fn() }
})
import { apiFetch } from '@/lib/api'
import { fetchAllProjects } from '@/lib/api'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

describe('fetchAllProjects', () => {
  it('fetches local plus every UP host, tagging each project with its hostId', async () => {
    useHostConfigStore.setState({ hosts: [
      { id: 'h1', name: 'up-host', endpoint: 'x', pinnedFingerprint: 'x' },
      { id: 'h2', name: 'down-host', endpoint: 'x', pinnedFingerprint: 'x' },
    ]})
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    getOrCreateHostRuntime('h2').getState().setStatus('down')
    vi.mocked(apiFetch).mockImplementation(async (_path, _init, hostId) => {
      if (hostId === 'local') return [{ id: 'p-local', name: 'L', path: '/l', lastActivity: new Date().toISOString() }]
      if (hostId === 'h1') return [{ id: 'p-h1', name: 'R', path: '/r', lastActivity: new Date().toISOString() }]
      throw new Error('down-host must not be fetched')
    })

    const projects = await fetchAllProjects()
    expect(projects.find((p) => p.id === 'p-local')?.hostId).toBe('local')
    expect(projects.find((p) => p.id === 'p-h1')?.hostId).toBe('h1')
    expect(projects.find((p) => p.id === 'p-h2')).toBeUndefined()
  })

  it('one host failing to fetch does not fail the others', async () => {
    useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'flaky', endpoint: 'x', pinnedFingerprint: 'x' }] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    vi.mocked(apiFetch).mockImplementation(async (_path, _init, hostId) => {
      if (hostId === 'h1') throw new Error('boom')
      return []
    })
    const projects = await fetchAllProjects()
    expect(projects).toEqual([])
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && bunx vitest run src/__tests__/lib/api.test.ts`

- [ ] **Step 3: Implement**

In `types.ts`: add `hostId: string` to both `Project` and `ProjectDTO`.

In `api.ts`, change `fetchProjects` to take a `hostId` and add `fetchAllProjects`:

```ts
export async function fetchProjectsForHost(hostId: string): Promise<Project[]> {
  const dtos = await apiFetch<ProjectDTO[]>('/v0/projects', undefined, hostId)
  return dtos.map((dto) => ({ ...projectFromDTO(dto), hostId })) // reuse the existing DTO->model mapper this file already has near fetchProjects; confirm its exact name by reading the file (this plan's research flagged lib/store/project-from-dto.ts's projectFromDTO as currently UNUSED by the store — using it here is a legitimate first real caller, not a regression)
}

export async function fetchAllProjects(): Promise<Project[]> {
  const { useHostConfigStore } = await import('@/features/hosts/stores/host-config-store')
  const { getHostRuntime } = await import('@/features/hosts/stores/host-runtime-registry')
  const hosts = ['local', ...useHostConfigStore.getState().hosts.map((h) => h.id)]
  const reachable = hosts.filter((id) => id === 'local' || getHostRuntime(id)?.getState().status === 'up')
  const results = await Promise.allSettled(reachable.map((id) => fetchProjectsForHost(id)))
  return results.flatMap((r) => (r.status === 'fulfilled' ? r.value : []))
}
```

The dynamic `import()`s avoid a circular import between `lib/api.ts` and `features/hosts/` (a `features/` module already imports FROM `lib/api.ts`; a static top-level import the other way would cycle) — if this repo's bundler/lint setup already tolerates a static import here without a cycle (check `bun run knip`/`bunx eslint`'s import-cycle rule, if one exists, before assuming dynamic import is necessary), use a static import instead; keep whichever form the tooling actually requires.

In `projects.ts`: change `useProjectDataStore`'s `fetcher: () => fetchProjects()` to `fetcher: () => fetchAllProjects()`.

- [ ] **Step 4: Run to verify the fetch tests pass**

Run: `cd web && bunx vitest run src/__tests__/lib/api.test.ts`

- [ ] **Step 5: Write the failing test for workspace hostId binding**

```ts
it('a workspace store created for a project records that project\'s hostId', () => {
  const project = { id: 'p1', name: 'X', path: '/x', lastActivity: new Date(), hostId: 'h1' }
  // Copy this repo's existing "how a workspace gets created for a project"
  // call from an existing workspace-store-registry or project-open test,
  // adding the hostId assertion.
  const store = getOrCreateWorkspaceStore(/* ...whatever this call site already needs, now also passing project.hostId */)
  expect(store.getState().hostId).toBe('h1')
})
```

- [ ] **Step 6: Run to verify failure, then implement**

Read `getOrCreateWorkspaceStore` and `createWorkspaceStore` in full (already partially quoted above). Add a `hostId: string` field to the workspace store's state, set from a new required parameter on `createWorkspaceStore`/`getOrCreateWorkspaceStore`, threaded from every call site that currently calls it with a known project (grep every call site; a workspace created with no known project context — the exact ambient-fallback trap this task exists to close — must NOT default to `'local'` silently; if a call site genuinely has no project/hostId available, that is itself a bug to surface, not paper over with a default). Then find every project-scoped `apiFetch`/`wsUrl`/`openSocket` call currently reached from workspace-scoped code (git, terminal, LSP, chat, file tree — the same seams Task 2 touched) and pass that workspace's OWN `hostId` explicitly, never the module-level default.

- [ ] **Step 7: Run the FULL existing workspace-store test suite plus the new test**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/`
Expected: every pre-existing test passes (confirms this task did not silently break local-only workspaces, which must still bind `hostId: 'local'` correctly) plus the new one.

- [ ] **Step 8: `tsc`, lint, knip**

Run: `cd web && bunx tsc --noEmit && bunx eslint . --max-warnings 0 && bun run knip`

- [ ] **Step 9: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts web/src/lib/store/projects.ts web/src/features/workspace/ web/src/__tests__/
git commit -m "feat(web): multi-host project aggregation; every workspace binds its OWN host, never an ambient default"
```

---

### Task 9: Coss UI tree + remote file picker

**Files:**
- Run the Coss UI CLI to add its tree component (creates files under `web/src/components/ui/` — exact paths determined by the CLI's own output).
- Create: `web/src/features/hosts/components/remote-file-picker.tsx`
- Create: `web/src/features/hosts/lib/browse.ts`
- Test: `web/src/__tests__/features/hosts/lib/browse.test.ts`, `web/src/__tests__/features/hosts/components/remote-file-picker.test.tsx`

**Interfaces:**
- Consumes: `GET /v0/system/browse` (daemon plan Task 6), `apiFetch` with `hostId` (Task 2).
- Produces: `browseDirectory(hostId: string, path?: string): Promise<BrowseEntry[]>`, `RemoteFilePicker` component (single-select folder tree, per the approved `.superpowers/brainstorm/*/content/remote-file-picker-v2.html` mockup).

- [ ] **Step 1: Install the Coss UI tree component**

Run: `cd web && pnpm dlx shadcn@latest add <the tree-with-icons component's registry URL from coss.com/ui/docs/tree — confirm the exact URL/command from that page directly before running, since this plan's research did not capture the literal install command for this specific variant>`

If this repo uses `bun` rather than `pnpm` for its shadcn CLI invocations elsewhere (check `web/package.json`'s scripts / any existing `web/components.json` for precedent), use `bunx shadcn@latest add ...` instead — match the existing convention.

Confirm the installed files land under `web/src/components/ui/` (or wherever this repo's `components.json` configures the shadcn CLI to write to) and that `cd web && bunx tsc --noEmit` still passes immediately after install, before writing any of this task's own code — a failing install must be caught here, not blamed on later code.

- [ ] **Step 2: Write the failing `browseDirectory` test**

```ts
vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))
import { apiFetch } from '@/lib/api'
import { browseDirectory } from '@/features/hosts/lib/browse'

describe('browseDirectory', () => {
  it('calls GET /v0/system/browse with the given hostId and path', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce([{ name: 'projects', path: '/home/mateo/projects', isDir: true }])
    const entries = await browseDirectory('h1', '/home/mateo')
    expect(apiFetch).toHaveBeenCalledWith('/v0/system/browse?path=%2Fhome%2Fmateo', undefined, 'h1')
    expect(entries).toHaveLength(1)
  })

  it('omits the path query param when no path is given (daemon defaults to home)', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce([])
    await browseDirectory('h1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/system/browse', undefined, 'h1')
  })
})
```

- [ ] **Step 3: Run to verify failure, then implement**

```ts
import { apiFetch } from '@/lib/api'

export interface BrowseEntry {
  name: string
  path: string
  isDir: boolean
}

export async function browseDirectory(hostId: string, path?: string): Promise<BrowseEntry[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch<BrowseEntry[]>(`/v0/system/browse${query}`, undefined, hostId)
}
```

- [ ] **Step 4: Write the failing `RemoteFilePicker` test**

```tsx
vi.mock('@/features/hosts/lib/browse', () => ({ browseDirectory: vi.fn() }))
import { browseDirectory } from '@/features/hosts/lib/browse'
import { RemoteFilePicker } from '@/features/hosts/components/remote-file-picker'

describe('RemoteFilePicker', () => {
  it('loads the root on mount and renders folders before files', async () => {
    vi.mocked(browseDirectory).mockResolvedValueOnce([
      { name: 'readme.txt', path: '/readme.txt', isDir: false },
      { name: 'projects', path: '/projects', isDir: true },
    ])
    render(<RemoteFilePicker hostId="h1" onSelect={() => {}} />)
    const projectsRow = await screen.findByText('projects')
    const readmeRow = screen.getByText('readme.txt')
    expect(projectsRow.compareDocumentPosition(readmeRow) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('expanding a folder lazily fetches its children — only one call up front', async () => {
    vi.mocked(browseDirectory).mockResolvedValueOnce([{ name: 'projects', path: '/projects', isDir: true }])
    render(<RemoteFilePicker hostId="h1" onSelect={() => {}} />)
    await screen.findByText('projects')
    expect(browseDirectory).toHaveBeenCalledTimes(1)

    vi.mocked(browseDirectory).mockResolvedValueOnce([{ name: 'quiver.core', path: '/projects/quiver.core', isDir: true }])
    fireEvent.click(screen.getByText('projects'))
    await screen.findByText('quiver.core')
    expect(browseDirectory).toHaveBeenCalledTimes(2)
    expect(browseDirectory).toHaveBeenLastCalledWith('h1', '/projects')
  })

  it('clicking a file does not select it (files are inert)', async () => {
    const onSelect = vi.fn()
    vi.mocked(browseDirectory).mockResolvedValueOnce([{ name: 'readme.txt', path: '/readme.txt', isDir: false }])
    render(<RemoteFilePicker hostId="h1" onSelect={onSelect} />)
    fireEvent.click(await screen.findByText('readme.txt'))
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('selecting a folder and confirming calls onSelect with its path', async () => {
    const onSelect = vi.fn()
    vi.mocked(browseDirectory).mockResolvedValueOnce([{ name: 'projects', path: '/projects', isDir: true }])
    render(<RemoteFilePicker hostId="h1" onSelect={onSelect} />)
    fireEvent.click(await screen.findByText('projects')) // select, not expand — see implementation note
    fireEvent.click(screen.getByRole('button', { name: /select/i }))
    expect(onSelect).toHaveBeenCalledWith('/projects')
  })
})
```

- [ ] **Step 5: Run to verify failure, then implement `RemoteFilePicker`**

Build on the Coss UI tree component installed in Step 1 (import from wherever it landed), feeding it `browseDirectory`-sourced nodes with LAZY children (an `undefined`/absent `children` array is "not yet loaded," populated on expand — mirror `file-tree-api.ts`'s own `FileNodeDTO`→`AppFile` lazy convention, already established in this codebase, rather than inventing a different lazy-loading shape), single-select restricted to directories (files render dimmed/inert per the approved mockup), a footer "Select" button that calls `onSelect(selectedPath)`.

- [ ] **Step 6: Run to verify the tests pass, then `tsc`/lint**

Run: `cd web && bunx vitest run src/__tests__/features/hosts/lib/browse.test.ts src/__tests__/features/hosts/components/remote-file-picker.test.tsx && bunx tsc --noEmit`

- [ ] **Step 7: Wire it in where the mockup calls for it**

In the Create Space panel (Task 7) and the repo-import path field (per the design spec §5.3's scope note), when the selected daemon/host is not `'local'`, render `RemoteFilePicker` in place of the native OS folder picker; when `'local'`, keep the existing native picker untouched.

- [ ] **Step 8: Commit**

```bash
git add web/src/components/ui/ web/src/features/hosts/components/remote-file-picker.tsx web/src/features/hosts/lib/browse.ts web/src/__tests__/features/hosts/ web/components.json
git commit -m "feat(web): remote file picker — Coss UI tree, lazy per-directory, replaces native picker for non-local hosts"
```

---

### Task 10: down/up/revoke behavior in the sidebar

**Files:**
- Modify: the component that renders the project's action surface (opening a chat/workspace) to catch a host-unreachable error — locate via how `host-fallback-notice.tsx`-style patterns are found in this codebase today (grep for any existing "unreachable"/error-toast pattern on a project-scoped action first) and extend rather than invent a parallel mechanism.
- Modify: `useProjectDataStore`'s merge/refresh behavior — a host coming back `up` must trigger a re-fetch of ITS projects specifically (not a full-list refetch that would also hit every other host), and a host being removed must apply the existing deleted-project tombstone path to every project it owned.
- Test: extend `web/src/__tests__/lib/store/projects.test.ts`.

**Interfaces:**
- Consumes: `subscribeHostRuntimeRegistry`/host status changes (Task 1/3), `useProjectDataStore.applyDelta` (existing, per `projects.ts`'s `mergeProjectFrame`/`status: 'deleted'` handling already quoted above).

- [ ] **Step 1: Write the failing test for host-removal tombstoning**

```ts
it('removing a host tombstones every project it owned', async () => {
  useProjectDataStore.setState({ data: { status: 'ready', value: [
    { id: 'p1', name: 'A', path: '/a', lastActivity: new Date(), hostId: 'local' },
    { id: 'p2', name: 'B', path: '/b', lastActivity: new Date(), hostId: 'h1' },
  ]}})
  await tombstoneProjectsForHost('h1')
  const remaining = useProjectDataStore.getState().data
  expect(remaining.status === 'ready' && remaining.value.map((p) => p.id)).toEqual(['p1'])
})
```

Adjust the `LoadableSlice`'s exact `data`/`status` shape to match `loadable-slice.ts`'s real `Loadable<T>` type (read it in full — this plan's research only captured `LoadableSlice`'s outer shape, not the inner `Loadable<T>` union) before finalizing this test.

- [ ] **Step 2: Run to verify failure, then implement**

```ts
export async function tombstoneProjectsForHost(hostId: string): Promise<void> {
  const current = useProjectDataStore.getState().data
  if (current.status !== 'ready') return
  for (const project of current.value.filter((p) => p.hostId === hostId)) {
    await useProjectDataStore.getState().applyDelta({ id: project.id, status: 'deleted' })
  }
}
```

Call this from the Hosts settings tab's revoke handler (Task 5), right after `useHostConfigStore.getState().removeHost(id)`.

- [ ] **Step 3: Write the failing test for a host coming back up**

```ts
it('a host transitioning to "up" re-fetches only that host\'s projects', async () => {
  // Wire a subscribeHostRuntimeRegistry-or-per-host-store watcher (component
  // or store-level effect — pick whichever this plan's existing patterns
  // favor, e.g. a small hook mounted once near the app root, mirroring how
  // startHostProbing's own interval is mounted) that calls
  // fetchProjectsForHost(hostId) and merges each result via applyDelta on a
  // 'unknown'|'down' -> 'up' transition. Assert fetchProjectsForHost is
  // called with exactly the transitioning host's id, not refetch-all.
})
```

- [ ] **Step 4: Implement the up-transition watcher and run the full projects test suite**

Run: `cd web && bunx vitest run src/__tests__/lib/store/projects.test.ts`

- [ ] **Step 5: Host-unreachable action error**

Find (or, if genuinely absent, add minimally) the point where opening/acting on a project's workspace makes its first API call; catch a network/connection failure there and surface a clear "host unreachable" message via this codebase's existing toast mechanism (`features/window/stores/toast-store.ts`, already used elsewhere — do not build a new notification surface), rather than letting the action hang or fail silently. Write a test asserting the toast fires when `apiFetch` for that host's `hostId` rejects.

- [ ] **Step 6: `tsc`, lint, full web test suite**

Run: `cd web && bunx tsc --noEmit && bunx eslint . --max-warnings 0 && bun run test:coverage`

- [ ] **Step 7: Commit**

```bash
git add <touched files>
git commit -m "feat(web): host down/up/revoke — projects stay listed until reconciled, revoke tombstones cleanly"
```

---

### Task 11: full-branch gate

- [ ] **Step 1: Full web gate**

Run: `cd web && bunx eslint . --max-warnings 0 && bun run knip && bunx tsc --noEmit && bunx prettier --check . && bun run test:coverage && bun run lint:doctor`
Expected: clean across the board.

- [ ] **Step 2: `.gitignore` check**

Confirm `.superpowers/` is in `.gitignore` (the brainstorming mockups under `.superpowers/brainstorm/` should not ship) — add it if missing.

- [ ] **Step 3: Fix anything the gates surface, then commit**
