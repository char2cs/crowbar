import { act, render, renderHook as renderHookIn } from '@testing-library/react'
import { DialogContent, DialogPrimitive } from '@/components/ui/dialog'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useNativeOverlayActive } from '@/features/browser/hooks/use-native-overlay-active'

/** The app's real root is the one body child the watcher ignores. */
function renderHook<T>(callback: () => T) {
  const container = document.createElement('div')
  container.id = 'root'
  document.body.appendChild(container)
  return renderHookIn(callback, { container })
}

/** The node itself becomes the body child, as library helpers do. */
function appendNodeToBody(html: string): HTMLElement {
  const template = document.createElement('template')
  template.innerHTML = html
  const node = template.content.firstElementChild as HTMLElement
  document.body.appendChild(node)
  return node
}

function appendToBody(html: string): HTMLElement {
  const el = document.createElement('div')
  el.innerHTML = html
  document.body.appendChild(el)
  return el
}

const DIALOG = '<div data-slot="dialog-viewport"><div data-slot="dialog-popup"></div></div>'
const TOOLTIP = '<div><div data-slot="tooltip-content">tip</div></div>'
const TOAST = '<div data-slot="toast-portal-anchored"><div data-slot="toast-popup"></div></div>'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe('useNativeOverlayActive', () => {
  it('follows a popup wrapper appended to and removed from the body', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    expect(result.current).toBe(false)

    let wrapper!: HTMLElement
    await act(async () => {
      wrapper = appendToBody(DIALOG)
    })
    expect(result.current).toBe(true)

    await act(async () => wrapper.remove())
    expect(result.current).toBe(false)
  })

  it('treats a popup of a kind it has never seen as blocking', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      appendToBody('<div><div data-slot="brand-new-popup"></div></div>')
    })
    expect(result.current).toBe(true)
  })

  it('treats a wrapper with no slot at all as blocking', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      appendToBody('<div><div role="menu"></div></div>')
    })
    expect(result.current).toBe(true)
  })

  it('ignores tooltips and toasts, which never need the pane under them', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      appendToBody(TOOLTIP)
      appendToBody(TOAST)
    })
    expect(result.current).toBe(false)
  })

  it('stays blocking for a popup that merely contains a tooltip', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      appendToBody(`<div>${'<div data-slot="dialog-popup"></div>'}${TOOLTIP}</div>`)
    })
    expect(result.current).toBe(true)
  })

  it('ignores the app root and non-visual nodes', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      const root = document.createElement('div')
      root.id = 'root'
      document.body.appendChild(root)
      document.body.appendChild(document.createElement('script'))
      document.body.appendChild(document.createElement('style'))
    })
    expect(result.current).toBe(false)
  })

  it('never reports a node that was added and removed within one batch', async () => {
    const seen: boolean[] = []
    renderHook(() => {
      const active = useNativeOverlayActive()
      seen.push(active)
      return active
    })
    await act(async () => {
      appendToBody(DIALOG).remove()
    })
    expect(seen).not.toContain(true)
  })

  it('reads the DOM on subscribe, so an overlay already open is seen at once', () => {
    appendToBody(DIALOG)
    const { result } = renderHook(() => useNativeOverlayActive())
    expect(result.current).toBe(true)
  })

  it('shares one observer between subscribers and disconnects it at zero', () => {
    const real = MutationObserver
    let created = 0
    let disconnected = 0
    vi.stubGlobal(
      'MutationObserver',
      class extends real {
        constructor(callback: MutationCallback) {
          super(callback)
          created++
        }
        disconnect() {
          disconnected++
          super.disconnect()
        }
      },
    )

    const a = renderHook(() => useNativeOverlayActive())
    const b = renderHook(() => useNativeOverlayActive())
    expect(created).toBe(1)

    a.unmount()
    expect(disconnected).toBe(0)
    b.unmount()
    expect(disconnected).toBe(1)
  })

  it.each([
    ['Slate shadow input', '<input class="slate-shadow-input" style="position:fixed;opacity:0">'],
    ['a bare textarea helper', '<textarea></textarea>'],
    [
      'dnd-kit live region',
      `<div id="DndLiveRegion-0" role="status" aria-live="assertive" style="position:fixed;width:1px;height:1px;margin:-1px;overflow:hidden;clip:rect(0 0 0 0);clip-path:inset(100%);white-space:nowrap">x</div>`,
    ],
    [
      'Monaco aria container',
      '<div class="monaco-aria-container" style="position:absolute;width:1px;height:1px;clip:rect(1px, 1px, 1px, 1px);clip-path:inset(50%);display:block;visibility:visible;opacity:1">x</div>',
    ],
    [
      'a helper clipped only by a zero-area rect',
      '<div style="position:absolute;clip:rect(1px, 1px, 1px, 1px)">x</div>',
    ],
    ['dnd-kit described-by text', '<div id="DndDescribedBy-0" hidden>Press space</div>'],
    ['a display:none helper', '<div style="display:none">x</div>'],
    ['a visibility:hidden helper', '<div style="visibility:hidden">x</div>'],
    ['a fully transparent helper', '<div style="opacity:0">x</div>'],
  ])('ignores a persistent %s', async (_name, html) => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      appendNodeToBody(html)
    })
    expect(result.current).toBe(false)
  })

  it('still counts a painting drag preview that is aria-hidden', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      appendNodeToBody('<div aria-hidden="true" data-drop-indicator style="position:fixed"></div>')
    })
    expect(result.current).toBe(true)
  })

  it('still counts a real-sized popup that is only partly clipped', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    await act(async () => {
      appendNodeToBody(
        '<div role="menu" style="position:fixed;width:200px;height:100px;clip-path:inset(10%);clip:rect(0px, 100px, 50px, 0px)">x</div>',
      )
    })
    expect(result.current).toBe(true)
  })

  it('counts the real base-ui dialog portal while open, zero-size wrapper and all', async () => {
    const { result } = renderHook(() => useNativeOverlayActive())
    const container = document.createElement('div')
    container.id = 'root'
    document.body.appendChild(container)
    const view = render(
      <DialogPrimitive.Root open>
        <DialogContent>hello</DialogContent>
      </DialogPrimitive.Root>,
      { container },
    )
    await vi.waitFor(() => expect(result.current).toBe(true))

    view.rerender(
      <DialogPrimitive.Root open={false}>
        <DialogContent>hello</DialogContent>
      </DialogPrimitive.Root>,
    )
    await vi.waitFor(() => expect(result.current).toBe(false))
  })
})
