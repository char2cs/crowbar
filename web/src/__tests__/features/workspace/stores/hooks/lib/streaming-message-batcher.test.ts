import { describe, expect, it, vi } from 'vitest'
import { createStreamingMessageBatcher } from '@/features/workspace/stores/hooks/lib/streaming-message-batcher'

// A fake rAF the test drives by hand, rather than a real one — precise control
// over exactly when "the next frame" happens, with no timing flakiness.
function fakeFrame() {
  let queued: (() => void) | null = null
  return {
    raf: (cb: () => void) => {
      queued = cb
      return 1
    },
    cancel: vi.fn(() => {
      queued = null
    }),
    fire: () => {
      const cb = queued
      queued = null
      cb?.()
    },
    isArmed: () => queued !== null,
  }
}

describe('createStreamingMessageBatcher', () => {
  it('does not flush before the frame fires', () => {
    const flush = vi.fn()
    const { raf, cancel, fire } = fakeFrame()
    const batcher = createStreamingMessageBatcher(flush, raf, cancel)

    batcher.schedule('c1', { id: 'm1', text: 'Buil' })

    expect(flush).not.toHaveBeenCalled()
    fire()
    expect(flush).toHaveBeenCalledWith('c1', { id: 'm1', text: 'Buil' })
  })

  it('collapses several deltas for the same chat into one flush with the latest text', () => {
    const flush = vi.fn()
    const { raf, cancel, fire } = fakeFrame()
    const batcher = createStreamingMessageBatcher(flush, raf, cancel)

    batcher.schedule('c1', { id: 'm1', text: 'B' })
    batcher.schedule('c1', { id: 'm1', text: 'Bu' })
    batcher.schedule('c1', { id: 'm1', text: 'Bui' })
    fire()

    expect(flush).toHaveBeenCalledTimes(1)
    expect(flush).toHaveBeenCalledWith('c1', { id: 'm1', text: 'Bui' })
  })

  it('only arms one frame for a burst — later schedules in the same tick do not re-request', () => {
    const raf = vi.fn(() => 1)
    const batcher = createStreamingMessageBatcher(vi.fn(), raf, vi.fn())

    batcher.schedule('c1', { id: 'm1', text: 'a' })
    batcher.schedule('c1', { id: 'm1', text: 'ab' })
    batcher.schedule('c2', { id: 'm2', text: 'x' })

    expect(raf).toHaveBeenCalledTimes(1)
  })

  it('flushes each chat that had activity, independently', () => {
    const flush = vi.fn()
    const { raf, cancel, fire } = fakeFrame()
    const batcher = createStreamingMessageBatcher(flush, raf, cancel)

    batcher.schedule('c1', { id: 'm1', text: 'one' })
    batcher.schedule('c2', { id: 'm2', text: 'two' })
    fire()

    expect(flush).toHaveBeenCalledTimes(2)
    expect(flush).toHaveBeenCalledWith('c1', { id: 'm1', text: 'one' })
    expect(flush).toHaveBeenCalledWith('c2', { id: 'm2', text: 'two' })
  })

  // Regression: Codex can have more than one message item open in the SAME
  // chat at once. Collapsing by chatId alone (the old keying) would let a
  // second id's delta silently supersede the first id's still-open text in
  // the SAME animation frame, before either ever reached the store.
  it('flushes two different message ids for the SAME chat independently, neither superseding the other', () => {
    const flush = vi.fn()
    const { raf, cancel, fire } = fakeFrame()
    const batcher = createStreamingMessageBatcher(flush, raf, cancel)

    batcher.schedule('c1', { id: 'm1', text: 'first item' })
    batcher.schedule('c1', { id: 'm2', text: 'second item' })
    fire()

    expect(flush).toHaveBeenCalledTimes(2)
    expect(flush).toHaveBeenCalledWith('c1', { id: 'm1', text: 'first item' })
    expect(flush).toHaveBeenCalledWith('c1', { id: 'm2', text: 'second item' })
  })

  it('arms a new frame for activity scheduled after a flush', () => {
    const rafSpy = vi.fn()
    const { raf, cancel, fire } = fakeFrame()
    rafSpy.mockImplementation(raf)
    const batcher = createStreamingMessageBatcher(vi.fn(), rafSpy, cancel)

    batcher.schedule('c1', { id: 'm1', text: 'a' })
    fire()
    expect(rafSpy).toHaveBeenCalledTimes(1)

    batcher.schedule('c1', { id: 'm1', text: 'ab' })
    expect(rafSpy).toHaveBeenCalledTimes(2)
  })

  it('dispose cancels a pending frame and drops what was pending, without flushing it', () => {
    const flush = vi.fn()
    const { raf, cancel } = fakeFrame()
    const batcher = createStreamingMessageBatcher(flush, raf, cancel)

    batcher.schedule('c1', { id: 'm1', text: 'a' })
    batcher.dispose()

    expect(cancel).toHaveBeenCalledWith(1)
    expect(flush).not.toHaveBeenCalled()
  })

  it('dispose is safe to call with nothing scheduled', () => {
    const cancel = vi.fn()
    const batcher = createStreamingMessageBatcher(vi.fn(), vi.fn(), cancel)

    expect(() => batcher.dispose()).not.toThrow()
    expect(cancel).not.toHaveBeenCalled()
  })

  it('scheduling again after dispose arms a fresh frame', () => {
    const flush = vi.fn()
    const { raf, cancel, fire } = fakeFrame()
    const batcher = createStreamingMessageBatcher(flush, raf, cancel)

    batcher.schedule('c1', { id: 'm1', text: 'a' })
    batcher.dispose()

    batcher.schedule('c1', { id: 'm1', text: 'b' })
    fire()

    expect(flush).toHaveBeenCalledTimes(1)
    expect(flush).toHaveBeenCalledWith('c1', { id: 'm1', text: 'b' })
  })
})
