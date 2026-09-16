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

describe('useMacTrafficLightSync', () => {
  let invoke: ReturnType<typeof vi.fn>

  beforeEach(() => {
    invoke = vi.fn().mockResolvedValue(undefined)
    // isTauri() checks '__TAURI_INTERNALS__' in window; tauriInvoke calls its invoke.
    ;(window as unknown as { __TAURI_INTERNALS__: unknown }).__TAURI_INTERNALS__ = { invoke }
  })

  afterEach(() => {
    delete (window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('sidebar-left restores the exact static config value, even with no pane row mounted', async () => {
    renderHook(() => useMacTrafficLightSync('left'))
    await act(async () => {})

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 23 })
  })

  it("sidebar-right translates the static value by the top-left row's own live offset", async () => {
    addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })

    renderHook(() => useMacTrafficLightSync('right'))
    await act(async () => {})

    // Static (12, 23) tuned for a rect.top=0 row; this row sits 10px lower,
    // so y translates by that same amount — x is unaffected since left is 0.
    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  it('sidebar-right with no row at the true top-left corner makes no call', async () => {
    // Present, but nowhere near the window's top-left corner.
    addPaneTopRow({ left: 500, top: 300, width: 400, height: 44 })

    renderHook(() => useMacTrafficLightSync('right'))
    await act(async () => {})

    expect(invoke).not.toHaveBeenCalled()
  })

  it('re-applies on window resize', async () => {
    addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })
    renderHook(() => useMacTrafficLightSync('right'))
    await act(async () => {})
    invoke.mockClear()

    await act(async () => {
      window.dispatchEvent(new Event('resize'))
    })

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  it('retries once a pane-top-row mounts after cold boot, with no row present yet', async () => {
    // Cold-boot regression: sidebarPosition can rehydrate to 'right' before
    // the pane tree has mounted anything at all.
    renderHook(() => useMacTrafficLightSync('right'))
    await act(async () => {})

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

    renderHook(() => useMacTrafficLightSync('right'))
    await act(async () => {})

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 33 })
  })

  it('re-applies when sidebarPosition flips from right back to left', async () => {
    addPaneTopRow({ left: 0, top: 10, width: 400, height: 44 })
    const { rerender } = renderHook(({ side }) => useMacTrafficLightSync(side), {
      initialProps: { side: 'right' as 'left' | 'right' },
    })
    await act(async () => {})
    invoke.mockClear()

    rerender({ side: 'left' })
    await act(async () => {})

    expect(invoke).toHaveBeenCalledWith('set_traffic_light_position', { x: 12, y: 23 })
  })
})
