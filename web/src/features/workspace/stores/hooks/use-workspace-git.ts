import { useEffect } from 'react'
import deepEqual from 'fast-deep-equal'
import { useStore } from 'zustand'
import {
  markGitStatusChanged,
  onGitRefreshRequested,
  onGitStatusChanged,
} from '@/features/git/stores/git-refresh'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'
import { wsManager } from '@/lib/ws/manager'
import { useWorkspaceStore } from '../workspace-context'

const GIT_REFRESH_DEBOUNCE_MS = 400

// The push stream repeats identical git/status frames far faster than the
// reload debounce, so only a frame that actually differs from the previous
// one should retrigger a reload. `prev` is `null` before the first frame of
// a session arrives — null must never compare equal to an incoming frame, or
// that first frame would silently fail to trigger a reload.
function framesEqual(prev: unknown, next: unknown): boolean {
  return prev !== null && deepEqual(prev, next)
}

/**
 * Keeps `wsId`'s own git slice loaded and live. Loaded the first time the
 * workspace is shown, then kept current by the git/status stream while the
 * workspace is retained — so a switch never reloads it. Not for the home
 * workspace (no git surface). `owningChatId` is null until the sidebar records
 * it; the git routes are chat-scoped and throw without one.
 */
export function useWorkspaceGit(
  wsId: string,
  active: boolean,
  owningChatId: string | null,
  homeWorkspace: boolean,
) {
  const store = useWorkspaceStore()
  const loaded = useStore(store, (s) => s.gitLoad === 'ready')
  const canAddress = !homeWorkspace && owningChatId !== null

  useEffect(() => {
    if (!active || !canAddress) return
    void store.getState().gitActions.loadGitData()
  }, [store, active, canAddress])

  useEffect(() => {
    if (!canAddress || !loaded) return

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
          .catch(() => {})
      }, GIT_REFRESH_DEBOUNCE_MS)
    }
    // The first push repeats the status the load returned; seeding from it
    // keeps that frame (and any identical repeat) from reloading.
    let lastFrame: unknown = store.getState().gitStatus
    const unsubscribe = wsManager.subscribe(`${gitBaseForWorkspace(wsId)}/status`, (frame) => {
      if (framesEqual(lastFrame, frame)) return
      lastFrame = frame
      scheduleStatusReload()
    })
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
      unsubscribe()
    }
  }, [store, wsId, canAddress, loaded])
}
