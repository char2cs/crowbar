import { afterEach, describe, it, expect, vi } from 'vitest'
import {
  setWindowTransparency,
  setMacOSWindowAppearance,
  setTrafficLightPosition,
} from '@/lib/crowbar-bridge'

// The terminal transport is covered by crowbar-bridge-terminal.test.ts.
describe('crowbar-bridge', () => {
  it('setWindowTransparency resolves without error', async () => {
    await expect(setWindowTransparency(true)).resolves.toBeUndefined()
  })

  it('setMacOSWindowAppearance resolves without error', async () => {
    await expect(setMacOSWindowAppearance('dark', false)).resolves.toBeUndefined()
  })

  it('setTrafficLightPosition resolves without error', async () => {
    await expect(setTrafficLightPosition(12, 33)).resolves.toBeUndefined()
  })
})

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

  // The phone app is Tauri too, but has no desktop window: those commands do not exist there.
  it('are never invoked in the phone app', async () => {
    const { invoke, bridge } = await loadInTauri(IPHONE)
    expect(bridge.isTauri()).toBe(true)
    expect(bridge.isDesktopApp()).toBe(false)
    await bridge.setTrafficLightPosition(12, 33)
    await bridge.setMacOSWindowAppearance('dark', false)
    await bridge.setConsoleMenuChord('cmd+`')
    await bridge.revealItemInFinder('/tmp/a')
    const unlisten = await bridge.onConsoleMenuToggle(() => {})
    unlisten()
    expect(invoke).not.toHaveBeenCalled()
  })
})
