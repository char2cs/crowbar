import { entityStore, type EntityState, type StoredKind } from '@/core/entities/entity-store'
import { entityKey, hostScope } from '@/core/stream/host-scope'
import type { ChatEntity } from '@/core/entities/chat-entity'
import type { Chat, Folder, Repo, Workspace } from '@/core/entities/repo-types'
import type { HomeTree } from '@/core/entities/views-from-entities'
import type { FolderDTO, Project, RepoDTO, WorkspaceDTO } from '@/lib/types'

type EntityOf<K extends StoredKind> = EntityState['byKind'][K][string]['data']

let v = 0

/** No entity anywhere, and no scope synced. */
export function resetEntities(): void {
  entityStore.setState(entityStore.getInitialState(), true)
}

/** Writes `hostId`'s rows as its projects scope's frames leave them, each at a newer version. */
export function putEntities<K extends StoredKind>(
  kind: K,
  rows: readonly EntityOf<K>[],
  hostId = 'local',
): void {
  entityStore.setState((s) => {
    const next = { ...s.byKind[kind] } as Record<string, { v: number; data: EntityOf<K> }>
    for (const row of rows) {
      next[entityKey(hostId, (row as { id: string }).id)] = { v: ++v, data: row }
    }
    return { byKind: { ...s.byKind, [kind]: next } }
  })
}

/** `hostId`'s projects scope's snapshot has landed. */
export function markProjectsSynced(hostId = 'local'): void {
  const scope = hostScope(hostId, 'projects')
  entityStore.setState((s) => ({
    synced: { ...s.synced, [scope]: true },
    loaded: { ...s.loaded, [scope]: true },
  }))
}

export function projectDTO(over: Partial<Project> & { id: string }): Project {
  return { name: over.id, path: `/${over.id}`, lastActivity: new Date(0), hostId: 'local', ...over }
}

export function workspaceDTO(over: Partial<WorkspaceDTO> & { id: string }): WorkspaceDTO {
  return {
    repoId: '',
    projectId: 'p1',
    branch: over.id,
    parentId: '',
    forkPointSha: '',
    status: '' as WorkspaceDTO['status'],
    working: false,
    added: 0,
    deleted: 0,
    mergeStrategy: '',
    canMergeLocally: false,
    mergeConflicts: false,
    parentBranch: '',
    prUrl: '',
    prTitle: '',
    prTargetBranch: '',
    provisioning: 'provisioned',
    ...over,
  }
}

export function chatEntity(over: Partial<ChatEntity> & { id: string }): ChatEntity {
  return {
    projectId: 'p1',
    repoId: '',
    workspaceId: '',
    title: '',
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: '',
    working: false,
    createdAt: '2026-01-01T00:00:00Z',
    version: 1,
    phase: 'dormant',
    parentId: '',
    order: 0,
    ...over,
  }
}

function fromWorkspace(repo: Repo, ws: Workspace): WorkspaceDTO {
  return workspaceDTO({
    id: ws.id,
    repoId: repo.id,
    projectId: repo.projectId ?? '',
    branch: ws.branch,
    parentId: ws.parentId ?? '',
    status: (ws.status ?? '') as WorkspaceDTO['status'],
    working: ws.working ?? false,
    added: ws.added ?? 0,
    deleted: ws.deleted ?? 0,
    canMergeLocally: ws.canMergeLocally ?? false,
    mergeConflicts: ws.mergeConflicts ?? false,
    parentBranch: ws.parentBranch ?? '',
    prUrl: ws.prUrl ?? '',
    ...(ws.provisioning && { provisioning: ws.provisioning }),
    ...(ws.localPath && { localPath: ws.localPath }),
    ...(ws.heldByPath && { heldByPath: ws.heldByPath }),
    folderId: ws.folderId ?? '',
    order: ws.order ?? 0,
    ...(ws.createdAt && { createdAt: ws.createdAt }),
    owningChatId: ws.owningChatId ?? '',
  })
}

function fromFolder(projectId: string, f: Folder): FolderDTO {
  return {
    id: f.id,
    repoId: f.repoId,
    projectId,
    parentId: f.parentId ?? '',
    name: f.name,
    order: f.order ?? 0,
    ...(f.createdAt && { createdAt: f.createdAt }),
  }
}

function fromChat(projectId: string, repoId: string, c: Chat): ChatEntity {
  return chatEntity({
    id: c.id,
    projectId,
    repoId,
    workspaceId: c.workspaceId ?? '',
    parentId: c.parentId ?? '',
    title: c.title,
    order: c.order ?? 0,
    ...(c.type && { type: c.type }),
    ...(c.createdAt && { createdAt: c.createdAt }),
  })
}

