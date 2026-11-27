import { afterEach, describe, it, expect, vi } from 'vitest'

const IPHONE =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148'
const MAC =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)'

async function loadInTauri(userAgent: string) {
  const invoke = vi.fn(async () => undefined)
  vi.stubGlobal('navigator', { userAgent, maxTouchPoints: 0 })
  ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke }
  vi.resetModules()
  return { invoke, bridge: await import('@/lib/crowbar-bridge') }
}

describe('the desktop window commands', () => {
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    vi.unstubAllGlobals()
    vi.resetModules()
  })

  it('reach the desktop app', async () => {
    const { invoke, bridge } = await loadInTauri(MAC)
    expect(bridge.isDesktopApp()).toBe(true)
    await bridge.setMacOSWindowAppearance('dark')
    expect(invoke).toHaveBeenCalledWith('set_vibrancy_appearance', { dark: true })
  })

  it('are never invoked in the phone app', async () => {
    const { invoke, bridge } = await loadInTauri(IPHONE)
    expect(bridge.isTauri()).toBe(true)
    expect(bridge.isDesktopApp()).toBe(false)
    await bridge.setMacOSWindowAppearance('dark')
    await bridge.revealItemInFinder('/tmp/a')
    expect(invoke).not.toHaveBeenCalled()
  })
})
