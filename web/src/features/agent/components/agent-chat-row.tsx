import { memo } from 'react'
import { useStore } from 'zustand'
import { cn } from '@/lib/utils'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { AgentChatGlyph } from './agent-chat-glyph'

interface AgentChatRowProps {
  wsId: string
  chatId: string
  title: string
  providerIcon: string
  active: boolean
  renaming: boolean
  /** This row is the one under the pointer mid-drag (dimmed, as in the tree). */
  dragging?: boolean
  onSelect: (chatId: string) => void
  onStartRename: (chatId: string) => void
  onConfirmRename: (chatId: string, title: string) => void
  onCancelRename: () => void
  onPointerDownDrag: (chatId: string, e: React.PointerEvent) => void
}

// Sidebar row for one agent chat — a visual sibling of WorkspaceTreeItem
// (components/layout/workspace-tree-item.tsx), sharing its row surface classes
// but with no ⋯ menu / × delete: deletion is drag-to-trash, owned by the
// Task 16 panel via the data-agent-chat-drop + pointer-down drag affordance
// exposed here.
//
// Memoised, and it reads its OWN turn state straight from the store rather than
// taking a `working` prop — exactly like AgentChatTabIcon. That combination is
// load-bearing for a long list: a `turn_started`/`turn_stopped` frame is the
// hottest thing on the chat feed, and it must re-render ONLY the one row it
// concerns. Every other prop is a primitive or a stable callback, so the panel
// re-rendering for an unrelated reason (a new chat, a drag) skips untouched rows.
function AgentChatRowComponent({
  wsId,
  chatId,
  title,
  providerIcon,
  active,
  renaming,
  dragging = false,
  onSelect,
  onStartRename,
  onConfirmRename,
  onCancelRename,
  onPointerDownDrag,
}: AgentChatRowProps) {
  // Own turn state, self-subscribed: a primitive selector, so this row re-renders
  // only when THIS chat's spinner flips — never when a sibling's does.
  const working = useStore(
    getOrCreateWorkspaceStore(wsId),
    (s) => s.agentChats.working[chatId] ?? false,
  )

  return (
    <div
      // react-doctor-disable-next-line prefer-tag-over-role -- can't be a real <button>: while `renaming`, this renders a nested <WorkspaceInlineInput> (a real <input>), and HTML forbids interactive content inside a <button>. role="button" + tabIndex + onKeyDown is the correct fallback.
      role="button"
      tabIndex={0}
      data-agent-chat-drop={chatId}
      className={cn(
        ROW_BASE,
        active ? ROW_ACTIVE : ROW_INACTIVE,
        // Drag feedback: the row being dragged fades. WHERE it would land is
        // drawn by the panel's insertion line, in the gap between rows — a ring
        // on a row can only mean "in front of this one", which cannot name the
        // end of the list (see agent-chat-drop-geometry.ts).
        dragging && 'opacity-40',
      )}
      onPointerDown={renaming ? undefined : (e) => onPointerDownDrag(chatId, e)}
      onClick={renaming ? undefined : () => onSelect(chatId)}
      onDoubleClick={renaming ? undefined : () => onStartRename(chatId)}
      onKeyDown={
        renaming
          ? undefined
          : (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onSelect(chatId)
              }
            }
      }
    >
      {/* Provider icon → flip-dot spinner while working. Neither bakes in a color;
          they inherit currentColor from the theme token on this wrapper. */}
      <span className="flex size-4 shrink-0 items-center justify-center text-foreground">
        <AgentChatGlyph providerIcon={providerIcon} working={working} />
      </span>

      {renaming ? (
        <WorkspaceInlineInput
          defaultValue={title}
          placeholder="chat title"
          kind="prose"
          onConfirm={(value) => onConfirmRename(chatId, value)}
          onCancel={onCancelRename}
        />
      ) : (
        <span className="min-w-0 flex-1 truncate">{title}</span>
      )}
    </div>
  )
}

export const AgentChatRow = memo(AgentChatRowComponent)
