import { useSidebarStore, type Workspace } from '@/lib/store/sidebar'

/**
 * Waiting out a fork re-parent, so the calls that depend on it can follow.
 *
 * `POST .../reparent` answers 202: the daemon accepted the move, it has not
 * made it. The rebase runs behind the response and the workspace is persisted —
 * and broadcast — only once it settles. A drop that both moves the fork edge and
 * names a slot therefore cannot fire its `order` immediately: it would be
 * indexing into a level the row has not arrived in, and the daemon would append
 * it instead. So the drop waits here and orders the row afterwards.
 *
 * The signal is a FRAME about that workspace carrying the new parent, not the
 * value alone: the drop has already painted the new parent optimistically, so
 * reading the field back would answer its own question. A frame is what
 * overwrites that paint with the server's truth (every field of a WorkspaceDTO
 * is applied — see toSidebarWorkspace), and only a frame about this workspace
 * replaces its object, so a new object plus the new parent is the move having
 * genuinely landed.
 */

/**
 * How long the wait gives the rebase before it gives up.
 *
 * Generous, because a rebase is real work on a real branch — but bounded, so a
 * wedged daemon leaves a snapped-back row rather than a dimmed one and a live
 * store subscription for the rest of the session.
 */
export const REPARENT_SETTLE_TIMEOUT_MS = 30_000

export class ReparentTimeoutError extends Error {
  constructor(wsId: string) {
    super(`reparent of ${wsId} did not land`)
    this.name = 'ReparentTimeoutError'
  }
}

export class ReparentFailedError extends Error {
  constructor(wsId: string, reason: string) {
    super(`reparent of ${wsId} failed: ${reason}`)
    this.name = 'ReparentFailedError'
  }
}

function findWorkspace(wsId: string): Workspace | undefined {
  for (const repo of useSidebarStore.getState().repos) {
    const ws = repo.workspaces.find((w) => w.id === wsId)
    if (ws) return ws
  }
  return undefined
}

/**
 * Take a baseline now, and hand back the wait.
 *
 * Split in two because the baseline has to be the OPTIMISTIC row, taken in the
 * same tick the drop painted it and before the request goes out. A frame can
 * beat the 202 back on a rebase with nothing to replay, and a wait that only
 * began afterwards would be waiting for something that had already happened.
 *
 * The returned wait resolves once `wsId` is reported under `parentId`, and
 * rejects if the move reports a failure or nothing lands inside `timeoutMs`.
 * Every exit unsubscribes and clears the timer — a drag must never leave a live
 * store subscription behind, and there are three ways out of this one.
 */
export function watchReparent(
  wsId: string,
  parentId: string,
  timeoutMs = REPARENT_SETTLE_TIMEOUT_MS,
): () => Promise<void> {
  const painted = findWorkspace(wsId)
  // A failure surfaces as LastError on the entity, and the field is sticky: a
  // workspace can already be carrying one from something unrelated. Only a
  // CHANGE means this move failed.
  const priorError = painted?.lastError ?? ''

  /** Has the daemon answered about this row, and does it agree with the drop? */
  const landed = (ws: Workspace | undefined): boolean =>
    ws !== undefined && ws !== painted && (ws.parentId ?? '') === parentId

  return () =>
    new Promise<void>((resolve, reject) => {
      if (landed(findWorkspace(wsId))) {
        resolve()
        return
      }

      let unsubscribe: (() => void) | null = null
      let timer: ReturnType<typeof setTimeout> | null = null

      /** Every exit goes through here, so none of them can leave one behind. */
      const settle = () => {
        if (timer !== null) clearTimeout(timer)
        timer = null
        unsubscribe?.()
        unsubscribe = null
      }

      unsubscribe = useSidebarStore.subscribe(() => {
        const ws = findWorkspace(wsId)
        // The row is gone (deleted under us, or its repo left the visible set):
        // there is nothing left to order, and nothing left to wait for either.
        if (!ws) {
          settle()
          reject(new ReparentFailedError(wsId, 'workspace is gone'))
          return
        }
        if (landed(ws)) {
          settle()
          resolve()
          return
        }
        const lastError = ws.lastError ?? ''
        if (lastError !== '' && lastError !== priorError) {
          settle()
          reject(new ReparentFailedError(wsId, lastError))
        }
      })

      timer = setTimeout(() => {
        settle()
        reject(new ReparentTimeoutError(wsId))
      }, timeoutMs)
    })
}
