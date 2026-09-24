import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { bindActiveWorkspaceId } from '@/lib/workspace-scope'
import { bestEffort } from '@/lib/best-effort'
import { clearWorkspaceFreshness } from '../lib/activation-freshness'

const registry = new Map<string, WorkspaceStore>()

let _activeWorkspaceId: string | null = null

// §3/§7: workspace-scoped API/WS URLs are now hierarchical
// (/v0/projects/:p/repos/:r/workspaces/:w/...). The owning project+repo of the
// active workspace are threaded from the TanStack route and recorded so the many
// wsId-keyed callers (files, git, terminal, editor) can resolve the full scope
// without every signature growing two params. The scope MAP itself lives in the
// dependency-free `@/lib/workspace-scope` module so those lightweight builders
// don't import this heavy registry (which pulls in the editor/Monaco graph and
// timed out their dynamic-import unit tests). We re-export the setter/getter here
// for callers that already depend on the registry.
export { setWorkspaceScope, getWorkspaceScope, type WorkspaceScope } from '@/lib/workspace-scope'

/**
 * THE active workspace (C4) — one id, written by exactly one owner:
 * `WorkspaceHost`, in the same layout effect that mounts the stores, so every
 * reader (scoped URLs, the active store, jump navigation) agrees within a
 * commit. The scope module reads it through the binding below rather than
 * keeping a copy of its own.
 */
export function setActiveWorkspaceId(wsId: string | null): void {
  _activeWorkspaceId = wsId
}

bindActiveWorkspaceId(() => _activeWorkspaceId)

/** Test-only: make `store` (registered under its own workspace id) the
 *  active one, or clear the active id — what the host does for a real mount. */
export function setActiveWorkspaceStoreForTests(store: WorkspaceStore | null): void {
  if (!store) {
    _activeWorkspaceId = null
    return
  }
  const wsId = store.getState().workspaceId
  registry.set(wsId, store)
  _activeWorkspaceId = wsId
}

export function getActiveWorkspaceStore(): WorkspaceStore | null {
  if (!_activeWorkspaceId) return null
  return registry.get(_activeWorkspaceId) ?? null
}

export function getActiveWorkspaceId(): string | null {
  return _activeWorkspaceId
}

/**
 * A registered workspace store, or `undefined` if none exists — never
 * creates one. Task 26 fix round 1 (I3): `editorManagerFor(workspaceId)`
 * (pane-slice.ts/buffer-slice.ts) resolves a buffer's per-workspace Monaco
 * manager by id, and a buffer can outlive its owning workspace's eviction
 * (buffers are window-level now, panes/tabs can still reference one whose
 * workspace was already destroyed). Looking that id up with
 * `getOrCreateWorkspaceStore` would silently re-register a store
 * `WorkspaceHost` never mounted and will never destroy — a real per-session
 * leak. Callers that only want to read an existing store, never mint one,
 * must use this instead.
 */
export function getWorkspaceStore(wsId: string): WorkspaceStore | undefined {
  return registry.get(wsId)
}

let detached: WorkspaceStore | undefined

/** An empty store that belongs to no workspace and is never registered — the
 *  read-only stand-in a render path gets for a workspace that is not mounted. */
export function detachedWorkspaceStore(): WorkspaceStore {
  detached ??= createWorkspaceStore('')
  return detached
}

/**
 * Mint (or return) `wsId`'s store. Only `WorkspaceHost` calls this in the app
 * — from its reconcile effect, never from render (C6): the registry's keys are
 * exactly the host's mounted set (C5). Everything else reads with
 * {@link getWorkspaceStore} and copes with a workspace that is not mounted.
 */
export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    // Task 26: pane/buffer layout no longer lives on this per-workspace
    // snapshot (it's window-level now — see window-pane-store.ts), so the
    // only fields left to restore here are recentFiles/terminalLayout.
    const snapshot = loadFromLocalStorage(wsId) ?? undefined
    const store = createWorkspaceStore(wsId, snapshot)
    registry.set(wsId, store)
    notifyRegistryListeners('registered')
  }
  return registry.get(wsId)!
}

