import { cn } from '@/lib/utils'
import { useWorkspaceTreeContext } from './workspace-tree-context'

export function WorkspaceTreeFooter() {
  const { draggingWs, hoverTargetId } = useWorkspaceTreeContext()
  const isOver = hoverTargetId === 'trash'

  // Always rendered so the ScrollArea doesn't resize on drag start/end.
  // Slides in with max-height transition instead of mount/unmount.
  return (
    <div
      className={cn(
        'shrink-0 overflow-hidden transition-[max-height] duration-150 ease-out',
        draggingWs ? 'max-h-16' : 'max-h-0',
      )}
    >
      <div className="flex items-center justify-center border-t border-border bg-background p-2">
        <div
          data-trash-drop="true"
          className={cn(
            'flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-dashed text-[13px] font-medium transition-colors',
            isOver
              ? 'border-destructive bg-destructive/10 text-destructive'
              : 'border-destructive/40 text-destructive/40',
          )}
        >
          <svg aria-hidden="true" className="size-4 pointer-events-none" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 9a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1l1-9" />
          </svg>
          Drop to delete
        </div>
      </div>
    </div>
  )
}
