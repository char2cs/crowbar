import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { waitFor } from '@testing-library/react'
import { noteSocketOpened, probeDaemon } from '@/lib/ws/daemon-probe'
import { createWSManager } from '@/lib/ws/manager'
import {
  reportChannelState,
  resetConnectionState,
  useConnectionStore,
} from '@/lib/ws/connection-store'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  onmessage: ((e: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  readyState: number = MockWebSocket.CONNECTING
  send = vi.fn()
  close = vi.fn()
  addEventListener = vi.fn()
  static last: MockWebSocket | null = null
  constructor() {
    MockWebSocket.last = this
  }
  open() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.()
  }
  drop() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.()
  }
}

function health(data: unknown) {
  return { ok: true, status: 200, json: async () => ({ success: true, data }) }
}

const halted = health({
  status: 'halted',
  detail: { reason: 'layout_too_new', message: 'layout 9 is newer than this Crowbar knows' },
})

beforeEach(() => {
  resetConnectionState()
  useDaemonHaltStore.getState().clear()
  // A closed channel, so a migrating report is not ignored as stale.
  reportChannelState('/v0/ws/test', false)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('probeDaemon', () => {
  it('a migrating health read sets the step from detail', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        health({
          status: 'migrating',
          detail: {
            phase: 'pre-open',
            index: 2,
            total: 5,
            step: { number: 7, name: 'chat-ledger' },
          },
        }),
      ),
    )

    await probeDaemon()

    expect(useConnectionStore.getState().status).toBe('migrating')
    expect(useConnectionStore.getState().migrating?.step).toEqual({
      index: 2,
      total: 5,
      name: 'chat-ledger',
    })
  })

  it('a daemon migrating before its first step is starting, not upgrading', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(health({ status: 'migrating', detail: null })))

    await probeDaemon()

    expect(useConnectionStore.getState().migrating).toEqual({ message: 'Crowbar is starting' })
  })

  it('a step the daemon did not name is called by its number', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          health({ status: 'migrating', detail: { index: 1, total: 3, step: { number: 7 } } }),
        ),
    )

    await probeDaemon()

    expect(useConnectionStore.getState().migrating?.step?.name).toBe('step 7')
  })

  it('a halted health read halts the daemon-halt store with its reason', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(halted))

    await probeDaemon()

    expect(useDaemonHaltStore.getState().message).toBe('layout 9 is newer than this Crowbar knows')
  })

  it('one probe at a time: a second call while one is in flight shares it', async () => {
    let answer!: (res: unknown) => void
    const fetchMock = vi.fn(() => new Promise((resolve) => (answer = resolve)))
    vi.stubGlobal('fetch', fetchMock)

    const first = probeDaemon()
    const second = probeDaemon()
    expect(second).toBe(first)
    answer(health({ status: 'ok' }))
    await first

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const next = probeDaemon()
    answer(health({ status: 'ok' }))
    await next
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('an unreachable daemon leaves the state unchanged', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Load failed'))
    vi.stubGlobal('fetch', fetchMock)

    await probeDaemon()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(useConnectionStore.getState().status).toBe('disconnected')
    expect(useDaemonHaltStore.getState().message).toBeNull()
  })

  it('a daemon halted at load reaches the halt store through the first refused socket', async () => {
    vi.stubGlobal('WebSocket', MockWebSocket)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(halted))
    const mgr = createWSManager()
    const unsubscribe = mgr.subscribe('/v0/ws/git', vi.fn())

    MockWebSocket.last!.readyState = MockWebSocket.CLOSED
    MockWebSocket.last!.onclose?.()

    await waitFor(() =>
      expect(useDaemonHaltStore.getState().message).toBe(
        'layout 9 is newer than this Crowbar knows',
      ),
    )
    unsubscribe()
  })

  it.each([
    ['halted', halted],
    ['migrating', health({ status: 'migrating', detail: null })],
  ])('a %s answer sent before a socket opened is ignored after it drops', async (_, late) => {
    vi.stubGlobal('WebSocket', MockWebSocket)
    let answer!: (res: unknown) => void
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise((resolve) => (answer = resolve))),
    )
    const mgr = createWSManager()
    const leaveGit = mgr.subscribe('/v0/ws/git', vi.fn())
    MockWebSocket.last!.drop()
    const inFlight = probeDaemon() // the one the refused socket sent
    const leaveFiles = mgr.subscribe('/v0/ws/files', vi.fn())
    const files = MockWebSocket.last!

    files.open()
    files.drop()
    answer(late)
    await inFlight

    expect(useDaemonHaltStore.getState().message).toBeNull()
    expect(useConnectionStore.getState().status).toBe('disconnected')
    leaveGit()
    leaveFiles()
  })

  it('a read that never answers does not hold a probe sent after a socket opened', async () => {
    const fetchMock = vi
      .fn()
      .mockReturnValueOnce(new Promise(() => {}))
      .mockResolvedValueOnce(halted)
    vi.stubGlobal('fetch', fetchMock)
    void probeDaemon()

    noteSocketOpened()
    await probeDaemon()

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(useDaemonHaltStore.getState().message).toBe('layout 9 is newer than this Crowbar knows')
  })
})
