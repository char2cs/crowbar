import { DragOverlay } from '@dnd-kit/core'
import { createPortal } from 'react-dom'
import type { PaneContent } from '@/features/panes/types/pane-content'

/** The lifted tab under the pointer while a tab is dragged. */
export function TabDragPreview({ buffer }: { buffer: PaneContent | null }) {
  // In the body, so the preview sits among the other body-level overlays that hide a browser
  // tab's native webview.
  return createPortal(
    <DragOverlay dropAnimation={null}>
      {buffer ? (
        <div className="tab-drag-preview ui-font flex items-center gap-1.5 rounded-lg border border-border/70 bg-background/95 px-2 py-1 ui-text-xs opacity-95 shadow-sm">
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
