# Remote Instances: Web/Desktop UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the desktop app hold N daemon connections (local + paired remotes) at once, show every connected host's projects in one flat sidebar list, and add/manage hosts through Settings — using the daemon auth surface and Tauri bridge commands this plan's two sibling plans already build.

**Architecture:** A new `web/src/features/hosts/` module owns a persisted config list (`{id,name,endpoint,pairedFingerprint}`, local implicit) and a per-host runtime registry (status, mirroring `workspace-store-registry.ts`'s Map-keyed pattern). `apiFetch`/`wsUrl`/the Tauri WS bridge gain an explicit `hostId` parameter (default `'local'`, zero behavior change for every existing call site) instead of reading a single module-level `API_BASE`. `useProjectDataStore` fetches every live host's `/v0/projects` and merges into one tagged list; a project's `hostId` is recorded on its workspace store at creation and is the ONLY source a project-scoped call ever resolves its host from — never an ambient "currently viewed" fallback. Three UI surfaces: the project row (title/subtitle/dot), Create Space's daemon picker, and a Coss UI tree-based remote file picker, all approved via the visual-companion mockups in `.superpowers/brainstorm/`.

**Tech Stack:** React, Zustand, TypeScript, Tailwind, Base UI (`@base-ui/react`), Coss UI (shadcn-CLI-sourced).

**Spec:** `docs/superpowers/specs/2026-09-27-remote-instances-design.md` (§2 component boundaries, §4 project aggregation & routing, §5 UI). This plan's Tauri command calls are built in `docs/superpowers/plans/2026-09-27-remote-instances-tauri-bridge.md` (now fully complete, including its own final review and fix round) — confirm before this plan's Task 2.

> **READ FIRST — Replan (2026-10-09, after rebasing onto develop).** Task 8 Steps 5–6 and Task 8b are SUPERSEDED, Task 10 is re-scoped, Task 9 gains a wiring step, and Tasks 12–19 are new (see "## Replan" at the bottom of this file). Execution order: 8 (Steps 1–4, done) → 12 → 20 → 13 → 14 → 15 → 16 → 17 → 18 → 6 → 9 → 10 → 11 → whole-plan review → 19. Where this banner conflicts with older text below, this banner wins.

## Amendment (written before Task 1 dispatch — the Tauri bridge plan's real command surface diverged from this draft)

This plan was drafted before the Tauri bridge plan's Tasks 5/5b/6-8 (and that plan's final-review fix round) landed. The command surface below is what actually exists now in `web/src/lib/crowbar-bridge.ts`; every reference to `storeHostToken` or a 3-argument `registerRemoteHost` anywhere below (including inside code samples) is the STALE original draft and must not be followed literally:

