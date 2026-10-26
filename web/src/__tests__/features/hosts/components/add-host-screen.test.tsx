import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import type { NativeChrome, NativeChromeIntent } from '@/lib/native-chrome'

vi.mock('@/features/hosts/lib/pairing', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/hosts/lib/pairing')>()),
  redeemPairingCode: vi.fn(),
}))
vi.mock('@/lib/crowbar-bridge', () => ({ probeRemoteFingerprint: vi.fn() }))
const chrome = vi.hoisted(() => ({
  models: [] as Partial<NativeChrome>[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
}))
vi.mock('@/lib/native-chrome', () => ({
  setNativeChrome: (model: Partial<NativeChrome>) => chrome.models.push(model),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))

import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { probeRemoteFingerprint } from '@/lib/crowbar-bridge'
import { AddHostScreen } from '@/features/hosts/components/add-host-screen'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

const shown = () => chrome.models.at(-1)!
const sheet = () => shown().connect!
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))
const brand = () => screen.getByRole('img', { name: 'Crowbar' }).parentElement!

beforeEach(() => {
  vi.mocked(probeRemoteFingerprint).mockReset()
  vi.mocked(redeemPairingCode).mockReset()
  useHostConfigStore.setState({ hosts: [] })
  chrome.models.length = 0
  chrome.listeners.clear()
})

describe('AddHostScreen', () => {
  // The web draws the brand; the way in is the native glass button over it.
  it('welcomes with the wordmark and tagline over the native start button', () => {
    render(<AddHostScreen onPaired={vi.fn()} />)
    expect(screen.getByRole('img', { name: 'Crowbar' })).toBeInTheDocument()
    expect(screen.getByText(/the ide where agents do the heavy lifting/i)).toBeInTheDocument()
    expect(shown().welcome).toEqual({ visible: true, label: 'Let’s build together' })
    expect(sheet().open).toBe(false)
    expect(brand()).toHaveAttribute('data-state', 'closed')
  })

  it('opens the connect sheet on start, and the brand moves up for it', async () => {
    render(<AddHostScreen onPaired={vi.fn()} />)
    await send({ type: 'start' })
    expect(sheet()).toMatchObject({ open: true, stage: 'address', busy: false, error: null })
    expect(shown().welcome?.visible).toBe(false)
    expect(brand()).toHaveAttribute('data-state', 'open')
  })

  // Before any host there is no shell: the welcome itself hands the sheet the theme's colours.
  it('paints the connect sheet from the theme’s palette', () => {
    document.documentElement.style.setProperty('--background', '#1c1c1a')
    render(<AddHostScreen onPaired={vi.fn()} />)
    expect(shown().palette?.surface).toBe('#1c1c1a')
  })

  it('pairs a host through the sheet’s stages and reports it', async () => {
    let answer!: (fingerprint: string) => void
    vi.mocked(probeRemoteFingerprint).mockReturnValueOnce(new Promise((r) => (answer = r)))
    vi.mocked(redeemPairingCode).mockResolvedValueOnce(undefined)
    const onPaired = vi.fn()
    render(<AddHostScreen onPaired={onPaired} />)
    await send({ type: 'start' })

    await send({ type: 'submitAddress', name: 'homelab', address: '1.2.3.4:3737' })
    expect(sheet()).toMatchObject({ busy: true, name: 'homelab', address: '1.2.3.4:3737' })
    await act(async () => answer('SHA256:aaaa'))
    expect(sheet()).toMatchObject({ stage: 'fingerprint', fingerprint: 'SHA256:aaaa', busy: false })

    await send({ type: 'confirmFingerprint' })
    expect(sheet().stage).toBe('code')
    await send({ type: 'submitCode', code: '123456' })

    await waitFor(() => expect(onPaired).toHaveBeenCalledWith(expect.any(String)))
    expect(redeemPairingCode).toHaveBeenCalledWith(
      onPaired.mock.calls[0][0],
      '1.2.3.4:3737',
      'SHA256:aaaa',
      '123456',
      'homelab',
    )
    expect(sheet().open).toBe(false)
  })

  it('says in the sheet why an address could not be reached, staying on it', async () => {
    vi.mocked(probeRemoteFingerprint).mockRejectedValueOnce('cannot reach 1.2.3.4:3737: refused')
    render(<AddHostScreen onPaired={vi.fn()} />)
    await send({ type: 'start' })
    await send({ type: 'submitAddress', name: 'homelab', address: '1.2.3.4:3737' })

    await waitFor(() => expect(sheet().error).toMatch(/^Can’t reach 1\.2\.3\.4:3737\./))
    expect(sheet()).toMatchObject({ stage: 'address', busy: false })
  })

  it('turns away a code that is not six digits without dialing the host', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    render(<AddHostScreen onPaired={vi.fn()} />)
    await send({ type: 'start' })
    await send({ type: 'submitAddress', name: 'homelab', address: '1.2.3.4:3737' })
    await waitFor(() => expect(sheet().stage).toBe('fingerprint'))
    await send({ type: 'confirmFingerprint' })

    await send({ type: 'submitCode', code: '123' })

    expect(sheet().error).toBe('A pairing code is exactly 6 digits.')
    expect(redeemPairingCode).not.toHaveBeenCalled()
  })

  // A dismissed attempt is over: its confirmed fingerprint never reaches the next.
  it('settles back on dismiss and starts the next attempt afresh', async () => {
    vi.mocked(probeRemoteFingerprint).mockResolvedValueOnce('SHA256:aaaa')
    render(<AddHostScreen onPaired={vi.fn()} />)
    await send({ type: 'start' })
    await send({ type: 'submitAddress', name: 'homelab', address: '1.2.3.4:3737' })
    await waitFor(() => expect(sheet().stage).toBe('fingerprint'))

    await send({ type: 'dismiss' })
    expect(sheet().open).toBe(false)
    expect(brand()).toHaveAttribute('data-state', 'closed')

    await send({ type: 'start' })
    expect(sheet()).toMatchObject({ open: true, stage: 'address', name: '', fingerprint: '' })
  })

  // Opened from the app to add another host: straight to the sheet, and dismissing goes back.
  it('opens on the sheet and cancels back when there is somewhere to return to', async () => {
    const onCancel = vi.fn()
    render(<AddHostScreen onPaired={vi.fn()} onCancel={onCancel} />)
    expect(sheet().open).toBe(true)

    await send({ type: 'dismiss' })
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it('takes the native chrome down with it', () => {
    const { unmount } = render(<AddHostScreen onPaired={vi.fn()} />)
    unmount()
    expect(shown()).toEqual({})
    expect(chrome.listeners.size).toBe(0)
  })
})
