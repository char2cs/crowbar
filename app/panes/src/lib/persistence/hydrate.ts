import type { WorkspaceLayout } from './schemas'
import {
  loadWindowPaneLayout,
  saveWorkspaceLayout,
  WINDOW_LAYOUT_VERSION,
} from './workspace-layout'
import { entityOf, entityStore, type EntityState } from '@/core/entities/entity-store'
import { chatHostOf } from '@/lib/host-resolve'
import type { PaneGroup } from '@/features/panes/types/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { restoreWindowPaneState } from '@/features/panes/lib/view-repair'
import {
  splitLegacyEditorContent,
  upgradeLegacyBuffers,
  validateLoadedBuffers,
} from '@/features/panes/utils/persisted-layout'
import { getFirstLeafId } from '@/features/panes/utils/pane-layout'
import { putDrafts } from '@/features/editor/documents/draft-store'
import { restoreDocuments } from '@/features/editor/documents/restore-documents'
import { loadSidebarUI } from './sidebar-ui'
import { parseSidebarTab, treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'

export async function hydrateWindowPaneLayout(): Promise<void> {
  const stored = await loadWindowPaneLayout()
  if (!stored) return
  const layout = await upgradeWindowPaneLayout(stored)
  if (layout !== stored) await saveWorkspaceLayout(layout)
  const restored = restoreWindowPaneState(layout)
  const adoptInto = getFirstLeafId(restored.stage)
  const before = new Set(restored.panes[adoptInto].editorTabIds)
  const { panes, buffers } = validateLoadedBuffers(
    { panes: restored.panes, buffers: layout.buffers ?? [] },
    { adoptInto, keepUnlisted: !layout.views },
  )
  if (panes[adoptInto].editorTabIds.some((id) => !before.has(id))) {
    panes[adoptInto] = { ...panes[adoptInto], editorOpen: true }
  }
  for (const [id, pane] of Object.entries(panes)) {
    if (isDraftChatId(pane.chatId)) panes[id] = { ...pane, workspaceId: null }
  }
  windowPaneStore.setState({
    ...restored,
    panes,
    activeProjectId: windowPaneStore.getState().activeProjectId,
    buffers,
  })
  restoreDocuments().catch((err: unknown) => console.error('Restoring documents failed:', err))
}

async function upgradeWindowPaneLayout(layout: WorkspaceLayout): Promise<WorkspaceLayout> {
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

function placeFrom(s: EntityState): boolean {
  const definitive = !s.unread.projects?.length
  const { panes, views } = windowPaneStore.getState()
  const placed = new Map<string, string>()
  const gone = new Set<string>()
  for (const pane of Object.values(panes)) {
    const inView = pane.viewId ? views[pane.viewId]?.projectId !== undefined : false
    if (!pane.chatId || pane.workspaceId || !inView) continue
    const hostId = chatHostOf(pane.chatId)
    if (!hostId) continue
    const chat = entityOf(s, 'chat', hostId, pane.chatId)
    if (chat?.workspaceId) placed.set(pane.chatId, chat.workspaceId)
    else if (!chat && definitive) gone.add(pane.chatId)
  }
  if (placed.size > 0 || gone.size > 0) {
    windowPaneStore.getState().paneActions.placeRestoredMembers(placed, gone)
  }
  return definitive
}

export function placeRestoredPanesOnceSynced(): void {
  const now = entityStore.getState()
  if (now.loaded.projects && placeFrom(now)) return
  const stop = entityStore.subscribe((s, prev) => {
    if (!s.loaded.projects || (prev.loaded.projects && s.unread === prev.unread)) return
    if (placeFrom(s)) stop()
  })
}

export async function hydrateSidebar(): Promise<void> {
  const sidebarUI = await loadSidebarUI()
  if (!sidebarUI) return
  treeUiStore.setState({
    collapsed: new Set(sidebarUI.collapsedChatRows ?? []),
    activeTab: parseSidebarTab(sidebarUI.activeTab),
    hostFilter: sidebarUI.hostFilter ?? '',
    lastProjectByHost: sidebarUI.lastProjectByHost ?? {},
  })
}
