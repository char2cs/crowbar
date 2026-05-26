import { create } from 'zustand'
import { getAllMockChats } from '@/lib/mock/chats'

export interface ProjectChat {
  id: string
  title: string
  age: string
}

export interface Workspace {
  id: string
  num?: number
  branch: string
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
  addWorkspace: (repoId: string, wsId: string, branch: string) => void
  deleteWorkspace: (wsId: string) => void
  toggleRepo: (repoId: string) => void
  setActiveTab: (tab: SidebarTab) => void
}

const INITIAL_REPOS: Repo[] = [
  {
    id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws3', num: 3, branch: 'feature/app-design', added: 5672, age: '16h ago' },
      { id: 'ws2', num: 2, branch: 'feature/api-backend', added: 27347, deleted: 455, age: '1d ago' },
      { id: 'ws1', num: 1, branch: 'enhancement/scaffold', added: 22892, age: '3d ago' },
    ],
  },
  {
    id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
    workspaces: [{ id: 'qc1', branch: 'develop', age: '3d ago' }],
  },
  {
    id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'qd1', branch: 'develop', age: '6d ago' },
      { id: 'qd2', branch: 'feature/quiver-shell', added: 13485, deleted: 69, age: '3d ago' },
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

  addWorkspace: (repoId, wsId, branch) =>
    set(s => ({
      repos: s.repos.map(r =>
        r.id !== repoId ? r : {
          ...r,
          workspaces: [...r.workspaces, { id: wsId, branch, age: 'just now' }],
        },
      ),
    })),

  deleteWorkspace: (wsId) =>
    set(s => ({
      repos: s.repos.map(r => ({ ...r, workspaces: r.workspaces.filter(w => w.id !== wsId) })),
    })),

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
