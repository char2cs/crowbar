import { createStore, useStore } from 'zustand'
import type { WorkAtRisk } from '@/lib/errors/api-error'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'

// The sidebar's view-only state: one store for the whole window, apart from
// the rows it draws. Nothing here talks to the daemon.

/** How long a held row waits before its removal fires. */
const REMOVAL_DRAIN_MS = 8000

/** Stable empties, so a store nothing has written to hands out one identity. */
const NO_IDS: ReadonlySet<string> = new Set<string>()
const NO_REMOVALS: readonly RemovalEntry[] = []

export type SidebarTab = 'chats' | 'files' | 'git'

const SIDEBAR_TABS: readonly SidebarTab[] = ['chats', 'files', 'git']

/** A persisted mode, or Chats for anything this build has no panel for. */
export function parseSidebarTab(value: unknown): SidebarTab {
  return SIDEBAR_TABS.find((tab) => tab === value) ?? 'chats'
}

/** One row on its way out, worked out before anything is hidden. */
export interface RemovalDraft {
  kind: 'workspace' | 'folder' | 'repo' | 'project' | 'chat'
  /** The row itself: a workspace, folder, repo, project or chat id. */
  id: string
  /** What the row reads as, for the tray row. */
  label: string
  projectId: string
  /** The owning repo; the repo itself for a repo, '' for a project. */
  repoId: string
  /** The workspace a chat's DELETE is scoped through; '' for every other kind. */
  wsId: string
  providerIcon: string
  /** Every row hidden while this waits: the row and whatever it takes with it. */
  hiddenIds: readonly string[]
  /** A workspace row draws under its owning chat's id; that row must stay
   *  un-hidden during the hold or the row re-keys (`renderedHiddenIds`). */
  primaryRowId?: string
  /** How many rows go with it beyond itself; 0 draws no count. */
  extra: number
  /** Where to go if the viewed workspace is inside what is going, resolved before hiding. */
  fallbackWsId: string | null
  /** The work the daemon refused this removal over; confirming consents to destroy it. */
  atRisk?: readonly WorkAtRisk[]
}

export interface RemovalEntry extends RemovalDraft {
  /** Identity of the tray row, distinct from the row's own id. */
  entryId: string
  /** When the removal fires, or null when it waits on an answer (a repo or a
   *  project cascades too far for a clock that runs out unseen). */
  deadlineAt: number | null
}

/** A create the user is still naming: the sidebar draws an input at its slot. */
export interface CreateDraft {
  kind: 'branch' | 'chat'
  projectId: string
  repoId?: string
  /** The workspace the create forks from or runs in. */
  workspaceId: string
  /** The SidebarRow.parentId the created row lands under. */
  parentId: string
  order: number
  ownsWorktree: boolean
}

interface TreeUiState {
  /** The row mid inline-rename, and which of its two instances (tree or
   *  Recents mirror) draws the editor. */
  renaming: string | null
  renamingRole: 'tree' | 'recents' | null
  /** The thread whose title is, for now, the input naming the branch it is made. */
  branching: string | null
  /** Folded rows (folder and chat ids in one set); unknown means open.
   *  Persisted as the sidebar-ui record's `collapsedChatRows`. */
  collapsed: ReadonlySet<string>
  /** The removal tray's rows, oldest first. */
  removals: readonly RemovalEntry[]
  /** Rows the sidebar must not draw while their removal is pending. Kept apart
   *  from `removals`: a sent entry leaves the tray before its tombstones arrive. */
  hiddenIds: ReadonlySet<string>
  draft: CreateDraft | null
  /** The mode the user chose; what shows also depends on availability (sidebar-panel-tabs.ts). */
  activeTab: SidebarTab
}

interface TreeUiActions {
  /** `role` defaults to the instance on screen; a row not drawn yet (just created) names it. */
  startRenaming: (rowId: string, role?: 'tree' | 'recents') => void
  stopRenaming: () => void
  /** One row edits at a time, so this closes any rename. */
  startBranching: (rowId: string) => void
  stopBranching: () => void
  /** Fold a row away, or open it again; the fold is persisted. */
  toggleChatRow: (rowId: string) => void
  /** Hold rows: hide them and put a row in the tray for each. */
  hold: (drafts: readonly RemovalDraft[], drainMs?: number) => void
  /** The user kept it: drop the tray row and show its rows again. */
  cancel: (entryId: string) => void
  /** The removal has been sent: drop the tray row, keep its rows hidden. */
  settle: (entryId: string) => void
  /** Stop hiding these rows: the daemon confirmed, or refused. */
  release: (ids: readonly string[]) => void
  /** The daemon refused a sent removal over work at risk: put it back,
   *  still hidden, waiting on an answer with that work attached. */
  askToDiscard: (entry: RemovalEntry, atRisk: readonly WorkAtRisk[]) => void
  /** Opens the naming input, unfolding its parent; one is open at a time, so it
   *  replaces any other. */
  startDraft: (draft: CreateDraft) => void
  cancelDraft: () => void
  setActiveTab: (tab: SidebarTab) => void
}

type TreeUi = TreeUiState & TreeUiActions

/** The tree instance wins when one is mounted; a row folded away under a
 *  collapsed parent is edited in its Recents mirror, its only instance. */
function renamingRoleFor(rowId: string): 'tree' | 'recents' {
  const tree =
    typeof document !== 'undefined' &&
    document.querySelector(
      `[role="treeitem"][data-sidebar-row-id="${rowId}"]:not([data-sidebar-recents-row])`,
    ) != null
  return tree ? 'tree' : 'recents'
}

// Folds and mode share one record, so every write carries both.
function persist(collapsed: ReadonlySet<string>, activeTab: SidebarTab): void {
  void saveSidebarUI({ collapsedChatRows: [...collapsed], activeTab })
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

  startRenaming: (rowId, role) =>
    set({ renaming: rowId, renamingRole: role ?? renamingRoleFor(rowId) }),
  stopRenaming: () => set({ renaming: null, renamingRole: null }),
  startBranching: (rowId) => set({ branching: rowId, renaming: null, renamingRole: null }),
  stopBranching: () => set({ branching: null }),

  toggleChatRow: (rowId) =>
    set((s) => {
      const next = new Set(s.collapsed)
      if (!next.delete(rowId)) next.add(rowId)
      persist(next, s.activeTab)
      return { collapsed: next }
    }),

  hold: (drafts, drainMs = REMOVAL_DRAIN_MS) =>
    set((s) => {
      if (drafts.length === 0) return s
      const now = Date.now()
      const held = drafts.map((draft) => ({
        ...draft,
        entryId: crypto.randomUUID(),
        // A repo or a project waits on Cancel / Remove; everything else drains.
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
      // The input is drawn under its parent; a folded parent would hide it.
      if (!s.collapsed.has(draft.parentId)) return { draft }
      const collapsed = new Set(s.collapsed)
      collapsed.delete(draft.parentId)
      persist(collapsed, s.activeTab)
      return { draft, collapsed }
    }),
  cancelDraft: () => set({ draft: null }),
  setActiveTab: (activeTab) =>
    set((s) => {
      if (s.activeTab === activeTab) return s
      persist(s.collapsed, activeTab)
      return { activeTab }
    }),
}))

export function useTreeUi<T>(selector: (s: TreeUi) => T): T {
  return useStore(treeUiStore, selector)
}
