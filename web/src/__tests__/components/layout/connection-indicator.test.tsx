import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { show: vi.fn(), dismissByKey: vi.fn() },
}))

import { toast } from '@/features/window/stores/toast-store'
import {
  useConnectionStore,
  resetConnectionState,
  reportChannelState,
  reportMigrating,
} from '@/lib/ws/connection-store'
import { ConnectionIndicator } from '@/components/layout/connection-indicator'
import { useDaemonCrashStore } from '@/features/window/stores/daemon-crash-store'

const SHOW_DELAY_MS = 500

function shown() {
  return vi.mocked(toast.show).mock.calls.map(([opts]) => opts)
}

beforeEach(() => {
  vi.clearAllMocks()
  resetConnectionState()
  useDaemonCrashStore.getState().clear()
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('ConnectionIndicator', () => {
  it('never says "Reconnecting" in a crash loop, where nothing reconnects', () => {
    render(<ConnectionIndicator />)
    act(() => useConnectionStore.setState({ status: 'disconnected' }))
    act(() => {
      vi.advanceTimersByTime(SHOW_DELAY_MS)
    })
    expect(shown()).toHaveLength(1)

    act(() => useDaemonCrashStore.getState().crashed('panic: boom'))
    expect(toast.dismissByKey).toHaveBeenCalledWith('connection-indicator')
    act(() => {
      vi.advanceTimersByTime(SHOW_DELAY_MS * 10)
    })
    expect(shown()).toHaveLength(1)
  })

  it('renders nothing', () => {
    const { container } = render(<ConnectionIndicator />)
    expect(container).toBeEmptyDOMElement()
  })

  it('fires a persistent warning toast once the disconnect outlasts the debounce', () => {
    render(<ConnectionIndicator />)

    act(() => {
      useConnectionStore.setState({ status: 'disconnected' })
    })
    expect(toast.show).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(SHOW_DELAY_MS)
    })

    expect(toast.show).toHaveBeenCalledTimes(1)
    const arg = (toast.show as unknown as { mock: { calls: unknown[][] } }).mock.calls[0][0] as {
      type: string
      key: string
      duration?: number
    }
    expect(arg.type).toBe('warning')
    expect(arg.key).toBe('connection-indicator')
    // duration: 0 is how this toast opts out of auto-dismiss (see
    // ToastStore.addToast, which only schedules a timer when duration > 0) —
    // it must stay up for as long as the outage lasts.
    expect(arg.duration).toBe(0)
  })

  it('does not fire for a blip shorter than the debounce', () => {
    render(<ConnectionIndicator />)

    act(() => {
      useConnectionStore.setState({ status: 'disconnected' })
    })
    act(() => {
      vi.advanceTimersByTime(SHOW_DELAY_MS - 100)
    })
    act(() => {
      useConnectionStore.setState({ status: 'connected' })
    })
    act(() => {
      vi.advanceTimersByTime(SHOW_DELAY_MS)
    })

    expect(toast.show).not.toHaveBeenCalled()
    expect(toast.dismissByKey).not.toHaveBeenCalled()
  })

  it('dismisses the warning and confirms recovery once a channel reconnects', () => {
    render(<ConnectionIndicator />)

    act(() => {
      useConnectionStore.setState({ status: 'disconnected' })
    })
    act(() => {
      vi.advanceTimersByTime(SHOW_DELAY_MS)
    })
    expect(toast.show).toHaveBeenCalledTimes(1)

    act(() => {
      useConnectionStore.setState({ status: 'connected' })
    })

    expect(toast.dismissByKey).toHaveBeenCalledWith('connection-indicator')
    expect(toast.show).toHaveBeenCalledTimes(2)
    const successArg = (toast.show as unknown as { mock: { calls: unknown[][] } }).mock
      .calls[1][0] as { type: string; key: string }
    expect(successArg.type).toBe('success')
    expect(successArg.key).toBe('connection-indicator')
  })

  it('does not stack duplicate toasts across repeated reconnect attempts', () => {
    render(<ConnectionIndicator />)

    for (let i = 0; i < 3; i++) {
      act(() => {
        useConnectionStore.setState({ status: 'disconnected' })
      })
      act(() => {
        vi.advanceTimersByTime(SHOW_DELAY_MS)
      })
      act(() => {
        useConnectionStore.setState({ status: 'connected' })
      })
    }

    // Every warning (and every recovery) reuses the same dedup key, so the
    // toast system updates/replaces in place instead of the caller ever
    // needing to track more than one live toast at a time.
    const keysUsed = new Set(
      (toast.show as unknown as { mock: { calls: unknown[][] } }).mock.calls.map(
        (call) => (call[0] as { key: string }).key,
      ),
    )
    expect(keysUsed).toEqual(new Set(['connection-indicator']))
    expect(toast.show).toHaveBeenCalledTimes(6) // 3x warning + 3x success
  })

  it('a migrating daemon shows its step instead of Reconnecting', () => {
    render(<ConnectionIndicator />)

    act(() => {
      reportChannelState('/v0/ws/git', false, 'local')
      reportMigrating({
        message: 'Crowbar is upgrading its data (step 2 of 5: chat-ledger)',
        step: { index: 2, total: 5, name: 'chat-ledger' },
      })
    })
    act(() => {
      vi.advanceTimersByTime(SHOW_DELAY_MS)
    })

    expect(shown()).toEqual([
      expect.objectContaining({
        message: 'Crowbar is upgrading its data',
        description: 'Step 2 of 5: chat-ledger',
        key: 'connection-indicator',
        duration: 0,
      }),
    ])
  })

  it('the upgrade toast gives way to Reconnected when a socket opens', () => {
    render(<ConnectionIndicator />)
    act(() => {
      reportChannelState('/v0/ws/git', false, 'local')
      reportMigrating({ message: 'm', step: { index: 1, total: 2, name: 'chat-ledger' } })
    })

    act(() => {
      reportChannelState('/v0/ws/git', true, 'local')
    })

    expect(toast.dismissByKey).toHaveBeenCalledWith('connection-indicator')
    expect(shown().at(-1)).toEqual(
      expect.objectContaining({ message: 'Reconnected', key: 'connection-indicator' }),
    )
  })

  it('a daemon starting before its first step says Crowbar is starting', () => {
    render(<ConnectionIndicator />)

    act(() => {
      reportMigrating({ message: 'Crowbar is starting' })
    })

    expect(shown()).toEqual([
      expect.objectContaining({ message: 'Crowbar is starting', description: undefined }),
    ])
  })
})
