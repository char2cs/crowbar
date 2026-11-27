import { useCallback, useEffect, useRef } from 'react'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { treeUiStore, useTreeUi, type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import { commitRemoval, flushDrainingRemovals, type RemovalNavigate } from './removal-commit'
import { startRemovalSecondsClock } from './removal-clock'
import { RemovalConfirmDialog } from './removal-confirm-dialog'

export function RemovalTray() {
  const entries = useTreeUi((s) => s.removals)
  const pending = entries.find((e) => e.deadlineAt === null) ?? null

  const navigate = useNavigate()
  const router = useRouter()

  const commitRef = useRef<(entry: RemovalEntry) => void>(() => {})
  commitRef.current = (entry) => {
    const go: RemovalNavigate = (target) => {
      if (!target) void navigate({ to: '/' })
      else void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: target })
    }
    void commitRemoval(entry, {
      activeWorkspaceId:
        router.state.location.pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1] ?? '',
      navigate: go,
    })
  }

  const commit = useCallback((entry: RemovalEntry) => commitRef.current(entry), [])

  useEffect(() => {
    const flush = () => flushDrainingRemovals()
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [])

  useEffect(() => {
    let soonest: number | null = null
    for (const entry of entries) {
      if (entry.deadlineAt === null) continue
      if (soonest === null || entry.deadlineAt < soonest) soonest = entry.deadlineAt
    }
    if (soonest === null) return
    const timer = setTimeout(
      () => {
        const now = Date.now()
        for (const entry of treeUiStore.getState().removals) {
          if (entry.deadlineAt !== null && entry.deadlineAt <= now) commit(entry)
        }
      },
      Math.max(0, soonest - Date.now()),
    )
    return () => clearTimeout(timer)
  }, [entries, commit])

  useEffect(() => {
    const deadlines = new Map<string, number>()
    for (const entry of entries) {
      if (entry.deadlineAt !== null) deadlines.set(entry.entryId, entry.deadlineAt)
    }
    return startRemovalSecondsClock(deadlines)
  }, [entries])

  return (
    <RemovalConfirmDialog
      entry={pending}
      onCancel={() => {
        if (pending) treeUiStore.getState().cancel(pending.entryId)
      }}
      onConfirm={(entry) => commit(entry)}
    />
  )
}
