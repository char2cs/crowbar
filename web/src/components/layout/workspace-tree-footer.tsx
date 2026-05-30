import { useState } from 'react'
import { cn } from '@/lib/utils'
import { useWorkspaceTreeContext } from './workspace-tree-context'

export function WorkspaceTreeFooter() {
  const { draggingWs, dropOnTrash } = useWorkspaceTreeContext()
  const [isOver, setIsOver] = useState(false)

  if (!draggingWs) return null

  return (
    <div className="shrink-0 flex items-center justify-center border-t border-border bg-background p-2">
      <div
        className={cn(
          'flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-dashed text-[13px] font-medium transition-colors',
          isOver
            ? 'border-destructive bg-destructive/10 text-destructive'
            : 'border-destructive/40 text-destructive/40',
        )}
        onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
        onDragEnter={(e) => { e.preventDefault(); setIsOver(true) }}
        onDragLeave={() => setIsOver(false)}
        onDrop={(e) => { e.preventDefault(); setIsOver(false); dropOnTrash() }}
      >
        <svg aria-hidden="true" className="size-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 9a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1l1-9" />
        </svg>
        Drop to delete
      </div>
    </div>
  )
}
