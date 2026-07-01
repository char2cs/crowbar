import { getDB } from './idb'
import type { WorkspaceLayout, EditorState, UIPreferences } from './schemas'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
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
  layout: WorkspaceLayout | null
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

export async function hydrateWorkspace(workspaceId: string): Promise<WorkspaceHydrationResult> {
  const db = await getDB()

  const [layout, editorStates] = await Promise.all([
    db.get('workspace-layout', workspaceId).then((r) => r ?? null),
    db.getAllFromIndex('editor-state', 'workspaceId', workspaceId),
  ])

  const store = getOrCreateWorkspaceStore(workspaceId)

  if (layout) {
    const buffers = (layout.buffers ?? []).map(restoreBufferDirtyState)

    store.setState({
      activePaneId: layout.activePaneId,
      mostRecentActivePaneIds: layout.mostRecentActivePaneIds ?? [layout.activePaneId],
      panes: layout.panes,
      rootLayout: layout.rootLayout,
      bottomLayout: layout.bottomLayout,
      buffers,
    })

    await reconcileRestoredBuffers(store, buffers)
  }

  return { layout, editorStates }
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
  store: WorkspaceStore,
  buffers: PaneContent[],
): Promise<void> {
  const realFileBuffers = buffers.filter(isPersistableContent)
  if (realFileBuffers.length === 0) return
  await Promise.allSettled(
    realFileBuffers.map(async (buffer) => {
      // Read from the hydrating workspace explicitly: hydration can still be
      // in flight when the user switches workspaces, and the active-workspace
      // readFile would then load the sibling worktree's file into this store.
      let diskContent: string | null = null
      try {
        diskContent = await readWorkspaceFile(store.getState().workspaceId, buffer.path)
      } catch (err) {
        // BUG-001: a 404 means the file is gone (e.g. its worktree was
        // deleted). Mark a clean buffer terminally so the pane shows a "file
        // not found" placeholder and nothing re-fetches the dead path. A
        // dirty buffer keeps its editor: the unsaved edits are the only copy
        // left, and saving recreates the file. Other errors (network, 5xx)
        // stay silent and non-terminal as before.
        if (isNotFoundError(err) && !buffer.isDirty) {
          setBufferFileMissing(store, buffer.path, true)
        }
        return
      }
      // The file is back (or was never gone) — clear a stale missing flag
      // that may have been persisted by a previous session.
      if (buffer.fileMissing) setBufferFileMissing(store, buffer.path, false)
      if (diskContent === buffer.savedContent) return
      await syncBufferWithDisk(store, buffer.path)
    }),
  )
}

function setBufferFileMissing(store: WorkspaceStore, path: string, fileMissing: boolean): void {
  store.setState((state) => ({
    buffers: state.buffers.map((b) =>
      isEditorContent(b) && !b.isVirtual && b.path === path ? { ...b, fileMissing } : b,
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
      collapsedChats: new Set(sidebarUI.collapsedChats ?? []),
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
