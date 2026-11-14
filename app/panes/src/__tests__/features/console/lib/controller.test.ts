import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createConsoleController } from '@/features/console/lib/controller'
import type { LogSocket } from '@/features/console/lib/log-stream'
import { useConsoleStore } from '@/features/console/stores/console-store'

class FakeSocket implements LogSocket {
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: ((event: unknown) => void) | null = null
  closed = false
  constructor(readonly path: string) {}
  close() {
    this.closed = true
  }
  send(frame: object) {
    this.onmessage?.({ data: JSON.stringify(frame) })
  }
}

const FLUSH_MS = 50
const log = (seq: number) => ({ type: 'log', seq, time: '', level: 'info', msg: `m${seq}` })
const ready = (seq: number, reset?: boolean) => ({ type: 'ready', seq, ...(reset && { reset }) })

let sockets: FakeSocket[]
let controller: ReturnType<typeof createConsoleController>
const last = () => sockets[sockets.length - 1]
const shown = () =>
  useConsoleStore.getState().entries.map((e) => (e.kind === 'log' ? e.record.msg : e.kind))

beforeEach(() => {
  vi.useFakeTimers()
  useConsoleStore.setState(useConsoleStore.getInitialState())
  sockets = []
  controller = createConsoleController({
    store: useConsoleStore,
    open: (path) => {
      const socket = new FakeSocket(path)
      sockets.push(socket)
      return socket
    },
    timers: {
      set: (fn, ms) => setTimeout(fn, ms),
      clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
    },
  })
})

afterEach(() => {
  controller.dispose()
  vi.useRealTimers()
})

describe('console controller', () => {
  it('opens no socket until the console is opened, and closes it when it is closed', () => {
    expect(sockets).toHaveLength(0)
    useConsoleStore.getState().setOpen(true)
    expect(sockets).toHaveLength(1)
    expect(last().path).toBe('/v0/console/logs?level=debug')
    useConsoleStore.getState().setOpen(false)
    expect(last().closed).toBe(true)
    expect(useConsoleStore.getState().stream).toBe('idle')
  })

  it('shows nothing of a replay until its ready, then folds it in as a batch', () => {
    useConsoleStore.getState().setOpen(true)
    last().send(log(1))
    last().send(log(2))
    vi.advanceTimersByTime(FLUSH_MS * 2)
    expect(shown()).toEqual([])
    last().send(ready(2))
    vi.advanceTimersByTime(FLUSH_MS)
    expect(shown()).toEqual(['m1', 'm2'])
  })

  it('passes live frames after ready through on the next flush', () => {
    useConsoleStore.getState().setOpen(true)
    last().send(ready(0))
    last().send(log(1))
    vi.advanceTimersByTime(FLUSH_MS)
    expect(shown()).toEqual(['m1'])
  })

  it('holds frames that arrive before onopen as part of the replay', () => {
    useConsoleStore.getState().setOpen(true)
    last().send(log(1))
    last().send(ready(1))
    vi.advanceTimersByTime(FLUSH_MS)
    expect(shown()).toEqual(['m1'])
    last().onopen?.()
    expect(useConsoleStore.getState().stream).toBe('live')
  })

  it('reconnects from the cursor, and a reset replay replaces what was shown', () => {
    useConsoleStore.getState().setOpen(true)
    last().send(log(1))
    last().send(log(2))
    last().send(ready(2))
    vi.advanceTimersByTime(FLUSH_MS)

    last().onclose?.()
    expect(useConsoleStore.getState().stream).toBe('reconnecting')
    vi.advanceTimersByTime(1000)
    expect(sockets).toHaveLength(2)
    expect(last().path).toBe('/v0/console/logs?level=debug&since=2')

    last().send(log(1))
    last().send(ready(1, true))
    vi.advanceTimersByTime(FLUSH_MS)
    expect(shown()).toEqual(['restarted', 'm1'])
  })

  it('resumes from the cursor when the console is closed and reopened', () => {
    useConsoleStore.getState().setOpen(true)
    last().send(ready(0))
    last().send(log(5))
    useConsoleStore.getState().setOpen(false)
    useConsoleStore.getState().setOpen(true)
    expect(last().path).toBe('/v0/console/logs?level=debug&since=5')
  })
})