- `registerRemoteHost(hostId: string, endpoint: string): Promise<void>` — **2 arguments, no fingerprint.** The trusted fingerprint now lives ONLY in the OS keychain, bound at pairing time to the real TLS handshake `pair_remote_host` itself performed — never re-suppliable by JS or this store afterward. Do not add a fingerprint parameter back.
- `unregisterRemoteHost(hostId: string): Promise<void>`, `forgetHostToken(hostId: string): Promise<void>` — unchanged from the original draft.
- `probeRemoteFingerprint(endpoint: string): Promise<string>` — the stage-1 TOFU dial. Dials the address directly (unpinned, first-contact-only), returns the presented certificate's fingerprint for display. **The caller must show this to the user and require explicit confirmation before ever calling `pairRemoteHost`.**
- `pairRemoteHost(hostId: string, endpoint: string, fingerprint: string, code: string, name: string): Promise<string>` — replaces the old `registerRemoteHost`+`apiFetch('/v0/pair')`+`storeHostToken` sequence entirely. `fingerprint` MUST be the user-confirmed value from `probeRemoteFingerprint`. Internally this one call: dials pinned against that confirmed fingerprint, redeems the code over that channel, stores the token AND the full (untruncated) confirmed fingerprint together in the keychain, and registers the host's endpoint — there is no separate `apiFetch('/v0/pair', ...)` call for the web layer to make; pairing does not go through the generic HTTP proxy at all. **Returns the daemon's `clientId`** (not a secret — safe to store alongside the host's config), needed for the spec-mandated server-side revoke (§3: "Revoke ... marks the device permanently revoked server-side"). `storeHostToken` does not exist as a callable command — do not reference it.
- Every proxy-originated response (local or remote) now carries a readable `x-crowbar-proxy` header on request failure, extending the pattern `web/src/lib/api.ts` already has for the local `daemon-unavailable` marker (`PROXY_HEADER = 'x-crowbar-proxy'`, checked at `apiFetchRaw`'s error-status branch). Two new values exist: `host-unreachable` (dial/handshake failed — transient, matches the existing "down" concept) and `host-identity-changed` (the presented certificate's fingerprint no longer matches what's pinned in the keychain — NOT transient, must never be silently retried or folded into a generic "unreachable" toast; spec §3: "a mismatch is a hard reject with a clear 'this host's identity changed' warning, never a silent fall-through"). Task 2 below is amended to add these; Task 3 and Task 10 are amended to branch on them.
- Spec §3 also requires: "A 401 mid-session on an otherwise-'up' host marks it needs-repair rather than silently retrying." `HostStatus` (Task 1) needs a `'needs-repair'` value; a 401 on an already-`'up'` host's request should NOT be treated as a generic connectivity failure or silently retried.
- The daemon's `DELETE /v0/clients/:id` (already shipped, daemon plan) needs the `clientId` `pairRemoteHost` now returns — Task 1's `Host` type gains a `clientId` field, and Task 5's revoke handler is amended to call it server-side before the local cleanup. This was previously a gap in this draft (it only unregistered/forgot the token locally, never revoking server-side) — spec §3 requires the server-side revoke, not just a local forget.
- `pairedFingerprint` (the value the user confirmed at pairing time) stays in `Host` for **display only** — spec §3's persisted shape (`{id, name, endpoint, pairedFingerprint}`) still applies for showing the user what they paired against, but it now carries no enforcement weight and is never sent back to any Tauri command. Named `pairedFingerprint` (not the spec prose's `pinnedFingerprint`) throughout this plan to make that non-authoritative role explicit and avoid confusion with the keychain-bound value that actually gates dials.
- Out of scope for this amendment, carried forward as a real open question for Task 2's implementer to check (not prescribed, since it may already be moot): before the Tauri bridge plan's CORS fix, a remote-host failure surfaced as an opaque `fetch()` rejection, which `apiFetchRaw`'s existing retry loop (`DEFAULT_RETRY = {attempts: 8, ...}`) treats as a retryable transient transport failure — worst case ~8 retries before surfacing. Post-fix, a proxy-originated failure resolves with a real HTTP status (502/503/401) and a readable marker, which should already route through the *error-status* branch (line ~162 of `api.ts`, "a real daemon response, not a transport failure") rather than the retry branch. Task 2 should verify this is actually true for `host-unreachable`/`host-identity-changed` specifically (write a test), and only add explicit retry-skip logic if it finds the retry loop still fires for these.

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
- Produces: `Host{id, name, endpoint, pairedFingerprint, clientId}`, `HostStatus = 'unknown' | 'probing' | 'up' | 'down' | 'needs-repair'`, `useHostConfigStore` (persisted Zustand: `hosts: Host[]`, `addHost(h)`, `removeHost(id)`; local's implicit `{id:'local', name:'Local', endpoint:'', pairedFingerprint:'', clientId:''}` is NEVER stored in this list — it's a constant merged in at read time, so it can't be accidentally removed), `getOrCreateHostRuntime(hostId)`, `getHostRuntime(hostId)`, `destroyHostRuntime(hostId)`, `subscribeHostRuntimeRegistry(cb)`.

- [ ] **Step 1: Write `types.ts`**

```ts
export interface Host {
  id: string
  name: string
  endpoint: string
  /** Display only — what the user confirmed at pairing time. Never sent to
   *  any Tauri command; the keychain-bound fingerprint is what actually
   *  gates dials (see this plan's Amendment). */
  pairedFingerprint: string
  /** The daemon's client id, returned by `pairRemoteHost`. Needed to revoke
   *  server-side (`DELETE /v0/clients/:id`) on removal — see Task 5. */
  clientId: string
}

/** 'needs-repair': a 401 on an otherwise-'up' host mid-session (spec §3) —
 *  distinct from 'down' so the UI doesn't silently retry a dead credential. */
export type HostStatus = 'unknown' | 'probing' | 'up' | 'down' | 'needs-repair'

/** The implicit local daemon — never persisted, never removable. */
export const LOCAL_HOST: Host = { id: 'local', name: 'Local', endpoint: '', pairedFingerprint: '', clientId: '' }
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
    useHostConfigStore.getState().addHost({ id: 'h1', name: 'homelab', endpoint: '1.2.3.4:3737', pairedFingerprint: 'SHA256:aaaa' })
    expect(useHostConfigStore.getState().hosts).toHaveLength(1)
    expect(useHostConfigStore.getState().hosts[0].id).toBe('h1')
  })

  it('removeHost drops it', () => {
    useHostConfigStore.getState().addHost({ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x' })
    useHostConfigStore.getState().removeHost('h1')
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })

  it('addHost never lets id "local" be added — it is implicit, not stored', () => {
    useHostConfigStore.getState().addHost({ id: 'local', name: 'sneaky', endpoint: 'x', pairedFingerprint: 'x' })
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

- [ ] **Step 5b: Recognize the two remote proxy markers**

`api.ts` already has a marker-checking pattern for the local case — read it in full: `PROXY_HEADER = 'x-crowbar-proxy'`, checked at the error-status branch inside `apiFetchRaw` (`res.headers?.get(PROXY_HEADER) === 'daemon-unavailable'` → throws `ApiError` with `DAEMON_UNAVAILABLE_CODE`). The Tauri bridge plan's proxy now also sets this header to two remote-specific values on a proxy-originated failure: `'host-unreachable'` (dial/handshake failed) and `'host-identity-changed'` (the presented certificate no longer matches what's pinned in the keychain for this host — never treat this as transient, never retry it, never fold it into the same UI path as `host-unreachable`; spec §3 requires a distinct "hard reject ... this host's identity changed" warning).

Extend the same pattern: add `HOST_UNREACHABLE_CODE`/`HOST_IDENTITY_CHANGED_CODE` constants and corresponding `isHostUnreachableError`/`isHostIdentityChangedError` type-guard helpers (mirroring the existing `isDaemonUnavailableError`-style helper for the local marker — find its exact name and copy its shape), thrown with the matching `ApiError.code` when the header carries either value.

Also verify (write a test, don't just assume) whether `apiFetchRaw`'s transient-retry loop (`DEFAULT_RETRY`, 8 attempts) fires for a proxy-originated `host-unreachable`/`host-identity-changed` response. The comment at the error-status branch already states "an HTTP error status is a real daemon response, not a transport failure," which suggests a *resolved* response with one of these markers should route through that branch (no retry) rather than the retry branch (which only fires on an actual `fetch()` *rejection*). If your test proves the retry loop still fires for either marker, that's a real bug to fix here (skip the retry for these codes specifically); if it proves the retry loop is already correctly bypassed, that's sufficient — no code change needed beyond the test itself.

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
- Consumes: `apiFetch` with `hostId` (Task 2), `getOrCreateHostRuntime` (Task 1), `isHostIdentityChangedError` (Task 2's Step 5b).
- Produces: `probeHost(hostId: string): Promise<void>` (fetches `/v0/health` for that host, sets its runtime status to `'up'`, `'down'`, or `'needs-repair'`), `startHostProbing(hostId: string, intervalMs?: number): () => void` (returns a stop function).

**Amendment:** a plain connectivity failure (`host-unreachable`, or any other rejection) still means `'down'`. But a health probe that comes back `host-identity-changed` is not a connectivity problem — the periodic background probe is exactly where a changed identity would first surface, possibly with nobody looking at Settings, so it must not be silently folded into the same gray "down" status a transient network blip gets (spec §3: "never a silent fall-through"). Use `'needs-repair'` for this case too, the same status a mid-session 401 uses (Task 10) — both mean "this host needs explicit user attention/re-pairing before it can be trusted again," a coherent single concept rather than a new one-off status.

- [ ] **Step 1: Write the failing tests**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { probeHost } from '@/features/hosts/lib/host-probe'
import { getOrCreateHostRuntime, destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(), isHostIdentityChangedError: vi.fn(() => false) }))
import { apiFetch, isHostIdentityChangedError } from '@/lib/api'

describe('probeHost', () => {
  beforeEach(() => {
    destroyHostRuntime('h1')
    vi.mocked(apiFetch).mockReset()
    vi.mocked(isHostIdentityChangedError).mockReset().mockReturnValue(false)
  })

  it('sets status "up" when the health call succeeds', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce(undefined)
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('up')
  })

  it('sets status "down" when the health call rejects with an ordinary error', async () => {
    vi.mocked(apiFetch).mockRejectedValueOnce(new Error('unreachable'))
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('down')
  })

  it('sets status "needs-repair" when the health call rejects with a host-identity-changed error', async () => {
    const err = new Error('identity changed')
    vi.mocked(apiFetch).mockRejectedValueOnce(err)
    vi.mocked(isHostIdentityChangedError).mockReturnValue(true)
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('needs-repair')
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
import { apiFetch, isHostIdentityChangedError } from '@/lib/api'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

/** Probes one host's /v0/health and records the result on its runtime store. */
export async function probeHost(hostId: string): Promise<void> {
  const runtime = getOrCreateHostRuntime(hostId)
  runtime.getState().setStatus('probing')
  const started = performance.now()
  try {
    await apiFetch('/v0/health', undefined, hostId)
    runtime.getState().setStatus('up', performance.now() - started)
  } catch (err) {
    runtime.getState().setStatus(isHostIdentityChangedError(err) ? 'needs-repair' : 'down')
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
- Consumes: `probeRemoteFingerprint`/`pairRemoteHost` (Tauri bridge plan, `crowbar-bridge.ts` — see this plan's Amendment for their real signatures), `useHostConfigStore.addHost` (Task 1), `Dialog`/`DialogContent`/etc. from `@/components/ui/dialog`.
- Produces: `redeemPairingCode(hostId: string, endpoint: string, confirmedFingerprint: string, code: string, name: string): Promise<void>` (pairs via the bridge, then saves the host config — see Step 2), `AddHostDialog` component.

- [ ] **Step 1: Write the failing `redeemPairingCode` test**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/crowbar-bridge', () => ({
  pairRemoteHost: vi.fn(),
}))

import { pairRemoteHost } from '@/lib/crowbar-bridge'
import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

describe('redeemPairingCode', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
    vi.mocked(pairRemoteHost).mockReset()
  })

  it('on success: saves the host config with the returned clientId', async () => {
    vi.mocked(pairRemoteHost).mockResolvedValueOnce('client-7')
    await redeemPairingCode('h1', '1.2.3.4:3737', 'SHA256:aaaa', '123456', 'my-device')
    expect(pairRemoteHost).toHaveBeenCalledWith('h1', '1.2.3.4:3737', 'SHA256:aaaa', '123456', 'my-device')
    const saved = useHostConfigStore.getState().hosts.find((h) => h.id === 'h1')
    expect(saved).toMatchObject({ clientId: 'client-7', pairedFingerprint: 'SHA256:aaaa' })
  })

  it('on redeem failure: does NOT save the host config', async () => {
    vi.mocked(pairRemoteHost).mockRejectedValueOnce(new Error('bad code'))
    await expect(redeemPairingCode('h1', '1.2.3.4:3737', 'SHA256:aaaa', '000000', 'my-device')).rejects.toThrow()
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })
})
```

Note this is simpler than the original draft: `pairRemoteHost` does its own registration/dial/redeem/token-storage internally (Rust side), so there is no separate "register, then redeem, then unregister-on-failure" choreography left for the web layer to own — a failed `pairRemoteHost` call already leaves nothing paired on the Rust side (Tauri bridge plan, Task 5's fix round), so the web layer's only job on failure is to not save a config row.

- [ ] **Step 2: Run to verify failure, then implement `pairing.ts`**

```ts
import { pairRemoteHost } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

/**
 * Redeems a pairing code against a host the user has confirmed the
 * fingerprint of (via probeRemoteFingerprint, in the dialog's stage 1).
 * `confirmedFingerprint` must be exactly what the user saw and approved —
 * never re-derive or re-probe it here.
 */
export async function redeemPairingCode(
  hostId: string,
  endpoint: string,
  confirmedFingerprint: string,
  code: string,
  name: string,
): Promise<void> {
  const clientId = await pairRemoteHost(hostId, endpoint, confirmedFingerprint, code, name)
  useHostConfigStore.getState().addHost({
    id: hostId,
    name,
    endpoint,
    pairedFingerprint: confirmedFingerprint,
    clientId,
  })
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
vi.mock('@/lib/crowbar-bridge', () => ({ probeRemoteFingerprint: vi.fn() }))

import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { probeRemoteFingerprint } from '@/lib/crowbar-bridge'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'

describe('AddHostDialog', () => {
  it('stage 1: entering a name and address and confirming the fingerprint advances to the code stage', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa bbbb cccc dddd eeee ffff')
    render(<AddHostDialog open onOpenChange={() => {}} />)

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
    fireEvent.click(screen.getByRole('button', { name: /continue|next/i }))

    await waitFor(() => expect(screen.getByText(/SHA256:aaaa bbbb cccc dddd eeee ffff/i)).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /confirm|looks right/i }))

    expect(await screen.findByLabelText(/code/i)).toBeInTheDocument()
  })

  it('does NOT call redeemPairingCode until the fingerprint has been confirmed', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    render(<AddHostDialog open onOpenChange={() => {}} />)
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'homelab' } })
    fireEvent.change(screen.getByLabelText(/address/i), { target: { value: '1.2.3.4:3737' } })
    fireEvent.click(screen.getByRole('button', { name: /continue|next/i }))
    await waitFor(() => expect(screen.getByText(/SHA256:aaaa/i)).toBeInTheDocument())
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  it('stage 2: submitting a code calls redeemPairingCode with the confirmed fingerprint', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
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

Build a two-stage form component: stage 1 collects `name`/`endpoint`, on submit calls `probeRemoteFingerprint(endpoint)` (already built — Tauri bridge plan Task 5b — do not add a new command or route this through `apiFetch`) and displays the returned fingerprint for the user to confirm against what the CLI printed. Only on explicit confirmation does stage 2 (6-digit code entry) become reachable, and only submitting stage 2 calls `redeemPairingCode` with the confirmed fingerprint from stage 1 — never re-probe or silently reuse a stale value.

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
- Consumes: `useHostConfigStore` (Task 1), `useHostRuntime`/`getOrCreateHostRuntime` (Task 1), `startHostProbing` (Task 3), `AddHostDialog` (Task 4), `unregisterRemoteHost`/`forgetHostToken` (Tauri bridge), `apiFetch` (Task 2, called against the host being revoked, to reach its own `DELETE /v0/clients/:id` before dropping the local token).

**Amendment — revoke must be server-side, not just local.** The original draft's revoke handler only called `unregisterRemoteHost`+`forgetHostToken` (Rust-local cleanup) and never told the daemon anything. Spec §3 is explicit: "Revoke ... marks the device permanently revoked server-side and drops the local keychain entry/config row" — both halves are required, not just the second. `pairRemoteHost` (Task 4) now returns the daemon's `clientId`, stored on each `Host` (Task 1) for exactly this purpose. Revoke order: `apiFetch('/v0/clients/' + host.clientId, {method: 'DELETE'}, host.id)` (server-side revoke, using the host's OWN clientId, over that host's own authed channel) **first**, then `unregisterRemoteHost`/`forgetHostToken` (Rust-local cleanup) **only after the server call succeeds** — if the server-side revoke fails (e.g. the host is currently unreachable), do not silently drop the local credential; surface the failure (this codebase's existing toast mechanism, per Task 10's pattern) so the user knows the host may still trust this device. Getting this ordering backwards (local cleanup first) would leave an orphaned, un-revocable authorized client on the daemon with no way for the desktop to ever clean it up again.

- [ ] **Step 1: Add the tab**

In `ui-state-store.ts`: add `| 'hosts'` to the `SettingsTab` union. In `settings-tab-items.ts`: add `{ id: 'hosts', label: 'Hosts', icon: <pick an appropriate phosphor icon, e.g. HardDrives or Cloud> }` to `SETTINGS_TAB_ITEMS`, gated behind `isTauri()` the same way `developer` is gated behind `import.meta.env.DEV` — per this plan's Review Focus, this whole surface must not appear in a non-Tauri context:

```ts
  ...(isTauri() ? [{ id: 'hosts' as SettingsTabItem['id'], label: 'Hosts', icon: HardDrives }] : []),
```

Import `isTauri` from `@/lib/crowbar-bridge` and the icon from `@phosphor-icons/react`.

- [ ] **Step 2: Write the failing test**

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'

vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: () => true, unregisterRemoteHost: vi.fn(), forgetHostToken: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))

import { apiFetch } from '@/lib/api'
import { unregisterRemoteHost, forgetHostToken } from '@/lib/crowbar-bridge'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { HostsSettings } from '@/features/settings/components/tabs/hosts-settings'

describe('HostsSettings', () => {
  it('lists Local plus every saved host', () => {
    useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab', endpoint: '1.2.3.4:3737', pairedFingerprint: 'SHA256:aaaa', clientId: 'c1' }] })
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

  it('clicking revoke calls the daemon DELETE first, then local cleanup, then drops the config row', async () => {
    useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }] })
    vi.mocked(apiFetch).mockResolvedValueOnce(undefined)
    render(<HostsSettings />)
    const row = screen.getByText('homelab').closest('[data-testid="host-row"]')!
    fireEvent.click(row.querySelector('[aria-label*="revoke" i]')!)
    await waitFor(() => expect(useHostConfigStore.getState().hosts).toEqual([]))
    expect(apiFetch).toHaveBeenCalledWith('/v0/clients/c1', { method: 'DELETE' }, 'h1')
    expect(unregisterRemoteHost).toHaveBeenCalledWith('h1')
    expect(forgetHostToken).toHaveBeenCalledWith('h1')
  })

  it('a failed server-side revoke leaves the host config and local token untouched', async () => {
    useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab', endpoint: 'x', pairedFingerprint: 'x', clientId: 'c1' }] })
    vi.mocked(apiFetch).mockRejectedValueOnce(new Error('host unreachable'))
    render(<HostsSettings />)
    const row = screen.getByText('homelab').closest('[data-testid="host-row"]')!
    fireEvent.click(row.querySelector('[aria-label*="revoke" i]')!)
    await waitFor(() => expect(apiFetch).toHaveBeenCalled())
    expect(unregisterRemoteHost).not.toHaveBeenCalled()
    expect(useHostConfigStore.getState().hosts).toHaveLength(1)
  })
})
```

- [ ] **Step 3: Run to verify failure, then implement `HostsSettings`**

Render a list: `LOCAL_HOST` first (no revoke control), then `useHostConfigStore((s) => s.hosts)` (narrow selector), each row showing name/endpoint and a status dot sourced via `useEffect` + `getOrCreateHostRuntime(host.id)`/`startHostProbing(host.id)` (subscribe in an effect, clean up the interval on unmount — side effects belong in the component per this plan's Global Constraints, not the store). An "Add host" button opens `AddHostDialog` (local `useState` for open/closed, per `AppDialog`'s own convention if that's what Task 4 used). Each non-local row's revoke button, from an event handler (not render): `await apiFetch('/v0/clients/' + host.clientId, { method: 'DELETE' }, host.id)` first; on success, `unregisterRemoteHost(host.id)`, `forgetHostToken(host.id)`, then `useHostConfigStore.getState().removeHost(host.id)`, in that order; on failure, toast the error and leave the host config/token untouched (see this task's Amendment above).

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
  useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab-prox', endpoint: 'x', pairedFingerprint: 'x' }] })
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
  useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'homelab-prox', endpoint: 'x', pairedFingerprint: 'x' }] })
  getOrCreateHostRuntime('h1').getState().setStatus('up')
  render(<CreateSpacePanel /* ... */ />)
  fireEvent.click(screen.getByRole('combobox', { name: /daemon/i }))
  const option = screen.getByRole('option', { name: /homelab-prox/i })
  expect(option).not.toHaveAttribute('aria-disabled', 'true')
})