function fromRepo(repo: Repo): {
  repo: RepoDTO
  workspaces: WorkspaceDTO[]
  folders: FolderDTO[]
  chats: ChatEntity[]
} {
  const projectId = repo.projectId ?? ''
  const workspaces = repo.workspaces.map((ws) => fromWorkspace(repo, ws))
  // A repo whose tree names its default checkout as a row keeps that row.
  if (repo.defaultWorkspaceId && !workspaces.some((w) => w.id === repo.defaultWorkspaceId)) {
    workspaces.push(
      workspaceDTO({
        id: repo.defaultWorkspaceId,
        repoId: repo.id,
        projectId,
        branch: repo.defaultBranch ?? 'main',
        isDefault: true,
        status: (repo.defaultWorkspaceStatus ?? '') as WorkspaceDTO['status'],
        working: repo.defaultWorking ?? false,
        owningChatId: repo.defaultOwningChatId ?? '',
      }),
    )
  }
  // A chat that owns its worktree is its workspace's owning chat.
  for (const c of repo.chats ?? []) {
    const ws = workspaces.find((w) => w.id === c.workspaceId)
    if (c.ownsWorktree && ws && !ws.owningChatId) ws.owningChatId = c.id
  }
  return {
    repo: {
      id: repo.id,
      projectId,
      name: repo.name ?? repo.id,
      path: repo.localPath ?? '',
      defaultBranch: repo.defaultBranch ?? '',
      avatarLabel: repo.avatarLabel ?? '',
      avatarColor: repo.avatarColor ?? '',
      avatarUrl: '',
      avatarEmoji: '',
      order: repo.order ?? 0,
      folderId: repo.folderId ?? '',
      ...(repo.deleting && { deleting: true }),
    },
    workspaces,
    folders: (repo.folders ?? []).map((f) => fromFolder(projectId, f)),
    chats: (repo.chats ?? []).map((c) => fromChat(projectId, repo.id, c)),
  }
}

type Rows = { [K in StoredKind]?: (row: EntityOf<K>) => boolean }
type Row = { id: string; hostId?: string }

/** Drops the held rows `drop` names, then writes `rows` over the rest, in one write as
 *  a snapshot lands, and marks the projects scope synced. */
function replaceEntities(drop: Rows, rows: { [K in StoredKind]?: readonly EntityOf<K>[] }): void {
  entityStore.setState((s) => {
    const byKind = { ...s.byKind }
    for (const kind of Object.keys(drop) as StoredKind[]) {
      const keep = drop[kind] as (row: unknown) => boolean
      byKind[kind] = Object.fromEntries(
        Object.entries(byKind[kind]).filter(([, held]) => !keep(held.data)),
      ) as never
    }
    for (const kind of Object.keys(rows) as StoredKind[]) {
      const next = { ...byKind[kind] } as Record<string, { v: number; data: unknown }>
      // A project row names its host; every other fixture row is the local daemon's.
      for (const row of (rows[kind] ?? []) as unknown as readonly Row[]) {
        const hostId = row.hostId ?? 'local'
        const data = kind === 'project' ? { ...row, hostId } : row
        next[entityKey(hostId, row.id)] = { v: ++v, data }
      }
      byKind[kind] = next as never
    }
    return {
      byKind,
      synced: { ...s.synced, projects: true },
      loaded: { ...s.loaded, projects: true },
    }
  })
}

/** The repos the projects scope carries, in the shape the sidebar draws: replaces every
 *  repo and every row of one, and marks the snapshot landed. */
export function seedRepos(repos: readonly Repo[]): void {
  const parts = repos.map(fromRepo)
  replaceEntities(
    {
      repo: () => true,
      workspace: (w) => !!w.repoId,
      folder: (f) => !!f.repoId,
      chat: (c) => !!c.repoId,
    },
    {
      repo: parts.map((p) => p.repo),
      workspace: parts.flatMap((p) => p.workspaces),
      folder: parts.flatMap((p) => p.folders),
      chat: parts.flatMap((p) => p.chats),
    },
  )
}

/** `projectId`'s home chats and folders, replacing what its home held. */
export function seedHomeTree(projectId: string, tree: Partial<HomeTree>): void {
  const home = (row: { projectId: string; repoId: string }) =>
    row.projectId === projectId && !row.repoId
  // A chat that owns its worktree is its home workspace's owning chat.
  const held = entityStore.getState().byKind.workspace
  const owned = (tree.chats ?? [])
    .filter((c) => c.ownsWorktree && c.workspaceId)
    .map((c) => ({
      ...(held[c.workspaceId!]?.data ??
        workspaceDTO({ id: c.workspaceId!, projectId, kind: 'home' })),
      owningChatId: c.id,
    }))
  replaceEntities(
    { folder: home, chat: home },
    {
      folder: (tree.folders ?? []).map((f) => fromFolder(projectId, { ...f, repoId: '' })),
      chat: (tree.chats ?? []).map((c) => fromChat(projectId, '', c)),
      ...(owned.length > 0 && { workspace: owned }),
    },
  )
}

/** `projectId`'s home workspace as the projects scope carries it. */
export function seedHome(projectId: string, wsId: string, over: Partial<WorkspaceDTO> = {}): void {
  replaceEntities(
    { workspace: (w) => w.projectId === projectId && !w.repoId },
    { workspace: [workspaceDTO({ id: wsId, projectId, kind: 'home', ...over })] },
  )
}

/** The project list, replacing the one held. */
export function seedProjects(projects: readonly Project[]): void {
  replaceEntities({ project: () => true }, { project: projects })
}
