import { useRef, useEffect } from 'react'
import { useBottomRoot } from '@/features/workspace/stores/hooks/use-pane-store'
import { PaneNodeRenderer } from '@/features/panes/components/pane-node-renderer'
import { useUIState } from '@/features/window/stores/ui-state-store'

function BottomPaneContent() {
  const bottomRoot = useBottomRoot()
  return <PaneNodeRenderer node={bottomRoot} />
}

export function BottomPane() {
  const height = useUIState(s => s.bottomPaneHeight)
  const setHeight = useUIState(s => s.setBottomPaneHeight)
  const cleanupRef = useRef<(() => void) | null>(null)

  // Clean up any in-progress drag on unmount
  useEffect(() => () => { cleanupRef.current?.() }, [])

  const handleResizeDragStart = (e: React.MouseEvent) => {
    e.preventDefault()
    const startY = e.clientY
    const startHeight = height

    const handleMouseMove = (ev: MouseEvent) => {
      // Dragging the top border upward increases height
      const delta = startY - ev.clientY
      const next = Math.max(120, Math.min(600, startHeight + delta))
      setHeight(next)
    }

    const cleanup = () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', cleanup)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      cleanupRef.current = null
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', cleanup)
    document.body.style.cursor = 'ns-resize'
    document.body.style.userSelect = 'none'
    cleanupRef.current = cleanup
  }

  return (
    <div className="flex flex-col border-t border-border" style={{ height }}>
      {/* Drag handle */}
      <div
        className="h-1 w-full shrink-0 cursor-ns-resize hover:bg-primary/30 transition-colors"
        onMouseDown={handleResizeDragStart}
        aria-label="Resize bottom pane"
        role="separator"
        aria-orientation="horizontal"
      />
      <div className="min-h-0 flex-1 overflow-hidden">
        <BottomPaneContent />
      </div>
    </div>
  )
}