it('lists an unreachable host but disabled, not hidden', () => {
  useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'office-vm', endpoint: 'x', pairedFingerprint: 'x' }] })
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
- Produces: `Project.hostId: string`, `fetchProjects(): Promise<Project[]>` becomes `fetchProjectsForHost(hostId: string): Promise<Project[]>` (fetches that one host's `/v0/projects` via `apiFetch(path, init, retry, hostId)` — **`hostId` is the 4th positional arg, not the 3rd**; Task 2 established `apiFetch<T>(path, init?, retry?, hostId?)`, and two earlier tasks in this plan (3 and 5) already had to catch-and-fix this exact same off-by-one in their own plan-text samples, so don't reintroduce it here — pass `undefined` for `retry` if you don't need to override it), a new `fetchAllProjects(): Promise<Project[]>` that calls `fetchProjectsForHost` for `'local'` plus every `up` saved host and concatenates, tagging each result with its `hostId`. `useProjectDataStore`'s `fetcher` becomes `fetchAllProjects`. Every workspace store gains a `hostId` field, set ONCE at construction from its owning project's `hostId` and never re-derived.

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
      { id: 'h1', name: 'up-host', endpoint: 'x', pairedFingerprint: 'x' },
      { id: 'h2', name: 'down-host', endpoint: 'x', pairedFingerprint: 'x' },
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
    useHostConfigStore.setState({ hosts: [{ id: 'h1', name: 'flaky', endpoint: 'x', pairedFingerprint: 'x' }] })
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
  const dtos = await apiFetch<ProjectDTO[]>('/v0/projects', undefined, undefined, hostId)
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

> **SUPERSEDED (Replan, 2026-10-09): Steps 5–6 below are NOT to be implemented.** A store-held `hostId` cannot be built (the workspace store is created before its project list loads; ~85 workspace calls never read the store). The host is resolved inside the URL builders and the transport instead — Tasks 13–18. Steps 1–4 and 7–9 were executed for the project-fetch half only (commit `91dceb09b`).

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

### Task 8b: SUPERSEDED — do not dispatch

Replaced by Tasks 14 (transport), 17 (stream gates) and 18 (per-host project streams). `wsManager`'s `channels` map is already keyed by the endpoint string, so a host-qualified endpoint (`crowbar://<hostId>/v0/...`) separates hosts without re-keying, and `awaitEntity` needs no `hostId` option — callers pass a qualified endpoint.

---

### Task 9: Coss UI tree + remote file picker

**Replan addendum — wiring (this task now also owns it).** Building the component is not enough: remote hosts must actually use it. Audit every native folder/file picker call (`web/src/lib/native-dialog.ts`, the dialog-plugin use in `crowbar-bridge.ts`, and their callers). Classify each: (a) picks a path on the DAEMON's filesystem (Create Space's Folder field, add/import repository, …) vs (b) picks a LOCAL file to upload (chat attachments) — (b) always stays native. For every (a): when the target host is `local`, keep the native OS dialog exactly as today; when it is a remote host, open `RemoteFilePicker` browsing that host (`GET /v0/system/browse`, host-qualified). Put the classified list in the task report. `RemoteFilePicker` must handle: loading, empty directory, permission denied, an unreachable host, symlinked directories, and the host going down while open. Create Space's Folder field is mandatory; any other (a)-site is in scope unless it needs a new backend endpoint.

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

