import { useSyncExternalStore } from 'react'
import { fetchHomeWorkspace } from '@/lib/api'

/**
 * Resolves (and caches) the stable home-workspace id for a project.
 *
 * The backend returns the SAME persisted workspace id for a project's home on
 * every call — a lookup, not a mint (see resolveHome in
 * api/internal/api/v0/endpoints/home/handlers/handlers.go) — so caching for
 * the lifetime of the IDE session is safe. Before this, every single arrival
 * at project home re-fetched from scratch AND cold-mounted a fresh
 * WorkspaceView (see ide-shell.tsx's use of getKnownHomeWorkspaceIds /
 * ensureHomeWorkspaceResolved), which is what made switching TO project home
 * ~2x more expensive than a normal warm workspace<->workspace switch.
 *
 * Dependency-free module singleton, in the same style as workspace-scope.ts —
 * IDEShell (which resolves) and the home route (which reads loading/error
 * state) both need the same resolution without either owning the other.
 */
export interface HomeWorkspaceState {
  wsId: string | null
  error: boolean
}

// Stable sentinel so useSyncExternalStore's snapshot never changes identity
// while nothing has resolved yet (a fresh `{}` literal every call would loop).
const UNRESOLVED: HomeWorkspaceState = { wsId: null, error: false }

const states = new Map<string, HomeWorkspaceState>()
const inflight = new Set<string>()
const listeners = new Set<() => void>()

function notify(): void {
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/**
 * Kick off (once per project id, ever) resolving the home workspace. Safe to
 * call unconditionally from an effect on every render while on the home
 * route — a no-op once resolved or while already in flight.
 */
export function ensureHomeWorkspaceResolved(projectId: string): void {
  if (states.has(projectId) || inflight.has(projectId)) return
  inflight.add(projectId)
  fetchHomeWorkspace(projectId)
    .then((ws) => {
      states.set(projectId, { wsId: ws.id, error: false })
    })
    .catch(() => {
      states.set(projectId, { wsId: null, error: true })
    })
    .finally(() => {
      inflight.delete(projectId)
      notify()
    })
}

/**
 * Every home-workspace id resolved so far this session (any project). Fed
 * into WorkspaceHost's retention so a project's home is never pruned just for
 * being absent from the sidebar's repo/workspace id set — home is a
 * project-level concept, not a repo workspace, and isn't tracked there at
 * all, so without this the existence-prune would treat it as "closed" the
 * instant it goes hidden.
 */
export function getKnownHomeWorkspaceIds(): string[] {
  return [...states.values()].map((s) => s.wsId).filter((id): id is string => id !== null)
}

function getSnapshot(projectId: string | null): HomeWorkspaceState {
  return (projectId ? states.get(projectId) : undefined) ?? UNRESOLVED
}

/** The resolution state of `projectId`'s home workspace (or the stable
 * "unresolved" sentinel while a fetch is in flight / not yet started). */
export function useHomeWorkspaceState(projectId: string | null): HomeWorkspaceState {
  return useSyncExternalStore(subscribe, () => getSnapshot(projectId))
}

/** Testing only — clears cached resolutions between tests. */
export function __resetHomeWorkspaceResolverForTest(): void {
  states.clear()
  inflight.clear()
}
