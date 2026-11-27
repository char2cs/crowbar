import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { playArrival } from '@/features/agent/chat/lib/arrival-animation'

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

  it('does nothing when the origin already sits at the resting spot', () => {
    playArrival(node, { top: 500 })

    expect(node.dataset.arriving).toBeUndefined()
    expect(node.style.transform).toBe('')
  })

  it('pins a node below its rest position at a positive offset', () => {
    playArrival(node, { top: 600 })

    expect(node.style.transform).toBe('translateY(100px)')
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

    expect(node.style.transition).toBe('transform 320ms cubic-bezier(0, 0, 0.2, 1)')
    expect(node.style.transform).toBe('')
    expect(node.dataset.arriving).toBe('true')
  })

  it('lifts data-arriving once the transform transition actually ends', () => {
    playArrival(node, { top: 120 })
    vi.runOnlyPendingTimers()

    node.dispatchEvent(new TransitionEvent('transitionend', { propertyName: 'transform' }))

    expect(node.dataset.arriving).toBeUndefined()
    expect(node.style.transition).toBe('')
  })

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