### Task 10: action-time failure UX + revoke robustness (re-scoped by the Replan)

The original Task 10's host up/down reconciliation, tombstoning and "host comes back up" watcher moved to Task 18. What remains is what the user sees when an action targets a host that is down or untrusted, and revoke robustness.

**Files:** the project-scoped action surface (opening/creating a workspace or chat — find it via the existing toast/error patterns first; extend, don't invent a parallel mechanism); `features/settings/components/tabs/hosts-settings.tsx` (revoke handler, from Task 5); tests alongside.

**Requirements**
1. A request failing with `isHostUnreachableError` → existing toast mechanism, "<host name> is unreachable"; runtime status `'down'`; rows stay.
2. `isHostIdentityChangedError` → a distinctly worded, alarming message (spec §3: hard reject, "this host's identity changed"), status `'needs-repair'`, never retried; tell the user re-pairing is required and needs the host's CLI.
3. A 401 on a host whose status was `'up'` → `'needs-repair'` (spec §3), no retry.
4. Classified in ONE place — reuse the `classifyHostFailure(err)` that Task 18 creates for the prober — shared by the actions, not re-implemented per call site. Task 14 makes `apiFetch` attach the target `hostId` to `ApiError`, so the classifier needs no lookup.
5. Revoke (carry-forward from Task 5's review): if the server-side DELETE succeeds but a local cleanup call (`unregisterRemoteHost`/`forgetHostToken`) then throws, toast the failure, log it, and STILL remove the host row (the credential is dead server-side; a "live" row is wrong).
6. Tests for every branch, including that no retry fires for 2 and 3.

---

### Task 11: full-branch gate (re-scoped by the Replan)

- [ ] **Step 1: Web gate** — `cd web && export PATH="$HOME/.bun/bin:$PATH" && bun install && bunx eslint . --max-warnings 0 && bun run knip && bun tsc --noEmit && bunx prettier --check . && bun run lint:doctor`. **`knip` must now be fully clean in BOTH passes** (the three accepted-transient `features/hosts` findings must have real production consumers by now; if any remain that is a real defect to fix here, not to defer). Then TARGETED vitest only — never the whole suite on this machine (CI runs it): `src/__tests__/lib/`, `src/__tests__/features/hosts/`, `src/__tests__/features/workspace/`, `src/__tests__/features/settings/`, `src/__tests__/components/` for every touched component. `bunx eslint .` may also report errors from the gitignored `web/coverage/` artifact — ignore those, they are not tracked.
- [ ] **Step 2: Go gate** — from `api/`, `-tags noEmbed`: `go build ./...`, `make lint`, `make lint-backlog`, `make deadcode`, and `go test` for `./internal/api/v0/...`, `./internal/core/...`, `./internal/app/usecases/pairing/...`, `./cmd/...`. Two integration failures reproduce on develop itself (`TestScripted_SlowHook/claude`, live Codex tests that wait on a trust prompt) — confirm they are the only failures and report them; do not skip or fix them here.
- [ ] **Step 3: Rust gate** — from `desktop/src-tauri/` (`export PATH="$HOME/.cargo/bin:$PATH"`): `cargo build`, `cargo test`, `cargo clippy --all-targets`, `cargo fmt --check`. Check `df -h /` first.
- [ ] **Step 4: hygiene** — `.superpowers/` is gitignored; no scratch/QA/plan-as-scratch files staged; fix anything the gates surface, then commit.

---

## Replan (2026-10-09, written after rebasing onto develop and after Task 8's NEEDS_CONTEXT report)

### Why

Task 8's implementer proved the original "bind a `hostId` to each workspace store and pass it to every workspace call" design cannot be built: the workspace store is created before its project list loads, ~85 workspace-scoped calls never read the store at all, and passing `hostId` as a 4th argument at ~130 sites means one forgotten argument silently targets Local. Until project/workspace traffic is host-aware, a remote project's repos, trees, chats, git, files, LSP and terminals all hit the LOCAL daemon (404s). Two further gaps surfaced: **nothing registers persisted hosts with the Rust proxy at app start** (its registry is in-memory, so after any restart every `crowbar://<hostId>/...` request is a 502 until the host is re-paired), and **host probing only runs while Settings → Hosts is open** (no remote projects appear at boot, and any re-read of the project list drops a down host's rows, against spec §4).

### Design — the URL builders and the transport own the host; call sites do not change

A project/workspace-scoped URL is built by a small set of builders (`lib/workspace-scope-url.ts`, `agent-api.ts`'s `chatBase`) and by 29 hardcoded `/v0/projects/${id}/...` literals. Make those produce **host-qualified paths**: bare `/v0/...` for `local`, `crowbar://<hostId>/v0/...` for a remote host. Call sites concatenate suffixes onto the base exactly as today, so the prefix rides along. The transport (`apiFetchRaw`, `wsManager.createTransport`, `openSocket`/`openTerminal`, raw `API_BASE` fetches, `assetURL`) splits the prefix and routes to that host. `wsManager`'s channel map is keyed by the endpoint string, so a qualified endpoint separates hosts automatically.

### Invariants (every task below, every review)

- **I1 — no silent local.** Any code path that cannot determine a host must throw `HostNotResolvedError` or wait on a readiness hook; it must never default to `'local'`. Only two legitimate sources of `'local'`: (1) the entity's own row says `hostId: 'local'`, or the entity is NOT FOUND while *no remote host has EVER been paired on this install* (a persisted one-way bit `remoteEverPaired` in `useHostConfigStore`, set by `addHost`, treated as true whenever any host is configured) *and no host is configured and no held project row carries a non-local `hostId`* — then no remote entity can exist. "Zero hosts configured" alone is NOT proof: a revoked host's ids outlive its rows (the project list is not the only holder — workspace scopes, the route and persisted panes keep remote ids, and a re-read without the host's rows must not turn them into `'local'`). After a remote has ever been paired, an unknown id is NEVER local: it is unresolved (the cached list makes local lookups fast, so local entities still resolve once their row is held), and (2) an explicit `LOCAL_HOST` at a call site that is local by nature. The entity lookup always runs FIRST; a row found with a host that is neither `'local'` nor currently configured is unresolved (throw / readiness false), never a stale id and never `'local'`.
- **I2 — no ambient fallback.** Never call `getWorkspaceScope()` without a `wsId` (it falls back to the active workspace — the documented bug class), never read an "active/current project/host". Host comes from the entity's own recorded project.
- **I3 — zero change for local-only users.** With no remote hosts configured every builder returns byte-identical strings to today and every existing test passes unchanged. Add golden tests proving it.
- **I4 — `apiFetch` signature is `apiFetch<T>(path, init?, retry?, hostId?)`.** `hostId` is the 4th positional argument. This off-by-one has already bitten four tasks.
- **I5 — gates per task:** `bunx prettier --write` on touched files, `bunx eslint <touched> --max-warnings 0`, `bun tsc --noEmit` (use `bun tsc`, not `bunx tsc`), targeted vitest (NEVER the whole suite), `bun run knip` reported (the three known `features/hosts` findings are accepted transient state and must be gone by Task 11).

### Execution order

8 (Steps 1–4, done: `91dceb09b`) → **12 → 20 → 13 → 14 → 15 → 16 → 17 → 18** → 6 → 9 → 10 → 11 → whole-plan final review → Task 19 (end-to-end run). Strictly one task at a time in this worktree.

---

### Task 12: remote daemon container image

**Files:** `docker/Dockerfile`, `docker/docker-compose.yml` (modify). No other files unless a build needs one.

**Facts (verified):** the existing Dockerfile builds on `golang:1.25-alpine` but `api/go.mod` requires `go 1.26.2` — the image cannot currently build. It runs as root, has no `git`/`openssh-client`/init, and requires a prebuilt `web/dist` (embedded SPA). The daemon is started with `crowbar serve --host tcp://0.0.0.0:3737`; a non-loopback TCP host terminates TLS with a self-signed identity, requires paired clients, and also binds the local unix socket that `crowbar pair` dials. The TLS identity and the client registry live in `CROWBAR_HOME`.

**Requirements**
1. Builder image matches `go.mod` (verify the tag exists). Keep the default build behaviour (embedded web) working; add a headless path that builds with `-tags noEmbed` and does not need `web/dist` (e.g. a build arg) — verify BOTH build.
2. Runtime image: `ca-certificates`, `git`, `openssh-client`, `tini` as PID 1 (the daemon spawns child processes), a non-root user with its own home, `CROWBAR_HOME` on a declared volume so identity and pairings survive restarts, `EXPOSE 3737`.
3. Compose: keep the service; volume for `CROWBAR_HOME`; host port overridable (the maintainer's machine runs other services — do not hard-require 3737).

**Verification (run it, report command + output; this is the daemon half of the end-to-end proof)**
a. `docker build` succeeds for both paths on this machine; image runs as non-root.
b. Start a container with a fresh named volume and a non-default host port. From the host: unauthenticated `curl -k https://localhost:<port>/v0/projects` → 401 and `/v0/health` → 401 (guarded); `/debug/pprof/` → 403.
c. `docker exec <c> crowbar pair` (find the real subcommand in `api/cmd/crowbar/`) prints a 6-digit code AND the TLS fingerprint; the printed fingerprint equals the SHA-256 of the served certificate computed independently with `openssl s_client`/`openssl x509` (truncated display form per `tlsid`).
d. Redeem the code with `curl -k -X POST .../v0/pair`, use the returned bearer token against `GET /v0/projects` → 200; a SECOND redeem of the same code → refused; a wrong code repeated ≥6× → rate limited.
e. `docker restart <c>`: fingerprint unchanged (identity persisted), token still valid. `docker rm -f` + new empty volume: fingerprint CHANGES (this is what the app's identity-changed path will see).
f. With the token: `GET /v0/system/browse?path=<container home>` lists directories; create a project on a `git init`'d directory inside the container via `POST /v0/projects` and see it in `GET /v0/projects`.
g. Revoke: `DELETE /v0/clients/:id` with the token → the same token now returns 401 and re-redeeming a fresh code for the same device name does NOT resurrect it.
Do not touch the maintainer's existing containers or `~/.crowbar`. Remove your test containers/volumes afterwards.

---

### Task 20: guard `/v0/health` over TLS; persist remote projects (daemon + container)

**Why:** `health.Register(rg)` is registered before `requireBearerAuth` (`api/internal/api/v0/router.go`, comment: "health is probed before a host is paired"). The app never probes health before pairing (its fingerprint probe is a bare TLS handshake), so the open route buys nothing and costs two things: it hands pid + version to anyone who can reach the TCP port, and a revoked or invalid token still sees the host as `up` on the periodic probe (spec §3 says a 401 must mark the host needs-repair). Plaintext listeners (unix socket, loopback dev TCP) already pass `requireBearerAuth`, so the desktop supervisor and the e2e harness are unaffected.

**Files:** `api/internal/api/v0/router.go`, `api/internal/api/v0/route_audit_test.go`, the health endpoint tests, `docker/Dockerfile`, `docker/docker-compose.yml`.

**Requirements**
1. Move `health.Register(rg)` below `rg.Use(requireBearerAuth(...))`. Over TLS an unauthenticated `GET /v0/health` → 401; with a valid bearer → 200; over the unix socket and loopback TCP it still answers without a token. Update `route_audit_test.go`'s open-vs-guarded lists and the TLS no-auth test (it must now require 401 from `/v0/health` too, and fail if health is moved back above the guard — break it once to prove it). Pairing's `RegisterOpen` routes stay first and open.
2. Grep every other consumer of `/v0/health` (desktop `sidecar/mod.rs`, `diagnostics.rs`, `scripts/e2e/lib/daemon.mjs`, CI workflows, compose/Docker healthchecks) and confirm each reaches the daemon over a plaintext listener; adapt any that don't.
3. Docker: declare a second volume for projects (e.g. `/home/crowbar/projects`, owned by the non-root user) so repos created on the remote survive container recreation; document in the compose file. Verify: create a project on a git repo under that path, `docker rm -f` + run a new container on the same volumes → the repo is still there and the daemon lists the project.
5. **Task 12 review carry-forward — Important.** (a) `COPY api/ ./` also copies a developer's gitignored `api/cmd/crowbar/web/dist` (left by `make build`/`embed-web`, already holding `*.gz` siblings), and the unconditional gzip step then dies with `File exists` — on BOTH build paths, so `make docker-up` breaks for anyone who has built locally. Fix with a `docker/Dockerfile.dockerignore` (exclude `api/cmd/crowbar/web`, `api/bin`, `api/coverage*`, `api/logs`, `api/crowbar*`; keep `web/dist`) and skip the gzip step when `WEB=none`; reproduce the failure first by creating a stale `api/cmd/crowbar/web/dist/*.gz` locally (then delete it). (b) A fresh container cannot `git commit` (no identity) or clone over ssh (no known_hosts), and `~/.gitconfig`/`~/.ssh` live outside the volume: set `GIT_CONFIG_GLOBAL` to a file inside the `CROWBAR_HOME` volume, add commented `GIT_AUTHOR_*`/`GIT_COMMITTER_*` stubs to compose, and document how to mount `~/.ssh`/known_hosts. Verify a `git commit` works after `git config --global user.name/email` survives `docker rm` + recreate on the same volume.
6. **Minor (do, they are cheap):** top-level `name: crowbar` in compose (the generic project name `docker` risks `down -v` from another compose file deleting the volume that holds the TLS identity); a compose comment that an old `/root/.crowbar` volume must be recreated (and Linux bind mounts need `chown 1000:1000`; `CROWBAR_HOME` stays a named volume because Docker Desktop on macOS cannot host a unix socket on a bind mount); `make docker-up` and README must say `https://localhost:${CROWBAR_PORT:-3737}` (self-signed TLS; plaintext gets 400) instead of `http://localhost:3737`; tag the headless variant differently from the embed one so `CROWBAR_WEB` values don't overwrite each other's local image. Skip: the bad-`CROWBAR_WEB` error text and alpine age.
4. Verify with the real image: rebuild `crowbar-remote:dev`; unauthenticated `curl -k https://localhost:<port>/v0/health` → 401; after pairing, with the token → 200; `docker exec <c> crowbar pair` still works (it dials the local socket).

---

### Task 13: `lib/host-path.ts` — host-qualified paths and host resolution

**Files:** create `web/src/lib/host-path.ts`; test `web/src/__tests__/lib/host-path.test.ts`. Depends on Task 8 (`Project.hostId`, `useProjectDataStore`) and Task 1 (`useHostConfigStore`, `LOCAL_HOST`).

**Interfaces**
- `qualify(hostId, path)`: `'local'` → `path` unchanged; otherwise `` `crowbar://${hostId}${path}` `` (`path` must start with `/`). Reject host ids that the Rust side rejects (`^[a-z0-9-]+$`, not `localhost`).
- `splitHostPath(input)` → `{ hostId, path }`: the inverse. Anything not shaped `crowbar://<id>/...` is `local`. Define and test what happens to the local desktop base (`crowbar://localhost...`): it is local, never a remote host id. Reject (don't misread as local) a wrongly-cased scheme, leading whitespace, a leading `//` path, and `qualify('local', 'crowbar://…')`.
- `class HostNotResolvedError extends Error` — typed; model it on the existing `OwningChatNotRecordedError` pattern.
- `hostIdForProject(projectId)`: look the project up in `useProjectDataStore` FIRST. Found (and not ambiguous across hosts) → its `hostId` if that is `'local'` or a configured host, else unresolved. Not found → `'local'` only if no remote host has ever been paired (`remoteEverPaired` false) AND none is configured AND no held row has a non-local `hostId`; otherwise throw `HostNotResolvedError` (carry a reason + the project id).
- `hostIdForWorkspace(wsId)`: `wsId` REQUIRED. Read that workspace's recorded scope from the wsId-keyed scope registry (`lib/workspace-scope.ts`) → `projectId` → `hostIdForProject`; missing scope → throw. Must never consult the active workspace.
- `useHostResolved(key)`: (false forever is legitimate for a deleted project — Task 17 must not show an unbounded spinner on it alone) readiness hook (`useSyncExternalStore`), true once `hostIdForProject/Workspace` would not throw. Mirror `useWorkspaceScopeReady`.

**Tests (these are the invariants — write them first and break each once):** no hosts → `'local'` with no store reads; hosts configured + known remote project → its host; known local project → `'local'`; unknown project → throws; `hostIdForWorkspace` with no recorded scope throws even when a DIFFERENT workspace on another host is active; readiness hook flips when the project list loads; `qualify`/`splitHostPath` round-trip; invalid host ids rejected.

---

### Task 14: transport — apiFetch, wsManager, terminal, raw fetches, assets

**Files:** `web/src/lib/api.ts`, `web/src/lib/ws/manager.ts`, `web/src/lib/crowbar-bridge.ts` (`openSocket`/`openTerminal`), `web/src/lib/ws/tauri-transport.ts` only if needed, the three raw `API_BASE` users (`agent-api.ts` `getToolPayload`, `lib/chat-asset-resolver.ts`, `lib/upload-chat-attachment.ts`), `assetURL`; tests next to each. Line numbers in older reports are stale after rebase — grep.

**Requirements**
1. `apiFetchRaw`/`apiFetch`: a `path` starting `crowbar://` is split with `splitHostPath`; the resolved host is used for base resolution, retry/in-flight-read keys and the `ApiError` (attach `hostId` to `ApiError`). The explicit 4th-arg `hostId` keeps working; if BOTH are given and disagree, throw — never pick one silently.
2. `wsManager.createTransport(endpoint)`: split the prefix; pass `hostId` to `TauriWebSocket` / `wsUrl(path, hostId)`. The channel map stays keyed by the FULL (qualified) endpoint string. Check `connection-store`/`reportChannelState` consumers cope with qualified endpoints. `awaitEntity` needs no change — add a test that it works with a qualified endpoint (this is what fixes "create a project on a remote host then hang for 30s").
3. `openTerminal(sessionId, base)`: a qualified base flows to `openSocket(..., hostId)`; the `sessionBases` reuse for kill keeps the qualified base.
4. The three raw `API_BASE` fetches and `assetURL(path)`: accept a qualified path and resolve the base through `apiBaseFor(host)`.
5. Zero change for unqualified paths (I3) — run the existing `lib/`, `lib/ws/` suites unchanged. `lib/api.ts` is already ~800 lines and sits on ESLint's max-lines baseline (baselines only shrink — never raise the cap): put new transport code in a `lib/api/` sibling module rather than growing `api.ts` if the rule trips.
6. `connect`-time failures for a remote host must surface the proxy markers (`host-unreachable`, `host-identity-changed`) exactly as Task 2 made `apiFetch` do; verify the WS path surfaces them too (the Rust side delivers WS failures as an error string with the marker as a prefix) — add handling if the manager swallows it.

---

### Task 15: workspace-scoped URL builders become host-aware

**Files:** `web/src/lib/workspace-scope-url.ts` (the 10 builders: `chatBase`, `terminalsBase…`, `gitBase…`, `filesBase…`, `lspBase…`, `reviewBase…`, `worktreeVerbBase…`, `workspaceBase`, and the rest — read the file), `agent-api.ts`'s `chatBase`; tests. Depends on Tasks 13–14.

**Requirements**
1. Every builder returns `qualify(hostIdForWorkspace(wsId), <the path it returns today>)`.
2. **Render-path safety.** Some callers (e.g. `workspaceBase`) run during render; a throw there reaches an ErrorBoundary. List every builder call reachable during render and make each safe (gate on `useHostResolved`, or catch and render a not-ready state). This is the main risk of the task — enumerate them in the report.
3. Golden tests: with no remote hosts configured each builder returns exactly the string it returned before the change (record the "before" strings from the old code first). With a remote project configured each returns the `crowbar://<hostId>/…` form; with hosts configured and the project not yet loaded each throws `HostNotResolvedError` (not `'local'`).
4. Builders addressed by CHAT id (`owningChatId`) must still resolve their host through the chat's workspace scope → project; do not invent a chat → host map or default to Local — report any chat-addressed builder you cannot resolve that way.
5. Golden tests must stub the DESKTOP base too: jsdom runs with `API_BASE=''` but on desktop it is `crowbar://localhost`, so a builder that routes an `${API_BASE}…` string through `qualify('local', …)` would pass its golden tests and throw on the desktop. Builders hand `qualify` bare `/v0/...` paths only.
6. Do not add a `hostId` parameter to any workspace-scoped function; the whole point is that call sites are untouched. If a call site cannot get its host through a builder, report it rather than patching it with an argument.

---

### Task 16: project/repo builders, the 29 hardcoded literals, and the lint rule

**Files:** create `web/src/lib/project-url.ts` (`projectBase(projectId)`, `repoBase(projectId, repoId)`), migrate the literals: `lib/api.ts` (~15), `lib/app-sync-engine.ts` (3), `sidebar-placement.ts` (3), `home-tree.ts`, `add-repository-modal.tsx`, `create-space-panel.tsx`, `repo-import-dialog.tsx`, `repo-icon-mark.tsx`, `project-icon-mark.tsx`, `agent-api.ts` — `grep -rn '/v0/projects/' web/src` to get the real current list; `eslint.config.js` (new rule); tests.

**Requirements**
1. `projectBase`/`repoBase` = `qualify(hostIdForProject(projectId), '/v0/projects/…')`. Every project-scoped read/write/stream in the list above goes through them: repos, workspaces, trees, home tree, home workspace, project writes (`postRepo`, `renameRepo`, `renameProject`, `deleteProject`, `importBranches`, `deleteRepo`, `getRepoPullRequests`), icons.
2. Calls that CREATE a project (no project id yet: `POST /v0/projects`, import) keep their explicit host argument from Task 7/8 — unchanged.
3. New ESLint rule (e.g. `no-restricted-syntax` on template literals/concatenations containing `/v0/projects/` or the other id-bearing prefixes) allowed ONLY in `lib/host-path.ts`, `lib/project-url.ts`, `lib/workspace-scope-url.ts`. No baseline additions (CLAUDE.md: baselines only shrink) — the rule must pass with zero violations after the migration.
4. `app-sync-engine` opens its streams per VISIBLE project: those projects come from the list, so the host is known by construction; assert that with a test using one local and one remote project (each stream's endpoint is qualified correctly and the remote stream is never opened against Local).
5. I3 golden tests for every migrated call (no hosts → identical URLs).
   - **Render-time throw (carried from Task 14).** `assetURL` / the transport's URL helper now throw for a host that is not configured, and `assetURL` runs during render (project/repo icons). Gate or catch it: an icon for a host that is down/unconfigured must render a placeholder, never crash the tree.
6. **Create Space provenance (carried from Task 8's review).** `create-space-panel` must `awaitEntity` on `qualify(selectedHostId, '/v0/projects')` — the host the project was created on — so the DTO it stamps with `projectFromDTO(dto, hostId)` really came from that host. Today a remote create waits on the LOCAL socket and, on a path collision with a local project, would mislabel the local one as remote. Add a test where a local project with the same path exists and a remote create resolves to the remote one.

---

### Task 17: stream readiness gates and terminal attach

**Files:** `use-workspace-agent-chats-stream.ts`, `use-workspace-threads-stream.ts`, `lsp-client.ts`, `use-workspace-file-tree.ts`, `use-workspace-git.ts` (post-#212 names; grep `wsManager.subscribe`), `use-terminal-attachment.ts`; tests. Depends on Tasks 13–16.

**Requirements (carried from Task 14):** `use-terminal-attachment.ts` re-attaches on the LOCAL connection status — wrong for a remote terminal; it must follow the connection state of the terminal's own (qualified) channel. Unmarked stream failures reach the app as plain errors (HTTP 401 on upgrade, "unknown remote host", "no stored credential" have no marker) — do not treat them as `host-unreachable`; the prober/status owner (Task 18) decides the host's status. Also: each effect that subscribes or fetches for a workspace waits until the host resolves (AND-ed with its existing scope-ready gate) instead of throwing, then uses the qualified endpoint. No subscription and no request is made for a workspace whose host is unresolved. Cleanup/unsubscribe unchanged. Tests per hook: hosts configured + project list not loaded → nothing opened; list loads → remote workspace subscribes with the `crowbar://<hostId>/…` endpoint, local workspace with the bare one; switching between a local and a remote workspace tears down and opens the right streams with no frames from one landing in the other's store.

---

### Task 18: host bootstrap, app-wide prober, per-host project streams, reconciliation

**Files:** the app boot/sync path (`app-sync-engine.ts` and wherever the app mounts globally — find the right owner, do not add a parallel one), `web/src/lib/store/projects.ts`, `features/hosts/lib/host-probe.ts`, `hosts-settings.tsx` (probing moves out of it), `host-config-store`/`host-runtime-registry` as needed; tests.

**Requirements**
1. **Bootstrap registration.** In Tauri, before the first project fetch, call `registerRemoteHost(id, endpoint)` for every persisted host (the Rust registry is in-memory). Await it; expose a readiness flag so nothing fetches a remote host earlier. Add/remove keep registry and store in step (`unregisterRemoteHost` on remove).
2. **App-wide prober.** One owner starts probing every configured host for the app's lifetime (not only while Settings → Hosts is open) and stops on removal. Create `classifyHostFailure(err)` here (unreachable → `'down'`; identity-changed → `'needs-repair'`; HTTP 401 → `'needs-repair'`; anything else → `'down'`) and use it in `probeHost` — with Task 20 guarding `/v0/health`, a revoked/invalid token now shows up as 401 on the periodic probe (spec §3), so the prober must not treat it as plain `'down'`. Task 10 reuses this classifier. Settings only reads status (it may offer "check now"). Boot with a down remote host must not delay or fail Local.
3. **Rows survive.** The project list keeps showing a host's projects (with a down/unknown dot) while that host is `unknown`/`down`; a re-read replaces only the rows of hosts that were read successfully. At boot, remote rows come from the `projects-by-host` cache until the host is probed `up`. A local read failure keeps today's behaviour (Task 8 deviation 3).
4. **Per-host live streams.** For every host that is `up`, subscribe `qualify(hostId, '/v0/projects')` through `wsManager`; frames merge with the host taken from the endpoint (`mergeProjectFrame` stops defaulting to Local for non-local sockets — Task 8 gap D7). Unsubscribe when the host leaves `up` or is removed; on `down→up` re-fetch that host's projects (only that host) and resubscribe, no app restart.
5. **Stream failures → host status (carried from Task 14).** The manager now records stream failures as `ApiError`s (codes `host_unreachable` / `host_identity_changed`, with `hostId`) in `useConnectionStore.failures[<qualified endpoint>]`, but nothing reads them yet: map them to the host's runtime status (`classifyHostFailure`), so a stream that hits an identity change flags `'needs-repair'` even between prober ticks. Unmarked failures are ignored for status (the prober decides).
6. **Removal.** Removing/revoking a host tombstones that host's projects through the existing deleted-project path, destroys its runtime store, closes its subscriptions; if the open route belonged to it, the existing "project missing" redirect applies.
7. **Local never waits on a remote (carried from Task 8's review).** `fetchAllProjects` currently awaits the slowest remote (`Promise.allSettled`) before publishing the local list, and the shell's `beforeLoad` awaits it; a hung `'up'` remote is bounded only by the proxy's own timeouts. Publish the local list as soon as it is read and merge each remote's rows as they arrive, with a short abort (≲5 s) on remote project reads. Boot with a black-holed remote must show Local projects immediately.
8. Tests: boot with two persisted hosts (one up, one down) → Local interactive immediately, up host's projects appear after probe, down host's cached rows stay with a down dot; flip down→up → only that host refetched; remove host → rows gone, subscriptions closed; frame from a remote stream is tagged with the remote host (regression test for D7); `registerRemoteHost` called for each persisted host before any remote request.

---

### Task 19: end-to-end run (controller-driven; the production-readiness gate)

Run against the REAL stack, not mocks: `make dev-desktop` (isolated dev home per the repo's dev-isolation recipe; never touch `~/.crowbar` or the production socket; exactly one dev instance) driven through the Tauri MCP bridge, plus the Task 12 container as the remote. Pass criteria — every item observed, with evidence:
1. Add host: wrong address → clean error; right address → fingerprint shown; fingerprint equals the CLI's; wrong code rejected; right code pairs; host row shows `up`.
2. Remote projects appear in the sidebar with the host subtitle + dot; Local projects unchanged.
3. Create a project on the remote via the daemon picker + the REMOTE file picker (browse the container's FS); it appears and opens.
4. In a remote project: open a workspace, chat round-trip (provider availability permitting; otherwise exercise the non-provider streams), terminal typing, git status, file tree, file open. Same in a Local project.
5. **Switch back and forth between remote and local projects repeatedly (including quickly):** every request/stream lands on the right daemon (verify with the container's logs vs the local daemon's — nothing for a remote id ever reaches Local and vice-versa), no stale frames, no crash.
6. `docker stop` the remote: rows stay, dot goes down, an action shows the unreachable message quickly (< ~6 s), Local unaffected. `docker start`: dot returns to up and the host's projects/streams resume without restarting the app.
7. Restart the desktop app: remote projects are back (bootstrap registration works).
8. Recreate the container with a fresh volume (identity change): hard-reject, identity-changed warning, host marked needs-repair, no token ever sent to the new identity.
9. Revoke from Settings: the remote rows disappear; the old token is dead server-side; re-pairing needs a new CLI code.
10. Final gates green: Go (`make lint`, `make deadcode`, targeted tests), Rust (`cargo test/clippy/fmt`), web (`tsc`, `prettier`, `eslint`, `knip` fully clean, targeted vitest).
