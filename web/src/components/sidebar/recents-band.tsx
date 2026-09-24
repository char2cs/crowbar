import { useRef } from 'react'
import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { cn } from '@/lib/utils'
import { Separator } from '@/components/ui/separator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { useRecentsChat } from '@/components/sidebar/lib/use-recents-chat'
import { recentsChatIcon } from '@/components/sidebar/lib/recents-for-project'
import { ROW_ACTIVE } from '@/components/layout/workspace-row-base'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import {
  useSidebarDrag,
  type SidebarDrag,
  type SidebarPaneZone,
} from '@/components/sidebar/hooks/use-sidebar-drag'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import type { ChatIconFields } from '@/components/sidebar/lib/rows-from-repo'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { viewMembers } from '@/features/panes/lib/view-state'
import type { ViewMember } from '@/features/panes/types/pane'
import { useSidebarStore } from '@/lib/store/sidebar'

const NO_ICON: Partial<ChatIconFields> = {}
const LOADING_CHAT_LABEL = 'Loading…'

interface RecentsBandProps {
  /** This project's view records, in band order — one row each. */
  viewIds: readonly string[]
  onFocus: (viewId: string) => void
  onClose: (viewId: string) => void
  /** A group member's own ×: closes that one chat, never the group. */
  onCloseChat: (chatId: string) => void
  /** The panel's own scroll container — what an edge-held drag scrolls. */
  scrollRef: React.RefObject<HTMLElement | null>
  onDrop: (subjects: SidebarRowType[], target: SidebarRowType, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRowType[], paneId: string, zone: SidebarPaneZone) => void
}

/**
 * "What is up": one row per view record. Each row subscribes to its own
 * labels (its chats, whether it is showing, each member's working flag), so a
 * streaming delta or a view switch re-renders the rows it touches, not the band.
 *
 * Deliberately NOT `SidebarRow`'s `onTrash` slot for the ×: that slot is the
 * tree's destructive delete (Trash icon, red hover); this × ends a view and
 * never touches the chat.
 */
export function RecentsBand({
  viewIds,
  onFocus,
  onClose,
  onCloseChat,
  scrollRef,
  onDrop,
  onPaneDrop,
}: RecentsBandProps) {
  const rowsRef = useRef(new Map<string, SidebarRowType>())
  const registerRow = (row: SidebarRowType) => {
    rowsRef.current.set(row.id, row)
  }
  const drag = useSidebarDrag({
    scrollRef,
    subjectsFor: (rowId) => {
      const row = rowsRef.current.get(rowId)
      return row ? [row] : []
    },
    onDrop,
    onPaneDrop,
  })

  // After the hook: hooks run every render regardless of the row count.
  if (viewIds.length === 0) return null

  return (
    <div data-testid="recents-band">
      <div className="flex h-[22px] items-center gap-1.5 px-1.5">
        <Separator className="flex-1 bg-border" />
        <span className="shrink-0 font-mono text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          Recents
        </span>
      </div>
      {viewIds.map((viewId) => (
        <RecentsViewRow
          key={viewId}
          viewId={viewId}
          onFocus={onFocus}
          onClose={onClose}
          onCloseChat={onCloseChat}
          drag={drag}
          registerRow={registerRow}
        />
      ))}
      {drag.dragging && <DropIndicator ref={drag.attachDropLine} />}
      {drag.ghostRows && (
        <DragGhost ref={drag.ghostRef} origin={drag.ghostOrigin}>
          <DragGhostRows rows={drag.ghostRows} />
        </DragGhost>
      )}
    </div>
  )
}

function RecentsViewRow({
  viewId,
  onFocus,
  onClose,
  onCloseChat,
  drag,
  registerRow,
}: {
  viewId: string
  onFocus: (viewId: string) => void
  onClose: (viewId: string) => void
  onCloseChat: (chatId: string) => void
  drag: SidebarDrag
  registerRow: (row: SidebarRowType) => void
}) {
  const members = useStore(
    windowPaneStore,
    useShallow((s) => viewMembers(s, viewId).map(memberKey)),
  )
  const isShowing = useStore(windowPaneStore, (s) => s.activeViewId === viewId)
  if (members.length === 0) return null
  // 2+ chats draw a shell around their members; one chat is a bare row.
  const isSet = members.length >= 2
  // A lone showing row IS the active row, pixel-for-pixel the tree's own
  // footprint: this wrapper takes over `SidebarRow`'s own margin (suppressed
  // at the source via `suppressOwnMargin`) so the spacing applies once.
  const soloActive = !isSet && isShowing

  return (
    <div
      data-view-showing={isShowing || undefined}
      className={cn(
        'group relative',
        // The shell is the painted box: `mx-1.5 my-0.5` is its only gutter
        // (horizontal margins never collapse), and `p-0.5 gap-0.5` makes the
        // edge-to-member and member-to-member gaps the same 2px.
        isSet && 'mx-1.5 my-0.5 flex items-center gap-0.5 rounded-lg p-0.5',
        isSet && isShowing && ROW_ACTIVE,
        // One hover surface for the whole group, not per member.
        isSet &&
          !isShowing &&
          'group-hover:bg-sidebar-element-hover group-hover:shadow-xs ' +
            'group-hover:shadow-black/10 group-hover:inset-shadow-[0_1px_var(--elevated-highlight)]',
        // Explicit `h-9`, not auto: dueling margins across nested boxes
        // collapse to max(+) + min(-), not a sum.
        soloActive && cn('mx-1.5 my-0.5 flex h-9 items-center rounded-lg', ROW_ACTIVE),
      )}
      data-testid={isSet ? `recents-set-${viewId}` : undefined}
    >
      {members.map(parseMemberKey).map(({ chatId, workspaceId }) => (
        <RecentsMemberRow
          key={chatId}
          chatId={chatId}
          workspaceId={workspaceId ?? ''}
          // An off-screen view greys its label; the showing one sits on
          // ROW_ACTIVE, which already says "you are here".
          hasView={!isShowing}
          isSet={isSet}
          suppressOwnMargin={soloActive}
          isShowingGround={soloActive}
          activeGround={isShowing}
          onOpen={() => onFocus(viewId)}
          // A member's × leaves the group; a solo row's × ends the view.
          onClose={isSet ? () => onCloseChat(chatId) : () => onClose(viewId)}
          drag={drag}
          registerRow={registerRow}
        />
      ))}
    </div>
  )
}

