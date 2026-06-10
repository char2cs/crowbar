import { create } from 'zustand'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'

export interface ProjectChat {
  id: string
  wsId: string // workspace this chat belongs to
  title: string
  age: string
  parentId?: string // for forks
  status: ChatStatus
  type: ChatType
}

export type ChatStatus = 'idle' | 'agent-running'
export type ChatType = 'chat' | 'workflow'

export type WorkspaceStatus =
  | 'locked'
  | 'new'
  | 'pr-open'
  | 'pr-closed'
  | 'pr-merged'
  | 'agent-running'

export interface Workspace {
  id: string
  branch: string
  parentId?: string
  status?: WorkspaceStatus
  added?: number
  deleted?: number
  age: string
  hasConflicts?: boolean
}

export interface Repo {
  id: string
  name: string
  avatarLabel: string
  avatarColor: string
  workspaces: Workspace[]
}

export type SidebarTab = 'workspaces' | 'chats' | 'files' | 'git'

interface SidebarState {
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos: Set<string>
  collapsedWorkspaces: Set<string>
  /** Persisted active tab so re-mounts don't reset it. */
  activeTab: SidebarTab
  addChat: (chat: ProjectChat) => void
  deleteChat: (id: string) => void
  renameChat: (id: string, title: string) => void
  collapsedChats: Set<string>
  toggleChat: (chatId: string) => void
  addWorkspace: (repoId: string, wsId: string, branch: string, parentId?: string) => void
  deleteWorkspace: (wsId: string) => void
  renameWorkspace: (wsId: string, branch: string) => void
  reparentWorkspace: (wsId: string, newParentId: string | undefined) => void
  toggleRepo: (repoId: string) => void
  toggleWorkspace: (wsId: string) => void
  setActiveTab: (tab: SidebarTab) => void
  setRepos: (repos: Repo[]) => void
}

function getInitialState() {
  return {
    chats: [],
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedChats: new Set<string>(),
    activeTab: 'workspaces' as SidebarTab,
  }
}

export const useSidebarStore = create<SidebarState>()((set) => ({
  ...getInitialState(),

  addChat: (chat) => set((s) => ({ chats: [...s.chats, chat] })),

  deleteChat: (id) => set((s) => ({ chats: s.chats.filter((c) => c.id !== id) })),

  renameChat: (id, title) =>
    set((s) => ({ chats: s.chats.map((c) => (c.id === id ? { ...c, title } : c)) })),

  toggleChat: (chatId) =>
    set((s) => {
      const next = new Set(s.collapsedChats)
      next.has(chatId) ? next.delete(chatId) : next.add(chatId)
      void saveSidebarUI([...s.collapsedRepos], [...s.collapsedWorkspaces], [...next])
      return { collapsedChats: next }
    }),

  addWorkspace: (repoId, wsId, branch, parentId) =>
    set((s) => ({
      repos: s.repos.map((r) =>
        r.id !== repoId
          ? r
          : {
              ...r,
              workspaces: [
                ...r.workspaces,
                {
                  id: wsId,
                  branch,
                  ...(parentId !== undefined && { parentId }),
                  status: 'new' as WorkspaceStatus,
                  age: 'just now',
                },
              ],
            },
      ),
    })),

  deleteWorkspace: (wsId) =>
    set((s) => {
      // BFS to collect the target and all non-locked descendants
      const allWorkspaces = s.repos.flatMap((r) => r.workspaces)
      const toDelete = new Set<string>()
      const queue = [wsId]
      while (queue.length > 0) {
        const id = queue.shift()!
        const ws = allWorkspaces.find((w) => w.id === id)
        if (ws?.status === 'locked') continue
        toDelete.add(id)
        for (const child of allWorkspaces.filter((w) => w.parentId === id)) {
          queue.push(child.id)
        }
      }
      return {
        repos: s.repos.map((r) => ({
          ...r,
          workspaces: r.workspaces.filter((w) => !toDelete.has(w.id)),
        })),
      }
    }),

  renameWorkspace: (wsId, branch) =>
    set((s) => ({
      repos: s.repos.map((r) => ({
        ...r,
        workspaces: r.workspaces.map((w) => (w.id === wsId ? { ...w, branch } : w)),
      })),
    })),

  reparentWorkspace: (wsId, newParentId) =>
    set((s) => {
      const repo = s.repos.find((r) => r.workspaces.some((w) => w.id === wsId))
      if (!repo) return s
      // newParentId must exist in the same repo (or be undefined for root)
      if (newParentId !== undefined && !repo.workspaces.some((w) => w.id === newParentId)) return s
      // Reject cycles: walk up from newParentId; if we reach wsId it's a cycle
      if (newParentId !== undefined) {
        const wsMap = new Map(repo.workspaces.map((w) => [w.id, w]))
        const visited = new Set<string>()
        let cursor: string | undefined = newParentId
        while (cursor !== undefined) {
          if (cursor === wsId || visited.has(cursor)) return s
          visited.add(cursor)
          cursor = wsMap.get(cursor)?.parentId
        }
      }
      return {
        repos: s.repos.map((r) =>
          r.id !== repo.id
            ? r
            : {
                ...r,
                workspaces: r.workspaces.map((w) =>
                  w.id === wsId ? { ...w, parentId: newParentId } : w,
                ),
              },
        ),
      }
    }),

  toggleRepo: (repoId) =>
    set((s) => {
      const next = new Set(s.collapsedRepos)
      next.has(repoId) ? next.delete(repoId) : next.add(repoId)
      void saveSidebarUI([...next], [...s.collapsedWorkspaces])
      return { collapsedRepos: next }
    }),

  toggleWorkspace: (wsId) =>
    set((s) => {
      const next = new Set(s.collapsedWorkspaces)
      next.has(wsId) ? next.delete(wsId) : next.add(wsId)
      void saveSidebarUI([...s.collapsedRepos], [...next])
      return { collapsedWorkspaces: next }
    }),

  setActiveTab: (tab) => set({ activeTab: tab }),

  setRepos: (repos) => set({ repos }),
}))

// Expose for test reset
;(useSidebarStore as any).getInitialState = getInitialState
