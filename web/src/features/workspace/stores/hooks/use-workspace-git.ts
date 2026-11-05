import { useEffect } from 'react'
import deepEqual from 'fast-deep-equal'
import { useStore } from 'zustand'
import {
  markGitStatusChanged,
  onGitRefreshRequested,
  onGitStatusChanged,
} from '@/features/git/stores/git-refresh'
import { holdScope, onSignal } from '@/core/stream/stream'
import { report } from '@/lib/errors/report'
import { useResolvedHost } from '@/lib/host-resolve'
import { useWorkspaceStore } from '../workspace-context'

const GIT_REFRESH_DEBOUNCE_MS = 400

// The stream repeats identical git_status frames faster than the debounce, so only a changed
// frame reloads. `prev` is null before a session's first frame and never equal, so that one does.
function framesEqual(prev: unknown, next: unknown): boolean {
  return prev !== null && deepEqual(prev, next)
}

/** Keeps `wsId`'s git slice live: loaded when first shown, then kept current by its git: scope
 *  while retained. Not for home. Git routes are chat-scoped, so it waits on `owningChatId` (null
 *  until the sidebar records it) and a known host. */
export function useWorkspaceGit(
  wsId: string,
  active: boolean,
  owningChatId: string | null,
  homeWorkspace: boolean,
) {
  const store = useWorkspaceStore()
  const loaded = useStore(store, (s) => s.gitLoad === 'ready')
  const hostId = useResolvedHost({ wsId })
  const canAddress = !homeWorkspace && owningChatId !== null && hostId !== null

  useEffect(() => {
    if (!active || !canAddress) return
    void store.getState().gitActions.loadGitData()
  }, [store, active, canAddress])

  useEffect(() => {
    if (!canAddress || !hostId || !loaded) return

    // Coalescing (non-resetting) timer. The backend can stream git frames more
    // often than the debounce window; a resetting debounce would starve forever
    // under that load, so a reload fires within the window of the FIRST trigger.
    let timer: ReturnType<typeof setTimeout> | null = null
    let cancelled = false
    const scheduleStatusReload = () => {
      if (timer) return
      timer = setTimeout(() => {
        timer = null
        if (cancelled) return
        // Status + commit log together (a terminal-side commit changes History
        // with no UI action), then tell open diff views to refetch.
        void store
          .getState()
          .gitActions.reloadStatusAndLog()
          .then(() => {
            if (!cancelled) markGitStatusChanged(wsId)
          })
          .catch((err: unknown) => report(err, { action: 'Refresh git status' }))
      }, GIT_REFRESH_DEBOUNCE_MS)
    }
    // The first push repeats the status the load returned; seeding from it
    // keeps that frame (and any identical repeat) from reloading.
    let lastFrame: unknown = store.getState().gitStatus
    const scope = `git:${wsId}`
    const stopSignals = onSignal(hostId, scope, 'git_status', (frame) => {
      if (framesEqual(lastFrame, frame)) return
      lastFrame = frame
      scheduleStatusReload()
    })
    const release = holdScope(hostId, scope)
    // Local actions that know THIS workspace's status is stale (an editor save,
    // push/pull) request a refresh without waiting for the backend watcher.
    const stopRefreshRequests = onGitRefreshRequested(wsId, scheduleStatusReload)
    // The changed-files summary follows every git change, including ones the
    // daemon reports without a status frame for this stream.
    const stopChanged = onGitStatusChanged(
      wsId,
      () => void store.getState().gitActions.reloadReviewFiles(),
    )
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      stopRefreshRequests()
      stopChanged()
      release()
      stopSignals()
    }
  }, [store, wsId, canAddress, hostId, loaded])
}