/**
 * Notified whenever a workspace store is REGISTERED or DESTROYED — i.e.
 * whenever `getWorkspaceStore(wsId)` might start (or stop) answering.
 *
 * The registry is a plain Map, so there has never been anything to subscribe
 * to; every caller either held a store already or minted one. That is fine for
 * code that owns a workspace, and wrong for code that merely WATCHES one it
 * must not bring into existence — the sidebar tree, which draws a row per
 * workspace in the repo and would otherwise mint (and permanently leak, see
 * `getWorkspaceStore`'s own doc) a store for every row the user has never
 * opened. Those watchers need to re-bind when the real store finally appears,
 * and this is the only signal that says it has.
 *
 * WHICH KIND of registry change happened. The distinction is not cosmetic —
 * it decides whether a watcher may PUSH a change at its React subscriber, or
 * must only re-bind itself:
 *
 * - `'registered'`: a brand-new store was just minted. Every registry-wide
 *   answer this module exposes ({@link isChatWorking},
 *   {@link readChatWorking}, and the `agentChats` scans built on
 *   {@link getAllActiveWorkspaceIds}) is derived from `agentChats`, which a
 *   freshly created store has none of — `createWorkspaceStore`'s persisted
 *   snapshot restores only recentFiles/terminalLayout. So registration cannot
 *   move any watcher's answer, and pushing one is not merely redundant: it is
 *   a setState fired from the RENDER PATH. `getOrCreateWorkspaceStore` is
 *   deliberately called during render (`WorkspaceView`, `WindowPaneSurface`) —
 *   a workspace forced into the mounted set by the route has no store until
 *   its own render mints one — so the push landed inside React's render phase
 *   and updated `IDEShell`'s `useSyncExternalStore` hooks
 *   while `WorkspaceView` was still rendering:
 *   "Cannot update a component (`IDEShell`) while rendering a different
 *   component (`WorkspaceView`)". The RE-BIND still has to happen
 *   synchronously — the very next write to the new store (its chats stream
 *   landing, usually in the same tick) is what carries the real change, and a
 *   watcher not yet attached would miss it.
 * - `'destroyed'`: a store went away, taking its chats with it. That DOES
 *   change the answers, and only ever happens from an effect
 *   (`WorkspaceHost`'s eviction/unmount), never from render — so it pushes.
 */
export type WorkspaceRegistryChange = 'registered' | 'destroyed'

const registryListeners = new Set<(change: WorkspaceRegistryChange) => void>()

function notifyRegistryListeners(change: WorkspaceRegistryChange): void {
  for (const listener of registryListeners) listener(change)
}

export function subscribeWorkspaceRegistry(
  callback: (change: WorkspaceRegistryChange) => void,
): () => void {
  registryListeners.add(callback)
  return () => {
    registryListeners.delete(callback)
  }
}

/**
 * Fire `callback` whenever ANY registered workspace store changes, and
 * whenever the set of registered stores itself changes.
 *
 * The subscribe half of the registry-wide scans this module already exposes
 * ({@link isChatWorking}) — those answer
 * "right now" and had no way to say "ask again". A render path resolving a
 * chat's owning workspace needs both: which workspace a pane's chat belongs to
 * is unknowable until SOME store has been seeded with that chat, and the pane
 * mounts before that happens.
 *
 * Re-binds on every registry change, so a workspace mounted after this was
 * armed is watched too — the same rebind {@link subscribeChatWorking} makes
 * for one id, widened to all of them because a chat's owning store is exactly
 * what the caller does not know yet.
 */
export function subscribeWorkspaceStores(callback: () => void): () => void {
  let bound: Array<() => void> = []
  // `notify` is false for the FIRST bind only: a subscriber has just read the
  // current answer for itself, and firing at it there is an update during
  // subscription that no caller asked for (React's own `useSyncExternalStore`
  // re-checks after subscribing anyway, and warns about the stray one).
  // It is false for a `'registered'` change too — an empty new store moves no
  // answer, and the push would land mid-render; see
  // {@link WorkspaceRegistryChange}.
  const rebind = (notify: boolean) => {
    for (const unbind of bound) unbind()
    bound = [...registry.values()].map((store) => store.subscribe(callback))
    if (notify) callback()
  }
  const unsubscribeRegistry = subscribeWorkspaceRegistry((change) => rebind(change === 'destroyed'))
  rebind(false)
  return () => {
    unsubscribeRegistry()
    for (const unbind of bound) unbind()
    bound = []
  }
}

