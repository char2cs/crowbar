import { useEffect } from 'react'
import { wsManager } from '@/lib/ws/manager'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { listThreads, mapThread } from '@/features/git/api/review-api'
import type { ThreadDTO } from '@/features/git/api/review-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

/**
 * Subscribe to the workspace-scoped /threads WebSocket stream while a
 * workspace is active. On each pushed ThreadDTO the wire shape is mapped via
 * mapThread and stored via upsertReviewThread.
 *
 * Lifecycle mirrors the git/status effect in use-workspace-effects.ts:
 *  1. Seed: fetch all current threads via listThreads and upsert each.
 *  2. Subscribe: on every incoming ThreadDTO frame, upsert the mapped thread.
 *  3. Reconnect guard: the manager signals reconnect frames with { reconnected: true };
 *     re-seed on reconnect so we don't miss pushes received during the outage.
 *  4. Cleanup: unsubscribe (and cancel any in-flight seed) on wsId change / unmount.
 */
export function useWorkspaceThreadsStream(wsId: string): void {
  useEffect(() => {
    let cancelled = false

    const seed = async () => {
      try {
        const threads = await listThreads(wsId)
        if (cancelled) return
        const store = getOrCreateWorkspaceStore(wsId)
        const { upsertReviewThread } = store.getState()
        for (const t of threads) {
          upsertReviewThread(t)
        }
      } catch {
        // seed failure is non-fatal; the WS stream will still push updates
      }
    }

    void seed()

    const unsubscribe = wsManager.subscribe(
      `${workspaceBase(wsId)}/threads`,
      (frame) => {
        if (cancelled) return
        // Reconnect sentinel emitted by the manager after a socket drop+reopen.
        if (frame && typeof frame === 'object' && 'reconnected' in frame) {
          void seed()
          return
        }
        const thread = mapThread(frame as ThreadDTO)
        getOrCreateWorkspaceStore(wsId).getState().upsertReviewThread(thread)
      },
    )

    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [wsId])
}
