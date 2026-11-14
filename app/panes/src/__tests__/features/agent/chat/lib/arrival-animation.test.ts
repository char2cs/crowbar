import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ARRIVAL_EASE,
  ARRIVAL_SLIDE_MS,
  arrivalOffset,
  playArrival,
} from '@/features/agent/chat/lib/arrival-animation'

describe('arrivalOffset', () => {
  it('is null with no origin — nothing to arrive from', () => {
    expect(arrivalOffset(null, { top: 400 })).toBeNull()
  })

  it('is the distance the dock must travel up to sit where the handle was', () => {
    // The handle rode near the top of the doc; the dock rests near the bottom.
    expect(arrivalOffset({ top: 120 }, { top: 500 })).toBe(-380)
  })

  it('is positive when the handle was actually BELOW the dock’s resting spot', () => {
    expect(arrivalOffset({ top: 600 }, { top: 500 })).toBe(100)
  })

  it('is null when the handle already sat exactly at the resting spot', () => {
    expect(arrivalOffset({ top: 500 }, { top: 500 })).toBeNull()
  })
})

describe('playArrival', () => {
  let node: HTMLDivElement

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame'] })
    node = document.createElement('div')
    document.body.appendChild(node)
    vi.spyOn(node, 'getBoundingClientRect').mockReturnValue({ top: 500 } as DOMRect)
  })

  afterEach(() => {
    node.remove()
    vi.useRealTimers()
  })

  it('does nothing at all with no origin to arrive from', () => {
    playArrival(node, null)

    expect(node.dataset.arriving).toBeUndefined()
    expect(node.style.transform).toBe('')
  })

  it('pins the node at the origin, transition-less, on the very first frame', () => {
    playArrival(node, { top: 120 })

    expect(node.dataset.arriving).toBe('true')
    expect(node.style.transition).toBe('none')
    expect(node.style.transform).toBe('translateY(-380px)')
  })

  it('releases into an eased transition back to rest on the next frame', () => {
    playArrival(node, { top: 120 })

    vi.runOnlyPendingTimers()

    expect(node.style.transition).toBe(`transform ${ARRIVAL_SLIDE_MS}ms ${ARRIVAL_EASE}`)
    expect(node.style.transform).toBe('')
    // Still arriving — the underbar stays hidden until the slide actually ends,
    // not merely once it has been told to start.
    expect(node.dataset.arriving).toBe('true')
  })

  it('lifts data-arriving once the transform transition actually ends', () => {
    playArrival(node, { top: 120 })
    vi.runOnlyPendingTimers()

    node.dispatchEvent(new TransitionEvent('transitionend', { propertyName: 'transform' }))

    expect(node.dataset.arriving).toBeUndefined()
    expect(node.style.transition).toBe('')
  })

  // A transitionend for some OTHER property (or bubbled from a descendant)
  // must not end the arrival early — the underbar would flash in mid-slide.
  it('ignores a transitionend for a different property', () => {
    playArrival(node, { top: 120 })
    vi.runOnlyPendingTimers()

    node.dispatchEvent(new TransitionEvent('transitionend', { propertyName: 'opacity' }))

    expect(node.dataset.arriving).toBe('true')
  })

  it('only ever fires the cleanup once, even if transitionend somehow fires twice', () => {
    playArrival(node, { top: 120 })
    vi.runOnlyPendingTimers()
    const removeSpy = vi.spyOn(node, 'removeEventListener')

    node.dispatchEvent(new TransitionEvent('transitionend', { propertyName: 'transform' }))
    node.dispatchEvent(new TransitionEvent('transitionend', { propertyName: 'transform' }))

    expect(removeSpy).toHaveBeenCalledTimes(1)
  })
})
