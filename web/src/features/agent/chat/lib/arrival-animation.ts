/** How long the dock takes to ease from the empty document's handle down to its
 *  resting spot at the bottom of the pane. Also lives as a literal in
 *  composer.css (`.dock .underbar`'s own transition) — CSS cannot import a
 *  JS constant, so that's the one place this number is duplicated on purpose. */
export const ARRIVAL_SLIDE_MS = 320

export const ARRIVAL_EASE = 'cubic-bezier(0, 0, 0.2, 1)'

/**
 * How far the dock must travel to arrive exactly where the empty document's
 * own handle was — the whole trick behind the slide: it starts at wherever the
 * eye already is and eases to its resting spot, never the other way round.
 *
 * `null` for "don't animate": there is no origin to arrive FROM (not a first
 * send — every later send in the same chat leaves this unset), or the handle
 * happened to already be exactly where the dock rests, in which case a
 * transform of 0 would be indistinguishable from no animation at all and is
 * not worth the transitionend bookkeeping.
 */
export function arrivalOffset(
  origin: { top: number } | null,
  rest: { top: number },
): number | null {
  if (!origin) return null
  const dy = origin.top - rest.top
  return dy === 0 ? null : dy
}

/**
 * Plays the arrival slide on a freshly mounted dock: pins it, with no
 * transition, to wherever the empty document's own handle was — so the FIRST
 * paint of the real dock lands exactly on top of the bar it replaced, not at
 * its own resting spot — then releases it into a normal eased transition back
 * to rest. Marks the node `data-arriving` for the transition's own duration,
 * which is what holds `.underbar`'s reveal until the slide is actually over
 * (see composer.css) — a NEW slide invented for this, not a name borrowed
 * from the lab prototype's own DOM.
 *
 * A no-op with no `origin`, or when the handle already sat exactly at the
 * dock's own resting spot — see `arrivalOffset`.
 */
export function playArrival(node: HTMLElement, origin: { top: number } | null): void {
  const dy = arrivalOffset(origin, node.getBoundingClientRect())
  if (dy === null) return
  node.dataset.arriving = 'true'
  node.style.transition = 'none'
  node.style.transform = `translateY(${dy}px)`
  // Commits the instant position before the next frame reverses it. Without a
  // forced reflow here the two writes coalesce into one paint and the browser
  // never actually shows the pinned frame — it would just ease-out from
  // wherever it already was, which is nowhere.
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
