import { useEffect, type RefObject } from 'react'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

/**
 * Focus `paneId` on any mousedown inside it — a native capture listener, not a
 * React prop: the editor is portaled in from EditorHostRegistry (a React
 * sibling of the pane tree), and React's synthetic events walk the fiber tree,
 * so only a DOM listener sees a click on it.
 */
export function usePaneActivation(paneId: string, containerRef: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const node = containerRef.current
    if (!node) return
    const onMouseDownCapture = (e: MouseEvent) => {
      const target = e.target as HTMLElement
      // Monaco's and xterm's real typing surfaces are <textarea>s that must
      // still activate the pane.
      const isTypingSurface =
        target.classList?.contains('inputarea') ||
        target.classList?.contains('xterm-helper-textarea')
      // The target itself only, never `.closest()`: chat content and editor
      // toolbars nest buttons everywhere, and an ancestor walk swallowed them.
      const isDirectInteractiveHit = target.matches?.(
        "button, input, textarea, [role='button'], [role='menu']",
      )
      if (!isTypingSurface && isDirectInteractiveHit) return
      // Read live: a parked view's pane never receives a real gesture.
      if (windowPaneStore.getState().activePaneId !== paneId) {
        windowPaneStore.getState().paneActions.setActivePane(paneId)
      }
    }
    node.addEventListener('mousedown', onMouseDownCapture, true)
    return () => node.removeEventListener('mousedown', onMouseDownCapture, true)
  }, [paneId, containerRef])
}
