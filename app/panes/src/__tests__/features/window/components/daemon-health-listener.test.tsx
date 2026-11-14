import { act, render, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { DaemonHealthListener } from '@/features/window/components/daemon-health-listener'
import { useDaemonCrashStore } from '@/features/window/stores/daemon-crash-store'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { toast } from '@/features/window/stores/toast-store'
import { reportChannelState, resetConnectionState } from '@/lib/ws/connection-store'
import { watchToasts } from '@/__tests__/__fixtures__/reports'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    info: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
  },
}))

const handlers = new Map<string, (event: { payload: unknown }) => void>()
const unlistenSpies: Array<ReturnType<typeof vi.fn>> = []

vi.mock('@tauri-apps/api/event', () => ({
  listen: vi.fn(async (name: string, handler: (event: { payload: unknown }) => void) => {
    handlers.set(name, handler)
    const unlisten = vi.fn()
    unlistenSpies.push(unlisten)
    return unlisten
  }),
}))

type TauriWindow = Window & {
  __TAURI_INTERNALS__?: { invoke?: (cmd: string) => Promise<unknown> }
}
// What the supervisor answers `daemon_crash_report` with when the window loads.
let crashReportAtLoad: string | null = null
const tauriWindow = window as TauriWindow

describe('DaemonHealthListener', () => {
  beforeEach(() => {
    handlers.clear()
    unlistenSpies.length = 0
    vi.clearAllMocks()
    useDaemonHaltStore.getState().clear()
    useDaemonCrashStore.getState().clear()
    crashReportAtLoad = null
    // isTauri() keys off this global; the listener must be active in Tauri.
    tauriWindow.__TAURI_INTERNALS__ = {
      invoke: async (cmd) => (cmd === 'daemon_crash_report' ? crashReportAtLoad : undefined),
    }
  })
  afterEach(() => {
    delete tauriWindow.__TAURI_INTERNALS__
  })

  it('toasts daemon lifecycle events with the right severity', async () => {
    render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(4))

    handlers.get('daemon:unhealthy')!({ payload: null })
    expect(toast.warning).toHaveBeenCalledWith(
      expect.stringMatching(/unresponsive/i),
      expect.any(String),
    )

    handlers.get('daemon:restarted')!({ payload: null })
    expect(toast.success).toHaveBeenCalledWith(
      expect.stringMatching(/restarted/i),
      expect.any(String),
    )
  })

  it('drops a recorded crash loop as soon as the backend is serving again', async () => {
    resetConnectionState()
    render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(4))

    handlers.get('daemon:dead')!({ payload: 'daemon already running' })
    expect(useDaemonCrashStore.getState().report).toBe('daemon already running')

    act(() => reportChannelState('/ws/feed', true, 'local'))
    expect(useDaemonCrashStore.getState().report).toBeNull()
    resetConnectionState()
  })

  it('does not record a crash loop that was reported after the backend was already serving', async () => {
    resetConnectionState()
    act(() => reportChannelState('/ws/feed', true, 'local'))
    crashReportAtLoad = 'exited 3 times'
    render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(4))
    handlers.get('daemon:dead')!({ payload: 'late' })
    expect(useDaemonCrashStore.getState().report).toBeNull()
    resetConnectionState()
  })

  it('records a crash loop the supervisor gives up on, and clears it on restart', async () => {
    render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(4))

    handlers.get('daemon:dead')!({ payload: 'panic: boom' })
    expect(useDaemonCrashStore.getState().report).toBe('panic: boom')
    expect(toast.error).not.toHaveBeenCalled()

    handlers.get('daemon:restarted')!({ payload: null })
    expect(useDaemonCrashStore.getState().report).toBeNull()
  })

  it('asks for a crash loop that began before the window loaded', async () => {
    crashReportAtLoad = 'exited 3 times'
    render(<DaemonHealthListener />)
    await waitFor(() => expect(useDaemonCrashStore.getState().report).toBe('exited 3 times'))
  })

  it('reports, without claiming a crash, a supervisor that cannot be asked for a crash loop', async () => {
    const add = watchToasts()
    // Tauri rejects a failed command with the shell's own string, not an Error.
    tauriWindow.__TAURI_INTERNALS__ = {
      invoke: () => Promise.reject('command daemon_crash_report not found'),
    }
    render(<DaemonHealthListener />)
    await waitFor(() =>
      expect(add).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'error', title: expect.stringMatching(/crash-looping/) }),
      ),
    )
    expect(useDaemonCrashStore.getState().report).toBeNull()
  })

  it('records a halt the supervisor announces, and clears it when the daemon is restarted', async () => {
    render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(4))

    handlers.get('daemon:halted')!({ payload: 'layout 9 is newer than this Crowbar knows' })
    expect(useDaemonHaltStore.getState().message).toBe('layout 9 is newer than this Crowbar knows')

    handlers.get('daemon:restarted')!({ payload: null })
    expect(useDaemonHaltStore.getState().message).toBeNull()
  })

  it('unsubscribes every listener on unmount', async () => {
    const { unmount } = render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(4))

    unmount()
    await waitFor(() => {
      for (const unlisten of unlistenSpies) expect(unlisten).toHaveBeenCalled()
    })
  })
})
