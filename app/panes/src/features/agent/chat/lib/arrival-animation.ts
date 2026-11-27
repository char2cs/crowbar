const ARRIVAL_SLIDE_MS = 320

const ARRIVAL_EASE = 'cubic-bezier(0, 0, 0.2, 1)'

function arrivalOffset(origin: { top: number } | null, rest: { top: number }): number | null {
  if (!origin) return null
  const dy = origin.top - rest.top
  return dy === 0 ? null : dy
}

export function playArrival(node: HTMLElement, origin: { top: number } | null): void {
  const dy = arrivalOffset(origin, node.getBoundingClientRect())
  if (dy === null) return
  node.dataset.arriving = 'true'
  node.style.transition = 'none'
  node.style.transform = `translateY(${dy}px)`
  void node.offsetHeight
  requestAnimationFrame(() => {
    node.style.transition = `transform ${ARRIVAL_SLIDE_MS}ms ${ARRIVAL_EASE}`
    node.style.transform = ''
  })
  const onTransitionEnd = (event: TransitionEvent) => {
    if (event.target !== node || event.propertyName !== 'transform') return
    node.removeEventListener('transitionend', onTransitionEnd)
    node.style.transition = ''
    delete node.dataset.arriving
  }
  node.addEventListener('transitionend', onTransitionEnd)
}
