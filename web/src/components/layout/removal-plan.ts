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

/** The little a removal needs to know about a project: which one, and its label. */
export interface ProjectRow {
  id: string
  name: string
}

/** The five movable classes. They do not mix. */
export type DropKind = 'workspace' | 'folder' | 'repo' | 'project' | 'chat'

/** A row as a removal/drag subject: its class, its id, and enough placement to find its repo. */
export interface DragSubject extends DragSubjectBase {
  kind: DropKind
  /** Repo scope, for the same-repo rule. Absent on repos and projects. */
  repoId?: string
  /** A protected branch: reorders among its own siblings and nothing else. */
  locked?: boolean
  /** Its current parent, for the locked same-parent rule. */
  parentId?: string
}

// A hold is not a delete: this pure plan says what to HIDE, mirroring each daemon cascade — a
// workspace or chat takes its subtree, a folder only itself (children reparent), a repo or project
// every worktree under it. Undo puts the ids back; the API call happens only on commit.
export function planRemoval(
  subjects: readonly DragSubject[],
  repos: readonly Repo[],
  projects: readonly ProjectRow[] = [],
): RemovalDraft[] {
  const drafts: RemovalDraft[] = []
  const claimed = new Set<string>()

  for (const subject of subjects) {
    const draft = draftFor(subject, repos, projects)
    // A row inside another subject's subtree is part of that removal, not a second tray row.
    if (!draft || claimed.has(draft.id)) continue
    drafts.push(draft)
    for (const id of draft.hiddenIds) claimed.add(id)
  }

  return drafts
}

/** Every chat hanging off `id`, transitively, by `parentId` — ids only, no rendered tree. */
function chatDescendantsOf(chats: readonly Chat[], id: string): string[] {
  const childrenOf = new Map<string, string[]>()
  for (const c of chats) {
    if (!c.parentId) continue
    const siblings = childrenOf.get(c.parentId)
    if (siblings) siblings.push(c.id)
    else childrenOf.set(c.parentId, [c.id])
  }
  const out: string[] = []
  // A corrupt parentId cycle would otherwise recurse forever.
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
      // A project spans every repo under it, so there is no single owning one.
      repoId: '',
      // Sidebar rows are not workspace-scoped: `wsId`/`providerIcon` stay empty.
      wsId: '',
      providerIcon: '',
      // The project row AND every repo row in it: the delete cascades, so hiding only the
      // header would leave its repos on screen with nothing above them.
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

  // A project-home chat or folder rides no repo, so it resolves BEFORE the repo lookup below,
  // which would find nothing for its unset `repoId` and refuse the removal.
  if (subject.kind === 'chat' || subject.kind === 'folder') {
    const homeRow = resolveHomeRowScope(subject.id)
    if (homeRow) {
      const tree = getHomeTree(homeRow.projectId)
      if (subject.kind === 'chat') {
        const chat = tree.chats.find((c) => c.id === subject.id)
        if (!chat) return null
        // Same subtree rule as a repo chat below: every thread off this one goes with it.
        const descendants = chatDescendantsOf(tree.chats, chat.id)
        return {
          kind: 'chat',
          id: chat.id,
          label: chat.title || UNTITLED_CHAT_LABEL,
          projectId: homeRow.projectId,
          // No owning repo; the DELETE route is scoped by the project's home workspace.
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
    // A branch row shares the chat id space but is a WORKSPACE (`resolveChatRow`'s rule).
    if (workspaceIdOfBranchRow(repos, subject.id) !== null) return null
    const chat = repo.chats?.find((c) => c.id === subject.id)
    if (!chat) return null
    // The DELETE route is repo-scoped: any workspace of this repo resolves the URL.
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
      // Reparenting and deleting both take the whole subtree (spec §8.3).
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
  // A protected branch keeps its worktree; the daemon refuses the delete, so the
  // tray must never accept one and promise otherwise.
  if (!workspace || workspace.status === 'locked') return null

  // Its branches and folders as drawn; chats are collected below by the workspace they run in.
  const descendants = descendantIds(legacyTreeIndex([{ ...repo, chats: [] }], null), workspace.id)
  // Hide both halves of every row that goes: a workspace row IS its owning chat, and hiding only
  // the Workspace leaves that chat to resurface as a bubble during the countdown. Its threads go
  // too, since the daemon's delete cascades the whole subtree.
  const goingWorkspaceIds = new Set([workspace.id, ...descendants])
  const hiddenChatIds: string[] = []
  for (const chat of repo.chats ?? EMPTY_CHATS) {
    if (!chat.workspaceId || !goingWorkspaceIds.has(chat.workspaceId)) continue
    hiddenChatIds.push(chat.id, ...chatDescendantsOf(repo.chats ?? EMPTY_CHATS, chat.id))
  }
  return {
    kind: 'workspace',
    id: workspace.id,
    // The rendered row is keyed by its OWNING CHAT; `renderedHiddenIds` keeps it on screen so it
    // transforms in place instead of re-keying. The workspace id covers a not-yet-owned workspace.
    primaryRowId: workspace.owningChatId ?? workspace.id,
    label: workspace.branch,
    projectId: repo.projectId,
    repoId: repo.id,
    wsId: '',
    providerIcon: '',
    hiddenIds: [workspace.id, ...descendants, ...new Set(hiddenChatIds)],
    // WORKSPACES only: the chat ids above are the same rows, not extra worktrees.
    extra: descendants.length,
    // Resolved now, against a tree that still has the row in it.
    fallbackWsId: getPostDeleteNavigationTarget(repos as Repo[], workspace.id),
  }
}

// The repos as they read with held rows taken out — a projection, so Cancel just drops an id. A
// held FOLDER rewrites rather than filters: its delete reparents children to its parent, and the
// preview must show them there, not re-rooted.
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
    // A held chat's descendants are in the same hold, so a plain filter suffices. An untouched
    // repo keeps `chats` by identity (`undefined` stays `undefined`, not `[]`).
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

    // Walk past held ancestors so survivors of nested held folders land on a parent still shown.
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

// The store's `hiddenIds` minus each live entry's primary row, which stays on screen transformed
// in place (repo/project entries have none). The store set outlives `entries` until tombstones
// land, so "on screen" cannot be derived from `entries` alone.
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

// Stamps `row.removal` on rows matching a held entry so they render transformed in place. A
// workspace entry holds the RAW workspace id, matched via the branch row's `workspaceId` (its row
// id is the owning chat's); chat/folder ids match directly; repo/project have no row here.
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
