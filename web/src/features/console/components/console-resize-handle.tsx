import { useRef, type KeyboardEvent, type PointerEvent, type RefObject } from 'react'
import { DEFAULT_SIZE, clampSize, type ConsoleDock } from '@/features/console/lib/dock-prefs'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { cn } from '@/utils/cn'

const KEY_STEP = 16

const isVertical = (dock: ConsoleDock) => dock === 'top' || dock === 'bottom'
const windowExtent = (dock: ConsoleDock) =>
  isVertical(dock) ? window.innerHeight : window.innerWidth

/** How far the pointer moving by `dx`/`dy` grows the panel: toward the app is outward. */
function growth(dock: ConsoleDock, dx: number, dy: number): number {
  if (dock === 'top') return dy
  if (dock === 'bottom') return -dy
  return dock === 'left' ? dx : -dx
}

/**
 * The drag edge on the side facing the app. During a drag only the panel's CSS
 * variable moves; the store (and storage) see the size once, on release.
 */
export function ConsoleResizeHandle({ panel }: { panel: RefObject<HTMLElement | null> }) {
  const dock = useConsoleStore((s) => s.dock)
  const size = useConsoleStore((s) => s.size)
  const drag = useRef<{ x: number; y: number; start: number; live: number } | null>(null)
  const vertical = isVertical(dock)

  const show = (px: number) => panel.current?.style.setProperty('--console-size', `${px}px`)

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return
    event.preventDefault()
    event.currentTarget.setPointerCapture?.(event.pointerId)
    const start = clampSize(size, windowExtent(dock))
    drag.current = { x: event.clientX, y: event.clientY, start, live: start }
    panel.current?.setAttribute('data-resizing', '')
  }

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const d = drag.current
    if (!d) return
    d.live = clampSize(
      d.start + growth(dock, event.clientX - d.x, event.clientY - d.y),
      windowExtent(dock),
    )
    show(d.live)
  }

  const end = () => {
    const d = drag.current
    if (!d) return
    drag.current = null
    panel.current?.removeAttribute('data-resizing')
    useConsoleStore.getState().setSize(d.live)
  }

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const outward = vertical
      ? dock === 'top'
        ? 'ArrowDown'
        : 'ArrowUp'
      : dock === 'left'
        ? 'ArrowRight'
        : 'ArrowLeft'
    const inward = vertical
      ? dock === 'top'
        ? 'ArrowUp'
        : 'ArrowDown'
      : dock === 'left'
        ? 'ArrowLeft'
        : 'ArrowRight'
    if (event.key !== outward && event.key !== inward) return
    event.preventDefault()
    const next = clampSize(
      size + (event.key === outward ? KEY_STEP : -KEY_STEP),
      windowExtent(dock),
    )
    useConsoleStore.getState().setSize(next)
  }

  return (
    <div
      role="separator"
      aria-label="Resize console"
      aria-orientation={vertical ? 'horizontal' : 'vertical'}
      aria-valuenow={clampSize(size, windowExtent(dock))}
      tabIndex={0}
      data-slot="console-resize"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={end}
      onPointerCancel={end}
      onDoubleClick={() => useConsoleStore.getState().setSize(DEFAULT_SIZE)}
      onKeyDown={onKeyDown}
      className={cn(
        'hover:bg-ring/40 focus-visible:bg-ring/40 absolute z-10 touch-none outline-none',
        vertical ? 'inset-x-0 h-1.5 cursor-row-resize' : 'inset-y-0 w-1.5 cursor-col-resize',
        dock === 'top' && 'bottom-0 translate-y-1/2',
        dock === 'bottom' && 'top-0 -translate-y-1/2',
        dock === 'left' && 'right-0 translate-x-1/2',
        dock === 'right' && 'left-0 -translate-x-1/2',
      )}
    />
  )
}
