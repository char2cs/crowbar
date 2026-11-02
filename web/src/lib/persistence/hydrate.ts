import type { WorkspaceLayout } from './schemas'
import {
  loadWindowPaneLayout,
  saveWorkspaceLayout,
  WINDOW_LAYOUT_VERSION,
} from './workspace-layout'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import type { PaneGroup } from '@/features/panes/types/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { ViewState } from '@/features/panes/lib/view-state'
import { repairViewState } from '@/features/panes/lib/view-repair'
import {
  splitLegacyEditorContent,
  upgradeLegacyBuffers,
  validateLoadedBuffers,
} from '@/features/panes/utils/persisted-layout'
import { getFirstLeafId } from '@/features/panes/utils/pane-layout'
import { putDrafts } from '@/features/editor/documents/draft-store'
import { restoreDocuments } from '@/features/editor/documents/restore-documents'
import { loadSidebarUI } from './sidebar-ui'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'

/**
 * One-time, window-level hydration of pane/buffer layout — awaited at boot
 * BEFORE `renderApp()`: a surface that mounts first and has its layout
 * replaced a frame later crashes. Local reads only; a restored member the
 * cache could not place is answered by `placeRestoredChatMembers` afterwards.
 */
export async function hydrateWindowPaneLayout(): Promise<void> {
  const stored = await loadWindowPaneLayout()
  if (!stored) return
  const layout = await upgradeWindowPaneLayout(stored)
  // Stamp the upgrade at once, losing nothing, so it runs exactly once.
  if (layout !== stored) await saveWorkspaceLayout(layout)
  const restored = restoreWindowPaneState(layout)
  const adoptInto = getFirstLeafId(restored.stage)
  const before = new Set(restored.panes[adoptInto].editorTabIds)
  const { panes, buffers } = validateLoadedBuffers(
    { panes: restored.panes, buffers: layout.buffers ?? [] },
    // A record without views held its buffers outside any view: keep them all.
    { adoptInto, keepUnlisted: !layout.views },
  )
  if (panes[adoptInto].editorTabIds.some((id) => !before.has(id))) {
    panes[adoptInto] = { ...panes[adoptInto], editorOpen: true }
  }
  windowPaneStore.setState({
    ...restored,
    panes,
    activeProjectId: windowPaneStore.getState().activeProjectId,
    buffers,
  })
  // Documents load behind the restored tabs: Monaco and disk are not on the boot path.
  restoreDocuments().catch((err: unknown) => console.error('Restoring documents failed:', err))
}

/**
 * The one load-time upgrade of an older saved layout to the current shape.
 * Idempotent: a layout already at `WINDOW_LAYOUT_VERSION` is returned as is.
 *
 * v1 → v2: a chat pane records the workspace its chat belongs to; one without it
 * stays unplaced (null) until the projects scope answers for it.
 * v2 → v3: buffers name their documents (`upgradeLegacyBuffers`).
 * v3 → v4: buffers drop their text; unsaved text is written as drafts first.
 */
export async function upgradeWindowPaneLayout(layout: WorkspaceLayout): Promise<WorkspaceLayout> {
  const version = layout.version ?? 1
  if (version >= WINDOW_LAYOUT_VERSION) return layout
  const placed = version < 2 ? unplaceChatPanes(layout) : layout
  const buffers = placed.buffers && upgradeLegacyBuffers(placed.buffers)
  const split = splitLegacyEditorContent({ ...placed, buffers })
  await putDrafts(split.drafts)
  return { ...split.layout, version: WINDOW_LAYOUT_VERSION }
}

function unplaceChatPanes(layout: WorkspaceLayout): WorkspaceLayout {
  const panes: Record<string, PaneGroup> = {}
  for (const [id, pane] of Object.entries(layout.panes ?? {})) {
    panes[id] = { ...pane, workspaceId: pane.chatId ? pane.workspaceId || null : null }
  }
  return { ...layout, panes }
}

function placeFrom(s: EntityState): void {
  const { panes, views } = windowPaneStore.getState()
  const placed = new Map<string, string>()
  const gone = new Set<string>()
  for (const pane of Object.values(panes)) {
    const inView = pane.viewId ? views[pane.viewId]?.projectId !== undefined : false
    if (!pane.chatId || pane.workspaceId || !inView) continue
    const chat = s.byKind.chat[pane.chatId]?.data
    if (chat?.workspaceId) placed.set(pane.chatId, chat.workspaceId)
    else if (!chat) gone.add(pane.chatId)
  }
  if (placed.size > 0 || gone.size > 0) {
    windowPaneStore.getState().paneActions.placeRestoredMembers(placed, gone)
  }
}

/** Once the projects scope's first snapshot lands, places each restored chat pane that
 *  names no workspace in its chat's, and drops one whose chat is gone. */
export function placeRestoredPanesOnceSynced(): void {
  if (entityStore.getState().loaded.projects) return placeFrom(entityStore.getState())
  const stop = entityStore.subscribe((s) => {
    if (!s.loaded.projects) return
    stop()
    placeFrom(s)
  })
}

/**
 * The persisted views, repaired: a record, pane or pointer that breaks an
 * invariant is dropped and every valid one is kept. A record without views
 * restores an empty band around its stage. No older shape is read.
 */
export function restoreWindowPaneState(layout: WorkspaceLayout): ViewState {
  const views = layout.views ?? {}
  return repairViewState({
    panes: layout.panes ?? {},
    views,
    viewOrder: layout.viewOrder ?? Object.keys(views),
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

export async function hydrateSidebar(): Promise<void> {
  const sidebarUI = await loadSidebarUI()
  if (!sidebarUI) return
  // `collapsedRepos`/`collapsedWorkspaces`/`collapsedProjects` are retired
  // keys the pre-restyle tree wrote (see schemas.ts) — never replayed.
  treeUiStore.setState({
    // Absent on a record written before the Chats panel was collapsible —
    // replays as "nothing folded", the product default (see schemas.ts).
    collapsed: new Set(sidebarUI.collapsedChatRows ?? []),
  })
}
