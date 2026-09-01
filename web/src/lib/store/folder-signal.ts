import { create } from 'zustand'
import { subscribeWithSelector } from 'zustand/middleware'

/**
 * Per-repo generation counter for "this repo's TREE moved, read it again" —
 * bumped by `use-workspace-agent-chats-stream.ts` for every folder_created/
 * folder_updated/folder_deleted frame it sees on a repo's chats WS, and for
 * every STRUCTURAL chat frame (created / deleted / title_set / placement_set /
 * order_set — see that file's NON_STRUCTURAL_CHAT_KINDS).
 *
 * ONE signal for folders and chats, not two, because there is one tree and one
 * aggregate behind it: a folder IS a `domain.Chat` row (design spec §3.1), both
 * halves are read back through the same repo-scoped mount, and both are reseeded
 * by the same subscription in one pass. Two counters would be two things to keep
 * in step for no invalidation either half can express alone. The name is
 * historical (Task 34 added it for folders); the meaning is the repo's tree.
 *
 * Task 34: the sidebar's folders resource lost its own dedicated push channel
 * (the backend's folders REST+WS resource was deleted; the backend plan that
 * carried it is closed) — a folder change now rides the chats WS only as an
 * id-only invalidation frame, "the tree moved, read it again". That frame
 * lands on a hook keyed by WORKSPACE; `app-sync-provider.tsx`'s folders
 * subscription is keyed by REPO; nothing bridged the two before this. This
 * store is that bridge — `app-sync-provider.tsx` subscribes to one repo's
 * generation via `subscribeWithSelector` and reseeds whenever it moves. It
 * never reads the number itself, only whether it changed.
 */
/**
 * Stable empty set, for the same reason EMPTY_FOLDERS exists: a selector that
 * hands back a fresh `new Set()` per read makes the Zustand snapshot compare
 * unstable and React eventually throws somewhere unrelated.
 */
const NO_REPOS: ReadonlySet<string> = new Set()

interface FolderSignalState {
  generations: Record<string, number>
  /**
   * Repos whose tree rows have come back from the daemon AT LEAST ONCE.
   *
   * `Repo.chats`'s own contract is that an absent list means "not yet", never
   * "this repo has no chats" — chats/folders arrive on this store's own reseed
   * loop, which is independent of the repo and workspace entity streams, so
   * there is a real window (every cold start, and every folded-away project
   * that has no subscription at all) where a repo has workspaces and no chats.
   *
   * Nothing recorded the difference before, so no consumer could tell the two
   * apart. `rows-from-repo.ts` is the one that must: a workspace's row is
   * identified by the chat that owns it, so asking during the window would
   * either hand out an id that changes when the seed lands — and a row id is
   * the React key, the `collapsedWorkspaces` key and the selection key — or
   * fail on data that is merely late.
   */
  seededRepoIds: ReadonlySet<string>
  bump: (repoId: string) => void
  /** Record that `repoId`'s tree rows have been read. Idempotent, and returns
   *  the SAME set when the repo is already known, so a reseed that changes
   *  nothing costs no subscriber a render. */
  markTreeSeeded: (repoId: string) => void
}

export const useFolderSignalStore = create<FolderSignalState>()(
  subscribeWithSelector((set) => ({
    generations: {},
    seededRepoIds: NO_REPOS,
    bump: (repoId) =>
      set((state) => ({
        generations: { ...state.generations, [repoId]: (state.generations[repoId] ?? 0) + 1 },
      })),
    markTreeSeeded: (repoId) =>
      set((state) =>
        state.seededRepoIds.has(repoId)
          ? state
          : { seededRepoIds: new Set([...state.seededRepoIds, repoId]) },
      ),
  })),
)
