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

const byWorkspace = new WeakMap<Threads, Map<string, ThreadDTO[]>>()

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

export type SelectState = SidebarViews

function selectState(): SelectState {
  return viewsFromEntities(entityStore.getState())
}

function subscribe(onChange: () => void): () => void {
  return entityStore.subscribe(onChange)
}

export function useSelect<T>(sel: (s: SelectState) => T): T {
  return useSyncExternalStore(subscribe, () => sel(selectState()))
}

export function readSelect<T>(sel: (s: SelectState) => T): T {
  return sel(selectState())
}

export function selectRepos(s: SelectState): Repo[] {
  return s.repos
}

export function selectRepo(s: SelectState, repoId: string): Repo | undefined {
  return s.repos.find((r) => r.id === repoId)
}

export function selectWorkspace(s: SelectState, wsId: string): Workspace | undefined {
  for (const repo of s.repos) {
    const ws = repo.workspaces.find((w) => w.id === wsId)
    if (ws) return ws
  }
  return undefined
}

export function selectHomeTrees(s: SelectState): Record<string, HomeTree> {
  return s.homeTrees
}

export function selectProjectsSynced(s: SelectState): boolean {
  return s.synced
}

export function selectProjects(s: SelectState): Project[] {
  return s.projects
}

export function selectHomeTree(s: SelectState, projectId: string): HomeTree {
  return s.homeTrees[projectId] ?? EMPTY_HOME_TREE
}

export function selectAgentChat(s: SelectState, chatId: string): AgentChat | undefined {
  return s.chats[chatId]
}

export function selectRunnerChat(s: SelectState, runnerId: string): string {
  return s.byRunner[runnerId] ?? ''
}
