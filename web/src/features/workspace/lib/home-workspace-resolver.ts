import { useSyncExternalStore } from 'react'
import { fetchHomeWorkspace } from '@/lib/api'
import { isHostResolved, subscribeHostResolution } from '@/lib/host-resolve'
import { hostUnavailability } from '@/lib/host-unavailability'
import { loadCache, saveCache } from '@/lib/persistence/cache-store'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

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
  /**
   * The chat that owns the home workspace's worktree, as the daemon reports it.
   *
   * Carried because chat-scoped routes (`/v0/chats/:chatId/...`) are addressed
   * by it, and the home route supplies no chat id of its own — unlike a repo
   * workspace, whose owning chat arrives with the sidebar tree. Without it a
   * terminal opened on project home has no way to build its URL.
   */
  owningChatId: string | null
  /**
   * The home workspace's own on-disk directory (`WorkspaceDTO.localPath`,
   * same field a repo workspace carries), so a project-home-scoped surface
   * (the sidebar's file explorer, the editor pane) can resolve the real
   * project root the instant `GET /home` answers — instead of waiting for
   * some OTHER signal (a chat getting registered into a workspace store,
   * which used to only happen once the user sent a message) to make the
   * active-pane resolution chain fall through to this workspace at all.
   * `null` before the first successful resolve, or if the daemon ever
   * answers without one.
   */
  localPath: string | null
  error: boolean
  /** Held from IndexedDB, not yet confirmed by GET /home: shown while the host is unreachable. */
  stale: boolean
}

/** What survives a restart: the home workspace as `GET /home` last answered it. */
interface CachedHomeWorkspace {
  wsId: string
  owningChatId: string | null
  localPath: string | null
}

const cacheKey = (projectId: string) => `home-workspace:${projectId}`

// Stable sentinel so useSyncExternalStore's snapshot never changes identity
// while nothing has resolved yet (a fresh `{}` literal every call would loop).
const UNRESOLVED: HomeWorkspaceState = {
  wsId: null,
  owningChatId: null,
  localPath: null,
  error: false,
  stale: false,
}

const states = new Map<string, HomeWorkspaceState>()
const inflight = new Set<string>()
/** Projects waiting for the project list to name their host, with the stop of that wait. */
const awaitingHost = new Map<string, () => void>()
const listeners = new Set<() => void>()

function notify(): void {
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/**
 * Kick off resolving the home workspace. Safe to call unconditionally from an
 * effect on every render while on the home route — a no-op once resolved or
 * while already in flight. A FAILED resolve is not terminal: the error state
 * stays visible until the next call re-fetches, so one lost GET at cold start
 * (daemon still replaying, sidecar respawn) cannot blank a project's home for
 * the whole session. A resolve that latched NO owning chat is not terminal
 * either: the daemon mints one on GET /home, so the next call re-reads. A
 * project whose host is not known yet is pending, not failed: it re-enters
 * once the project list places it, and fails once that host is not coming.
 */
export function ensureHomeWorkspaceResolved(projectId: string): void {
  const current = states.get(projectId)
  if ((current?.wsId && current.owningChatId && !current.stale) || inflight.has(projectId)) return
  if (!isHostResolved({ projectId })) return awaitHost(projectId)
  inflight.add(projectId)
  fetchHomeWorkspace(projectId)
    .then((ws) => {
      const resolved = {
        wsId: ws.id,
        owningChatId: ws.owningChatId || null,
        localPath: ws.localPath || null,
      }
      states.set(projectId, { ...resolved, error: false, stale: false })
      void saveCache<CachedHomeWorkspace>('workspaces-data', cacheKey(projectId), resolved)
      // Recorded here, outside any render, before anyone hears the id: a scope write from a
      // render body re-renders whatever already subscribes to it, mid-render.
      recordWorkspaceScope({
        projectId,
        repoId: '',
        wsId: ws.id,
        owningChatId: ws.owningChatId || undefined,
      })
    })
    .catch(() => {
      // A re-read for the owner keeps the workspace it already knows.
      if (states.get(projectId)?.wsId) return
      states.set(projectId, { ...UNRESOLVED, error: true })
    })
    .finally(() => {
      inflight.delete(projectId)
      notify()
    })
}

// Gives up on the rule boot placement gives up on, but keeps listening: a host paired again
// still brings the project's home back.
function awaitHost(projectId: string): void {
  if (awaitingHost.has(projectId)) return
  const follow = () => {
    if (hostUnavailability({ projectId }) !== null) failPending(projectId)
    if (!isHostResolved({ projectId })) return
    stop()
    awaitingHost.delete(projectId)
    ensureHomeWorkspaceResolved(projectId)
  }
  const stop = subscribeHostResolution(undefined, follow)
  awaitingHost.set(projectId, stop)
  follow()
}

function failPending(projectId: string): void {
  const current = states.get(projectId)
  if (current?.wsId || current?.error) return
  states.set(projectId, { ...UNRESOLVED, error: true })
  notify()
}

/**
 * Holds the cached home workspace of `projectId` and records its scope, so a restored home
 * pane resolves its host while GET /home is unreachable. The network stays the authority:
 * the state is `stale` until it answers, and an answer already in is never overwritten.
 */
export async function hydrateHomeWorkspace(projectId: string): Promise<void> {
  const cached = await loadCache<CachedHomeWorkspace>('workspaces-data', cacheKey(projectId))
  if (!cached || states.get(projectId)?.wsId) return
  const { wsId, owningChatId, localPath } = cached.data
  states.set(projectId, { wsId, owningChatId, localPath, error: false, stale: true })
  recordWorkspaceScope({ projectId, repoId: '', wsId, owningChatId: owningChatId ?? undefined })
  notify()
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

/**
 * `projectId`'s own resolved home-workspace id, or `null` if it hasn't
 * resolved (or hasn't been asked for) yet. Unlike `getKnownHomeWorkspaceIds`
 * (deliberately flattened across every project, for WorkspaceHost's
 * retention pruning), a caller scoped to ONE project — e.g. a project's
 * Recents band, which must not pull in another project's home chats — needs
 * this project's id specifically.
 */
export function getHomeWorkspaceId(projectId: string): string | null {
  return states.get(projectId)?.wsId ?? null
}

/** The chat that owns `projectId`'s home workspace, as GET /home reported it,
 *  or `null` before it resolved. The home chat list carries no marker for it,
 *  so this is what `rowsFromHome` needs to keep the owner off the tree. */
export function getHomeOwningChatId(projectId: string): string | null {
  return states.get(projectId)?.owningChatId ?? null
}

/** `projectId`'s home workspace's own on-disk directory, or `null` before it
 *  resolved. The real project root — never a stand-in borrowed from one of
 *  the project's repos. */
export function getHomeWorkspacePath(projectId: string): string | null {
  return states.get(projectId)?.localPath ?? null
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
  for (const stop of awaitingHost.values()) stop()
  awaitingHost.clear()
}