/**
 * Watch whether `chatId` is mid-turn inside `wsId`, WITHOUT creating `wsId`'s
 * store if it does not exist.
 *
 * The subscribe half of {@link isChatWorking}, narrowed to one workspace
 * because the caller (a sidebar row) already knows which workspace its chat
 * runs in and has no business waking on every other workspace's writes.
 *
 * Re-binds through {@link subscribeWorkspaceRegistry}, which is what makes the
 * not-yet-mounted case correct rather than merely safe: a row whose workspace
 * has no store reads `false` (nothing is running a turn in a workspace with no
 * live store — the `working` map is filled by that workspace's own chats
 * stream, which only runs while it is mounted), and the moment the workspace
 * IS mounted this attaches to the real store and the row starts spinning.
 * Without the re-bind the row would be stuck on that `false` for the life of
 * the session, which is precisely the "I never see the loading state" this
 * exists to end.
 */
export function subscribeChatWorking(wsId: string, callback: () => void): () => void {
  let bound: WorkspaceStore | undefined
  let unbind: (() => void) | null = null
  // `notify` follows the same rule as `subscribeWorkspaceStores` above: a
  // `'registered'` change re-binds silently (a new store's `working` map is
  // empty, so the row's answer cannot have moved, and the push would land in
  // the render phase — see {@link WorkspaceRegistryChange}); the row learns
  // the moment that store's own chats stream writes to it.
  const rebind = (notify: boolean) => {
    const store = registry.get(wsId)
    if (store === bound) return
    unbind?.()
    bound = store
    unbind = store ? store.subscribe(callback) : null
    if (notify) callback()
  }
  const unsubscribeRegistry = subscribeWorkspaceRegistry((change) => rebind(change === 'destroyed'))
  rebind(true)
  return () => {
    unsubscribeRegistry()
    unbind?.()
  }
}

/** The snapshot half of {@link subscribeChatWorking} — a plain read, no store
 *  minted, `false` for a workspace with no live store. */
export function readChatWorking(wsId: string, chatId: string): boolean {
  return registry.get(wsId)?.getState().agentChats.working[chatId] ?? false
}

/**
 * Whether `chatId` is currently mid-turn, per whichever active workspace
 * store's `agentChats.working` map actually names it. A chat's owning store
 * is not known ahead of time from the id alone — a chat belongs to exactly
 * one workspace, but which one is not encoded in the id — so this searches
 * every currently-registered workspace store. Used by the window-level pane
 * slice (`features/panes/stores/slices/pane-slice.ts`) in place of the old
 * same-store `state.agentChats.working[chatId]` read, now that panes no
 * longer live in the same store as a workspace's own agent-chat state.
 */
export function isChatWorking(chatId: string): boolean {
  for (const store of registry.values()) {
    if (store.getState().agentChats.working[chatId]) return true
  }
  return false
}

/**
 * Whether `wsId`'s store may be destroyed now. Its EditorManager still having
 * a Monaco widget mounted into a real pane means a pane is showing one of its
 * editors: destroying it would strand that widget on a dead manager. The host
 * asks this BEFORE evicting and keeps the workspace instead, so a destroy is
 * never vetoed after the host already let go — which used to leave a zombie
 * store registered that the next mount picked back up, spinner and all.
 */
export function canEvictWorkspace(wsId: string): boolean {
  return !registry.get(wsId)?.editorManager?.hasMountedPanes()
}

export function destroyWorkspaceStore(wsId: string): void {
  const store = registry.get(wsId)
  if (store) {
    // Every buffer that exists is listed by some pane (invariant C2): one of
    // this workspace's is on screen and keeps its resources, which are freed
    // when its last pane lets go (buffer-release.ts) — not here. What this
    // store alone owns is the Monaco model registry, disposed once none of
    // its editor buffers survive. Dynamic import avoids a registry ↔
    // window-pane-store cycle.
    bestEffort(
      import('@/features/panes/stores/window-pane-store').then(({ windowPaneStore }) => {
        const survives = windowPaneStore
          .getState()
          .buffers.some((b) => b.workspaceId === wsId && isEditorContent(b))
        if (!survives) store.editorManager?.disposeAll()
      }),
      'dispose editor models',
    )
  }

  // Drop the warm-reactivation freshness ledger for this workspace so a future
  // workspace reusing the id can't inherit a stale "hidden briefly" stamp.
  clearWorkspaceFreshness(wsId)

  registry.delete(wsId)
  // After the delete, so a watcher re-binding on this signal sees the store
  // already gone rather than re-attaching to the one being torn down.
  notifyRegistryListeners('destroyed')
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
