import { useEffect, useEffectEvent, useMemo, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useStore } from 'zustand'
import {
  applyPendingRemovals,
  attachRemovalState,
  renderedHiddenIds,
} from '@/components/layout/removal-plan'
import { rowsForProject } from '@/components/sidebar/lib/rows-for-project'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import {
  rowIsLocked,
  rowVerbs,
  runRowMenuAction,
  type RowMenuActionId,
  type RowMenuHandlers,
} from '@/components/sidebar/lib/row-menu'
import { performRenameRow } from '@/components/sidebar/lib/row-actions'
import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { dropTargetRow, rowDropTargets } from '@/components/sidebar/lib/row-drop-targets'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { providerCanStartOnTerminal } from '@/features/agent/api/agent-api'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { handleCreate } from '@/components/layout/create-actions'
import { handleOpen } from '@/components/layout/open-actions'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { HostDotState } from '@/features/hosts/types'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { toast } from '@/features/window/stores/toast-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import {
  readChatWorking,
  subscribeChatWorking,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  nativePrompt,
  onNativeChromeIntent,
  setNativeSidebar,
  type NativeChromeIntent,
  type NativeRowAction,
  type NativeSidebar,
} from '@/lib/native-chrome'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useConnectionStore } from '@/lib/ws/connection-store'
import type { Project } from '@/lib/types'
import { isHostResolved } from '@/lib/host-resolve'
import { useIconDataUris } from '@/lib/icon-data-uri'
import { projectAssetURL } from '@/lib/project-url'

interface NativeSidebarProps {
  activeProjectId: string | undefined
  projects: readonly Project[]
  onSelectProject: (projectId: string) => void
}

const EMPTY: NativeSidebar = {
  focusedChatId: null,
  activeProjectId: null,
  projects: [],
  hosts: [],
  rows: [],
  collapsed: [],
  actions: {},
  drop: null,
}

// The desktop renames inline; the phone's list page can't, so it asks natively.
async function renameNatively(row: SidebarRow) {
  const name = await nativePrompt({
    title: 'Rename',
    message: null,
    value: row.label,
    placeholder: row.label,
    confirmLabel: 'Rename',
  })
  const trimmed = name?.trim()
  if (trimmed && trimmed !== row.label) await performRenameRow(row.id, trimmed)
}

// The desktop names a fork inline under its parent; the phone asks for the name first.
async function forkNatively(row: SidebarRow, navigate: RowMenuHandlers['navigate']) {
  const name = await nativePrompt({
    title: 'New branch',
    message: `Forked from ${row.branchName || row.label}`,
    value: '',
    placeholder: 'branch-name',
    confirmLabel: 'Create',
  })
  const trimmed = name?.trim()
  if (trimmed) await handleCreate(row.id, 'workspace', navigate, undefined, trimmed)
}

const NOUN: Record<SidebarRow['kind'], string> = {
  chat: 'thread',
  folder: 'folder',
  branch: 'branch',
  workflow: 'workflow',
}

// Remove holds the row in the removal tray; the phone asks first, where the desktop's X doesn't.
// The name sits in the message, not between quotes in the title, which a name may contain.
// Delete Repo's question is the tray's own, asked natively there.
function confirmFor(id: RowMenuActionId, row: SidebarRow): NativeRowAction['confirm'] {
  if (id !== 'remove') return null
  return {
    title: `Remove ${NOUN[row.kind]}?`,
    message: `${row.label}\n\nIt stays in the list with a Keep button for a few seconds.`,
  }
}

function nativeRowActions(rows: SidebarRow[], canStartOnTerminal: boolean) {
  const actions: Record<string, NativeRowAction[]> = {}
  for (const row of rows) {
    const locked = row.kind === 'branch' && rowIsLocked(row.id)
    // A row on its way out offers Keep on the row itself, not a menu.
    if (row.removal) {
      actions[row.id] = []
      continue
    }
    actions[row.id] = rowVerbs(row, { locked, canStartOnTerminal }).map((action) => ({
      id: action.id,
      title: action.label,
      symbol: action.symbol,
      destructive: action.destructive,
      confirm: confirmFor(action.id, row),
    }))
  }
  return actions
}

const closeChatList = () => useMobileScreenStore.getState().setChatListOpen(false)

