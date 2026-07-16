import { cn } from '@/lib/utils'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'
import { AgentChatGlyph } from './agent-chat-glyph'

interface AgentChatRowProps {
  chatId: string
  title: string
  providerIcon: string
  working: boolean
  active: boolean
  renaming: boolean
  /** This row is the one under the pointer mid-drag (dimmed, as in the tree). */
  dragging?: boolean
  /** A drop right now would land here — same ring the workspace tree uses. */
  dropTarget?: boolean
  onSelect: () => void
  onStartRename: () => void
  onConfirmRename: (title: string) => void
  onCancelRename: () => void
  onPointerDownDrag: (e: React.PointerEvent) => void
}

// Sidebar row for one agent chat — a visual sibling of WorkspaceTreeItem
// (components/layout/workspace-tree-item.tsx), sharing its row surface classes
// but with no ⋯ menu / × delete: deletion is drag-to-trash, owned by the
// Task 16 panel via the data-agent-chat-drop + pointer-down drag affordance
// exposed here.
export function AgentChatRow({
  chatId,
  title,
  providerIcon,
  working,
  active,
  renaming,
  dragging = false,
  dropTarget = false,
  onSelect,
  onStartRename,
  onConfirmRename,
  onCancelRename,
  onPointerDownDrag,
}: AgentChatRowProps) {
  return (
    <div
      // react-doctor-disable-next-line prefer-tag-over-role -- can't be a real <button>: while `renaming`, this renders a nested <WorkspaceInlineInput> (a real <input>), and HTML forbids interactive content inside a <button>. role="button" + tabIndex + onKeyDown is the correct fallback.
      role="button"
      tabIndex={0}
      data-agent-chat-drop={chatId}
      className={cn(
        ROW_BASE,
        active ? ROW_ACTIVE : ROW_INACTIVE,
        // Drag feedback, identical to WorkspaceTreeItem: the row being dragged
        // fades, the row it would land in front of takes the focus ring.
        dragging && 'opacity-40',
        dropTarget && 'ring-1 ring-ring',
      )}
      onPointerDown={renaming ? undefined : onPointerDownDrag}
      onClick={renaming ? undefined : onSelect}
      onDoubleClick={renaming ? undefined : onStartRename}
      onKeyDown={
        renaming
          ? undefined
          : (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onSelect()
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
          onConfirm={onConfirmRename}
          onCancel={onCancelRename}
        />
      ) : (
        <span className="min-w-0 flex-1 truncate">{title}</span>
      )}
    </div>
  )
}
