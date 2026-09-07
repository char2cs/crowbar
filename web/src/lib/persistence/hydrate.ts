import { getDB } from './idb'
import type { EditorState, UIPreferences, WorkspaceLayout } from './schemas'
import { loadWindowPaneLayout } from './workspace-layout'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LayoutNode } from '@/features/panes/types/pane'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { partitionLayoutByView, viewIdOf } from '@/features/panes/lib/pane-views'
import { getAllLeafIds, getFirstLeafId } from '@/features/panes/utils/pane-layout'
import {
  isEditorContent,
  isPersistableContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { syncBufferWithDisk } from '@/features/workspace/lib/external-buffer-sync'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { useSettingsStore } from '@/features/settings/store'
import { isNotFoundError } from '@/lib/api'
import { loadSidebarUI } from './sidebar-ui'
import { loadAllWorkspaceHierarchies } from './workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'

export interface WorkspaceHydrationResult {
  editorStates: EditorState[]
}

export async function hydratePreferences(): Promise<UIPreferences | null> {
  const db = await getDB()
  const prefs = await db.get('ui-preferences', 'global').then((r) => r ?? null)

  if (prefs) {
    useSettingsStore.setState((state) => ({
      settings: {
        ...state.settings,
        theme: prefs.theme,
        fontSize: prefs.fontSize,
        fontFamily: prefs.fontFamily,
        tabSize: prefs.tabSize,
        wordWrap: prefs.wordWrap,
        showMinimap: prefs.minimap,
      },
    }))
  }

  return prefs
}

/**
 * One-time, WINDOW-level hydration of pane/buffer layout — call once at app
 * boot (from `HydrationGate`, alongside `hydratePreferences`/`hydrateSidebar`),
 * BEFORE any `WorkspaceView` mounts. Task 26 moved panes/buffers off the
 * per-workspace store registry onto one window-level store
 * (`window-pane-store.ts`) that is never destroyed, so there is exactly one
 * persisted layout row to restore here, not one per workspace — see
 * `workspace-layout.ts`'s `WINDOW_SESSION_ID`.
 */
export async function hydrateWindowPaneLayout(): Promise<void> {
  const layout = await loadWindowPaneLayout()
  if (!layout) return

  const buffers = (layout.buffers ?? []).map(restoreBufferDirtyState)
  const views = restoreWindowViews(layout)

  windowPaneStore.setState({
    activePaneId: views.activePaneId,
    mostRecentActivePaneIds: layout.mostRecentActivePaneIds ?? [views.activePaneId],
    panes: layout.panes,
    rootLayout: views.rootLayout,
    parkedViews: views.parkedViews,
    activeViewId: views.activeViewId,
    bottomLayout: layout.bottomLayout,
    buffers,
  })
}

export interface RestoredWindowViews {
  rootLayout: LayoutNode
  parkedViews: Record<string, LayoutNode>
  activeViewId: string
  activePaneId: string
}

type PersistedViewShape = Pick<
  WorkspaceLayout,
  'panes' | 'rootLayout' | 'parkedViews' | 'activeViewId' | 'activePaneId'
>

/**
 * Rebuild the window's per-view trees from a persisted layout — the half of
 * hydration that decides what is ON SCREEN after a reload.
 *
 * IT ENFORCES THE INVARIANT RATHER THAN TRUSTING THE RECORD: the showing tree
 * holds exactly one view, whatever was written to disk. Checking for the
 * presence of `parkedViews`/`activeViewId` instead is what an earlier pass did
 * and it is not enough — a record can carry both fields and STILL have a
 * `rootLayout` mixing views, which is precisely the state an upgrade produces
 * the first time the new store persists over a layout the old one wrote (the
 * new fields take their defaults, `rootLayout` is still the old tiled tree,
 * and `parkedViews: {}` is a perfectly truthy empty object). Trusting the
 * fields there restored the side-by-side tiling this whole feature removes —
 * observed live, not hypothesised.
 *
 * So `rootLayout` is always partitioned by `viewId` (already tagged on the
 * panes, which is why this needs no migration — just a correct reading of the
 * old shape), the view that should show is chosen from what the record says,
 * and every other view the tree was mixing in joins the parked set. A record
 * that already honours the invariant partitions to a single tree and comes
 * back untouched.
 */
export function restoreWindowViews(layout: PersistedViewShape): RestoredWindowViews {
  const panes = layout.panes ?? {}
  const settled = settleViewTrees(layout, panes)

  // `activePaneId` has to name a pane the SHOWING tree actually holds —
  // otherwise the active-pane ring, the keyboard commands and every
  // `getActivePane()` caller address a pane nobody can see.
  const leaves = getAllLeafIds(settled.rootLayout)
  const activePaneId = leaves.includes(layout.activePaneId)
    ? layout.activePaneId
    : getFirstLeafId(settled.rootLayout)

  return { ...settled, activePaneId }
}

function settleViewTrees(
  layout: PersistedViewShape,
  panes: WorkspaceLayout['panes'],
): Omit<RestoredWindowViews, 'activePaneId'> {
  const trees = partitionLayoutByView(layout.rootLayout, panes)
  const parkedViews = { ...(layout.parkedViews ?? {}) }

  // Which view should be the one showing, in descending order of what the
  // record actually knows: what it says was active, else the view owning the
  // pane it says was focused, else whichever the tree yields first.
  const focused = panes[layout.activePaneId]
  const showing =
    (layout.activeViewId && trees[layout.activeViewId] ? layout.activeViewId : undefined) ??
    (focused && trees[viewIdOf(focused)] ? viewIdOf(focused) : undefined) ??
    Object.keys(trees)[0]

  if (!showing) {
    // Nothing in the tree to show — the empty stage, or a record too broken to
    // read. Hand back what was written and let the caller's own activePaneId
    // healing take it from there.
    return {
      rootLayout: layout.rootLayout,
      parkedViews,
      activeViewId: layout.activeViewId ?? ROOT_PANE_ID,
    }
  }

  // Every view the showing tree was mixing in is a view in its own right,
  // parked. A stored parked entry under one of those ids is a corrupt record
  // (two authoritative copies of one arrangement) — the copy that was really
  // in the tree wins.
  for (const [viewId, tree] of Object.entries(trees)) {
    if (viewId !== showing) parkedViews[viewId] = tree
  }
  // ...and the showing view is never also parked, for the same reason.
  delete parkedViews[showing]

  return { rootLayout: trees[showing], parkedViews, activeViewId: showing }
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
  const [sidebarUI, hierarchies] = await Promise.all([
    loadSidebarUI(),
    loadAllWorkspaceHierarchies(),
  ])

  if (sidebarUI) {
    useSidebarStore.setState({
      collapsedRepos: new Set(sidebarUI.collapsedRepos),
      collapsedWorkspaces: new Set(sidebarUI.collapsedWorkspaces ?? []),
      // Absent on a record written before projects were collapsible — replays
      // as "nothing collapsed", i.e. every project open, which is the product
      // default (see sidebar.ts `collapsedProjects`).
      collapsedProjects: new Set(sidebarUI.collapsedProjects ?? []),
      // Absent on a record written before the Chats panel was collapsible —
      // replays as "nothing folded", the product default (see schemas.ts).
      collapsedChatRows: new Set(sidebarUI.collapsedChatRows ?? []),
    })
  }

  if (hierarchies.length > 0) {
    useSidebarStore.setState((s) => ({
      repos: s.repos.map((repo) => {
        const hierarchy = hierarchies.find((h) => h.repoId === repo.id)
        if (!hierarchy) return repo
        const entryMap = new Map(hierarchy.entries.map((e) => [e.wsId, e.parentId]))
        return {
          ...repo,
          workspaces: repo.workspaces.map((ws) =>
            entryMap.has(ws.id) ? { ...ws, parentId: entryMap.get(ws.id) } : ws,
          ),
        }
      }),
    }))
  }
}
