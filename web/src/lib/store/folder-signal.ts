import { create } from 'zustand'
import { subscribeWithSelector } from 'zustand/middleware'

/**
 * Per-repo generation counter, bumped once for every folder_created/
 * folder_updated/folder_deleted frame `use-workspace-agent-chats-stream.ts`
 * sees on a repo's chats WS.
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
interface FolderSignalState {
  generations: Record<string, number>
  bump: (repoId: string) => void
}

export const useFolderSignalStore = create<FolderSignalState>()(
  subscribeWithSelector((set) => ({
    generations: {},
    bump: (repoId) =>
      set((state) => ({
        generations: { ...state.generations, [repoId]: (state.generations[repoId] ?? 0) + 1 },
      })),
  })),
)
