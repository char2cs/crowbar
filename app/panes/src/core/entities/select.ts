import { useSyncExternalStore } from 'react'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import type { Repo, Workspace } from '@/core/entities/repo-types'
import {
  EMPTY_HOME_TREE,
  viewsFromEntities,
  type HomeTree,
  type SidebarViews,
} from '@/core/entities/views-from-entities'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { Project, ThreadDTO } from '@/lib/types'

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

/** The sidebar as the entity store holds it; selectors read it, never the store. */
export type SelectState = SidebarViews

function selectState(): SelectState {
  return viewsFromEntities(entityStore.getState())
}

function subscribe(onChange: () => void): () => void {
  return entityStore.subscribe(onChange)
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

/** Every loaded project's home chats and folders, by project id. */
export function selectHomeTrees(s: SelectState): Record<string, HomeTree> {
  return s.homeTrees
}

/** Whether the projects scope's snapshot has landed: before it, an absent row is unknown,
 *  after it, gone. */
export function selectProjectsSynced(s: SelectState): boolean {
  return s.synced
}

/** Every project, in sidebar order. */
export function selectProjects(s: SelectState): Project[] {
  return s.projects
}

/** A project's home chats and folders, empty when it has none. */
export function selectHomeTree(s: SelectState, projectId: string): HomeTree {
  return s.homeTrees[projectId] ?? EMPTY_HOME_TREE
}

/** A chat as the chat surfaces read it. */
export function selectAgentChat(s: SelectState, chatId: string): AgentChat | undefined {
  return s.chats[chatId]
}

/** The chat a live runner runs, '' when none does. */
export function selectRunnerChat(s: SelectState, runnerId: string): string {
  return s.byRunner[runnerId] ?? ''
}
