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

function framesEqual(prev: unknown, next: unknown): boolean {
  return prev !== null && deepEqual(prev, next)
}

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

    let timer: ReturnType<typeof setTimeout> | null = null
    let cancelled = false
    const scheduleStatusReload = () => {
      if (timer) return
      timer = setTimeout(() => {
        timer = null
        if (cancelled) return
        void store
          .getState()
          .gitActions.reloadStatusAndLog()
          .then(() => {
            if (!cancelled) markGitStatusChanged(wsId)
          })
          .catch((err: unknown) => report(err, { action: 'Refresh git status' }))
      }, GIT_REFRESH_DEBOUNCE_MS)
    }
    let lastFrame: unknown = store.getState().gitStatus
    const scope = `git:${wsId}`
    const stopSignals = onSignal(hostId, scope, 'git_status', (frame) => {
      if (framesEqual(lastFrame, frame)) return
      lastFrame = frame
      scheduleStatusReload()
    })
    const release = holdScope(hostId, scope)
    const files = `files:${wsId}`
    const stopFileChanges = onSignal(hostId, files, 'file_change', scheduleStatusReload)
    const releaseFiles = holdScope(hostId, files)
    const stopRefreshRequests = onGitRefreshRequested(wsId, scheduleStatusReload)
    const stopChanged = onGitStatusChanged(
      wsId,
      () => void store.getState().gitActions.reloadReviewFiles(),
    )
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      stopRefreshRequests()
      stopChanged()
      releaseFiles()
      stopFileChanges()
      release()
      stopSignals()
    }
  }, [store, wsId, canAddress, hostId, loaded])
}
