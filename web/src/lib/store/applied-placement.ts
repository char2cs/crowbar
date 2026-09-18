import { getEntity, upsertEntity } from '@/lib/persistence/entity-cache'
import { toSidebarRepo } from '@/lib/store/build-repo-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

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
  applyWrittenRows(written)
  const movedRepoId =
    useSidebarStore.getState().repos.find((r) => r.chats?.some((c) => c.id === chat.id))?.id ?? null
  if (movedRepoId) useFolderSignalStore.getState().bump(movedRepoId)
  return movedRepoId
}

/** Apply cache-written chat/folder rows to the store. A row the cache does
 *  not know (cold cache) is patched onto whichever chat or folder the store
 *  holds under that id. */
function applyWrittenRows(written: readonly CachedRow[]): void {
  const placements = new Map<string, RowPlacement>()
  const folders: FolderDTO[] = []
  for (const entry of written) {
    if (entry.kind === 'folder') folders.push(entry.dto)
    else if (entry.kind === 'chat') placements.set(entry.dto.id, entry.dto)
    else placements.set(entry.row.id, entry.row)
  }
  useSidebarStore.setState((s) => ({
    repos: s.repos.map((repo) => {
      const touchesChat = repo.chats?.some((c) => placements.has(c.id))
      const touchesFolder = repo.folders?.some((f) => placements.has(f.id))
      if (!touchesChat && !touchesFolder) return repo
      return {
        ...repo,
        chats: repo.chats?.map((c) => {
          const placement = placements.get(c.id)
          return placement
            ? { ...c, parentId: placement.parentId || undefined, order: placement.order }
            : c
        }),
        folders: repo.folders?.map((f) => {
          const placement = placements.get(f.id)
          return placement
            ? { ...f, parentId: placement.parentId || undefined, order: placement.order }
            : f
        }),
      }
    }),
  }))
  const applyFolder = useSidebarStore.getState().applyFolderDTO
  folders.forEach(applyFolder)
}

/**
 * Apply a workspace's own placement PATCH answer — the moved row's decided
 * folderId/order and every chat/folder sibling it shifted — to the cache and
 * the store, then bump the repo's tree signal. A branch row's order comes off
 * its WorkspaceDTO, which no placement frame refreshes, so this is the only
 * way the dragged row moves before a reload.
 */
export async function applyWorkspacePlacement(
  wsId: string,
  placement: { parentId: string; order: number },
  shifted: readonly RowPlacement[] = [],
): Promise<void> {
  const [cached, written] = await Promise.all([
    getEntity<WorkspaceDTO>('crowbar_workspaces', wsId),
    Promise.all(shifted.map(writePlacementThrough)),
  ])
  if (cached) {
    await upsertEntity('crowbar_workspaces', {
      ...cached,
      folderId: placement.parentId,
      order: placement.order,
    })
  }
  useSidebarStore.getState().applyPlacement({
    workspaces: [{ id: wsId, folderId: placement.parentId, order: placement.order }],
  })
  applyWrittenRows(written)
  const repoId = useSidebarStore
    .getState()
    .repos.find((r) => r.workspaces.some((w) => w.id === wsId))?.id
  if (repoId) useFolderSignalStore.getState().bump(repoId)
}

/** Apply a repo's re-read workspace rows to the cache, then merge their
 *  placement into the store — the siblings a workspace densify shifted are
 *  announced nowhere else. */
export async function applyWorkspacePlacements(rows: readonly WorkspaceDTO[]): Promise<void> {
  await Promise.all(rows.map((row) => upsertEntity('crowbar_workspaces', row)))
  useSidebarStore.getState().applyPlacement({
    workspaces: rows.map((row) => ({
      id: row.id,
      folderId: row.folderId ?? '',
      order: row.order ?? 0,
    })),
  })
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
