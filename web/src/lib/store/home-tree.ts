import { create } from 'zustand'
import { wsManager } from '@/lib/ws/manager'
import { fetchHomeChats, fetchHomeFolders } from '@/lib/api'
import { toSidebarChat, toSidebarFolder } from '@/lib/store/build-repo-tree'
import { EMPTY_CHATS, EMPTY_FOLDERS, type Chat, type Folder } from '@/lib/store/sidebar'
import { NON_STRUCTURAL_CHAT_KINDS } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

/**
 * A project's home-workspace chat rows and folders — the same two aggregates
 * `Repo.chats`/`Repo.folders` hold, kept in their OWN per-project map instead
 * of shoehorned into `useSidebarStore`'s `Repo[]`: project home rides no
 * repo at all (see `rows-from-home.ts`'s own doc), so nothing here can be a
 * `Repo` without faking one.
 */
export interface HomeTree {
  chats: Chat[]
  folders: Folder[]
}

const EMPTY_HOME_TREE: HomeTree = { chats: EMPTY_CHATS, folders: EMPTY_FOLDERS }

interface HomeTreeStore {
  trees: Record<string, HomeTree>
  setTree: (projectId: string, tree: HomeTree) => void
  clearTree: (projectId: string) => void
}

export const useHomeTreeStore = create<HomeTreeStore>()((set) => ({
  trees: {},
  setTree: (projectId, tree) => set((s) => ({ trees: { ...s.trees, [projectId]: tree } })),
  clearTree: (projectId) =>
    set((s) => {
      if (!(projectId in s.trees)) return s
      const trees = { ...s.trees }
      delete trees[projectId]
      return { trees }
    }),
}))

/** `projectId`'s home tree, or the stable empty one before its first seed. */
export function getHomeTree(projectId: string): HomeTree {
  return useHomeTreeStore.getState().trees[projectId] ?? EMPTY_HOME_TREE
}

/**
 * `id` among every VISIBLE project's home chats/folders, or null. THE
 * canonical resolver — home rows are not in `useSidebarStore`'s `repos` at
 * all (home rides no repo — home-workspace-resolver.ts's own doc), so
 * nothing downstream of that array can ever see them, and treating "not
 * found there" as "do nothing" is exactly how a home row came to render but
 * never open, and separately never drag — both caught live, both fixed by
 * consulting this FIRST. Every caller that needs to tell a home row apart
 * from a repo one (space-content-actions.ts's `handleOpen`,
 * sidebar-drop-policy.ts's `allowedModes`, drop-actions.ts's `planRowDrop`)
 * shares this one implementation rather than each re-deriving the same
 * chats/folders scan.
 */
export function resolveHomeRowScope(
  id: string,
): { kind: 'chat' | 'folder'; projectId: string; homeWorkspaceId: string } | null {
  const trees = useHomeTreeStore.getState().trees
  for (const projectId of Object.keys(trees)) {
    const tree = trees[projectId]
    const isChat = tree.chats.some((c) => c.id === id)
    const isFolder = !isChat && tree.folders.some((f) => f.id === id)
    if (!isChat && !isFolder) continue
    const homeWorkspaceId = getHomeWorkspaceId(projectId)
    if (!homeWorkspaceId) return null
    return { kind: isChat ? 'chat' : 'folder', projectId, homeWorkspaceId }
  }
  return null
}

/**
 * Merge one or more folder rows into `projectId`'s home tree by id — the
 * direct-apply half of every home folder write (create, drop-to-file,
 * drop-to-reorder), mirroring `useSidebarStore.applyFolderDTO`'s upsert for
 * a repo. Home has no dedicated push channel for folders either (same
 * reason a repo's doesn't — Task 34's plan closed it for both), so a
 * write's own response is the only confirmation any caller gets;
 * `subscribeHomeTree`'s reseed-on-signal is the eventual-consistency
 * backstop, not the primary path.
 */
export function applyHomeFolders(projectId: string, folders: readonly Folder[]): void {
  const store = useHomeTreeStore.getState()
  const current = store.trees[projectId] ?? EMPTY_HOME_TREE
  const byId = new Map(current.folders.map((f) => [f.id, f]))
  for (const folder of folders) byId.set(folder.id, folder)
  store.setTree(projectId, { ...current, folders: [...byId.values()] })
}

/**
 * Keep `projectId`'s home tree seeded: a GET on open, then a reseed on every
 * STRUCTURAL frame the home chat lifecycle feed carries — mirroring
 * `app-sync-provider.tsx`'s `openRepoTreeSubscription`, and for the identical
 * reason: the daemon's folders resource has no push channel of its own
 * (Task 34's plan closed it), and a chat's only live-update path is this
 * same id-only lifecycle feed, already mounted per project for the
 * working-spinner (`home-workspace.ts`'s `subscribeHomeWorkspace`, which
 * listens on the SAME endpoint for a disjoint kind set — `wsManager`
 * multiplexes multiple subscribers onto one socket, so this opens no second
 * connection).
 *
 * Unlike a repo's tree, this is not gated behind "some workspace of this
 * repo is mounted": home rides no repo, so there is no cheaper signal to
 * fall back to, and it is opened for every VISIBLE project (see
 * `app-sync-provider.tsx`'s `desiredKeys`), not only the active one — a
 * project's home row must render exactly as reliably as its repos do.
 */
export function subscribeHomeTree(projectId: string): () => void {
  let disposed = false
  let latestRead = 0

  async function reseed(): Promise<void> {
    const seq = ++latestRead
    try {
      const [chats, folders] = await Promise.all([
        fetchHomeChats(projectId),
        fetchHomeFolders(projectId),
      ])
      if (disposed || seq !== latestRead) return
      useHomeTreeStore.getState().setTree(projectId, {
        chats: chats.map(toSidebarChat),
        folders: folders.map(toSidebarFolder),
      })
    } catch (err) {
      // A transient failure leaves the last known tree in place; the next
      // structural frame (or reconnect reseed) retries.
      console.error(`home-tree: reseed failed for project ${projectId}`, err)
    }
  }

  void reseed()
  const unsubscribe = wsManager.subscribe(`/v0/projects/${projectId}/home/chats/ws`, (frame) => {
    if (disposed) return
    // The reconnect sentinel: structural frames may have been missed while
    // the socket was down, so reseed unconditionally.
    if (frame && typeof frame === 'object' && 'reconnected' in frame) {
      void reseed()
      return
    }
    const kind = (frame as { kind?: string } | null)?.kind
    if (kind !== undefined && !NON_STRUCTURAL_CHAT_KINDS.has(kind)) void reseed()
  })

  return () => {
    disposed = true
    unsubscribe()
    useHomeTreeStore.getState().clearTree(projectId)
  }
}
