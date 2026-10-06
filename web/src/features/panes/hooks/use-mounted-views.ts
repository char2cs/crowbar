import { startTransition, useEffect, useState } from 'react'
import { useStore } from 'zustand'
import { scheduleIdleTask } from '@/features/editor/lib/idle-task'
import { pendingChatSurfaces } from '@/features/panes/stores/pending-chat-surfaces'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LayoutNode } from '@/features/panes/types/pane'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'

interface ViewSlot {
  id: string
  showing: boolean
  layout: LayoutNode
}

const NONE: ReadonlySet<string> = new Set()

/** How recently any pane of the view was active: lower is more recent. */
function recencyRank(view: ViewSlot, recentPaneIds: readonly string[]): number {
  const leaves = new Set(getAllLeafIds(view.layout))
  const rank = recentPaneIds.findIndex((id) => leaves.has(id))
  return rank === -1 ? recentPaneIds.length : rank
}

/**
 * The view ids that should be mounted: the showing one at once, every parked
 * one a slot later — most recently used first, one per idle task, rendered in a
 * transition so the showing view's own updates preempt it, and not before the
 * showing chat surfaces have committed (the main thread is idle while their
 * chunk loads, which an idle task alone cannot tell apart from done). Once mounted a view
 * stays mounted: parking is not a teardown.
 */
export function useMountedViews<T extends ViewSlot>(views: readonly T[]): T[] {
  const [mounted, setMounted] = useState<ReadonlySet<string>>(NONE)
  const surfacesPending = useStore(pendingChatSurfaces, (state) => state.pending.size > 0)

  useEffect(() => {
    const showing = views.filter((v) => v.showing && !mounted.has(v.id))
    if (showing.length > 0) {
      setMounted((prev) => new Set([...prev, ...showing.map((v) => v.id)]))
      return
    }
    if (surfacesPending) return
    const recent = windowPaneStore.getState().mostRecentActivePaneIds
    const pending = views
      .filter((v) => !mounted.has(v.id))
      .sort((a, b) => recencyRank(a, recent) - recencyRank(b, recent))
    const next = pending[0]
    if (!next) return
    const task = scheduleIdleTask(() =>
      startTransition(() => setMounted((prev) => new Set(prev).add(next.id))),
    )
    return task.cancel
  }, [views, mounted, surfacesPending])

  return views.filter((v) => v.showing || mounted.has(v.id))
}
