import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { useHistoryStore } from '@/features/editor/stores/history-store'
import { cleanupBufferHistoryTracking } from '@/features/editor/stores/buffer-history-tracking'
import type { TerminalContent } from '@/features/panes/types/pane-content'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { setActiveScopeWorkspaceId } from '@/lib/workspace-scope'
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

export function setActiveWorkspaceId(wsId: string): void {
  _activeWorkspaceId = wsId
  setActiveScopeWorkspaceId(wsId)
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

export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    // Task 26: pane/buffer layout no longer lives on this per-workspace
    // snapshot (it's window-level now — see window-pane-store.ts), so the
    // only fields left to restore here are recentFiles/terminalLayout.
    const snapshot = loadFromLocalStorage(wsId) ?? undefined
    const store = createWorkspaceStore(wsId, snapshot)
    registry.set(wsId, store)
    notifyRegistryListeners()
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
 */
const registryListeners = new Set<() => void>()

function notifyRegistryListeners(): void {
  for (const listener of registryListeners) listener()
}

export function subscribeWorkspaceRegistry(callback: () => void): () => void {
  registryListeners.add(callback)
  return () => {
    registryListeners.delete(callback)
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
  const rebind = () => {
    const store = registry.get(wsId)
    if (store === bound) return
    unbind?.()
    bound = store
    unbind = store ? store.subscribe(callback) : null
    callback()
  }
  const unsubscribeRegistry = subscribeWorkspaceRegistry(rebind)
  rebind()
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
 * Which registered workspace store's `agentChats.chats` names `chatId`, or
 * null if none does — the general single-chat form of `isChatWorking`'s own
 * scan (a chat's owning workspace is not encoded in its id, so every
 * registered store has to be searched), returning the owning id itself
 * rather than a boolean.
 *
 * This is the real mechanism `recents-for-project.ts`'s `recentsForProject`
 * already uses inline to build a whole project's chatId->workspaceId map in
 * one pass (batch, not exposed as a per-chat function there because a
 * project's Recents band needs the map for every chat at once, not one
 * lookup at a time) — this is the single-id form for a caller that has
 * exactly one chat to resolve, no project to scope the scan to, and needs
 * the id itself rather than a batch.
 *
 * Returns the REGISTRY KEY the chat was found under, not `chat.workspaceId`
 * (a field `AgentChat` also carries) — the registry key is what tells a
 * caller "this workspace has a live store right now", which is what every
 * caller of this resolver actually needs (a store to read/act against), and
 * doesn't require trusting a denormalized field on the chat record to agree
 * with where it was actually found.
 *
 * REGISTRY-SCOPED, NOT OMNISCIENT: only searches currently-REGISTERED
 * stores (`WorkspaceHost`'s keep-alive set). `WorkspaceHost` evicts and
 * destroys a workspace's store on its own age/LRU window, while a pane
 * holding that workspace's chat deliberately outlives the eviction — that
 * survival is the entire point of Task 26's window-level pane hoist. So a
 * pane whose chat belongs to an EVICTED (no longer registered) workspace —
 * exactly the case this resolver exists to serve — resolves to null here,
 * same as a chat that never existed. A caller that must survive eviction
 * needs a second source (e.g. persisted chat metadata) this function
 * intentionally does not attempt to be.
 */
export function resolveWorkspaceIdForChat(chatId: string): string | null {
  for (const [wsId, store] of registry.entries()) {
    if (store.getState().agentChats.chats.some((chat) => chat.id === chatId)) return wsId
  }
  return null
}

export function destroyWorkspaceStore(wsId: string): void {
  const store = registry.get(wsId)
  if (store) {
    // Task 26: buffers are window-level now (window-pane-store.ts), not part
    // of this store's own state — scope the lookup to this workspace's own
    // buffers via `workspaceId` (a buffer persists past this destroy; only
    // the LIVE per-workspace resources below — the terminal transport, the
    // blame cache, the undo history, the Monaco model — are torn down here).
    // Dynamic import avoids a registry ↔ window-pane-store cycle (the pane
    // slice itself resolves an editor manager BY workspace id through this
    // very module).
    bestEffort(
      import('@/features/panes/stores/window-pane-store').then(({ windowPaneStore }) => {
        const paneState = windowPaneStore.getState()
        // Task 26 fix round 1 (I2): a buffer belonging to this workspace can
        // still be open in a pane RIGHT NOW — buffers/panes are window-level
        // and outlive this workspace's own destroy by design (that is the
        // whole point of the hoist). Tearing down its live resources anyway
        // would leave a still-visible terminal with a detached transport, or
        // a still-open editor with a disposed Monaco model and wiped undo
        // history. Only buffers no pane currently references (closed
        // everywhere, just not yet swept from the flat list) are safe to
        // tear down here.
        const openEditorTabIds = new Set(
          Object.values(paneState.panes).flatMap((pane) => pane.editorTabIds),
        )
        const buffers = paneState.buffers.filter(
          (b) => b.workspaceId === wsId && !openEditorTabIds.has(b.id),
        )

        // Detach (not kill) pane terminal PTY sessions on workspace switch.
        // The PTY stays alive in the daemon; the WS transport is closed and the
        // connectionId is persisted to localStorage so re-entry can re-attach with
        // scrollback replay. killTerminalSession is still used on real tab close.
        const terminalBuffers = buffers.filter((b) => b.type === 'terminal')
        if (terminalBuffers.length > 0) {
          bestEffort(
            import('@/features/terminal/lib/detach-terminal-session').then(
              ({ detachTerminalSession }) => {
                for (const buf of terminalBuffers) {
                  void detachTerminalSession(wsId, (buf as TerminalContent).sessionId).catch(
                    () => {},
                  )
                }
              },
            ),
            'detach terminal sessions',
          )
        }

        // Free cached git-blame for this workspace's open files. The blame store is a
        // global singleton keyed by file path, so clearAllBlame() would wipe blame for
        // OTHER still-active workspaces; we instead clear only this workspace's editor
        // buffer paths.
        const editorPaths: string[] = []
        for (const b of buffers) {
          if (isEditorContent(b) && b.path) editorPaths.push(b.path)
        }
        if (editorPaths.length > 0) {
          bestEffort(
            import('@/features/git/stores/git-blame-store').then(({ useGitBlameStore }) => {
              const { clearBlameForFile } = useGitBlameStore.getState()
              for (const path of editorPaths) {
                clearBlameForFile(path)
              }
            }),
            'clear blame for disposed workspace',
          )
        }

        // Cleanup undo tracker and history for each buffer
        for (const buf of buffers) {
          cleanupBufferHistoryTracking(buf.id)
          useHistoryStore.getState().actions.clearHistory(buf.id)
        }

        // Dispose editor resources (only if the workspace ever armed the
        // editor — a terminal/agent-only workspace never constructs the
        // manager) — and only when NONE of this workspace's editor buffers
        // are still open in a live pane. disposeAll() unmounts every pane
        // this workspace's EditorManager has a Monaco editor mounted into and
        // disposes its whole model registry; doing that while a buffer it
        // owns is still visible would kill a live editor out from under the
        // user (disposed model, wiped undo history) rather than merely
        // freeing a resource nobody can see any more.
        //
        // Task 26 fix round 2 (I2 revisited) reverted this gate to
        // unconditional, reasoning that editor-surface.tsx resolved its
        // EditorManager from the AMBIENT workspace, never from
        // `buf.workspaceId` — so a still-visible copy of this buffer,
        // rendered by a different WorkspaceView, could never be using the
        // manager being destroyed, making the gate dead weight that only cost
        // a permanent leak. editor-pane.tsx/editor-surface.tsx now resolve
        // the manager via `getWorkspaceStore(buf.workspaceId)` instead (the
        // ambient-hidden-copy leak that round 2 traded this gate away for),
        // which makes round 2's premise false: a still-open buffer's REAL
        // manager is once again this one, wherever it is rendered from. The
        // gate is restored so disposeAll() cannot yank a live widget's model.
        const hasSurvivingEditorBuffer = paneState.buffers.some(
          (b) => b.workspaceId === wsId && isEditorContent(b) && openEditorTabIds.has(b.id),
        )
        if (!hasSurvivingEditorBuffer) {
          store.editorManager?.disposeAll()
        }
      }),
      'window-pane-store buffer teardown',
    )
  }

  // Drop the warm-reactivation freshness ledger for this workspace so a future
  // workspace reusing the id can't inherit a stale "hidden briefly" stamp.
  clearWorkspaceFreshness(wsId)

  registry.delete(wsId)
  // After the delete, so a watcher re-binding on this signal sees the store
  // already gone rather than re-attaching to the one being torn down.
  notifyRegistryListeners()
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
