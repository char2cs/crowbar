import { useRef } from 'react'
import { useStore } from 'zustand'
import { ArrowsOutSimpleIcon, DotsSixVerticalIcon, XIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { DragGhost } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import { useSidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import { performSidebarDrop, performSidebarPaneDrop } from '@/components/sidebar/lib/drop-actions'
import { closeRecentChat } from '@/components/sidebar/lib/recents-actions'
import { recentsRowOf } from '@/components/sidebar/lib/recents-row-registry'
import { useMemberRow, ViewTabPreview } from '@/components/sidebar/view-row'
import { viewMembers } from '../lib/view-state'
import { windowPaneStore } from '../stores/window-pane-store'
import type { PaneGroup } from '../types/pane'

/**
 * The tab that hangs from a split pane's top edge: drag the chat out or give it
 * a tab of its own. Shown by CSS alone, only while the pointer is
 * over the pane (the pane carries `group/pane`) — never because it is active.
 * Absent when the pane is the only chat in its view.
 */
export function PaneHandle({ pane }: { pane: PaneGroup }) {
  const inSplit = useStore(
    windowPaneStore,
    (s) => !!pane.viewId && viewMembers(s, pane.viewId).length > 1,
  )
  if (!inSplit || !pane.chatId) return null
  return <PaneHandleBody paneId={pane.id} chatId={pane.chatId} workspaceId={pane.workspaceId} />
}

function PaneHandleBody({
  paneId,
  chatId,
  workspaceId,
}: {
  paneId: string
  chatId: string
  workspaceId: string | null | undefined
}) {
  const row = useMemberRow(chatId, workspaceId ?? '', false)
  const scrollRef = useRef<HTMLElement | null>(null)
  const drag = useSidebarDrag({
    scrollRef,
    // The tab a drop lands beside is a row of another chat: the registry names it.
    subjectsFor: (id) => {
      const other = id === row.id ? row : recentsRowOf(id)
      return other ? [other] : []
    },
    strip: () => document.querySelector<HTMLElement>('[data-view-tab-strip]'),
    onDrop: performSidebarDrop,
    onPaneDrop: performSidebarPaneDrop,
  })

  return (
    <div
      data-testid="pane-handle"
      className="pointer-events-none absolute left-1/2 top-0 z-30 flex -translate-x-1/2 items-center gap-[9px] rounded-b-md bg-primary px-[7.5px] pb-[4px] pt-[5px] text-primary-foreground opacity-0 shadow-[0_0_5px_1px_rgba(0,0,0,0.1)] transition-opacity group-hover/pane:pointer-events-auto group-hover/pane:opacity-100 focus-within:pointer-events-auto focus-within:opacity-100"
    >
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label="Drag this chat"
        tooltip="Drag this chat"
        tooltipSide="bottom"
        onPointerDown={(e) => drag.onPointerDownDrag(row, e)}
        className="size-3! min-h-0! min-w-0! border-0! p-0! cursor-grab touch-none text-primary-foreground hover:bg-transparent hover:text-primary-foreground"
      >
        <DotsSixVerticalIcon size={10} className="rotate-90" />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label="Open in its own tab"
        tooltip="Open in its own tab"
        tooltipSide="bottom"
        className="size-3! min-h-0! min-w-0! border-0! p-0! text-primary-foreground hover:bg-transparent hover:text-primary-foreground"
        onClick={() => windowPaneStore.getState().paneActions.detachPane(paneId)}
      >
        <ArrowsOutSimpleIcon size={10} />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label="Close this chat"
        tooltip="Close this chat"
        tooltipSide="bottom"
        className="size-3! min-h-0! min-w-0! border-0! p-0! text-primary-foreground hover:bg-transparent hover:text-primary-foreground"
        onClick={() => closeRecentChat(chatId)}
      >
        <XIcon size={8} />
      </Button>
      {drag.dragging && <DropIndicator ref={drag.attachDropLine} />}
      {drag.ghostRows && (
        <DragGhost ref={drag.ghostRef} origin={drag.ghostOrigin}>
          <ViewTabPreview row={row} width={drag.ghostRows.width} />
        </DragGhost>
      )}
    </div>
  )
}