// A member as one string, so the shallow-compared selector above re-renders
// only when a member actually changes. Ids never contain NUL.
const memberKey = (m: ViewMember): string => `${m.chatId}\x00${m.workspaceId ?? ''}`
const parseMemberKey = (key: string): ViewMember => {
  const [chatId, workspaceId] = key.split('\x00')
  return { chatId, workspaceId: workspaceId || null }
}

function RecentsMemberRow({
  chatId,
  workspaceId,
  hasView,
  isSet,
  suppressOwnMargin,
  isShowingGround,
  activeGround,
  onOpen,
  onClose,
  drag,
  registerRow,
}: {
  chatId: string
  /** Recorded on the view member when the chat was opened (C3). */
  workspaceId: string
  hasView: boolean
  /** One of 2+ chats sharing the shell: shares the row and cancels its own margin. */
  isSet: boolean
  /** The solo showing row's wrapper owns the margin; forwarded to `SidebarRow`. */
  suppressOwnMargin?: boolean
  /** Solo showing row: its own hover is silenced so only the × appears. */
  isShowingGround?: boolean
  /** The body sits on an inverted ROW_ACTIVE ground (solo or set alike). */
  activeGround?: boolean
  onOpen: () => void
  onClose: () => void
  drag: SidebarDrag
  registerRow: (row: SidebarRowType) => void
}) {
  const icon = useSidebarStore(useShallow((s) => recentsChatIcon(s.repos, chatId) ?? NO_ICON))
  // The workspace store when one is mounted (live title, the spinner), else
  // the sidebar's own chat record. A row is its record: with no chat data
  // yet it still draws (as loading), and its × works.
  const known = useRecentsChat(workspaceId, chatId)
  const chat = known ?? { id: chatId, title: '', workspaceId, working: false }
  const pending = !known

  const row: SidebarRowType = {
    id: chat.id,
    parentId: null,
    order: 0,
    // The tree's own fallback: both rows name an untitled chat alike.
    label: chat.title || (pending ? LOADING_CHAT_LABEL : UNTITLED_CHAT_LABEL),
    labelProvisional: !chat.title,
    workspaceId: chat.workspaceId,
    working: chat.working,
    hasView,
    kind: 'chat',
    ownsWorktree: false,
    ...icon,
  }

  return (
    <div
      data-testid={`recents-row-${chat.id}`}
      // A ref callback, not a render-body call: render must stay pure.
      ref={(el) => {
        if (el) registerRow(row)
      }}
      className={cn(
        // Load-bearing inside a flex parent (a set's shell, or the solo
        // showing wrapper): without it the item shrink-wraps and the
        // trailing buttons land beside the label instead of the far edge.
        'min-w-0 flex-1',
        // Flex items never collapse margins: the shell's gap/padding is the
        // only spacing between members.
        isSet && '-mx-1.5 -my-0.5',
        // `[role="treeitem"]` targets exactly SidebarRow's inner div, the one
        // `ROW_INACTIVE`'s hover paints.
        (isShowingGround || isSet) && '[&_[role="treeitem"]]:hover:bg-transparent',
      )}
    >
      <SidebarRow
        row={row}
        depth={0}
        onOpen={onOpen}
        // Lets a hit test tell this row from a tree row with the same id.
        dragProps={drag.dragProps(row, { inRecents: true })}
        isDragging={drag.draggingIds.has(row.id)}
        isNestTarget={drag.nestTargetId === row.id}
        onPointerDownDrag={(e) => drag.onPointerDownDrag(row, e)}
        // The chat also renders as a tree row with the same id: two inline
        // renames at once would steal each other's focus.
        inlineRenameDisabled
        activeGround={activeGround}
        suppressOwnMargin={suppressOwnMargin}
        compactHeight={isSet}
        onClose={onClose}
      />
    </div>
  )
}
