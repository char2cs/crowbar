import { useCallback, useEffect, useRef } from 'react'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { useRemovalTrayStore, type RemovalEntry } from '@/lib/store/sidebar-removal'
import { commitRemoval, flushDrainingRemovals, type RemovalNavigate } from './removal-commit'
import { startRemovalSecondsClock } from './removal-clock'
import { RemovalConfirmDialog } from './removal-confirm-dialog'

/**
 * The background service behind every removal, plus the one piece of chrome
 * it still draws: the confirmation for a repo or a project.
 *
 * Every OTHER held kind (workspace, folder, chat) drains in place, drawn by
 * the row itself (sidebar-row.tsx's `RemovingSidebarRow`) — this component
 * never renders a copy of them. It only runs their shared clock: the
 * soonest-deadline timer that commits a row once its hairline empties, the
 * once-a-second DOM write that keeps every drain's figures honest, and the
 * pagehide flush that sends a removal still draining when the page ends.
 *
 * A repo or a project never drains (sidebar-removal.ts's own note: eight
 * seconds of undo is not a proportionate safety net for either), so the
 * moment one is held, this asks instead — `RemovalConfirmDialog`, straight
 * off the hold, nothing held in between. There used to be a step before the
 * dialog: a row in a tray docked at the sidebar's foot, offering its own
 * Cancel/Remove before the dialog ever opened. That dock is gone; holding a
 * repo or a project now goes straight to the one question that matters.
 *
 * MOUNT IT UNCONDITIONALLY (ide-shell.tsx). Every held row of every kind is
 * drawn by the TREE, so an unmounted tray looks entirely healthy: the row
 * transforms, the hairline drains, and the commit simply never happens.
 */
export function RemovalTray() {
  const entries = useRemovalTrayStore((s) => s.entries)
  // A repo/project entry never drains (`deadlineAt === null` — hold()'s own
  // rule), and it is the only kind that ever needs an answer before
  // anything commits — so the first one in the list is exactly what to ask
  // about. Cancelling or confirming removes it from `entries`, surfacing
  // whichever (rare) second one was queued behind it.
  const pending = entries.find((e) => e.deadlineAt === null) ?? null

  const navigate = useNavigate()
  const router = useRouter()

  // Kept on a ref so the scheduling effect below is driven by the entry list and
  // nothing else — a router that hands out a fresh function every render would
  // otherwise rebuild every clock on every navigation.
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

  // The page can end mid-drain, and the intent must not end with it.
  //
  // `pagehide` rather than `beforeunload`: it fires on a reload, a navigation and
  // an app quit alike, and unlike beforeunload it is not skipped when the
  // document goes into the back/forward cache. Registered once for the tray's
  // life — the flush reads the store itself, so it never needs re-binding as
  // entries come and go.
  useEffect(() => {
    const flush = () => flushDrainingRemovals()
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [])

  // ONE timer for the whole tray, aimed at whichever deadline comes first.
  //
  // Every held row already draws its own eight seconds as a CSS animation, so
  // nothing here needs to tick — the timer's only job is to fire once, at the
  // instant the first hairline runs out. Committing that row shortens the list,
  // which re-runs this effect and re-aims at the next one, so N rows still cost
  // exactly one live timer.
  //
  // `deadlineAt` is a wall-clock instant rather than a duration, which is what
  // makes re-aiming free: a timer rebuilt halfway through resumes where the
  // drain already is instead of starting another eight seconds.
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
        for (const entry of useRemovalTrayStore.getState().entries) {
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
        if (pending) useRemovalTrayStore.getState().cancel(pending.entryId)
      }}
      onConfirm={(entry) => commit(entry)}
    />
  )
}
