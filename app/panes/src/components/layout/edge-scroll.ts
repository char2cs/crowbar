const EDGE_SCROLL_BAND_PX = 36

const EDGE_SCROLL_MAX_STEP_PX = 14

function ramp(distance: number): number {
  const t = Math.min(1, Math.max(0, 1 - distance / EDGE_SCROLL_BAND_PX))
  return Math.max(1, Math.round(t * EDGE_SCROLL_MAX_STEP_PX))
}

function edgeScrollStep(pointer: number, top: number, height: number): number {
  if (height <= 0) return 0
  const fromTop = pointer - top
  const fromBottom = top + height - pointer
  if (fromTop < EDGE_SCROLL_BAND_PX) return -ramp(fromTop)
  if (fromBottom < EDGE_SCROLL_BAND_PX) return ramp(fromBottom)
  return 0
}

export interface EdgeScroller {
  update: (pointer: number) => void
  stop: () => void
}

export interface EdgeScrollHandlers {
  onScrolled: () => void
  onRunningChange?: (running: boolean) => void
}

export function createEdgeScroller(
  el: HTMLElement,
  box: { top: number; height: number } | { left: number; width: number },
  handlers: EdgeScrollHandlers,
): EdgeScroller {
  const horizontal = 'left' in box
  const start = horizontal ? box.left : box.top
  const size = horizontal ? box.width : box.height
  const read = () => (horizontal ? el.scrollLeft : el.scrollTop)
  const write = (v: number) => {
    if (horizontal) el.scrollLeft = v
    else el.scrollTop = v
  }
  let frame: number | null = null
  let step = 0

  const halt = () => {
    if (frame === null) return
    cancelAnimationFrame(frame)
    frame = null
    handlers.onRunningChange?.(false)
  }

  const tick = () => {
    if (step === 0) {
      halt()
      return
    }
    const before = read()
    write(before + step)
    if (read() !== before) handlers.onScrolled()
    frame = requestAnimationFrame(tick)
  }

  return {
    update: (pointer: number) => {
      step = edgeScrollStep(pointer, start, size)
      if (step === 0 || frame !== null) return
      handlers.onRunningChange?.(true)
      frame = requestAnimationFrame(tick)
    },
    stop: () => {
      step = 0
      halt()
    },
  }
}
