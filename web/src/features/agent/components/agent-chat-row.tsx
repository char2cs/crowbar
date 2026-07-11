import { cn } from '@/lib/utils'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'

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
      {/* Leading glyph: provider icon → flip-dot spinner while working. The
          spinner is colored via a theme token (text-primary) since
          FlickerSpinner itself bakes in no color — it inherits currentColor. */}
      {working ? (
        <span className="flex size-4 shrink-0 items-center justify-center text-primary">
          <FlickerSpinner className="size-3.5" />
        </span>
      ) : (
        <span
          aria-hidden="true"
          className="flex size-4 shrink-0 items-center justify-center text-foreground [&>svg]:size-full"
          dangerouslySetInnerHTML={{ __html: providerIcon }}
        />
      )}

      {renaming ? (
        <WorkspaceInlineInput
          defaultValue={title}
          placeholder="chat title"
          onConfirm={onConfirmRename}
          onCancel={onCancelRename}
        />
      ) : (
        <span className="min-w-0 flex-1 truncate">{title}</span>
      )}
    </div>
  )
}
