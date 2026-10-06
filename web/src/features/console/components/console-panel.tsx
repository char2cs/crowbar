import { useEffect, useRef, type CSSProperties } from 'react'
import { useConsoleStore } from '@/features/console/stores/console-store'
import type { ConsoleDock } from '@/features/console/lib/dock-prefs'
import { cn } from '@/utils/cn'
import { ConsoleHeader } from './console-header'
import { ConsoleLog } from './console-log'
import { ConsoleResizeHandle } from './console-resize-handle'

const DOCK_CLASS: Record<ConsoleDock, string> = {
  top: 'top-0 inset-x-0 border-b order-first',
  bottom: 'bottom-0 inset-x-0 border-t order-last',
  left: 'left-0 inset-y-0 border-r order-first',
  right: 'right-0 inset-y-0 border-l order-last',
}

const CLOSED_OFFSET: Record<ConsoleDock, string> = {
  top: '-translate-y-full',
  bottom: 'translate-y-full',
  left: '-translate-x-full',
  right: 'translate-x-full',
}

/**
 * The quake-style console: it slides in from its dock edge and shows the
 * daemon's log. As an overlay it floats over the whole window; in push mode it
 * is a flex sibling of the app and the app shrinks. The version badge or Escape
 * from anywhere inside it closes it.
 */
export function ConsolePanel() {
  const open = useConsoleStore((s) => s.open)
  const entries = useConsoleStore((s) => s.entries)
  const expandedId = useConsoleStore((s) => s.expandedId)
  const dock = useConsoleStore((s) => s.dock)
  const mode = useConsoleStore((s) => s.mode)
  const size = useConsoleStore((s) => s.size)
  const toggleExpanded = useConsoleStore((s) => s.toggleExpanded)
  const panel = useRef<HTMLElement>(null)

  // Escape bubbles up from a log line's button or the log pane; a region is not
  // interactive, so the key handler is attached to the element rather than as a prop.
  useEffect(() => {
    const el = panel.current
    if (!el) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.isComposing) return
      event.preventDefault()
      useConsoleStore.getState().setOpen(false)
    }
    el.addEventListener('keydown', onKeyDown)
    return () => el.removeEventListener('keydown', onKeyDown)
  }, [])

  // Overlay is a non-modal dropdown: a press outside closes it and still reaches
  // its target. Pushed, the app beside it stays in use, so only Escape or the badge
  // close it. The panel's own trigger is excluded because its click already toggles.
  const overlay = mode === 'overlay'
  useEffect(() => {
    if (!open || !overlay) return
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target
      if (!(target instanceof Element)) return
      if (panel.current?.contains(target) || target.closest('[aria-controls="console-panel"]'))
        return
      useConsoleStore.getState().setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    return () => document.removeEventListener('pointerdown', onPointerDown, true)
  }, [open, overlay])

  const vertical = dock === 'top' || dock === 'bottom'

  return (
    <section
      ref={panel}
      id="console-panel"
      aria-label="Console"
      // Closed means unreachable, not merely off-screen: nothing in it may take focus.
      inert={!open}
      data-open={open}
      data-dock={dock}
      data-mode={mode}
      style={{ '--console-size': `${size}px` } as CSSProperties}
      className={cn(
        'bg-code text-code-foreground border-border z-[45] flex flex-col overflow-hidden',
        'data-resizing:transition-none',
        overlay
          ? 'absolute shadow-lg transition-[transform,visibility] duration-200 ease-out'
          : 'relative shrink-0 transition-[width,height] duration-200 ease-out',
        vertical
          ? 'w-full h-[min(var(--console-size),80dvh)]'
          : 'h-full w-[min(var(--console-size),80dvw)]',
        DOCK_CLASS[dock],
        open ? 'visible' : 'invisible',
        overlay && (open ? 'translate-0' : CLOSED_OFFSET[dock]),
        !overlay && !open && (vertical ? 'h-0 border-0' : 'w-0 border-0'),
      )}
    >
      <ConsoleResizeHandle panel={panel} />
      <ConsoleHeader />
      <ConsoleLog entries={entries} expandedId={expandedId} onToggle={toggleExpanded} open={open} />
    </section>
  )
}
