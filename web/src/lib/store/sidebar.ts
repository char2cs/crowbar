import { create } from 'zustand'
import { getAllMockChats } from '@/lib/mock/chats'

export interface ProjectChat {
  id: string
  title: string
  age: string
}

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
}

export interface Repo {
  id: string
  name: string
  avatarLabel: string
  avatarColor: string
  workspaces: Workspace[]
}

export type SidebarTab = 'workspaces' | 'files' | 'git'

interface SidebarState {
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos: Set<string>
  /** Persisted active tab so re-mounts of SidebarTabs don't reset it. */
  activeTab: SidebarTab
  addChat: (chat: ProjectChat) => void
  deleteChat: (id: string) => void
  addWorkspace: (repoId: string, wsId: string, branch: string, parentId?: string) => void
  deleteWorkspace: (wsId: string) => void
  renameWorkspace: (wsId: string, branch: string) => void
  reparentWorkspace: (wsId: string, newParentId: string | undefined) => void
  toggleRepo: (repoId: string) => void
  setActiveTab: (tab: SidebarTab) => void
}

const INITIAL_REPOS: Repo[] = [
  {
    id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
      { id: 'ws3', branch: 'feature/app-design', parentId: 'ws-develop', status: 'pr-open', added: 5672, age: '16h ago' },
      { id: 'ws1', branch: 'enhancement/scaffold', parentId: 'ws3', status: 'agent-running', added: 22892, age: '3d ago' },
      { id: 'ws-fix', branch: 'fix/toolbar-crash', parentId: 'ws3', status: 'new', age: 'just now' },
      { id: 'ws2', branch: 'feature/api-backend', parentId: 'ws-develop', status: 'pr-merged', added: 27347, deleted: 455, age: '1d ago' },
    ],
  },
  {
    id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
    workspaces: [
      { id: 'qc-develop', branch: 'develop', status: 'locked', age: '—' },
      { id: 'qc1', branch: 'feature/old-auth', parentId: 'qc-develop', status: 'pr-closed', age: '3d ago' },
    ],
  },
  {
    id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'qd-develop', branch: 'develop', status: 'locked', age: '—' },
      { id: 'qd2', branch: 'feature/quiver-shell', parentId: 'qd-develop', status: 'pr-open', added: 13485, deleted: 69, age: '3d ago' },
    ],
  },
]

function getInitialState() {
  return {
    chats: getAllMockChats().map(c => ({ id: c.id, title: c.title, age: c.age })),
    repos: INITIAL_REPOS,
    collapsedRepos: new Set<string>(),
    activeTab: 'workspaces' as SidebarTab,
  }
}

export const useSidebarStore = create<SidebarState>()((set) => ({
  ...getInitialState(),

  addChat: (chat) => set(s => ({ chats: [...s.chats, chat] })),

  deleteChat: (id) => set(s => ({ chats: s.chats.filter(c => c.id !== id) })),

  addWorkspace: (repoId, wsId, branch, parentId) =>
    set(s => ({
      repos: s.repos.map(r =>
        r.id !== repoId ? r : {
          ...r,
          workspaces: [...r.workspaces, {
            id: wsId, branch, ...(parentId !== undefined && { parentId }), status: 'new' as WorkspaceStatus, age: 'just now',
          }],
        },
      ),
    })),

  deleteWorkspace: (wsId) =>
    set(s => {
      // BFS to collect the target and all non-locked descendants
      const allWorkspaces = s.repos.flatMap(r => r.workspaces)
      const toDelete = new Set<string>()
      const queue = [wsId]
      while (queue.length > 0) {
        const id = queue.shift()!
        const ws = allWorkspaces.find(w => w.id === id)
        if (ws?.status === 'locked') continue
        toDelete.add(id)
        for (const child of allWorkspaces.filter(w => w.parentId === id)) {
          queue.push(child.id)
        }
      }
      return {
        repos: s.repos.map(r => ({
          ...r,
          workspaces: r.workspaces.filter(w => !toDelete.has(w.id)),
        })),
      }
    }),

  renameWorkspace: (wsId, branch) =>
    set(s => ({
      repos: s.repos.map(r => ({
        ...r,
        workspaces: r.workspaces.map(w => w.id === wsId ? { ...w, branch } : w),
      })),
    })),

  reparentWorkspace: (wsId, newParentId) =>
    set(s => {
      const repo = s.repos.find(r => r.workspaces.some(w => w.id === wsId))
      if (!repo) return s
      // newParentId must exist in the same repo (or be undefined for root)
      if (newParentId !== undefined && !repo.workspaces.some(w => w.id === newParentId)) return s
      // Reject cycles: walk up from newParentId; if we reach wsId it's a cycle
      if (newParentId !== undefined) {
        const wsMap = new Map(repo.workspaces.map(w => [w.id, w]))
        const visited = new Set<string>()
        let cursor: string | undefined = newParentId
        while (cursor !== undefined) {
          if (cursor === wsId || visited.has(cursor)) return s
          visited.add(cursor)
          cursor = wsMap.get(cursor)?.parentId
        }
      }
      return {
        repos: s.repos.map(r =>
          r.id !== repo.id ? r : {
            ...r,
            workspaces: r.workspaces.map(w =>
              w.id === wsId ? { ...w, parentId: newParentId } : w,
            ),
          },
        ),
      }
    }),

  toggleRepo: (repoId) =>
    set(s => {
      const next = new Set(s.collapsedRepos)
      next.has(repoId) ? next.delete(repoId) : next.add(repoId)
      return { collapsedRepos: next }
    }),

  setActiveTab: (tab) => set({ activeTab: tab }),
}))

// Expose for test reset
;(useSidebarStore as any).getInitialState = getInitialState
