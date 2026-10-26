import { useEffect, useMemo, useState } from 'react'
import { startRemovalSecondsClock } from '@/components/layout/removal-clock'
import { SidebarProjectSwitcher } from '@/components/layout/sidebar-project-switcher'
import { sidebarRowTree } from '@/components/sidebar/lib/row-tree'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { RowMetricsContext, TOUCH_ROW_METRICS } from '@/components/sidebar/lib/row-metrics'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { HostViewsContext } from '@/features/hosts/hooks/use-host-status'
import type { NativeSidebar } from '@/lib/native-chrome'
import { viewersLabel } from '@/lib/store/active-sessions'
import { postToShell, receiveShellSnapshots } from '@/lib/shell-page-bridge'
import { useSurfaceThemeColor } from '@/features/window/hooks/use-surface-theme-color'
import { DraggableRow, RowDragScope } from './sidebar-row-drag'

const selectRow = (id: string) => postToShell('sidebar', { type: 'selectRow', id })
const toggleFold = (id: string) => postToShell('sidebar', { type: 'toggleFold', id })
const selectProject = (id: string) => postToShell('sidebar', { type: 'selectProject', id })
const keepRemoval = (entryId: string) => postToShell('sidebar', { type: 'keepRemoval', entryId })
const openSettings = () => postToShell('sidebar', { type: 'openSettings' })

// On touch start, not after a delay: Swift's own long-press then has the row and its menu ready.
function pressRow(e: React.PointerEvent, actions: NativeSidebar['actions']) {
  if (!(e.target instanceof Element)) return
  const el = e.target.closest<HTMLElement>('[role="treeitem"]')
  const rowId = el?.getAttribute('data-sidebar-row-id')
  if (!el || !rowId) return
  const { x, y, width, height } = el.getBoundingClientRect()
  postToShell('sidebar', {
    type: 'rowPress',
    rowId,
    rect: { x, y, width, height },
    actions: actions[rowId] ?? [],
  })
}

/**
 * The phone's chat list: the desktop sidebar's switcher and rows, drawn from the snapshot the
 * main webview pushes. It owns no state and opens no connection; every pick goes back as an
 * intent. Gear and New thread are native buttons floating over its foot.
 */
export function MobileSidebarPage() {
  useSurfaceThemeColor()
  const [snapshot, setSnapshot] = useState<NativeSidebar | null>(null)
  useEffect(() => receiveShellSnapshots<NativeSidebar>('sidebar', setSnapshot), [])
  // This page is its own document, out of reach of the shell's clock for the held rows' seconds.
  useEffect(() => {
    const deadlines = new Map<string, number>()
    for (const row of snapshot?.rows ?? []) {
      if (row.removal?.deadlineAt != null)
        deadlines.set(row.removal.entryId, row.removal.deadlineAt)
    }
    return startRemovalSecondsClock(deadlines)
  }, [snapshot?.rows])
  const hostViews = useMemo(
    () => new Map(snapshot?.hosts.map((h) => [h.id, { name: h.name, state: h.state }])),
    [snapshot?.hosts],
  )

  return (
    <div className="flex h-dvh flex-col overflow-hidden bg-chrome-bg pt-[env(safe-area-inset-top)] text-foreground">
      {snapshot && (
        <HostViewsContext value={hostViews}>
          <RowMetricsContext value={TOUCH_ROW_METRICS}>
            <SidebarProjectSwitcher
              projects={snapshot.projects}
              activeProjectId={snapshot.activeProjectId ?? undefined}
              onSelectProject={selectProject}
              onOpenHostSettings={openSettings}
            />
          </RowMetricsContext>
          {/* No callout or text selection, so a long press is left to the native menu. */}
          <nav
            aria-label="Chats"
            className="min-h-0 flex-1 overflow-y-auto pb-[calc(env(safe-area-inset-bottom)+84px)] select-none [-webkit-touch-callout:none]"
            onPointerDown={(e) => pressRow(e, snapshot.actions)}
          >
            <RowMetricsContext value={TOUCH_ROW_METRICS}>
              <ActiveSection rows={snapshot.rows} active={snapshot.active} />
              <RowDragScope
                drop={snapshot.drop}
                overlay={(id) => <DragOverlayRow rows={snapshot.rows} id={id} />}
              >
                <SnapshotTree rows={snapshot.rows} collapsed={snapshot.collapsed} />
              </RowDragScope>
            </RowMetricsContext>
          </nav>
        </HostViewsContext>
      )}
    </div>
  )
}

/** The agents running now, above the tree: the ordinary rows of chats already in it. */
function ActiveSection({ rows, active }: Pick<NativeSidebar, 'rows' | 'active'>) {
  const byId = new Map(rows.map((r) => [r.id, r]))
  const running = active.flatMap(({ id, attached }) => {
    const row = byId.get(id)
    return row ? [{ row, attached }] : []
  })
  if (running.length === 0) return null
  return (
    <section aria-label="Active" className="pb-1">
      <h2 className="ui-font ui-text-sm px-3 py-1 text-muted-foreground">Active</h2>
      {running.map(({ row, attached }) => (
        <div key={row.id}>
          <SidebarRow row={row} depth={0} onOpen={selectRow} inlineRenameDisabled hasViewIdle />
          <p className="ui-font ui-text-sm truncate pr-3 pl-9 text-muted-foreground">
            {viewersLabel(attached)}
          </p>
        </div>
      ))}
    </section>
  )
}

/** The desktop tree's walk (sidebar-tree.tsx) over rows whose live facts arrive resolved. */
function SnapshotTree({ rows, collapsed }: { rows: SidebarRowType[]; collapsed: string[] }) {
  const { roots, childrenByParent } = sidebarRowTree(rows)
  const folded = new Set(collapsed)
  const renderRow = (row: SidebarRowType, depth: number) => {
    const children = childrenByParent.get(row.id) ?? []
    const isFolded = folded.has(row.id)
    return (
      <div key={row.id}>
        <DraggableRow id={row.id} disabled={!!row.removal}>
          <SidebarRow
            row={row}
            depth={depth}
            onOpen={selectRow}
            onKeepRemoval={keepRemoval}
            onToggleFold={children.length > 0 ? toggleFold : undefined}
            folded={isFolded}
            inlineRenameDisabled
            hasViewIdle
          />
        </DraggableRow>
        {!isFolded && children.map((child) => renderRow(child, depth + 1))}
      </div>
    )
  }
  return <>{roots.map((row) => renderRow(row, 0))}</>
}

/** The lifted copy of the row being dragged. */
function DragOverlayRow({ rows, id }: { rows: SidebarRowType[]; id: string }) {
  const row = rows.find((r) => r.id === id)
  return row ? (
    <div aria-hidden>
      <SidebarRow row={row} depth={0} onOpen={selectRow} inlineRenameDisabled hasViewIdle />
    </div>
  ) : null
}
