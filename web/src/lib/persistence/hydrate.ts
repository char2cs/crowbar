import type { WorkspaceLayout } from './schemas'
import {
  loadWindowPaneLayout,
  saveWorkspaceLayout,
  WINDOW_LAYOUT_VERSION,
} from './workspace-layout'
import { getAllEntities } from './entity-cache'
import type { ChatDTO } from '@/lib/types'
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
import { fetchHomeChats, fetchRepoChats, fetchRepos, isNotFoundError } from '@/lib/api'
import { loadSidebarUI } from './sidebar-ui'
import { useSidebarStore } from '@/lib/store/sidebar'

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
 * v1 → v2: a chat pane records the workspace its chat belongs to, read from
 * the chat's cached record (`crowbar_chats`). A chat the cache does not know
 * (a project-home chat, a repo never opened) keeps its pane unplaced (null)
 * for the daemon to answer — never dropped on the cache's silence.
 * v2 → v3: buffers name their documents (`upgradeLegacyBuffers`).
 * v3 → v4: buffers drop their text; unsaved text is written as drafts first.
 */
export async function upgradeWindowPaneLayout(layout: WorkspaceLayout): Promise<WorkspaceLayout> {
  const version = layout.version ?? 1
  if (version >= WINDOW_LAYOUT_VERSION) return layout
  const placed = version < 2 ? await placeChatPanes(layout) : layout
  const buffers = placed.buffers && upgradeLegacyBuffers(placed.buffers)
  const split = splitLegacyEditorContent({ ...placed, buffers })
  await putDrafts(split.drafts)
  return { ...split.layout, version: WINDOW_LAYOUT_VERSION }
}

async function placeChatPanes(layout: WorkspaceLayout): Promise<WorkspaceLayout> {
  const owner = new Map<string, string>()
  for (const chat of await getAllEntities<ChatDTO>('crowbar_chats')) {
    if (chat.workspaceId) owner.set(chat.id, chat.workspaceId)
  }
  const panes: Record<string, PaneGroup> = {}
  for (const [id, pane] of Object.entries(layout.panes ?? {})) {
    const workspaceId = pane.chatId ? pane.workspaceId || owner.get(pane.chatId) : null
    panes[id] = { ...pane, workspaceId: workspaceId ?? null }
  }
  return { ...layout, panes }
}

/**
 * Place every restored chat member that has no workspace, from the daemon's
 * chat listings of its view's project; drop a member only on a definitive
 * answer (the chat, or its whole project, is gone). A listing that fails
 * leaves its members unplaced for the next boot. No request when all placed.
 */
export async function placeRestoredChatMembers(): Promise<void> {
  const { panes, views } = windowPaneStore.getState()
  const byProject = new Map<string, Set<string>>()
  for (const pane of Object.values(panes)) {
    const projectId = pane.viewId ? views[pane.viewId]?.projectId : undefined
    if (!pane.chatId || pane.workspaceId || projectId === undefined) continue
    const chats = byProject.get(projectId) ?? new Set<string>()
    byProject.set(projectId, chats.add(pane.chatId))
  }
  const placed = new Map<string, string>()
  const gone = new Set<string>()
  await Promise.all(
    [...byProject].map(async ([projectId, chatIds]) => {
      const found = await findProjectChats(projectId, chatIds)
      if (found === undefined) return
      for (const chatId of chatIds) {
        const workspaceId = found?.get(chatId)
        if (workspaceId) placed.set(chatId, workspaceId)
        else if (!found?.has(chatId)) gone.add(chatId)
      }
    }),
  )
  if (placed.size > 0 || gone.size > 0) {
    windowPaneStore.getState().paneActions.placeRestoredMembers(placed, gone)
  }
}

/**
 * chatId → workspaceId ('' when the chat has none) for `wanted`, listing the
 * project home first and its repos only if some chat is still unfound; null
 * when the project is gone, undefined when the daemon could not answer.
 */
async function findProjectChats(
  projectId: string,
  wanted: ReadonlySet<string>,
): Promise<Map<string, string> | null | undefined> {
  const found = new Map<string, string>()
  const record = (chats: ChatDTO[]) => {
    for (const chat of chats) if (wanted.has(chat.id)) found.set(chat.id, chat.workspaceId)
  }
  try {
    record(await fetchHomeChats(projectId).catch(emptyWhenNotFound))
    if (found.size === wanted.size) return found
    const repos = await fetchRepos(projectId)
    const lists = await Promise.all(
      repos.map((repo) => fetchRepoChats(projectId, repo.id).catch(emptyWhenNotFound)),
    )
    for (const chats of lists) record(chats)
    return found
  } catch (err) {
    return isNotFoundError(err) ? null : undefined
  }
}

function emptyWhenNotFound(err: unknown): ChatDTO[] {
  if (isNotFoundError(err)) return []
  throw err
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
  useSidebarStore.setState({
    // Absent on a record written before the Chats panel was collapsible —
    // replays as "nothing folded", the product default (see schemas.ts).
    collapsedChatRows: new Set(sidebarUI.collapsedChatRows ?? []),
  })
}
