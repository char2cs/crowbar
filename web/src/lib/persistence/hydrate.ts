import { getDB } from './idb'
import type { EditorState, WorkspaceLayout } from './schemas'
import { loadWindowPaneLayout, WINDOW_LAYOUT_VERSION } from './workspace-layout'
import { getAllEntities } from './entity-cache'
import type { ChatDTO } from '@/lib/types'
import type { PaneGroup } from '@/features/panes/types/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { ViewState } from '@/features/panes/lib/view-state'
import { repairViewState } from '@/features/panes/lib/view-repair'
import { validateLoadedBuffers } from '@/features/panes/utils/persisted-layout'
import {
  isEditorContent,
  isPersistableContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { syncBufferWithDisk } from '@/features/workspace/lib/external-buffer-sync'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { isNotFoundError } from '@/lib/api'
import { loadSidebarUI } from './sidebar-ui'
import { useSidebarStore } from '@/lib/store/sidebar'

export interface WorkspaceHydrationResult {
  editorStates: EditorState[]
}

/**
 * One-time, window-level hydration of pane/buffer layout — awaited at boot
 * BEFORE `renderApp()`: a surface that mounts first and has its layout
 * replaced a frame later crashes.
 */
export async function hydrateWindowPaneLayout(): Promise<void> {
  const stored = await loadWindowPaneLayout()
  if (!stored) return
  const layout = await upgradeWindowPaneLayout(stored)
  const restored = restoreWindowPaneState(layout)
  if (!restored) return
  const { panes, buffers } = validateLoadedBuffers({
    panes: restored.panes,
    buffers: layout.buffers ?? [],
  })
  windowPaneStore.setState({
    ...restored,
    panes,
    activeProjectId: windowPaneStore.getState().activeProjectId,
    buffers: buffers.map(restoreBufferDirtyState),
  })
}

/**
 * The one load-time upgrade of an older saved layout to the current shape.
 *
 * v1 → v2: a chat pane records the workspace its chat belongs to. A v1 pane
 * carries only the chat id, so the workspace is read from the chat's own
 * cached record (the daemon's answer, `crowbar_chats`). A member whose chat
 * the cache does not know cannot be given a workspace without guessing, so it
 * is not restored; a view left without members goes with it (repair).
 */
export async function upgradeWindowPaneLayout(layout: WorkspaceLayout): Promise<WorkspaceLayout> {
  if ((layout.version ?? 1) >= WINDOW_LAYOUT_VERSION) return layout
  const owner = new Map<string, string>()
  for (const chat of await getAllEntities<ChatDTO>('crowbar_chats')) {
    if (chat.workspaceId) owner.set(chat.id, chat.workspaceId)
  }
  const panes: Record<string, PaneGroup> = {}
  for (const [id, pane] of Object.entries(layout.panes ?? {})) {
    if (!pane.chatId) {
      panes[id] = { ...pane, workspaceId: null }
      continue
    }
    const workspaceId = pane.workspaceId || owner.get(pane.chatId)
    if (workspaceId) panes[id] = { ...pane, workspaceId }
  }
  return { ...layout, panes, version: WINDOW_LAYOUT_VERSION }
}

/**
 * The persisted views, repaired: a record, pane or pointer that breaks an
 * invariant is dropped and every valid one is kept. Null only when the record
 * carries no views at all. No older shape is read.
 */
export function restoreWindowPaneState(layout: WorkspaceLayout): ViewState | null {
  if (!layout.views || !layout.panes) return null
  return repairViewState({
    panes: layout.panes,
    views: layout.views,
    viewOrder: layout.viewOrder ?? Object.keys(layout.views),
    activeViewId: layout.activeViewId ?? null,
    activeViewByProject: layout.activeViewByProject ?? {},
    activeProjectId: null,
    stage: layout.stage,
    bottomLayout: layout.bottomLayout,
    activePaneId: layout.activePaneId,
    mostRecentActivePaneIds: layout.mostRecentActivePaneIds ?? [],
    fullscreenPaneId: null,
  })
}

/**
 * Per-workspace hydration: fetches this workspace's own saved editor-view
 * state (cursor/scroll/folds — still workspace+buffer keyed, untouched by
 * Task 26) and reconciles whatever of ITS buffers already exist in the
 * (window-level, already-hydrated-once) pane store against disk — a restored
 * buffer's content can be stale the moment the app opens.
 */
export async function hydrateWorkspace(workspaceId: string): Promise<WorkspaceHydrationResult> {
  const db = await getDB()
  const editorStates = await db.getAllFromIndex('editor-state', 'workspaceId', workspaceId)

  const buffers = windowPaneStore.getState().buffers.filter((b) => b.workspaceId === workspaceId)
  if (buffers.length > 0) {
    await reconcileRestoredBuffers(workspaceId, buffers)
  }

  return { editorStates }
}

/**
 * Reconcile a LIVE workspace's open buffers against disk. Workspace keep-alive
 * skips re-hydration on a warm re-activation (the store never died), but files
 * can change on disk while the workspace sits hidden — its file watcher is
 * active-only, and agents/terminals keep working in hidden worktrees. Called on
 * every hidden→active transition; same policy as restore-time reconciliation:
 * clean buffers silently reload, dirty buffers keep edits and get flagged with
 * hasExternalChange.
 */
export async function reconcileWorkspaceBuffersWithDisk(workspaceId: string): Promise<void> {
  const buffers = windowPaneStore.getState().buffers.filter((b) => b.workspaceId === workspaceId)
  await reconcileRestoredBuffers(workspaceId, buffers)
}

/**
 * Recompute the dirty flag for a restored editor buffer. Persistence stores
 * the full buffer (content + savedContent + isDirty), but a buffer whose
 * content diverges from savedContent must always show as dirty after a
 * reload — even if the persisted snapshot raced and saved isDirty=false.
 */
function restoreBufferDirtyState(buffer: PaneContent): PaneContent {
  if (!isPersistableContent(buffer)) return buffer
  return { ...buffer, isDirty: buffer.isDirty || buffer.content !== buffer.savedContent }
}

/**
 * BUG-026/BUG-013: restored buffers may be stale — the file can change on
 * disk while the app is closed. Reconcile every restored real-file editor
 * buffer with disk using the same policy as live external FS events:
 * clean buffers silently reload, dirty buffers keep edits and get flagged
 * with hasExternalChange + a toast.
 *
 * Unlike a live FS event, at restore time we don't know whether the file
 * actually changed, so we first compare disk against savedContent and only
 * invoke syncBufferWithDisk when they diverge — otherwise every dirty
 * buffer would get a false "changed on disk" toast on every reload.
 */
async function reconcileRestoredBuffers(
  workspaceId: string,
  buffers: PaneContent[],
): Promise<void> {
  const realFileBuffers = buffers.filter(isPersistableContent)
  if (realFileBuffers.length === 0) return
  await Promise.allSettled(
    realFileBuffers.map(async (buffer) => {
      // isPersistableContent buffers always have a real path (openContent
      // requires one for 'editor'); skip defensively if that ever breaks.
      if (!buffer.path) return
      // Read from the hydrating workspace explicitly: hydration can still be
      // in flight when the user switches workspaces, and the active-workspace
      // readFile would then load the sibling worktree's file into this store.
      let diskContent: string | null
      try {
        diskContent = await readWorkspaceFile(workspaceId, buffer.path)
      } catch (err) {
        // BUG-001: a 404 means the file is gone (e.g. its worktree was
        // deleted). Mark a clean buffer terminally so the pane shows a "file
        // not found" placeholder and nothing re-fetches the dead path. A
        // dirty buffer keeps its editor: the unsaved edits are the only copy
        // left, and saving recreates the file. Other errors (network, 5xx)
        // stay silent and non-terminal as before.
        if (isNotFoundError(err) && !buffer.isDirty) {
          setBufferFileMissing(workspaceId, buffer.path, true)
        }
        return
      }
      // The file is back (or was never gone) — clear a stale missing flag
      // that may have been persisted by a previous session.
      if (buffer.fileMissing) setBufferFileMissing(workspaceId, buffer.path, false)
      if (diskContent === buffer.savedContent) return
      await syncBufferWithDisk(workspaceId, buffer.path)
    }),
  )
}

function setBufferFileMissing(workspaceId: string, path: string, fileMissing: boolean): void {
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) =>
      isEditorContent(b) && !b.isVirtual && b.path === path && b.workspaceId === workspaceId
        ? { ...b, fileMissing }
        : b,
    ),
  }))
}

export async function hydrateSidebar(): Promise<void> {
  const sidebarUI = await loadSidebarUI()
  if (!sidebarUI) return
  // `collapsedRepos`/`collapsedWorkspaces`/`collapsedProjects` are retired
  // keys the pre-restyle tree wrote (see schemas.ts) — never replayed.
  useSidebarStore.setState({
    // Absent on a record written before the Chats panel was collapsible —
    // replays as "nothing folded", the product default (see schemas.ts).
    collapsedChatRows: new Set(sidebarUI.collapsedChatRows ?? []),
  })
}
