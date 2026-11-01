import { useSyncExternalStore } from 'react'
import type { EntityState } from '@/core/entities/entity-store'
import type { Chat, Repo, Workspace } from '@/core/entities/repo-types'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useHomeTreeStore, type HomeTree } from '@/lib/store/home-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import type { ThreadDTO } from '@/lib/types'

type Threads = EntityState['byKind']['review_thread']

// Readers subscribe with this selector, so an unchanged kind must answer the same array.
const byWorkspace = new WeakMap<Threads, Map<string, ThreadDTO[]>>()

/** The review threads of `wsId`, one array per review_thread record identity. */
export function selectReviewThreads(s: EntityState, wsId: string): ThreadDTO[] {
  const threads = s.byKind.review_thread
  let memo = byWorkspace.get(threads)
  if (!memo) byWorkspace.set(threads, (memo = new Map()))
  const memoized = memo.get(wsId)
  if (memoized) return memoized
  const selected: ThreadDTO[] = []
  for (const { data } of Object.values(threads)) if (data.workspaceId === wsId) selected.push(data)
  memo.set(wsId, selected)
  return selected
}

/** The project tree as the sidebar stores hold it. The selectors below are the only readers
 *  of these fields, so moving them onto the entity store changes their bodies, not callers. */
export interface SelectState {
  repos: Repo[]
  homeTrees: Record<string, HomeTree>
  /** Repos whose workspace list the daemon has answered. */
  workspacesRead: ReadonlySet<string>
  /** Projects whose repo list the daemon has answered. */
  reposRead: ReadonlySet<string>
}

function selectState(): SelectState {
  const signal = useFolderSignalStore.getState()
  return {
    repos: useSidebarStore.getState().repos,
    homeTrees: useHomeTreeStore.getState().trees,
    workspacesRead: signal.seededWorkspaceRepoIds,
    reposRead: signal.seededRepoListProjectIds,
  }
}

function subscribe(onChange: () => void): () => void {
  const stops = [
    useSidebarStore.subscribe(onChange),
    useHomeTreeStore.subscribe(onChange),
    useFolderSignalStore.subscribe(onChange),
  ]
  return () => stops.forEach((stop) => stop())
}

/** `sel` in the render path: re-renders when its answer changes identity (as a store hook
 *  does), so a selector building a fresh object needs `useShallow`. */
export function useSelect<T>(sel: (s: SelectState) => T): T {
  return useSyncExternalStore(subscribe, () => sel(selectState()))
}

/** `sel` once, for event handlers and effect bodies. */
export function readSelect<T>(sel: (s: SelectState) => T): T {
  return sel(selectState())
}

/** Every repo of every visible project. */
export function selectRepos(s: SelectState): Repo[] {
  return s.repos
}

export function selectRepo(s: SelectState, repoId: string): Repo | undefined {
  return s.repos.find((r) => r.id === repoId)
}

/** A workspace drawn as a tree row; a repo's default checkout is on its `Repo`. */
export function selectWorkspace(s: SelectState, wsId: string): Workspace | undefined {
  for (const repo of s.repos) {
    const ws = repo.workspaces.find((w) => w.id === wsId)
    if (ws) return ws
  }
  return undefined
}

/** A repo chat, else a project-home chat. */
export function selectChat(s: SelectState, chatId: string): Chat | undefined {
  for (const repo of s.repos) {
    const chat = repo.chats?.find((c) => c.id === chatId)
    if (chat) return chat
  }
  for (const tree of Object.values(s.homeTrees)) {
    const chat = tree.chats.find((c) => c.id === chatId)
    if (chat) return chat
  }
  return undefined
}

/** Every loaded project's home chats and folders, by project id. */
export function selectHomeTrees(s: SelectState): Record<string, HomeTree> {
  return s.homeTrees
}

/** Whether the daemon has answered the part of the tree a route names: `repoId`'s workspaces
 *  once that repo is known, else `projectId`'s repos. */
export function selectProjectsSynced(s: SelectState, projectId: string, repoId: string): boolean {
  return selectRepo(s, repoId) ? s.workspacesRead.has(repoId) : s.reposRead.has(projectId)
}
