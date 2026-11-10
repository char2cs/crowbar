import { afterEach, describe, it, expect, vi } from 'vitest'

// The terminal transport is covered by crowbar-bridge-terminal.test.ts.
const IPHONE =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148'
const MAC =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)'

// The platform is read once at module load, so each case loads the bridge afresh.
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
    await bridge.setTrafficLightPosition(12, 33)
    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  // The shell destroys a closing window's page without `pagehide`, which drafts and held removals
  // flush on; the page must see it before the window goes.
  it('gives the page its pagehide before a closing desktop window is destroyed', async () => {
    const { invoke, bridge } = await loadInTauri(MAC)
    const callbacks: Array<(event: unknown) => unknown> = []
    const internals = window as unknown as Record<string, Record<string, unknown>>
    internals.__TAURI_INTERNALS__.metadata = { currentWindow: { label: 'main' } }
    internals.__TAURI_INTERNALS__.transformCallback = (cb: never) => callbacks.push(cb) - 1
    const seen: string[] = []
    const onPagehide = () => seen.push('pagehide')
    window.addEventListener('pagehide', onPagehide)
    invoke.mockImplementation(async (...args: unknown[]) => seen.push(String(args[0])) as never)

    await bridge.hidePageOnWindowClose()
    const listen = invoke.mock.calls.find((call: unknown[]) => call[0] === 'plugin:event|listen')
    const { event, handler } = (listen as unknown[])[1] as { event: string; handler: number }
    seen.length = 0
    await callbacks[handler]({ event, id: 7, payload: null })
    window.removeEventListener('pagehide', onPagehide)

    expect(event).toBe('tauri://close-requested')
    expect(seen).toEqual(['pagehide', 'plugin:window|destroy'])
  })

  // The phone app is Tauri too, but has no desktop window: those commands do not exist there.
  it('are never invoked in the phone app', async () => {
    const { invoke, bridge } = await loadInTauri(IPHONE)
    expect(bridge.isTauri()).toBe(true)
    expect(bridge.isDesktopApp()).toBe(false)
    await bridge.setTrafficLightPosition(12, 33)
    await bridge.setMacOSWindowAppearance('dark')
    await bridge.setConsoleMenuChord('cmd+`')
    await bridge.revealItemInFinder('/tmp/a')
    const unlisten = await bridge.onConsoleMenuToggle(() => {})
    unlisten()
    ;(await bridge.hidePageOnWindowClose())()
    expect(invoke).not.toHaveBeenCalled()
  })
})
