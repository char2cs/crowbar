import { useEffect } from 'react'
import { useHostResolved } from '@/lib/host-resolve'
import { subscribeGuarded } from '@/lib/ws/subscribe-guarded'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { useWorkspaceScopeReady } from '@/lib/workspace-scope'
import { listThreads, mapThread } from '@/features/git/api/review-api'
import type { ThreadDTO } from '@/features/git/api/review-api'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

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
  const scopeReady = useWorkspaceScopeReady(wsId)
  const hostResolved = useHostResolved({ wsId })
  useEffect(() => {
    // workspaceBase(wsId) throws without a recorded scope or a resolved host — see
    // useWorkspaceScopeReady above. Wait rather than crash; this re-runs the
    // instant either is known (both are dependencies below).
    if (!scopeReady || !hostResolved) return
    const store = getWorkspaceStore(wsId)
    if (!store) return
    let cancelled = false

    const seed = async () => {
      try {
        const threads = await listThreads(wsId)
        if (cancelled) return
        const { upsertReviewThread } = store.getState()
        for (const t of threads) {
          upsertReviewThread(t)
        }
      } catch {
        // seed failure is non-fatal; the WS stream will still push updates
      }
    }

    void seed()

    const onFrame = (frame: unknown) => {
      if (cancelled) return
      // Reconnect sentinel emitted by the manager after a socket drop+reopen.
      if (frame && typeof frame === 'object' && 'reconnected' in frame) {
        void seed()
        return
      }
      const dto = frame as ThreadDTO
      // Tombstone frame: the thread was deleted — drop it from the store.
      if (dto.deleted && dto.id) {
        store.getState().removeReviewThread(dto.id)
        return
      }
      store.getState().upsertReviewThread(mapThread(dto))
    }
    const unsubscribe = subscribeGuarded(() => `${workspaceBase(wsId)}/threads`, onFrame)

    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [wsId, scopeReady, hostResolved])
}
