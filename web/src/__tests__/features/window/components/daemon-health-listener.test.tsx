import { render, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { DaemonHealthListener } from '@/features/window/components/daemon-health-listener'
import { toast } from '@/features/window/stores/toast-store'

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

type TauriWindow = Window & { __TAURI_INTERNALS__?: object }
const tauriWindow = window as TauriWindow

describe('DaemonHealthListener', () => {
  beforeEach(() => {
    handlers.clear()
    unlistenSpies.length = 0
    vi.clearAllMocks()
    // isTauri() keys off this global; the listener must be active in Tauri.
    tauriWindow.__TAURI_INTERNALS__ = {}
  })
  afterEach(() => {
    delete tauriWindow.__TAURI_INTERNALS__
  })

  it('toasts daemon lifecycle events with the right severity', async () => {
    render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(3))

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

    handlers.get('daemon:dead')!({ payload: null })
    expect(toast.error).toHaveBeenCalledWith(
      expect.stringMatching(/backend/i),
      expect.stringMatching(/diagnostics/i),
    )
  })

  it('unsubscribes every listener on unmount', async () => {
    const { unmount } = render(<DaemonHealthListener />)
    await waitFor(() => expect(handlers.size).toBe(3))

    unmount()
    await waitFor(() => {
      for (const unlisten of unlistenSpies) expect(unlisten).toHaveBeenCalled()
    })
  })

  it('does nothing outside the desktop app', async () => {
    delete tauriWindow.__TAURI_INTERNALS__
    render(<DaemonHealthListener />)

    // Give any wrongly-registered listener a tick to appear.
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(handlers.size).toBe(0)
  })
})