/** The active project's rows, from the same derivers the desktop tree draws. */
function useActiveProjectRows(projectId: string | null): SidebarRow[] {
  const allRepos = useSidebarStore((s) => s.repos)
  // The desktop tree's own removal view (sidebar-tree-surface.tsx): what a held row takes with
  // it is hidden, and the row itself stays, marked, to draw its countdown and Keep.
  const removalEntries = useRemovalTrayStore((s) => s.entries)
  const trayHiddenIds = useRemovalTrayStore((s) => s.hiddenIds)
  const hiddenIds = useMemo(
    () => renderedHiddenIds(trayHiddenIds, removalEntries),
    [trayHiddenIds, removalEntries],
  )
  const repos = useMemo(() => applyPendingRemovals(allRepos, hiddenIds), [allRepos, hiddenIds])
  const seeded = useFolderSignalStore((s) => s.seededRepoIds)
  const seededWorkspaces = useFolderSignalStore((s) => s.seededWorkspaceRepoIds)
  const homeTree = useHomeTreeStore((s) => (projectId ? s.trees[projectId] : undefined))
  const home = useHomeWorkspaceState(projectId)
  return useMemo(() => {
    if (!projectId) return []
    const treeRepos = repos.filter((r) => seeded.has(r.id) || seededWorkspaces.has(r.id))
    const homeRows =
      home.wsId && homeTree
        ? rowsFromHome(
            home.wsId,
            homeTree.chats.filter((c) => !hiddenIds.has(c.id)),
            homeTree.folders.filter((f) => !hiddenIds.has(f.id)),
            home.owningChatId ?? undefined,
          )
        : []
    return attachRemovalState(
      [...homeRows, ...rowsForProject(treeRepos, projectId)],
      removalEntries,
    )
  }, [
    projectId,
    repos,
    seeded,
    seededWorkspaces,
    homeTree,
    home.wsId,
    home.owningChatId,
    hiddenIds,
    removalEntries,
  ])
}

export const isImageUrl = (url: string | undefined): url is string =>
  !!url && !url.startsWith('emoji:')

export function projectIconUrl(project: Pick<Project, 'id' | 'avatarUrl'>): string | undefined {
  if (!project.avatarUrl || !isHostResolved({ projectId: project.id })) return undefined
  return projectAssetURL(project.id, project.avatarUrl)
}

/** Repo and project icons as data the list page can draw: it cannot reach their daemon. */
function useIconsAsData(rows: readonly SidebarRow[], projects: readonly Project[]) {
  const rowUrls = rows.flatMap((r) =>
    isImageUrl(r.repoIcon?.avatarURL) ? [r.repoIcon.avatarURL] : [],
  )
  const projectUrls = projects.flatMap((p) => {
    const url = projectIconUrl(p)
    return isImageUrl(url) ? [url] : []
  })
  const icons = useIconDataUris([...rowUrls, ...projectUrls])
  return {
    row: (row: SidebarRow): SidebarRow => {
      const url = row.repoIcon?.avatarURL
      if (!row.repoIcon || !isImageUrl(url)) return row
      return { ...row, repoIcon: { ...row.repoIcon, avatarURL: icons.get(url) ?? undefined } }
    },
    project: (project: Project): Project => {
      const url = projectIconUrl(project)
      return url ? { ...project, avatarUrl: icons.get(url) ?? undefined } : project
    },
  }
}

/** Re-renders when any listed host's status or workspace's turn state changes. */
function useLiveFacts(hostIds: readonly string[], wsIds: readonly string[]): number {
  const [version, setVersion] = useState(0)
  const hostKey = hostIds.join('\n')
  const wsKey = wsIds.join('\n')
  useEffect(() => {
    const bump = () => setVersion((v) => v + 1)
    const stops = [
      ...hostKey
        .split('\n')
        .filter(Boolean)
        .map((id) => subscribeHostStatusOf(id, bump)),
      ...wsKey
        .split('\n')
        .filter(Boolean)
        .map((id) => subscribeChatWorking(id, bump)),
    ]
    return () => stops.forEach((stop) => stop())
  }, [hostKey, wsKey])
  return version
}

/**
 * Keeps the phone's native chat list showing the sidebar, and carries out what the user asks
 * of it there. The list holds no state of its own: every push is the whole snapshot.
 */
