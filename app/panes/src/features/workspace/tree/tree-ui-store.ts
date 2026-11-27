import { createStore, useStore } from 'zustand'
import type { WorkAtRisk } from '@/lib/errors/api-error'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'

const REMOVAL_DRAIN_MS = 8000

const NO_IDS: ReadonlySet<string> = new Set<string>()
const NO_REMOVALS: readonly RemovalEntry[] = []

export type SidebarTab = 'chats' | 'files' | 'git'

const SIDEBAR_TABS: readonly SidebarTab[] = ['chats', 'files', 'git']

export function parseSidebarTab(value: unknown): SidebarTab {
  return SIDEBAR_TABS.find((tab) => tab === value) ?? 'chats'
}

export interface RemovalDraft {
  kind: 'workspace' | 'folder' | 'repo' | 'project' | 'chat'
  id: string
  label: string
  projectId: string
  repoId: string
  wsId: string
  providerIcon: string
  hiddenIds: readonly string[]
  primaryRowId?: string
  extra: number
  fallbackWsId: string | null
  atRisk?: readonly WorkAtRisk[]
}

export interface RemovalEntry extends RemovalDraft {
  entryId: string
  deadlineAt: number | null
}

export interface CreateDraft {
  kind: 'branch' | 'chat'
  projectId: string
  repoId?: string
  workspaceId: string
  parentId: string
  order: number
  ownsWorktree: boolean
}

interface TreeUiState {
  renaming: string | null
  renamingRole: 'tree' | 'recents' | null
  branching: string | null
  collapsed: ReadonlySet<string>
  removals: readonly RemovalEntry[]
  hiddenIds: ReadonlySet<string>
  draft: CreateDraft | null
  activeTab: SidebarTab
  hostFilter: string
  lastProjectByHost: Readonly<Record<string, string>>
}

interface TreeUiActions {
  startRenaming: (rowId: string, role?: 'tree' | 'recents') => void
  stopRenaming: () => void
  startBranching: (rowId: string) => void
  stopBranching: () => void
  toggleChatRow: (rowId: string) => void
  hold: (drafts: readonly RemovalDraft[], drainMs?: number) => void
  cancel: (entryId: string) => void
  settle: (entryId: string) => void
  release: (ids: readonly string[]) => void
  askToDiscard: (entry: RemovalEntry, atRisk: readonly WorkAtRisk[]) => void
  startDraft: (draft: CreateDraft) => void
  cancelDraft: () => void
  setActiveTab: (tab: SidebarTab) => void
  setHostFilter: (hostId: string) => void
  rememberHostProject: (hostId: string, projectId: string) => void
}

type TreeUi = TreeUiState & TreeUiActions

function renamingRoleFor(rowId: string): 'tree' | 'recents' {
  const tree =
    typeof document !== 'undefined' &&
    document.querySelector(
      `[role="treeitem"][data-sidebar-row-id="${rowId}"]:not([data-sidebar-recents-row])`,
    ) != null
  return tree ? 'tree' : 'recents'
}

type Persisted = Pick<TreeUiState, 'collapsed' | 'activeTab' | 'hostFilter' | 'lastProjectByHost'>

function persist(s: Persisted): void {
  void saveSidebarUI({
    collapsedChatRows: [...s.collapsed],
    activeTab: s.activeTab,
    hostFilter: s.hostFilter,
    lastProjectByHost: { ...s.lastProjectByHost },
  })
}

function withIds(set: ReadonlySet<string>, ids: readonly string[]): Set<string> {
  const next = new Set(set)
  for (const id of ids) next.add(id)
  return next
}

export const treeUiStore = createStore<TreeUi>()((set) => ({
  renaming: null,
  renamingRole: null,
  branching: null,
  collapsed: NO_IDS,
  removals: NO_REMOVALS,
  hiddenIds: NO_IDS,
  draft: null,
  activeTab: 'chats',
  hostFilter: '',
  lastProjectByHost: {},

  startRenaming: (rowId, role) =>
    set({ renaming: rowId, renamingRole: role ?? renamingRoleFor(rowId) }),
  stopRenaming: () => set({ renaming: null, renamingRole: null }),
  startBranching: (rowId) => set({ branching: rowId, renaming: null, renamingRole: null }),
  stopBranching: () => set({ branching: null }),

  toggleChatRow: (rowId) =>
    set((s) => {
      const next = new Set(s.collapsed)
      if (!next.delete(rowId)) next.add(rowId)
      persist({ ...s, collapsed: next })
      return { collapsed: next }
    }),

  hold: (drafts, drainMs = REMOVAL_DRAIN_MS) =>
    set((s) => {
      if (drafts.length === 0) return s
      const now = Date.now()
      const held = drafts.map((draft) => ({
        ...draft,
        entryId: crypto.randomUUID(),
        deadlineAt: draft.kind === 'repo' || draft.kind === 'project' ? null : now + drainMs,
      }))
      return {
        removals: [...s.removals, ...held],
        hiddenIds: withIds(
          s.hiddenIds,
          held.flatMap((e) => e.hiddenIds),
        ),
      }
    }),

  cancel: (entryId) =>
    set((s) => {
      const entry = s.removals.find((e) => e.entryId === entryId)
      if (!entry) return s
      const hiddenIds = new Set(s.hiddenIds)
      for (const id of entry.hiddenIds) hiddenIds.delete(id)
      return { removals: s.removals.filter((e) => e !== entry), hiddenIds }
    }),

  settle: (entryId) =>
    set((s) => {
      const removals = s.removals.filter((e) => e.entryId !== entryId)
      return removals.length === s.removals.length ? s : { removals }
    }),

  release: (ids) =>
    set((s) => {
      if (ids.length === 0 || s.hiddenIds.size === 0) return s
      const hiddenIds = new Set(s.hiddenIds)
      let removed = false
      for (const id of ids) removed = hiddenIds.delete(id) || removed
      return removed ? { hiddenIds } : s
    }),

  askToDiscard: (entry, atRisk) =>
    set((s) => {
      const asked = { ...entry, entryId: crypto.randomUUID(), deadlineAt: null, atRisk }
      return {
        removals: [...s.removals, asked],
        hiddenIds: withIds(s.hiddenIds, entry.hiddenIds),
      }
    }),

  startDraft: (draft) =>
    set((s) => {
      if (!s.collapsed.has(draft.parentId)) return { draft }
      const collapsed = new Set(s.collapsed)
      collapsed.delete(draft.parentId)
      persist({ ...s, collapsed })
      return { draft, collapsed }
    }),
  cancelDraft: () => set({ draft: null }),
  setActiveTab: (activeTab) =>
    set((s) => {
      if (s.activeTab === activeTab) return s
      persist({ ...s, activeTab })
      return { activeTab }
    }),
  setHostFilter: (hostFilter) =>
    set((s) => {
      if (s.hostFilter === hostFilter) return s
      persist({ ...s, hostFilter })
      return { hostFilter }
    }),
  rememberHostProject: (hostId, projectId) =>
    set((s) => {
      if (s.lastProjectByHost[hostId] === projectId) return s
      const lastProjectByHost = { ...s.lastProjectByHost, [hostId]: projectId }
      persist({ ...s, lastProjectByHost })
      return { lastProjectByHost }
    }),
}))

export function useTreeUi<T>(selector: (s: TreeUi) => T): T {
  return useStore(treeUiStore, selector)
}
