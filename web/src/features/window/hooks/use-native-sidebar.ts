import { useEffect, useEffectEvent, useMemo, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useStore } from 'zustand'
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
import { killAgent, killAgentConfirmation } from '@/features/agent/lib/kill-agent'
import { useNativeReadout } from '@/components/layout/use-readout-lines'
import { hostNetStateOf } from '@/components/layout/build-readout-lines'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { handleCreate } from '@/components/layout/create-actions'
import { handleOpen } from '@/components/layout/open-actions'
import { readSelect, selectAgentChat, selectRepos, useSelect } from '@/core/entities/select'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { HostDotState } from '@/features/hosts/types'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { toast } from '@/features/window/stores/toast-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { useTreeInput } from '@/features/workspace/tree/input'
import { deriveRows } from '@/features/workspace/tree/rows'
import { treeUiStore, useTreeUi } from '@/features/workspace/tree/tree-ui-store'
import {
  nativePrompt,
  onNativeChromeIntent,
  setNativeSidebar,
  type NativeChromeIntent,
  type NativeRowAction,
  type NativeSidebar,
} from '@/lib/native-chrome'
import {
  selectActiveSession,
  useActiveSessionList,
  type ActiveSession,
} from '@/core/entities/active-sessions'
import { entityStore } from '@/core/entities/entity-store'
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
  active: [],
  collapsed: [],
  actions: {},
  drop: null,
  readout: null,
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
    message: `${row.label}${row.kind === 'folder' ? '\n\nWhat is inside moves up a level.' : ''}`,
  }
}

const KILL_AGENT = 'kill-agent'

type Running = ReadonlyMap<string, ActiveSession>

/** The rows with an agent running, as the desktop's Active section picks them. */
function activeRows(rows: readonly SidebarRow[], running: Running) {
  return rows.filter((r) => r.kind !== 'folder' && !r.operation && running.has(r.id))
}

function nativeRowActions(rows: SidebarRow[], running: Running) {
  const actions: Record<string, NativeRowAction[]> = {}
  for (const row of rows) {
    const locked = row.kind === 'branch' && rowIsLocked(row.id)
    // A row not yet created has no menu.
    if (row.operation) {
      actions[row.id] = []
      continue
    }
    actions[row.id] = // The phone has no terminal threads.
      rowVerbs(row, { locked, canStartOnTerminal: false }).map((action) => ({
        id: action.id,
        title: action.label,
        symbol: action.symbol,
        destructive: action.destructive,
        confirm: confirmFor(action.id, row),
      }))
    const session = running.get(row.id)
    if (row.kind !== 'folder' && session) {
      actions[row.id].push({
        id: KILL_AGENT,
        title: 'Kill agent',
        symbol: 'xmark.octagon',
        destructive: true,
        confirm: killAgentConfirmation(session),
      })
    }
  }
  return actions
}

const closeChatList = () => useMobileScreenStore.getState().setChatListOpen(false)

// A failed create is toasted in the shell, and a removal confirmed on the phone commits at once:
// neither leaves a row behind the way the desktop tree draws one. The phone names a fork first,
// so no naming input is drawn either.
const drawnNatively = (row: SidebarRow) =>
  !row.removal && !row.draft && row.operation?.state !== 'failed'

/** The active project's rows, from the same deriver the desktop tree draws. */
function useActiveProjectRows(projectId: string | null): SidebarRow[] {
  const inputFor = useTreeInput()
  return useMemo(
    () => (projectId ? deriveRows(inputFor(projectId)).filter(drawnNatively) : []),
    [projectId, inputFor],
  )
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

/** Re-renders when any listed host's status changes. */
function useHostStatusVersion(hostIds: readonly string[]): number {
  const [version, setVersion] = useState(0)
  const hostKey = hostIds.join('\n')
  useEffect(() => {
    const bump = () => setVersion((v) => v + 1)
    const stops = hostKey
      .split('\n')
      .filter(Boolean)
      .map((id) => subscribeHostStatusOf(id, bump))
    return () => stops.forEach((stop) => stop())
  }, [hostKey])
  return version
}

/** The rows whose chat is mid-turn, the live fact the desktop tree subscribes per row. */
function useWorkingRowIds(rows: readonly SidebarRow[]): ReadonlySet<string> {
  const key = useSelect((s) => {
    const working: string[] = []
    for (const r of rows)
      if (r.kind !== 'folder' && selectAgentChat(s, r.id)?.working) working.push(r.id)
    return working.join('\n')
  })
  return useMemo(() => new Set(key.split('\n')), [key])
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
  const collapsed = useTreeUi((s) => s.collapsed)
  const focusedChatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? null)
  const sessions = useActiveSessionList()
  const running = useMemo(() => new Map(sessions.map((s) => [s.chatId, s])), [sessions])
  const hostConfigs = useHostConfigStore((s) => s.hosts)
  const hostStreams = useConnectionStore((s) => s.hostStreams)
  const homeWsId = useHomeWorkspaceState(projectId).wsId
  const workingIds = useWorkingRowIds(rows)
  const [importRepoRowId, setImportRepoRowId] = useState<string | null>(null)
  const [dragSubjectId, setDragSubjectId] = useState<string | null>(null)
  const drop = useMemo(
    () => (dragSubjectId ? rowDropTargets(rows, collapsed, dragSubjectId) : null),
    [rows, collapsed, dragSubjectId],
  )
  const icons = useIconsAsData(rows, projects)
  const readout = useNativeReadout()
  const hostVersion = useHostStatusVersion(hostConfigs.map((h) => h.id))

  useEffect(() => {
    const hostState = (id: string): HostDotState => {
      const status = hostStatusOf(id)
      return status === 'up' && hostStreams[id] === 'disconnected' ? 'no-live-stream' : status
    }
    setNativeSidebar({
      focusedChatId,
      activeProjectId: projectId,
      projects: projects.map(icons.project),
      hosts: hostConfigs.map((h) => ({ id: h.id, name: h.name, state: hostState(h.id) })),
      rows: rows.map((row) => ({
        ...icons.row(row),
        // The page that draws these rows watches nothing: the live facts are resolved here.
        working: row.working || workingIds.has(row.id),
        hasView: row.id === focusedChatId,
      })),
      active: activeRows(rows, running).map((r) => ({ id: r.id })),
      collapsed: [...collapsed],
      actions: nativeRowActions(rows, running),
      drop,
      readout: readout && {
        ...readout,
        hosts: hostConfigs.map((h) => ({ name: h.name, net: hostNetStateOf(hostState(h.id)) })),
      },
    })
  }, [
    drop,
    readout,
    rows,
    collapsed,
    focusedChatId,
    projectId,
    projects,
    hostConfigs,
    hostStreams,
    hostVersion,
    workingIds,
    icons,
    running,
  ])

  useEffect(() => () => setNativeSidebar(EMPTY), [])

  const onIntent = useEffectEvent((intent: NativeChromeIntent) => {
    switch (intent.type) {
      case 'selectRow':
        if (handleOpen(intent.id, readSelect(selectRepos), navigate, { fold: false })) {
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
        treeUiStore.getState().toggleChatRow(intent.id)
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
          removeAtOnce: true,
        }
        if (intent.actionId === KILL_AGENT) {
          const session = selectActiveSession(entityStore.getState(), row.id)
          if (session) void killAgent(session)
          return
        }
        const actionId = intent.actionId as RowMenuActionId
        // Detach opens its sheet, and a new thread its draft, over the chat — not under the list.
        if (actionId === 'detach' || actionId === 'new-thread') closeChatList()
        runRowMenuAction(actionId, row, handlers)
        return
      }
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
