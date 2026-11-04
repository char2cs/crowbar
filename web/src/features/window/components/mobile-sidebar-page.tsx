import { useEffect, useMemo, useState } from 'react'
import { SidebarProjectSwitcher } from '@/components/layout/sidebar-project-switcher'
import { sidebarRowTree } from '@/components/sidebar/lib/row-tree'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { RowMetricsContext, TOUCH_ROW_METRICS } from '@/components/sidebar/lib/row-metrics'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { HostViewsContext } from '@/features/hosts/hooks/use-host-status'
import type { NativeSidebar } from '@/lib/native-chrome'
import { cn } from '@/lib/utils'
import { NativeReadoutView } from '@/components/layout/build-readout'
import { postToShell, receiveShellSnapshots } from '@/lib/shell-page-bridge'
import { useSurfaceThemeColor } from '@/features/window/hooks/use-surface-theme-color'
import { DraggableRow, RowDragScope } from './sidebar-row-drag'

// Gear and New thread float over the foot of the page.
const CLEAR_FLOATING_BUTTONS_PB = 'pb-[calc(env(safe-area-inset-bottom)+84px)]'

const selectRow = (id: string) => postToShell('sidebar', { type: 'selectRow', id })
const toggleFold = (id: string) => postToShell('sidebar', { type: 'toggleFold', id })
const selectProject = (id: string) => postToShell('sidebar', { type: 'selectProject', id })
const openSettings = () => postToShell('sidebar', { type: 'openSettings' })

// The page owns the hold; the shell only presents the menu for the row it is told of.
function showRowMenu(rowId: string, node: HTMLElement, actions: NativeSidebar['actions']) {
  const { x, y, width, height } = node.getBoundingClientRect()
  postToShell('sidebar', {
    type: 'rowMenu',
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
  const hostViews = useMemo(
    () => new Map(snapshot?.hosts.map((h) => [h.id, { name: h.name, state: h.state }])),
    [snapshot?.hosts],
  )

  const running = useMemo(() => {
    const byId = new Map(snapshot?.rows.map((r) => [r.id, r]))
    return snapshot?.active.flatMap(({ id }) => byId.get(id) ?? []) ?? []
  }, [snapshot?.rows, snapshot?.active])

  return (
    <div className="relative flex h-dvh flex-col overflow-hidden bg-chrome-bg pt-[env(safe-area-inset-top)] text-foreground">
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
          <RowMetricsContext value={TOUCH_ROW_METRICS}>
            <RowDragScope
              drop={snapshot.drop}
              overlay={(id) => <DragOverlayRow rows={snapshot.rows} id={id} />}
              onMenu={(id, node) => showRowMenu(id, node, snapshot.actions)}
            >
              {/* No callout or text selection, so a long press is the page's own gesture. */}
              <nav
                aria-label="Chats"
                className={cn(
                  'min-h-0 flex-1 overflow-y-auto select-none [-webkit-touch-callout:none]',
                  CLEAR_FLOATING_BUTTONS_PB,
                  running.length > 0 && 'pb-0',
                )}
              >
                <SnapshotTree rows={snapshot.rows} collapsed={snapshot.collapsed} />
              </nav>
              <ActiveSection running={running} />
            </RowDragScope>
          </RowMetricsContext>
        </HostViewsContext>
      )}
      {snapshot?.readout && (
        <NativeReadoutView
          readout={snapshot.readout}
          className="absolute right-4 bottom-1 items-end text-right"
        />
      )}
    </div>
  )
}

/** The agents running now, pinned above the floating buttons: the ordinary rows of chats already in it. */
function ActiveSection({ running }: { running: SidebarRowType[] }) {
  if (running.length === 0) return null
  return (
    <section
      aria-label="Active"
      className={cn(
        'flex max-h-[40%] shrink-0 flex-col select-none [-webkit-touch-callout:none]',
        CLEAR_FLOATING_BUTTONS_PB,
      )}
    >
      <div
        role="separator"
        className="pointer-events-none my-1 flex shrink-0 items-center gap-2 px-3 text-[10px] font-medium uppercase tracking-wider text-muted-foreground"
      >
        <span className="wavy-rule" />
        <span className="tabular-nums">Active · {running.length}</span>
        <span className="wavy-rule" />
      </div>
      <div className="min-h-0 overflow-y-auto">
        {running.map((row) => (
          <DraggableRow key={row.id} id={row.id} menuOnly>
            <SidebarRow row={row} depth={0} onOpen={selectRow} inlineRenameDisabled hasViewIdle />
          </DraggableRow>
        ))}
      </div>
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
        <DraggableRow id={row.id}>
          <SidebarRow
            row={row}
            depth={depth}
            onOpen={selectRow}
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
