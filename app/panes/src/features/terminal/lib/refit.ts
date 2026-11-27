export interface ResizeSettlePollDeps {
  isResizing: () => boolean
  onSettled: () => void
  requestFrame: (cb: () => void) => number
  cancelFrame: (id: number) => void
}

export function pollUntilResizeSettles(deps: ResizeSettlePollDeps): () => void {
  let frame: number | null = null
  let cancelled = false

  const tick = () => {
    frame = null
    if (cancelled) return
    if (deps.isResizing()) {
      frame = deps.requestFrame(tick)
      return
    }
    deps.onSettled()
  }

  frame = deps.requestFrame(tick)

  return () => {
    cancelled = true
    if (frame !== null) {
      deps.cancelFrame(frame)
      frame = null
    }
  }
}

function resolutionQuery(dpr: number): string {
  return `(resolution: ${dpr}dppx)`
}

export function watchDevicePixelRatio(onChange: () => void): () => void {
  let mql: MediaQueryList | null = null
  const onFlip = () => {
    onChange()
    arm()
  }
  const arm = () => {
    mql?.removeEventListener('change', onFlip)
    const query = resolutionQuery(window.devicePixelRatio)
    mql = window.matchMedia(query)
    mql.addEventListener('change', onFlip)
  }
  arm()
  return () => mql?.removeEventListener('change', onFlip)
}
