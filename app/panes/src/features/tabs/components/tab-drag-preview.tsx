import { DragOverlay } from '@dnd-kit/core'
import { createPortal } from 'react-dom'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { LIFTED } from '@/components/layout/drag-ghost'
import { cn } from '@/lib/utils'

/** The lifted tab under the pointer while a tab is dragged. */
export function TabDragPreview({ buffer }: { buffer: PaneContent | null }) {
  // In the body, so the preview sits among the other body-level overlays that hide a browser
  // tab's native webview.
  return createPortal(
    <DragOverlay dropAnimation={null}>
      {buffer ? (
        <div
          data-drag-overlay=""
          className={cn(
            LIFTED,
            'tab-drag-preview ui-font flex items-center gap-1.5 rounded-lg px-2 py-1 ui-text-xs',
          )}
        >
          <span className="max-w-[200px] truncate text-foreground">
            {buffer.name}
            {buffer.type === 'editor' && buffer.isDirty && (
              <span className="ml-1 text-muted-foreground">•</span>
            )}
          </span>
        </div>
      ) : null}
    </DragOverlay>,
    document.body,
  )
}
