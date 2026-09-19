import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useMacTrafficLightSync } from '@/features/tabs/hooks/use-mac-traffic-light-sync'

function stubRect(el: HTMLElement, rect: Partial<DOMRect>) {
  el.getBoundingClientRect = () =>
    ({
      left: 0,
      top: 0,
      right: 0,
      bottom: 0,
      width: 0,
      height: 0,
      x: 0,
      y: 0,
      toJSON() {},
      ...rect,
    }) as DOMRect
}

function addPaneTopRow(rect: Partial<DOMRect>): HTMLElement {
  const el = document.createElement('div')
  el.dataset.testid = 'pane-top-row'
  stubRect(el, rect)
  document.body.appendChild(el)
  return el
}

// rAF is stubbed and driven by hand — no timers, no real waiting (same
// pattern as ascii-crowbar.test.tsx's rAF stub).
let pendingFrames: Map<number, FrameRequestCallback>
let nextFrameId: number

function flushFrame() {
  const due = [...pendingFrames.entries()]
  pendingFrames.clear()
  for (const [, cb] of due) cb(0)
}

/** The hook waits two frames before its first sidebar-right measurement. */
function flushTwoFrames() {
  act(() => flushFrame())
  act(() => flushFrame())
}

// A controllable OS-level `(prefers-color-scheme: dark)` — the same shape
// useMediaQuery's useSyncExternalStore subscribes to, with a real dispatchable
// 'change' event standing in for macOS auto dark/light or a manual flip of
// System Settings' own appearance.
let systemPrefersDark: boolean
let systemChangeListeners: Set<(event: { matches: boolean }) => void>

function setSystemPrefersDark(next: boolean) {
  systemPrefersDark = next
  for (const cb of [...systemChangeListeners]) cb({ matches: next })
}

