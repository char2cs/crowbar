import type { Chat, Folder, Repo, Workspace } from '@/core/entities/repo-types'
import type { EntityRef, Operation } from '@/lib/store/operations'
import { type CreateDraft, type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { legacyTreeIndex, type LegacyHome } from '@/features/workspace/tree/legacy-tree-index'
import { viewsFromStores, type ProblemLookup } from '@/features/workspace/tree/views'
import { deriveRows, type TreeInput } from '@/features/workspace/tree/rows'
import { treeInputFor } from '@/features/workspace/tree/input'

/** A tree input built from store-shaped data, the way `treeInputFor` builds one. */
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

/** One repo's rows, as its project's panel draws them. */
export function repoRows(repo: Repo, problemOf?: ProblemLookup): SidebarRow[] {
  return deriveRows(inputOf({ repos: [repo], problemOf }))
}

/** A project home's rows: its chats and folders, flat at the root. */
export function homeRows(
  workspaceId: string,
  chats: readonly Chat[] = [],
  folders: readonly Folder[] = [],
  owningChatId?: string,
): SidebarRow[] {
  return deriveRows(inputOf({ home: { workspaceId, owningChatId, chats, folders } }))
}

/** The rows a project's in-flight creates draw, as the stores hold them now. */
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

/** The id of the row every fixture repo's home checkout is drawn as: its owning chat. */
export const HOME_ROW_ID = 'home-branch-row'

/** A workspace and the chat that owns it (`Workspace.owningChatId`). */
export function makeOwnedWorkspace(
  wsOver: Partial<Workspace> & { id: string; branch: string },
  chatOver: Partial<Chat> & { id: string },
): { workspace: Workspace; chat: Chat } {
  const chat = makeChat({ title: '', ...chatOver, workspaceId: wsOver.id, ownsWorktree: true })
  return { workspace: makeWorkspace({ ...wsOver, owningChatId: chat.id }), chat }
}

/** A repo whose home checkout, when it has one, is owned by `HOME_ROW_ID`. */
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
