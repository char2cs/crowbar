import { getEntity, upsertEntity } from '@/lib/persistence/entity-cache'
import { toSidebarRepo } from '@/lib/store/build-repo-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { ChatDTO, FolderDTO, RepoDTO } from '@/lib/types'

// A placement PATCH answers with the daemon's already-committed rows and no
// push channel confirms them, so the caller applies the response itself. Every
// tree REBUILD is `setRepos(readVisibleRepoTree())` — a wholesale replacement
// from the IndexedDB entity cache — so an apply that reaches only
// `useSidebarStore` survives exactly until the next rebuild, which puts the
// row back where the cache still says it was (the `placement_set` frame the
// same PATCH broadcast triggers one within a frame or two). These helpers
// write the cache FIRST, then the store, then bump the repo's reseed.

/** A row's placement as a PATCH response reports it. */
export interface RowPlacement {
  id: string
  /** A chat id, a folder id, or ''/absent for the root. */
  parentId?: string
  order: number
}

type CachedRow =
  | { kind: 'chat'; dto: ChatDTO }
  | { kind: 'folder'; dto: FolderDTO }
  | { kind: 'unknown'; row: RowPlacement }

/** A chat's `shifted` siblings share its level and may be chats or folders;
 *  the cache is keyed by one id space, so whichever store holds the id wins. */
async function writePlacementThrough(row: RowPlacement): Promise<CachedRow> {
  const parentId = row.parentId ?? ''
  const chat = await getEntity<ChatDTO>('crowbar_chats', row.id)
  if (chat) {
    const dto = { ...chat, parentId, order: row.order }
    await upsertEntity('crowbar_chats', dto)
    return { kind: 'chat', dto }
  }
  const folder = await getEntity<FolderDTO>('crowbar_folders', row.id)
  if (folder) {
    const dto = { ...folder, parentId, order: row.order }
    await upsertEntity('crowbar_folders', dto)
    return { kind: 'folder', dto }
  }
  return { kind: 'unknown', row }
}

/**
 * Apply a chat placement (the moved chat plus every sibling the daemon's dense
 * renumber shifted) to the cache and the store, and bump the repo's tree
 * signal. Resolves to the repo the chat lives in, or null when no repo in the
 * store holds it.
 */
export async function applyChatPlacement(
  chat: RowPlacement,
  shifted: readonly RowPlacement[] = [],
): Promise<string | null> {
  const written = await Promise.all([chat, ...shifted].map(writePlacementThrough))
  const placements = new Map<string, RowPlacement>()
  const folders: FolderDTO[] = []
  for (const entry of written) {
    if (entry.kind === 'folder') folders.push(entry.dto)
    else if (entry.kind === 'chat') placements.set(entry.dto.id, entry.dto)
    else placements.set(entry.row.id, entry.row)
  }

  let movedRepoId: string | null = null
  useSidebarStore.setState((s) => {
    const repos = s.repos.map((repo) => {
      if (!repo.chats?.some((c) => placements.has(c.id))) return repo
      if (repo.chats.some((c) => c.id === chat.id)) movedRepoId = repo.id
      return {
        ...repo,
        chats: repo.chats.map((c) => {
          const placement = placements.get(c.id)
          return placement
            ? { ...c, parentId: placement.parentId || undefined, order: placement.order }
            : c
        }),
      }
    })
    return { repos }
  })
  const applyFolder = useSidebarStore.getState().applyFolderDTO
  folders.forEach(applyFolder)
  if (movedRepoId) useFolderSignalStore.getState().bump(movedRepoId)
  return movedRepoId
}

/**
 * Apply a folder write's `{folder, shifted}` answer — a create, rename or
 * placement — to the cache and the store, then bump `repoId`'s tree signal.
 */
export async function applyFolderPlacements(
  repoId: string,
  folders: readonly FolderDTO[],
): Promise<void> {
  await Promise.all(folders.map((folder) => upsertEntity('crowbar_folders', folder)))
  const apply = useSidebarStore.getState().applyFolderDTO
  folders.forEach(apply)
  useFolderSignalStore.getState().bump(repoId)
}

/** Apply a project's re-read repo rows to the cache, then merge their
 *  placement into the store. */
export async function applyRepoPlacements(repos: readonly RepoDTO[]): Promise<void> {
  await Promise.all(repos.map((repo) => upsertEntity('crowbar_repos', repo)))
  useSidebarStore.getState().mergeRepos(repos.map((dto) => toSidebarRepo(dto, [])))
}
