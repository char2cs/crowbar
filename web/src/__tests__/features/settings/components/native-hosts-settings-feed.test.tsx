import { act, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent, NativeHostsSettings } from '@/lib/native-chrome'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import type { Host } from '@/features/hosts/types'

const chrome = vi.hoisted(() => ({
  pushed: [] as NativeHostsSettings[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
  confirm: vi.fn(async (): Promise<boolean> => false),
}))
vi.mock('@/lib/native-chrome', () => ({
  hasNativeChrome: () => true,
  nativeConfirm: chrome.confirm,
  setNativeHostsSettings: (s: NativeHostsSettings) => chrome.pushed.push(s),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))
const navigate = vi.hoisted(() => vi.fn(async () => {}))
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigate,
}))

const { NativeHostsSettingsFeed } =
  await import('@/features/settings/components/native-hosts-settings-feed')

const host = { id: 'h1', name: 'dockerlab', endpoint: 'https://lab:7777', clientId: 'c1' } as Host
const last = () => chrome.pushed.at(-1)!
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
  chrome.confirm.mockClear()
  navigate.mockClear()
  useHostConfigStore.setState({ hosts: [host] })
  useUIState.setState({ isSettingsOpen: true })
})

describe('NativeHostsSettingsFeed', () => {
  it('pushes every paired host with its endpoint and status', () => {
    render(<NativeHostsSettingsFeed />)
    expect(last().hosts).toEqual([
      { id: 'h1', name: 'dockerlab', endpoint: 'https://lab:7777', state: expect.any(String) },
    ])
    expect(last().revoking).toEqual([])
  })

  it('asks natively before revoking, and keeps the host on no', async () => {
    render(<NativeHostsSettingsFeed />)
    send({ type: 'revokeHost', id: 'h1' })
    await waitFor(() =>
      expect(chrome.confirm).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'Revoke dockerlab?', destructive: true }),
      ),
    )
    await waitFor(() => expect(last().revoking).toEqual([]))
    expect(useHostConfigStore.getState().hosts).toEqual([host])
  })

  it('puts Settings away and opens pairing for Add host', () => {
    render(<NativeHostsSettingsFeed />)
    send({ type: 'addHost' })
    expect(useUIState.getState().isSettingsOpen).toBe(false)
    expect(navigate).toHaveBeenCalledWith({ to: '/add-host' })
  })
})