export function useNativeSidebar({
  activeProjectId,
  projects,
  onSelectProject,
}: NativeSidebarProps) {
  const navigate = useNavigate()
  const projectId = activeProjectId ?? null
  const rows = useActiveProjectRows(projectId)
  const collapsed = useSidebarStore((s) => s.collapsedChatRows)
  const focusedChatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? null)
  const hostConfigs = useHostConfigStore((s) => s.hosts)
  const hostStreams = useConnectionStore((s) => s.hostStreams)
  const homeWsId = useHomeWorkspaceState(projectId).wsId
  const wsIds = useMemo(
    () => [...new Set(rows.flatMap((r) => (r.workspaceId ? [r.workspaceId] : [])))],
    [rows],
  )
  const canStartOnTerminal = useAgentProvidersStore((s) =>
    providerCanStartOnTerminal(s.providers.find((p) => p.enabled)),
  )
  const [importRepoRowId, setImportRepoRowId] = useState<string | null>(null)
  const [dragSubjectId, setDragSubjectId] = useState<string | null>(null)
  const drop = useMemo(
    () => (dragSubjectId ? rowDropTargets(rows, collapsed, dragSubjectId) : null),
    [rows, collapsed, dragSubjectId],
  )
  const icons = useIconsAsData(rows, projects)
  const live = useLiveFacts(
    hostConfigs.map((h) => h.id),
    wsIds,
  )

  useEffect(() => {
    const hostState = (id: string): HostDotState => {
      const status = hostStatusOf(id)
      return status === 'up' && hostStreams[id] === 'disconnected' ? 'no-live-stream' : status
    }
    // The two live facts the desktop tree subscribes per row (sidebar-tree.tsx), resolved here:
    // the page that draws these rows watches nothing.
    const chatWorking = (row: SidebarRow) =>
      row.kind !== 'folder' && row.workspaceId ? readChatWorking(row.workspaceId, row.id) : false
    setNativeSidebar({
      focusedChatId,
      activeProjectId: projectId,
      projects: projects.map(icons.project),
      hosts: hostConfigs.map((h) => ({ id: h.id, name: h.name, state: hostState(h.id) })),
      rows: rows.map((row) => ({
        ...icons.row(row),
        working: row.working || chatWorking(row),
        hasView: row.id === focusedChatId,
      })),
      collapsed: [...collapsed],
      actions: nativeRowActions(rows, canStartOnTerminal),
      drop,
    })
  }, [
    drop,
    rows,
    collapsed,
    focusedChatId,
    projectId,
    projects,
    hostConfigs,
    hostStreams,
    live,
    canStartOnTerminal,
    icons,
  ])

  useEffect(() => () => setNativeSidebar(EMPTY), [])

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    switch (intent.type) {
      case 'selectRow':
        if (handleOpen(intent.id, useSidebarStore.getState().repos, navigate, { fold: false })) {
          closeChatList()
        }
        return
      case 'newThread':
        if (!projectId || !homeWsId) {
          toast.error("Can't start a new thread yet")
          return
        }
        void handleCreateHomeThread(projectId, homeWsId, navigate)
        return
      case 'selectProject':
        onSelectProject(intent.id)
        return
      case 'toggleFold':
        useSidebarStore.getState().toggleChatRow(intent.id)
        return
      case 'rowAction': {
        const row = rows.find((r) => r.id === intent.rowId)
        if (!row) return
        const handlers: RowMenuHandlers = {
          navigate,
          rename: (target) => void renameNatively(target),
          importBranches: (target) => {
            closeChatList()
            setImportRepoRowId(target.id)
          },
          fork: (target) => void forkNatively(target, navigate),
        }
        const actionId = intent.actionId as RowMenuActionId
        // Detach opens its sheet over the chat, not under the list.
        if (actionId === 'detach') closeChatList()
        runRowMenuAction(actionId, row, handlers)
        return
      }
      case 'keepRemoval':
        useRemovalTrayStore.getState().cancel(intent.entryId)
        return
      case 'dragRow':
        setDragSubjectId(intent.rowId)
        return
      case 'moveRow': {
        setDragSubjectId(null)
        const subject = rows.find((r) => r.id === intent.rowId)
        const target = dropTargetRow(rows, collapsed, intent.targetId)
        const allowed = rowDropTargets(rows, collapsed, intent.rowId)?.targets[intent.targetId]
        if (!subject || !target || !allowed?.modes[intent.position]) return
        void performSidebarDrop([subject], target, intent.position)
        return
      }
      case 'openSettings':
        closeChatList()
        useUIState.getState().openSettingsDialog()
        return
    }
  })
  useEffect(() => onNativeChromeIntent(onIntent), [])

  return { importRepoRowId, closeImport: () => setImportRepoRowId(null) }
}
