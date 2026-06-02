import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// Ensure we're in non-Tauri environment (jsdom has no __TAURI_INTERNALS__)
// These tests verify the stub paths.

beforeEach(() => {
  // Make sure __TAURI_INTERNALS__ is absent
  delete (window as unknown as Record<string, unknown>)['__TAURI_INTERNALS__']
})

describe('isTauri (non-Tauri env)', () => {
  it('returns false when __TAURI_INTERNALS__ is absent', async () => {
    const { isTauri } = await import('@/lib/crowbar-bridge')
    expect(isTauri()).toBe(false)
  })
})

describe('browserPane stubs (non-Tauri env)', () => {
  it('browserPaneSync resolves without error', async () => {
    const { browserPaneSync } = await import('@/lib/crowbar-bridge')
    await expect(
      browserPaneSync('buf-1', { x: 0, y: 0, width: 800, height: 600 }, true),
    ).resolves.toBeUndefined()
  })

  it('browserPaneNavigate resolves without error', async () => {
    const { browserPaneNavigate } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneNavigate('buf-1', 'https://example.com')).resolves.toBeUndefined()
  })

  it('browserPaneGoBack resolves without error', async () => {
    const { browserPaneGoBack } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneGoBack('buf-1')).resolves.toBeUndefined()
  })

  it('browserPaneGoForward resolves without error', async () => {
    const { browserPaneGoForward } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneGoForward('buf-1')).resolves.toBeUndefined()
  })

  it('browserPaneReload resolves without error', async () => {
    const { browserPaneReload } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneReload('buf-1')).resolves.toBeUndefined()
  })

  it('browserPaneClose resolves without error', async () => {
    const { browserPaneClose } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneClose('buf-1')).resolves.toBeUndefined()
  })
})

describe('browserPane functions (Tauri env)', () => {
  let invokeMock: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    ;(window as unknown as Record<string, unknown>)['__TAURI_INTERNALS__'] = {}
    // The vite alias points @tauri-apps/api/core to the stub — spy on it
    const mod = await import('@tauri-apps/api/core')
    invokeMock = vi.spyOn(mod, 'invoke') as ReturnType<typeof vi.fn>
  })

  afterEach(() => {
    delete (window as unknown as Record<string, unknown>)['__TAURI_INTERNALS__']
    vi.restoreAllMocks()
  })

  it('browserPaneSync calls invoke with flattened rect args', async () => {
    const { browserPaneSync } = await import('@/lib/crowbar-bridge')
    await browserPaneSync('buf-1', { x: 10, y: 20, width: 800, height: 600 }, true)
    expect(invokeMock).toHaveBeenCalledWith('browser_pane_sync', {
      bufferId: 'buf-1',
      x: 10,
      y: 20,
      width: 800,
      height: 600,
      visible: true,
    })
  })

  it('browserPaneNavigate calls invoke with correct command', async () => {
    const { browserPaneNavigate } = await import('@/lib/crowbar-bridge')
    await browserPaneNavigate('buf-1', 'https://example.com')
    expect(invokeMock).toHaveBeenCalledWith('browser_pane_navigate', {
      bufferId: 'buf-1',
      url: 'https://example.com',
    })
  })
})
