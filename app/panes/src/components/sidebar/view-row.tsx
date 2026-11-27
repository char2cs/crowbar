import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { PushPinIcon, XIcon } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { TAB_IDLE } from '@/components/layout/workspace-row-base'
import { useRecentsChat } from '@/components/sidebar/lib/use-recents-chat'
import { recentsChatIcon } from '@/components/sidebar/lib/recents-for-project'
import type { SidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import type { ChatIconFields } from '@/components/sidebar/lib/chat-icon-index'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { viewMembers } from '@/features/panes/lib/view-state'
import type { ViewMember } from '@/features/panes/types/pane'
import { selectRepos, useSelect } from '@/core/entities/select'

const NO_ICON: Partial<ChatIconFields> = {}
const LOADING_CHAT_LABEL = 'Loading…'
const pendingLabel = (chatId: string) => (isDraftChatId(chatId) ? 'New thread' : LOADING_CHAT_LABEL)

export function useMemberRow(
  chatId: string,
  workspaceId: string,
  hasView: boolean,
): SidebarRowType {
  const icon = useSelect(useShallow((s) => recentsChatIcon(selectRepos(s), chatId) ?? NO_ICON))
  const known = useRecentsChat(workspaceId, chatId)
  const chat = known ?? { id: chatId, title: '', workspaceId, working: false }
  return {
    id: chat.id,
    parentId: null,
    order: 0,
    label: chat.title || (known ? UNTITLED_CHAT_LABEL : pendingLabel(chatId)),
    labelProvisional: !chat.title,
    workspaceId: chat.workspaceId,
    working: chat.working,
    hasView,
    kind: 'chat',
    ownsWorktree: false,
    ...icon,
  }
}

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
  registerRow: (row: SidebarRowType) => () => void
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
      className={cn(TAB_SHELL, isShowing ? TAB_ACTIVE : TAB_IDLE)}
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
            fadesForClose={isLast}
            separated={!isLast && !isShowing}
            onOpen={() => onFocus(viewId)}
            drag={drag}
            registerRow={registerRow}
          />
        )
      })}
      {pinned ? (
        <Button
          variant="ghost"
          data-no-dnd
          aria-label="Unpin tab"
          tooltip="Unpin tab"
          tooltipSide="bottom"
          onClick={() => windowPaneStore.getState().paneActions.setViewPinned(viewId, false)}
          className={cn(TAB_CLOSE, '!min-h-0 !min-w-0 !p-0 opacity-60')}
          tabIndex={-1}
          draggable={false}
        >
          <PushPinIcon className="pointer-events-none size-3.5 select-none fill-current text-secondary" />
        </Button>
      ) : (
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

const TAB_ACTIVE =
  'control-surface outline outline-1 shadow-xs shadow-black/10 ' +
  'inset-shadow-[0_1px_var(--elevated-highlight)] active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'
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
  fadesForClose,
  separated,
  onOpen,
  drag,
  registerRow,
}: {
  chatId: string
  workspaceId: string
  hasView: boolean
  isSet: boolean
  fadesForClose: boolean
  separated: boolean
  onOpen: () => void
  drag: SidebarDrag
  registerRow: (row: SidebarRowType) => () => void
}) {
  const row = useMemberRow(chatId, workspaceId, hasView)

  return (
    <div
      data-testid={`recents-row-${row.id}`}
      ref={(el) => (el ? registerRow(row) : undefined)}
      className={cn(
        'relative min-w-0 flex-1 [&_[role=tab]]:hover:bg-transparent',
        fadesForClose &&
          '[mask-image:linear-gradient(to_right,black_calc(100%_-_34px),transparent_calc(100%_-_24px))]',
      )}
    >
      <SidebarRow
        row={row}
        depth={0}
        onOpen={onOpen}
        dragProps={drag.dragProps(row, { inRecents: true })}
        isDragging={drag.draggingIds.has(row.id)}
        isNestTarget={drag.nestTargetId === row.id}
        onPointerDownDrag={(e) => drag.onPointerDownDrag(row, e)}
        inlineRenameDisabled
        tab={isSet ? 'tab-member' : 'tab'}
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
