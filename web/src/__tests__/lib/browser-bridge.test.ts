import { afterEach, describe, expect, it, vi } from 'vitest'

const host = vi.hoisted(() => ({ desktop: true, invoke: vi.fn(async () => undefined) }))
vi.mock('@tauri-apps/api/core', () => ({ invoke: host.invoke }))
vi.mock('@/lib/crowbar-bridge', () => ({ isDesktopApp: () => host.desktop }))

import { browserApi } from '@/lib/browser-bridge'

const rect = { x: 1, y: 2, width: 3, height: 4 }

afterEach(() => {
  host.desktop = true
  host.invoke.mockClear()
})

describe('browserApi', () => {
  it('drives the native webviews in the desktop app', async () => {
    await browserApi.open('t1', 'https://a.test', rect, 1, true)
    expect(host.invoke).toHaveBeenCalledWith('browser_open', {
      id: 't1',
      url: 'https://a.test',
      zoom: 1,
      parked: true,
      ...rect,
    })
    await browserApi.park('t1')
    expect(host.invoke).toHaveBeenLastCalledWith('browser_park', { id: 't1' })
    await browserApi.unpark('t1', rect)
    expect(host.invoke).toHaveBeenLastCalledWith('browser_unpark', { id: 't1', ...rect })
  })

  // The phone app has no native webview commands: a call is refused, never sent.
  it('refuses every command outside the desktop app', async () => {
    host.desktop = false
    await expect(browserApi.open('t1', 'https://a.test', rect, 1, false)).rejects.toThrow(
      /desktop app/,
    )
    await expect(browserApi.capabilities()).rejects.toThrow(/desktop app/)
    await expect(browserApi.close('t1')).rejects.toThrow(/desktop app/)
    expect(host.invoke).not.toHaveBeenCalled()
  })
})
