import { useSidebarStore, type Workspace } from '@/lib/store/sidebar'

/**
 * Waiting out a fork re-parent, so a call that depends on it can follow.
 *
 * `POST .../reparent` answers 202 (`hierarchy.go`'s `Reparent` handler): the
 * daemon accepted the move, it has not made it — the rebase runs behind the
 * response in `runAsync`'s detached goroutine, and a REFUSAL from
 * `guardReparent` (a working workspace, fork children of its own, a
 * cross-repo worktree move — all ordinary cases) only ever reaches
 * `Workspace.lastError`, never the HTTP response. A caller that fires its
 * next call the instant the POST resolves either indexes into a level the
 * row has not joined yet, or — worse — silently reorders a row whose
 * lineage change the daemon just refused.
 *
 * Adapted from the deleted `components/layout/reparent-settle.ts`'s
 * `watchReparent` (git show f119a402bdaa790390a0f622ea8510610398dfd^). This
 * plan has no optimistic paint, so unlike the original there is no painted
 * row to distinguish from the real thing — the baseline captured here is
 * simply the workspace's state before the reparent was asked for, and the
 * only way a later frame reports the new `parentId` is a real WS frame.
 */

/** How long the wait gives the rebase before it gives up. Generous, because a
 *  rebase is real work on a real branch — but bounded, so a wedged daemon
 *  leaves a stalled drop rather than a live store subscription for the rest
 *  of the session. */
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
 * Resolves once `wsId` is reported under `parentId` by a genuinely NEW
 * `Workspace` object (a WS frame replacing it, not the baseline read back —
 * see the module doc). Rejects if the workspace disappears, if its
 * `lastError` CHANGES (a fresh refusal, not one it already carried), or if
 * nothing lands inside `timeoutMs`.
 *
 * Call this BEFORE firing the `reparentWorkspace` POST — the baseline has to
 * be taken before the request goes out, or a frame that beats the 202 back
 * (a rebase with nothing to replay) would be missed.
 */
export function watchReparent(
  wsId: string,
  parentId: string,
  timeoutMs = REPARENT_SETTLE_TIMEOUT_MS,
): () => Promise<void> {
  const baseline = findWorkspace(wsId)
  const priorError = baseline?.lastError ?? ''

  const landed = (ws: Workspace | undefined): boolean =>
    ws !== undefined && ws !== baseline && (ws.parentId ?? '') === parentId

  return () =>
    new Promise<void>((resolve, reject) => {
      if (landed(findWorkspace(wsId))) {
        resolve()
        return
      }

      let unsubscribe: (() => void) | null = null
      let timer: ReturnType<typeof setTimeout> | null = null

      const settle = () => {
        if (timer !== null) clearTimeout(timer)
        timer = null
        unsubscribe?.()
        unsubscribe = null
      }

      unsubscribe = useSidebarStore.subscribe(() => {
        const ws = findWorkspace(wsId)
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
