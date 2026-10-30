import { act, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent, NativeHostPicker } from '@/lib/native-chrome'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import type { Host } from '@/features/hosts/types'

const chrome = vi.hoisted(() => ({
  pushed: [] as NativeHostPicker[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
  confirm: vi.fn(async (): Promise<boolean> => false),
}))
vi.mock('@/lib/native-chrome', () => ({
  hasNativeChrome: () => true,
  nativeConfirm: chrome.confirm,
  setNativeHostPicker: (p: NativeHostPicker) => chrome.pushed.push(p),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))

const { NativeHostPickerFeed } = await import('@/features/hosts/components/native-host-picker-feed')

const host = { id: 'h1', name: 'dockerlab', endpoint: 'https://lab:7777', clientId: 'c1' } as Host
const last = () => chrome.pushed.at(-1)!
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
  chrome.confirm.mockClear()
  useHostConfigStore.setState({ hosts: [host] })
  useUIState.setState({ isHostPickerOpen: true })
})

describe('NativeHostPickerFeed', () => {
  it('opens the sheet with every paired host, its status and the shown one', () => {
    render(<NativeHostPickerFeed shownHost="h1" />)
    expect(last()).toEqual({
      open: true,
      hosts: [
        { id: 'h1', name: 'dockerlab', endpoint: 'https://lab:7777', state: expect.any(String) },
      ],
      revoking: [],
      shownHost: 'h1',
    })
  })

  it('closes the sheet when it unmounts', () => {
    const { unmount } = render(<NativeHostPickerFeed shownHost={null} />)
    unmount()
    expect(last().open).toBe(false)
  })

  it('closes the picker once a host is picked, or the sheet is dismissed', () => {
    render(<NativeHostPickerFeed shownHost={null} />)
    send({ type: 'selectHost', id: 'h1' })
    expect(useUIState.getState().isHostPickerOpen).toBe(false)
    useUIState.setState({ isHostPickerOpen: true })
    send({ type: 'closeHostPicker' })
    expect(useUIState.getState().isHostPickerOpen).toBe(false)
  })

  it('asks natively before revoking, and keeps the host on no', async () => {
    render(<NativeHostPickerFeed shownHost={null} />)
    send({ type: 'revokeHost', id: 'h1' })
    await waitFor(() =>
      expect(chrome.confirm).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'Revoke dockerlab?', destructive: true }),
      ),
    )
    await waitFor(() => expect(last().revoking).toEqual([]))
    expect(useHostConfigStore.getState().hosts).toEqual([host])
    expect(useUIState.getState().isHostPickerOpen).toBe(true)
  })
})