describe('useMacTrafficLightSync', () => {
  let invoke: ReturnType<typeof vi.fn>

  beforeEach(() => {
    pendingFrames = new Map()
    nextFrameId = 1
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      const id = nextFrameId++
      pendingFrames.set(id, cb)
      return id
    })
    vi.stubGlobal('cancelAnimationFrame', (id: number) => {
      pendingFrames.delete(id)
    })

    systemPrefersDark = false
    systemChangeListeners = new Set()
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        get matches() {
          return systemPrefersDark
        },
        media: query,
        onchange: null,
        addListener: (cb: (event: { matches: boolean }) => void) => systemChangeListeners.add(cb),
        removeListener: (cb: (event: { matches: boolean }) => void) =>
          systemChangeListeners.delete(cb),
        addEventListener: (_type: string, cb: (event: { matches: boolean }) => void) =>
          systemChangeListeners.add(cb),
        removeEventListener: (_type: string, cb: (event: { matches: boolean }) => void) =>
          systemChangeListeners.delete(cb),
        dispatchEvent: () => false,
      })),
    )

    invoke = vi.fn().mockResolvedValue(undefined)
    // isTauri() checks '__TAURI_INTERNALS__' in window; tauriInvoke calls its invoke.
    ;(window as unknown as { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = { invoke }
  })

  afterEach(() => {
    delete (window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
    document.body.innerHTML = ''
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('sidebar-left restores the exact static config value, even with no pane row mounted', async () => {
    renderHook(() => useMacTrafficLightSync('left', 'crowbar:dark'))
    await act(async () => {})

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 23 })
  })

  it("sidebar-right translates the static value by the top-left row's own live offset", async () => {
    addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })

    renderHook(() => useMacTrafficLightSync('right', 'crowbar:dark'))
    flushTwoFrames()

    // Static (12, 23) tuned for a rect.top=0 row; this row sits 10px lower,
    // so y translates by that same amount — x is unaffected since left is 0.
    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  it('sidebar-right with no row at the true top-left corner makes no call', async () => {
    // Present, but nowhere near the window's top-left corner.
    addPaneTopRow({ left: 500, top: 300, width: 400, height: 44 })

    renderHook(() => useMacTrafficLightSync('right', 'crowbar:dark'))
    flushTwoFrames()

    expect(invoke).not.toHaveBeenCalled()
  })

  it('re-applies on window resize', async () => {
    addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })
    renderHook(() => useMacTrafficLightSync('right', 'crowbar:dark'))
    flushTwoFrames()
    invoke.mockClear()

    await act(async () => {
      window.dispatchEvent(new Event('resize'))
    })

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  it('retries once a pane-top-row mounts after cold boot, with no row present yet', async () => {
    // Cold-boot regression: sidebarPosition can rehydrate to 'right' before
    // the pane tree (its own async mount) has put a pane-top-row at the
    // top-left corner.
    renderHook(() => useMacTrafficLightSync('right', 'crowbar:dark'))
    flushTwoFrames()

    expect(invoke).not.toHaveBeenCalled()

    await act(async () => {
      addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })
    })

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  it('skips a hidden, kept-mounted pane-top-row (zero rect) that sorts before the real visible one', async () => {
    // WorkspaceHost keeps recently-visited workspaces mounted but hidden —
    // their pane-top-row still matches the selector, with a getBoundingClientRect
    // that degenerates to all-zero. That trivially satisfies "< EDGE_THRESHOLD"
    // for both axes, so without a size check it wins the very first iteration
    // over the real visible row that comes after it in DOM order.
    addPaneTopRow({ left: 0, top: 0, width: 0, height: 0 })
    addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })

    renderHook(() => useMacTrafficLightSync('right', 'crowbar:dark'))
    flushTwoFrames()

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  it('re-applies when sidebarPosition flips from right back to left', async () => {
    addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })
    const { rerender } = renderHook(({ side }) => useMacTrafficLightSync(side, 'crowbar:dark'), {
      initialProps: { side: 'right' as 'left' | 'right' },
    })
    flushTwoFrames()
    invoke.mockClear()

    rerender({ side: 'left' })
    await act(async () => {})

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 23 })
  })

  it('re-applies the same position after a theme switch, undoing the native reset', async () => {
    // Regression: switching Theme Mode pins the vibrancy view's NSAppearance
    // (set_vibrancy_appearance), and AppKit resets the standard button frames
    // as a side effect. Without themeKey in the dependency array, this effect
    // never re-ran and the traffic lights stayed stuck at their OS default.
    const { rerender } = renderHook(({ themeKey }) => useMacTrafficLightSync('left', themeKey), {
      initialProps: { themeKey: 'crowbar:system' },
    })
    await act(async () => {})
    invoke.mockClear()

    rerender({ themeKey: 'crowbar:dark' })
    await act(async () => {})

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 23 })
  })

  it('re-applies when the OS itself flips appearance while the app theme mode stays "system"', async () => {
    // Regression: themeKey is derived only from useSettingsStore's own
    // theme/themeMode fields. When themeMode is literally "system" it never
    // changes string value across an OS-level flip (macOS auto dark/light, or
    // the user flipping System Settings' own appearance directly) — so
    // themeKey alone can never re-trigger this effect for that case, even
    // though settings-effects.ts still re-applies the theme (and resets the
    // native buttons) when the OS signal fires.
    renderHook(() => useMacTrafficLightSync('left', 'crowbar:system'))
    await act(async () => {})
    invoke.mockClear()

    act(() => setSystemPrefersDark(true))
    await act(async () => {})

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 23 })
  })

  it("sidebar-right waits two frames before its first read, so a reflow still settling from a theme switch isn't measured stale", async () => {
    const row = addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })

    renderHook(() => useMacTrafficLightSync('right', 'crowbar:dark'))
    await act(async () => {})

    // Not applied yet — still waiting on the two deferred frames, so a
    // measurement taken synchronously at mount can't have happened.
    expect(invoke).not.toHaveBeenCalled()

    // Simulate the theme switch's own reflow (e.g. a border-width change)
    // still resizing the row's box between the effect firing and the
    // deferred read.
    stubRect(row, { left: 0, top: 15, width: 400, height: 44 })

    flushTwoFrames()

    // Picks up the settled rect read at the deferred frame (y translates by
    // 15, not the pre-switch 10 it would have read if measured immediately).
    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 38 })
  })
})
