import type { Chat, Folder, Repo, Workspace } from '@/core/entities/repo-types'
import type { EntityRef, Operation } from '@/core/entities/operations'
import { type CreateDraft, type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { legacyTreeIndex, type LegacyHome } from '@/features/workspace/tree/legacy-tree-index'
import { viewsFromStores, type ProblemLookup } from '@/features/workspace/tree/views'
import { deriveRows, type TreeInput } from '@/features/workspace/tree/rows'
import { treeInputFor } from '@/features/workspace/tree/input'

export function inputOf(over: {
  repos?: readonly Repo[]
  home?: LegacyHome | null
  projectId?: string
  problemOf?: ProblemLookup
  operations?: readonly Operation[]
  isPresent?: (ref: EntityRef) => boolean
  removals?: readonly RemovalEntry[]
  draft?: CreateDraft | null
}): TreeInput {
  const repos = over.repos ?? []
  const home = over.home ?? null
  return {
    projectId: over.projectId ?? 'p1',
    homeWorkspaceId: home?.workspaceId ?? null,
    index: legacyTreeIndex(repos, home),
    views: viewsFromStores(repos, home, over.problemOf),
    operations: over.operations ?? [],
    isPresent: over.isPresent ?? (() => false),
    ui: { removals: over.removals ?? [], draft: over.draft ?? null },
  }
}

export function repoRows(repo: Repo, problemOf?: ProblemLookup): SidebarRow[] {
  return deriveRows(inputOf({ repos: [repo], problemOf }))
}

export function homeRows(
  workspaceId: string,
  chats: readonly Chat[] = [],
  folders: readonly Folder[] = [],
  owningChatId?: string,
): SidebarRow[] {
  return deriveRows(inputOf({ home: { workspaceId, owningChatId, chats, folders } }))
}

export function operationRows(projectId: string): SidebarRow[] {
  return deriveRows(treeInputFor(projectId)).filter((r) => r.operation !== undefined)
}

export const makeWorkspace = (
  over: Partial<Workspace> & { id: string; branch: string },
): Workspace => ({
  age: '',
  ...over,
})

export const makeFolder = (over: Partial<Folder> & { id: string; name: string }): Folder => ({
  repoId: 'r1',
  order: 0,
  ...over,
})

export const makeChat = (over: Partial<Chat> & { id: string; title: string }): Chat => ({
  repoId: 'r1',
  order: 0,
  ...over,
})

export const HOME_ROW_ID = 'home-branch-row'

export function makeOwnedWorkspace(
  wsOver: Partial<Workspace> & { id: string; branch: string },
  chatOver: Partial<Chat> & { id: string },
): { workspace: Workspace; chat: Chat } {
  const chat = makeChat({ title: '', ...chatOver, workspaceId: wsOver.id, ownsWorktree: true })
  return { workspace: makeWorkspace({ ...wsOver, owningChatId: chat.id }), chat }
}

export function makeRepo(over: Partial<Repo> = {}): Repo {
  const repo: Repo = {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
  if (!repo.defaultWorkspaceId) return repo
  const home = makeChat({
    id: HOME_ROW_ID,
    title: '',
    workspaceId: repo.defaultWorkspaceId,
    repoId: repo.id,
    ownsWorktree: true,
  })
  return {
    ...repo,
    defaultOwningChatId: repo.defaultOwningChatId ?? HOME_ROW_ID,
    chats: [home, ...(repo.chats ?? [])],
  }
}
