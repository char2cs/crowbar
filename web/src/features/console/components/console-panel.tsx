import { useEffect, useRef } from 'react'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { cn } from '@/utils/cn'
import { ConsoleLog } from './console-log'

/**
 * The quake-style console: it drops from the top of the content and shows the
 * daemon's log. Nothing else; the version badge or Escape from anywhere inside
 * it closes it.
 */
export function ConsolePanel() {
  const open = useConsoleStore((s) => s.open)
  const entries = useConsoleStore((s) => s.entries)
  const expandedId = useConsoleStore((s) => s.expandedId)
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

  // Non-modal dropdown: a press outside closes it and still reaches its target.
  // The panel's own trigger is excluded because its click already toggles.
  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target
      if (!(target instanceof Element)) return
      if (panel.current?.contains(target) || target.closest('[aria-controls="console-panel"]'))
        return
      useConsoleStore.getState().setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    return () => document.removeEventListener('pointerdown', onPointerDown, true)
  }, [open])

  return (
    <section
      ref={panel}
      id="console-panel"
      aria-label="Console"
      // Closed means unreachable, not merely off-screen: nothing in it may take focus.
      inert={!open}
      data-open={open}
      className={cn(
        'bg-code text-code-foreground border-border absolute inset-x-0 top-0 z-20 flex h-[min(26.25rem,70%)] flex-col border-b shadow-lg transition-[transform,visibility] duration-200 ease-out',
        open ? 'visible translate-y-0' : 'invisible -translate-y-full',
      )}
    >
      <ConsoleLog entries={entries} expandedId={expandedId} onToggle={toggleExpanded} open={open} />
    </section>
  )
}
