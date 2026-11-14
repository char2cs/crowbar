import { useCallback, useEffect, useRef } from 'react'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { treeUiStore, useTreeUi, type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import { commitRemoval, flushDrainingRemovals, type RemovalNavigate } from './removal-commit'
import { startRemovalSecondsClock } from './removal-clock'
import { RemovalConfirmDialog } from './removal-confirm-dialog'

// The clock behind every removal: one timer commits the soonest drain, a seconds clock keeps
// in-row figures honest, pagehide flushes the rest; it draws only the repo/project confirm. Mount
// it unconditionally — held rows are drawn by the tree, so without it nothing ever commits.
export function RemovalTray() {
  const entries = useTreeUi((s) => s.removals)
  // A repo/project entry never drains and is the only kind awaiting an answer; ask about the
  // first, and resolving it surfaces any queued behind.
  const pending = entries.find((e) => e.deadlineAt === null) ?? null

  const navigate = useNavigate()
  const router = useRouter()

  // On a ref so the scheduling effect depends on the entry list alone, not a per-render router.
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

  // The page can end mid-drain. `pagehide`, not `beforeunload`: it also fires when the page enters
  // the back/forward cache. Bound once; the flush reads the store itself.
  useEffect(() => {
    const flush = () => flushDrainingRemovals()
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [])

  // ONE timer, aimed at the soonest deadline; each commit shortens the list and re-aims it. Rows
  // animate their own drain, and wall-clock `deadlineAt` makes a rebuilt timer resume, not restart.
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

  // The seconds numerals count on a clock that belongs to the tray, not to any row.
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
