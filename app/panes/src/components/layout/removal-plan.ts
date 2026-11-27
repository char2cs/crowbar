import type { Chat, Folder, Repo } from '@/core/entities/repo-types'
import { getPostDeleteNavigationTarget } from '@/core/entities/repo-queries'
import { EMPTY_CHATS, EMPTY_FOLDERS } from '@/core/entities/views-from-entities'
import type { RemovalDraft, RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import type { DragSubjectBase } from '@/components/tree-dnd/drop-core'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { resolveHomeRowScope, getHomeTree } from '@/core/entities/home-rows'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { legacyTreeIndex } from '@/features/workspace/tree/legacy-tree-index'
import { descendantIds } from '@/features/workspace/tree/tree-index'

export interface ProjectRow {
  id: string
  name: string
}

type DropKind = 'workspace' | 'folder' | 'repo' | 'project' | 'chat'

export interface DragSubject extends DragSubjectBase {
  kind: DropKind
  repoId?: string
  locked?: boolean
  parentId?: string
}

export function planRemoval(
  subjects: readonly DragSubject[],
  repos: readonly Repo[],
  projects: readonly ProjectRow[] = [],
): RemovalDraft[] {
  const drafts: RemovalDraft[] = []
  const claimed = new Set<string>()

  for (const subject of subjects) {
    const draft = draftFor(subject, repos, projects)
    if (!draft || claimed.has(draft.id)) continue
    drafts.push(draft)
    for (const id of draft.hiddenIds) claimed.add(id)
  }

  return drafts
}

function chatDescendantsOf(chats: readonly Chat[], id: string): string[] {
  const childrenOf = new Map<string, string[]>()
  for (const c of chats) {
    if (!c.parentId) continue
    const siblings = childrenOf.get(c.parentId)
    if (siblings) siblings.push(c.id)
    else childrenOf.set(c.parentId, [c.id])
  }
  const out: string[] = []
  const seen = new Set([id])
  const walk = (parentId: string) => {
    for (const childId of childrenOf.get(parentId) ?? []) {
      if (seen.has(childId)) continue
      seen.add(childId)
      out.push(childId)
      walk(childId)
    }
  }
  walk(id)
  return out
}

function draftFor(
  subject: DragSubject,
  repos: readonly Repo[],
  projects: readonly ProjectRow[],
): RemovalDraft | null {
  if (subject.kind === 'project') {
    const project = projects.find((p) => p.id === subject.id)
    if (!project) return null
    const owned = repos.filter((r) => r.projectId === project.id)
    return {
      kind: 'project',
      id: project.id,
      label: project.name,
      projectId: project.id,
      repoId: '',
      wsId: '',
      providerIcon: '',
      hiddenIds: [project.id, ...owned.map((r) => r.id)],
      extra: owned.reduce((n, r) => n + 1 + r.workspaces.length, 0),
      fallbackWsId: null,
    }
  }

  if (subject.kind === 'repo') {
    const repo = repos.find((r) => r.id === subject.id)
    if (!repo?.projectId) return null
    return {
      kind: 'repo',
      id: repo.id,
      label: repo.name,
      projectId: repo.projectId,
      repoId: repo.id,
      wsId: '',
      providerIcon: '',
      hiddenIds: [repo.id],
      extra: repo.workspaces.length,
      fallbackWsId: null,
    }
  }

  if (subject.kind === 'chat' || subject.kind === 'folder') {
    const homeRow = resolveHomeRowScope(subject.id)
    if (homeRow) {
      const tree = getHomeTree(homeRow.projectId)
      if (subject.kind === 'chat') {
        const chat = tree.chats.find((c) => c.id === subject.id)
        if (!chat) return null
        const descendants = chatDescendantsOf(tree.chats, chat.id)
        return {
          kind: 'chat',
          id: chat.id,
          label: chat.title || UNTITLED_CHAT_LABEL,
          projectId: homeRow.projectId,
          repoId: '',
          wsId: homeRow.homeWorkspaceId,
          providerIcon: '',
          hiddenIds: [chat.id, ...descendants],
          extra: descendants.length,
          fallbackWsId: null,
        }
      }
      const folder = tree.folders.find((f) => f.id === subject.id)
      if (!folder) return null
      return {
        kind: 'folder',
        id: folder.id,
        label: folder.name,
        projectId: homeRow.projectId,
        repoId: '',
        wsId: '',
        providerIcon: '',
        hiddenIds: [folder.id],
        extra: 0,
        fallbackWsId: null,
      }
    }
  }

  const repo = repos.find((r) => r.id === subject.repoId)
  if (!repo?.projectId) return null

  if (subject.kind === 'chat') {
    if (workspaceIdOfBranchRow(repos, subject.id) !== null) return null
    const chat = repo.chats?.find((c) => c.id === subject.id)
    if (!chat) return null
    const wsId = repo.defaultWorkspaceId ?? repo.workspaces[0]?.id
    if (!wsId) return null
    const descendants = chatDescendantsOf(repo.chats ?? [], chat.id)
    return {
      kind: 'chat',
      id: chat.id,
      label: chat.title || UNTITLED_CHAT_LABEL,
      projectId: repo.projectId,
      repoId: repo.id,
      wsId,
      providerIcon: '',
      hiddenIds: [chat.id, ...descendants],
      extra: descendants.length,
      fallbackWsId: null,
    }
  }

  if (subject.kind === 'folder') {
    const folder = (repo.folders ?? EMPTY_FOLDERS).find((f) => f.id === subject.id)
    if (!folder) return null
    return {
      kind: 'folder',
      id: folder.id,
      label: folder.name,
      projectId: repo.projectId,
      repoId: repo.id,
      wsId: '',
      providerIcon: '',
      hiddenIds: [folder.id],
      extra: 0,
      fallbackWsId: null,
    }
  }

  const workspace = repo.workspaces.find((w) => w.id === subject.id)
  if (!workspace || workspace.status === 'locked') return null

  const descendants = descendantIds(legacyTreeIndex([{ ...repo, chats: [] }], null), workspace.id)
  const goingWorkspaceIds = new Set([workspace.id, ...descendants])
  const hiddenChatIds: string[] = []
  for (const chat of repo.chats ?? EMPTY_CHATS) {
    if (!chat.workspaceId || !goingWorkspaceIds.has(chat.workspaceId)) continue
    hiddenChatIds.push(chat.id, ...chatDescendantsOf(repo.chats ?? EMPTY_CHATS, chat.id))
  }
  return {
    kind: 'workspace',
    id: workspace.id,
    primaryRowId: workspace.owningChatId ?? workspace.id,
    label: workspace.branch,
    projectId: repo.projectId,
    repoId: repo.id,
    wsId: '',
    providerIcon: '',
    hiddenIds: [workspace.id, ...descendants, ...new Set(hiddenChatIds)],
    extra: descendants.length,
    fallbackWsId: getPostDeleteNavigationTarget(repos as Repo[], workspace.id),
  }
}

export function applyPendingRemovals(
  repos: readonly Repo[],
  hiddenIds: ReadonlySet<string>,
): Repo[] {
  if (hiddenIds.size === 0) return repos as Repo[]

  const out: Repo[] = []
  for (const repo of repos) {
    if (hiddenIds.has(repo.id)) continue

    const folders = repo.folders ?? EMPTY_FOLDERS
    const heldFolders = folders.filter((f) => hiddenIds.has(f.id))
    const workspaces = repo.workspaces.filter((w) => !hiddenIds.has(w.id))
    const chats = repo.chats?.some((c) => hiddenIds.has(c.id))
      ? repo.chats.filter((c) => !hiddenIds.has(c.id))
      : repo.chats
    if (
      heldFolders.length === 0 &&
      workspaces.length === repo.workspaces.length &&
      chats === repo.chats
    ) {
      out.push(repo)
      continue
    }

    if (heldFolders.length === 0) {
      out.push({ ...repo, workspaces, chats })
      continue
    }

    const byId = new Map(folders.map((f) => [f.id, f]))
    const survivingParentOf = (folderId: string): string => {
      let cursor = byId.get(folderId)?.parentId ?? ''
      const seen = new Set<string>([folderId])
      while (cursor && hiddenIds.has(cursor) && !seen.has(cursor)) {
        seen.add(cursor)
        cursor = byId.get(cursor)?.parentId ?? ''
      }
      return cursor
    }
    const rehomed = new Map(heldFolders.map((f) => [f.id, survivingParentOf(f.id)]))

    const survivors: Folder[] = []
    for (const folder of folders) {
      if (hiddenIds.has(folder.id)) continue
      survivors.push(
        folder.parentId && rehomed.has(folder.parentId)
          ? { ...folder, parentId: rehomed.get(folder.parentId) }
          : folder,
      )
    }

    out.push({
      ...repo,
      workspaces: workspaces.map((w) =>
        w.folderId && rehomed.has(w.folderId) ? { ...w, folderId: rehomed.get(w.folderId) } : w,
      ),
      folders: survivors,
      chats,
    })
  }

  return out
}

export function renderedHiddenIds(
  hiddenIds: ReadonlySet<string>,
  entries: readonly RemovalEntry[],
): ReadonlySet<string> {
  if (hiddenIds.size === 0 || entries.length === 0) return hiddenIds
  const out = new Set(hiddenIds)
  for (const entry of entries) {
    if (entry.kind === 'repo' || entry.kind === 'project') continue
    out.delete(entry.id)
    if (entry.primaryRowId) out.delete(entry.primaryRowId)
  }
  return out
}

export function attachRemovalState(
  rows: readonly SidebarRow[],
  entries: readonly RemovalEntry[],
): SidebarRow[] {
  const byWorkspaceId = new Map<string, RemovalEntry>()
  const byRowId = new Map<string, RemovalEntry>()
  for (const entry of entries) {
    if (entry.kind === 'workspace') byWorkspaceId.set(entry.id, entry)
    else if (entry.kind === 'chat' || entry.kind === 'folder') byRowId.set(entry.id, entry)
  }
  if (byWorkspaceId.size === 0 && byRowId.size === 0) return rows as SidebarRow[]

  return rows.map((row) => {
    const entry =
      row.kind === 'branch' && row.workspaceId
        ? byWorkspaceId.get(row.workspaceId)
        : byRowId.get(row.id)
    if (!entry) return row
    return {
      ...row,
      removal: { entryId: entry.entryId, deadlineAt: entry.deadlineAt, extra: entry.extra },
    }
  })
}
