import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ContentSink } from '@/features/editor/lib/content-sink'

describe('ContentSink', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('coalesces rapid pushes into a single trailing write with the latest value', () => {
    const write = vi.fn()
    const sink = new ContentSink({ write, delayMs: 150 })
    sink.push('a')
    sink.push('ab')
    sink.push('abc')
    expect(write).not.toHaveBeenCalled() // nothing yet (trailing)
    vi.advanceTimersByTime(150)
    expect(write).toHaveBeenCalledTimes(1)
    expect(write).toHaveBeenCalledWith('abc') // latest value wins
  })

  it('flush() writes the pending value immediately and cancels the timer', () => {
    const write = vi.fn()
    const sink = new ContentSink({ write, delayMs: 150 })
    sink.push('x')
    sink.flush()
    expect(write).toHaveBeenCalledTimes(1)
    expect(write).toHaveBeenCalledWith('x')
    vi.advanceTimersByTime(150)
    expect(write).toHaveBeenCalledTimes(1) // timer did not double-fire
  })

  it('flush() with nothing pending is a no-op', () => {
    const write = vi.fn()
    const sink = new ContentSink({ write, delayMs: 150 })
    sink.flush()
    expect(write).not.toHaveBeenCalled()
  })

  it('starts a new trailing window after a flush', () => {
    const write = vi.fn()
    const sink = new ContentSink({ write, delayMs: 150 })
    sink.push('1')
    sink.flush()
    sink.push('2')
    vi.advanceTimersByTime(150)
    expect(write).toHaveBeenCalledTimes(2)
    expect(write).toHaveBeenLastCalledWith('2')
  })

  it('dispose() cancels any pending write', () => {
    const write = vi.fn()
    const sink = new ContentSink({ write, delayMs: 150 })
    sink.push('y')
    sink.dispose()
    vi.advanceTimersByTime(150)
    expect(write).not.toHaveBeenCalled()
  })

  it('does not write if the pending value equals the last flushed value', () => {
    const write = vi.fn()
    const sink = new ContentSink({ write, delayMs: 150 })
    sink.push('same')
    sink.flush()
    sink.push('same')
    vi.advanceTimersByTime(150)
    expect(write).toHaveBeenCalledTimes(1) // de-duped: 'same' already written
  })

  // Mirrors the save-time handshake wired in monaco-editor.tsx: the controller
  // effect registers a 'flush-editor-content' window listener that calls
  // sink.flush() synchronously, and the save flow dispatches that event before
  // reading buffer.content. This proves a pending value lands via write() the
  // moment the event is dispatched — no timer advance, no microtask.
  it('flushes synchronously when a flush-editor-content window event is dispatched', () => {
    const write = vi.fn()
    const sink = new ContentSink({ write, delayMs: 150 })
    const handler = () => sink.flush()
    window.addEventListener('flush-editor-content', handler)
    try {
      sink.push('pending-keystrokes')
      expect(write).not.toHaveBeenCalled() // still in the trailing window
      window.dispatchEvent(new Event('flush-editor-content'))
      // Synchronous: dispatch → listener → flush → write, before this line.
      expect(write).toHaveBeenCalledTimes(1)
      expect(write).toHaveBeenCalledWith('pending-keystrokes')
    } finally {
      window.removeEventListener('flush-editor-content', handler)
    }
  })
})
