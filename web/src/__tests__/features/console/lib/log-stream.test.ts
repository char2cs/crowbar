import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LogFrame } from '@/features/console/lib/frames'
import {
  createLogStream,
  type LogSocket,
  type LogStreamState,
} from '@/features/console/lib/log-stream'

function socket(): LogSocket & { closed: boolean } {
  return {
    onopen: null,
    onmessage: null,
    onclose: null,
    onerror: null,
    closed: false,
    close() {
      this.closed = true
    },
  }
}

let sockets: Array<ReturnType<typeof socket>>
let states: LogStreamState[]
let frames: LogFrame[]
let cursor: number | null

function make() {
  return createLogStream({
    open: () => {
      const s = socket()
      sockets.push(s)
      return s
    },
    cursor: () => cursor,
    onFrame: (f) => frames.push(f),
    onState: (s) => states.push(s),
    timers: {
      set: (fn, ms) => setTimeout(fn, ms),
      clear: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
    },
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  sockets = []
  states = []
  frames = []
  cursor = null
})
afterEach(() => vi.useRealTimers())

describe('log stream reconnect', () => {
  it('backs off 1s, 2s, 4s ... capped at 30s, and resets after a successful open', () => {
    const stream = make()
    stream.start()
    const dropAndWait = (ms: number) => {
      sockets[sockets.length - 1].onclose?.()
      const before = sockets.length
      vi.advanceTimersByTime(ms - 1)
      expect(sockets).toHaveLength(before)
      vi.advanceTimersByTime(1)
      expect(sockets).toHaveLength(before + 1)
    }
    for (const wait of [1000, 2000, 4000, 8000, 16000, 30_000, 30_000]) dropAndWait(wait)

    sockets[sockets.length - 1].onopen?.()
    expect(states.at(-1)).toBe('live')
    dropAndWait(1000)
  })

  it('reconnects from the latest cursor', () => {
    const stream = make()
    stream.start()
    cursor = 7
    const first = sockets[0]
    first.onclose?.()
    vi.advanceTimersByTime(1000)
    expect(sockets).toHaveLength(2)
    expect(states.slice(0, 3)).toEqual(['connecting', 'reconnecting', 'reconnecting'])
  })

  it('does not reconnect after stop, and ignores a stale socket', () => {
    const stream = make()
    stream.start()
    const first = sockets[0]
    stream.stop()
    expect(first.closed).toBe(true)
    first.onclose?.()
    first.onmessage?.({ data: '{"type":"ready","seq":1}' })
    vi.advanceTimersByTime(60_000)
    expect(sockets).toHaveLength(1)
    expect(frames).toEqual([])
    expect(states.at(-1)).toBe('idle')
  })

  it('hands decoded frames on', () => {
    make().start()
    sockets[0].onmessage?.({ data: '{"type":"ready","seq":4,"reset":true}' })
    expect(frames).toEqual([{ type: 'ready', seq: 4, reset: true }])
  })
})
