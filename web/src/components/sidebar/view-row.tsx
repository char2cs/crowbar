import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { XIcon } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { useRecentsChat } from '@/components/sidebar/lib/use-recents-chat'
import { recentsChatIcon } from '@/components/sidebar/lib/recents-for-project'
import type { SidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import type { ChatIconFields } from '@/components/sidebar/lib/rows-from-repo'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { viewMembers } from '@/features/panes/lib/view-state'
import type { ViewMember } from '@/features/panes/types/pane'
import { useSidebarStore } from '@/lib/store/sidebar'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'

const NO_ICON: Partial<ChatIconFields> = {}
const LOADING_CHAT_LABEL = 'Loading…'

/**
 * A chat as the sidebar row model: the workspace store's live title and
 * spinner when one is mounted, else the sidebar's own chat record. A row is
 * its record: with no chat data yet it still draws (as loading).
 */
export function useMemberRow(
  chatId: string,
  workspaceId: string,
  hasView: boolean,
): SidebarRowType {
  const icon = useSidebarStore(useShallow((s) => recentsChatIcon(s.repos, chatId) ?? NO_ICON))
  const known = useRecentsChat(workspaceId, chatId)
  const chat = known ?? { id: chatId, title: '', workspaceId, working: false }
  return {
    id: chat.id,
    parentId: null,
    order: 0,
    // The tree's own fallback: both rows name an untitled chat alike.
    // An unsent new thread has no chat to wait for: it is untitled, as it will be once sent.
    label:
      chat.title || (known || isDraftChatId(chatId) ? UNTITLED_CHAT_LABEL : LOADING_CHAT_LABEL),
    labelProvisional: !chat.title,
    workspaceId: chat.workspaceId,
    working: chat.working,
    hasView,
    kind: 'chat',
    ownsWorktree: false,
    ...icon,
  }
}

/**
 * One view as a tab: a bare row for a lone chat, or one shell around the
 * members of a split. Each tab subscribes to its own labels, so a streaming
 * delta or a view switch re-renders the tabs it touches only.
 *
 * The single × ends the whole view. It is not `SidebarRow`'s `onTrash` slot,
 * which is the tree's destructive delete.
 */
export function RecentsViewRow({
  viewId,
  onFocus,
  onClose,
  drag,
  registerRow,
}: {
  viewId: string
  onFocus: (viewId: string) => void
  onClose: (viewId: string) => void
  drag: SidebarDrag
  registerRow: (row: SidebarRowType) => void
}) {
  const members = useStore(
    windowPaneStore,
    useShallow((s) => viewMembers(s, viewId).map(memberKey)),
  )
  const isShowing = useStore(windowPaneStore, (s) => s.activeViewId === viewId)
  const pinned = useStore(windowPaneStore, (s) => !!s.views[viewId]?.pinned)
  if (members.length === 0) return null
  const isSet = members.length >= 2

  return (
    <div
      data-view-showing={isShowing || undefined}
      data-testid={isSet ? `recents-set-${viewId}` : undefined}
      className={cn(
        // One painted shell; members share it flush. The × is always shown and
        // overlays the shell's end, so no label ever moves.
        TAB_SHELL,
        isShowing ? TAB_ACTIVE : TAB_IDLE,
      )}
    >
      {members.map((member, i) => {
        const { chatId, workspaceId } = parseMemberKey(member)
        const isLast = i === members.length - 1
        return (
          <RecentsMemberRow
            key={chatId}
            chatId={chatId}
            workspaceId={workspaceId ?? ''}
            hasView={!isShowing}
            isSet={isSet}
            pinned={pinned}
            fadesForClose={isLast && !pinned}
            separated={!isLast && !isShowing}
            onOpen={() => onFocus(viewId)}
            drag={drag}
            registerRow={registerRow}
          />
        )
      })}
      {!pinned && (
        <button
          type="button"
          aria-label="Close view"
          onClick={() => onClose(viewId)}
          className={cn(TAB_CLOSE, 'hover:bg-sidebar-element-hover hover:text-foreground')}
        >
          <XIcon size={12} />
        </button>
      )}
    </div>
  )
}

const TAB_SHELL = 'group/tab relative mx-0.5 flex h-8 items-center rounded-full border'
const TAB_CLOSE =
  'absolute right-1.5 top-1/2 z-10 grid size-6 -translate-y-1/2 place-items-center rounded-full text-muted-foreground'

/**
 * A chat as the view tab it would become, lifted: the same shell, row and close
 * button a real single-chat tab renders, for a pane being dragged toward the strip.
 */
export function ViewTabPreview({ row, width }: { row: SidebarRowType; width: number }) {
  return (
    <div className={cn(TAB_SHELL, TAB_ACTIVE, 'shadow-md shadow-black/15')} style={{ width }}>
      <div className="relative min-w-0 flex-1 [mask-image:linear-gradient(to_right,black_calc(100%_-_34px),transparent_calc(100%_-_24px))]">
        <SidebarRow row={row} depth={0} onOpen={noop} tab="tab" />
      </div>
      <span aria-hidden="true" className={TAB_CLOSE}>
        <XIcon size={12} />
      </span>
    </div>
  )
}

const noop = () => {}

// The CossUI raised surface, on our plain background/foreground pair.
const TAB_ACTIVE =
  'border-transparent bg-background text-foreground outline outline-1 outline-background shadow-xs shadow-black/10 ' +
  'inset-shadow-[0_1px_var(--elevated-highlight)] active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'
const TAB_IDLE =
  'border-transparent hover:bg-sidebar-element-hover hover:shadow-xs hover:shadow-black/10 ' +
  'hover:inset-shadow-[0_1px_var(--elevated-highlight)]'

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
  pinned,
  fadesForClose,
  separated,
  onOpen,
  drag,
  registerRow,
}: {
  chatId: string
  /** Recorded on the view member when the chat was opened. */
  workspaceId: string
  hasView: boolean
  /** One of 2+ chats sharing the shell: no icon. */
  isSet: boolean
  /** The tab is pinned: every member shows its glyph alone. */
  pinned: boolean
  /** The tab's trailing member: its end fades under the ×. */
  fadesForClose: boolean
  /** Draws the hairline between this member and the next. */
  separated: boolean
  onOpen: () => void
  drag: SidebarDrag
  registerRow: (row: SidebarRowType) => void
}) {
  const row = useMemberRow(chatId, workspaceId, hasView)

  return (
    <div
      data-testid={`recents-row-${row.id}`}
      // A ref callback, not a render-body call: render must stay pure.
      ref={(el) => {
        if (el) registerRow(row)
      }}
      // Flex item: without `min-w-0 flex-1` it shrinks to its label. The
      // shell paints the hover, not the row.
      className={cn(
        'relative min-w-0 flex-1 [&_[role=treeitem]]:hover:bg-transparent',
        fadesForClose &&
          '[mask-image:linear-gradient(to_right,black_calc(100%_-_34px),transparent_calc(100%_-_24px))]',
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
        tab={pinned ? 'tab-pinned' : isSet ? 'tab-member' : 'tab'}
      />
      {separated && (
        <span
          aria-hidden="true"
          className="pointer-events-none absolute right-0 top-1/2 h-4 w-px -translate-y-1/2 bg-foreground/15"
        />
      )}
    </div>
  )
}
