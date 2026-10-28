import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { useSidebarPeekStore } from '@/features/window/stores/sidebar-peek-store'
import { PaneEditorViewContent } from '@/features/panes/components/pane-editor-view-content'
import { runAgentOp } from '@/features/browser/lib/agent-ops'
import { ensureWebview } from '@/features/browser/lib/ensure-webview'
import { useStore } from 'zustand'
import { BrowserPane } from '@/features/browser/components/browser-pane'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

const api = vi.hoisted(() => ({
  open: vi.fn().mockResolvedValue(undefined),
  setBounds: vi.fn().mockResolvedValue(undefined),
  park: vi.fn().mockResolvedValue(undefined),
  unpark: vi.fn().mockResolvedValue(undefined),
  navigate: vi.fn().mockResolvedValue(undefined),
  back: vi.fn().mockResolvedValue(undefined),
  forward: vi.fn().mockResolvedValue(undefined),
  reload: vi.fn().mockResolvedValue(undefined),
  close: vi.fn().mockResolvedValue(undefined),
  evaluate: vi.fn(),
  copyText: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/lib/browser-bridge', () => ({ browserApi: api }))
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: toasts }))

class FakeResizeObserver {
  static instances: FakeResizeObserver[] = []
  constructor(private callback: ResizeObserverCallback) {
    FakeResizeObserver.instances.push(this)
  }
  observe() {}
  unobserve() {}
  disconnect() {}
  trigger() {
    this.callback([], this as unknown as ResizeObserver)
  }
}
vi.stubGlobal('ResizeObserver', FakeResizeObserver)

class FakeIntersectionObserver {
  static instances: FakeIntersectionObserver[] = []
  constructor(private callback: IntersectionObserverCallback) {
    FakeIntersectionObserver.instances.push(this)
  }
  observe() {}
  disconnect() {}
  fire(ratio: number) {
    this.callback([{ intersectionRatio: ratio } as IntersectionObserverEntry], this as never)
  }
}
vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver)

let rect = { x: 10, y: 60, width: 400, height: 300 }
vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(
  () => ({ ...rect, left: rect.x, top: rect.y }) as DOMRect,
)

function openBrowser(url?: string): string {
  const { bufferActions } = windowPaneStore.getState()
  const id = bufferActions.openContent({ type: 'browser', workspaceId: 'ws-test' })
  if (url) bufferActions.setBrowserLocation(id, { url })
  return id
}
/** The app's real root: the overlay watcher ignores the body child named #root. */
function renderPane(ui: React.ReactElement) {
  const container = document.createElement('div')
  container.id = 'root'
  document.body.appendChild(container)
  return render(ui, { container })
}
const stored = (id: string) => windowPaneStore.getState().bufferActions.getBufferById(id)

beforeEach(() => {
  Object.values(api).forEach((fn) => fn.mockClear())
  rect = { x: 10, y: 60, width: 400, height: 300 }
  FakeResizeObserver.instances = []
  FakeIntersectionObserver.instances = []
  document.body.replaceChildren()
  resetWindowPaneStoreForTests()
  useSidebarPeekStore.setState({ occupied: false })
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  windowPaneStore.getState().paneActions.setActiveProject('p1')
})

describe('BrowserPane pick mode', () => {
  const picked = {
    kind: 'region',
    title: 'T',
    url: 'https://a.test/',
    items: [{ s: '#a', tag: 'p', role: '', name: '', text: 'A', r: [1, 2, 3, 4], h: '', i: 0 }],
    more: 1,
    box: [1, 2, 3, 4],
  }

  beforeEach(() => {
    api.evaluate.mockReset()
    api.copyText.mockReset().mockResolvedValue(undefined)
    toasts.success.mockClear()
    toasts.error.mockClear()
  })

  it('copies the pointer through the native pasteboard when the page finishes a pick', async () => {
    const user = userEvent.setup()
    const id = openBrowser('https://a.test')
    api.evaluate.mockResolvedValueOnce(JSON.stringify(picked))
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    await user.click(screen.getByRole('button', { name: 'Pick element' }))

    await vi.waitFor(() => expect(toasts.success).toHaveBeenCalledWith('Pointer copied', '2 items'))
    expect(api.copyText).toHaveBeenCalledWith(
      expect.stringMatching(
        new RegExp(
          `^⌖ region "A" · https://a.test/ · tab ${id} · 2 items\n[\\s\\S]*\n⌖data \\{"v":1`,
        ),
      ),
    )
    expect(screen.getByRole('button', { name: 'Pick element' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })

  it('cancels the pending pick from the same button and copies nothing', async () => {
    const user = userEvent.setup()
    const id = openBrowser('https://a.test')
    let finish!: (value: string) => void
    api.evaluate.mockImplementation((_id: string, body: string) =>
      body.includes('"pick"')
        ? new Promise<string>((resolve) => (finish = resolve))
        : Promise.resolve('true'),
    )
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    const button = screen.getByRole('button', { name: 'Pick element' })

    await user.click(button)
    expect(button).toHaveAttribute('aria-pressed', 'true')
    await user.click(button)
    expect(api.evaluate).toHaveBeenLastCalledWith(id, expect.stringContaining('"cancelPick"'), 5000)
    act(() => finish('null'))

    await vi.waitFor(() => expect(button).toHaveAttribute('aria-pressed', 'false'))
    expect(api.copyText).not.toHaveBeenCalled()
    expect(toasts.success).not.toHaveBeenCalled()
  })

  it('reports a failed native copy instead of claiming success', async () => {
    const user = userEvent.setup()
    const id = openBrowser('https://a.test')
    api.evaluate.mockResolvedValueOnce(JSON.stringify(picked))
    api.copyText.mockRejectedValueOnce('pasteboard unavailable')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    await user.click(screen.getByRole('button', { name: 'Pick element' }))

    await vi.waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith('Could not pick', 'pasteboard unavailable'),
    )
    expect(toasts.success).not.toHaveBeenCalled()
  })

  it('cancels the pick when the tab leaves the screen', async () => {
    const user = userEvent.setup()
    const id = openBrowser('https://a.test')
    api.evaluate.mockImplementation((_id: string, body: string) =>
      body.includes('"pick"') ? new Promise<string>(() => {}) : Promise.resolve('true'),
    )
    const view = renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    await user.click(screen.getByRole('button', { name: 'Pick element' }))

    view.rerender(<BrowserPane paneId="p-this" bufferId={id} visible={false} />)

    await vi.waitFor(() =>
      expect(api.evaluate).toHaveBeenLastCalledWith(
        id,
        expect.stringContaining('"cancelPick"'),
        5000,
      ),
    )
  })

  it('reports a failed pick instead of hanging in pick mode', async () => {
    const user = userEvent.setup()
    const id = openBrowser('https://a.test')
    api.evaluate.mockRejectedValueOnce(new Error('the page went away'))
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    await user.click(screen.getByRole('button', { name: 'Pick element' }))

    await vi.waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith('Could not pick', 'the page went away'),
    )
    expect(screen.getByRole('button', { name: 'Pick element' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })

  it('has nothing to pick before the tab has a page', () => {
    const id = openBrowser()
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    expect(screen.getByRole('button', { name: 'Pick element' })).toBeDisabled()
  })
})

describe('BrowserPane', () => {
  it('shows a focused empty address bar and creates no webview before a url exists', () => {
    const id = openBrowser()
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Address' }))
    expect(api.open).not.toHaveBeenCalled()
  })

  it('opens the webview at the placeholder rect once a url is submitted', async () => {
    const user = userEvent.setup()
    const id = openBrowser()
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    await user.type(screen.getByRole('textbox', { name: 'Address' }), 'example.com{Enter}')

    expect(api.open).toHaveBeenCalledTimes(1)
    expect(api.open).toHaveBeenCalledWith(id, 'https://example.com', rect, 1, false)
    expect(stored(id)).toMatchObject({ url: 'https://example.com' })
  })

  it('opens at mount when the tab already has a url', () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    expect(api.open).toHaveBeenCalledWith(id, 'https://a.test', rect, 1, false)
  })

  it('creates the webview at the configured page zoom', () => {
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), browserZoom: 150 },
    })
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    expect(api.open).toHaveBeenCalledWith(id, 'https://a.test', rect, 1.5, false)
  })

  it('searches with the configured engine when the address is not a host', async () => {
    const user = userEvent.setup()
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), browserSearchEngine: 'google' },
    })
    const id = openBrowser()
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    await user.type(screen.getByRole('textbox', { name: 'Address' }), 'tauri docs{Enter}')

    expect(stored(id)).toMatchObject({ url: 'https://www.google.com/search?q=tauri%20docs' })
  })

  it('navigates the existing webview instead of reopening it', async () => {
    const user = userEvent.setup()
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    const input = screen.getByRole('textbox', { name: 'Address' })
    await user.clear(input)
    await user.type(input, 'b.test{Enter}')

    expect(api.navigate).toHaveBeenCalledWith(id, 'https://b.test')
    expect(api.open).toHaveBeenCalledTimes(1)
  })

  it('sends no move before the webview exists, and follows the placeholder once it does', async () => {
    let created!: () => void
    api.open.mockReturnValueOnce(new Promise<void>((resolve) => (created = resolve)))
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    act(() => FakeResizeObserver.instances.forEach((o) => o.trigger()))

    expect(api.park).not.toHaveBeenCalled()
    expect(api.setBounds).not.toHaveBeenCalled()

    created()
    await vi.waitFor(() => expect(api.setBounds).toHaveBeenLastCalledWith(id, rect))
    expect(api.unpark).not.toHaveBeenCalled()
  })

  it('never parks an empty tab, and opens the webview its first url asks for', async () => {
    const user = userEvent.setup()
    const id = openBrowser()
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    expect(api.open).not.toHaveBeenCalled()
    expect(api.park).not.toHaveBeenCalled()

    await user.type(screen.getByRole('textbox', { name: 'Address' }), 'example.com{Enter}')

    expect(api.open).toHaveBeenCalledWith(id, 'https://example.com', rect, 1, false)
  })

  it('reports new bounds when the placeholder resizes', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    rect = { x: 10, y: 60, width: 250, height: 300 }
    act(() => FakeResizeObserver.instances.forEach((o) => o.trigger()))

    await vi.waitFor(() => expect(api.setBounds).toHaveBeenLastCalledWith(id, rect))
  })

  it('reports new bounds when the placeholder moves without resizing', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)
    const watcher = FakeIntersectionObserver.instances.at(-1)!
    watcher.fire(1)

    rect = { x: 210, y: 60, width: 400, height: 300 }
    act(() => watcher.fire(0.5))

    await vi.waitFor(() => expect(api.setBounds).toHaveBeenLastCalledWith(id, rect))
  })

  it('reports new bounds when the window resizes', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    rect = { x: 0, y: 60, width: 900, height: 700 }
    act(() => window.dispatchEvent(new Event('resize')))

    await vi.waitFor(() => expect(api.setBounds).toHaveBeenLastCalledWith(id, rect))
  })

  it('creates no webview for a restored tab until it is first shown, then parks and unparks it', async () => {
    const id = openBrowser('https://a.test')
    const { rerender } = renderPane(<BrowserPane paneId="p-this" bufferId={id} visible={false} />)
    expect(api.open).not.toHaveBeenCalled()
    expect(api.park).not.toHaveBeenCalled()

    rerender(<BrowserPane paneId="p-this" bufferId={id} visible />)
    expect(api.open).toHaveBeenCalledTimes(1)

    rerender(<BrowserPane paneId="p-this" bufferId={id} visible={false} />)
    await vi.waitFor(() => expect(api.park).toHaveBeenLastCalledWith(id))
    rerender(<BrowserPane paneId="p-this" bufferId={id} visible />)
    await vi.waitFor(() => expect(api.unpark).toHaveBeenLastCalledWith(id, rect))
    expect(api.open).toHaveBeenCalledTimes(1)
  })

  it('never hides a webview: leaving the screen moves it away and keeps it live', async () => {
    const id = openBrowser('https://a.test')
    const { rerender } = renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    rerender(<BrowserPane paneId="p-this" bufferId={id} visible={false} />)

    await vi.waitFor(() => expect(api.park).toHaveBeenCalledWith(id))
    expect(api.close).not.toHaveBeenCalled()
  })

  it('unparks a webview the agent made, at the placeholder, without reloading it', async () => {
    const id = openBrowser('https://a.test')
    await ensureWebview(id, { x: 0, y: 0, width: 1280, height: 800 }, true)
    expect(api.open).toHaveBeenCalledWith(id, 'https://a.test', expect.anything(), 1, true)

    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    await vi.waitFor(() => expect(api.unpark).toHaveBeenLastCalledWith(id, rect))
    expect(api.open).toHaveBeenCalledTimes(1)
  })

  it('remounting a tab whose webview exists does not open another', () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />).unmount()
    renderPane(<BrowserPane paneId="p-that" bufferId={id} visible />)
    expect(api.open).toHaveBeenCalledTimes(1)
  })

  it('parks the webview while a popup is open over the app, and shows it after', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    const popup = document.createElement('div')
    popup.innerHTML = '<div data-slot="dropdown-menu-content"></div>'
    await act(async () => {
      document.body.appendChild(popup)
    })
    await vi.waitFor(() => expect(api.park).toHaveBeenLastCalledWith(id))

    await act(async () => popup.remove())
    await vi.waitFor(() => expect(api.unpark).toHaveBeenLastCalledWith(id, rect))
  })

  it('parks the webview while the sidebar peek card is on screen', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    act(() => useSidebarPeekStore.setState({ occupied: true }))
    await vi.waitFor(() => expect(api.park).toHaveBeenLastCalledWith(id))

    act(() => useSidebarPeekStore.setState({ occupied: false }))
    await vi.waitFor(() => expect(api.unpark).toHaveBeenLastCalledWith(id, rect))
  })

  it('parks under another pane going fullscreen, but stays up when it is the fullscreen one', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    act(() => windowPaneStore.setState({ fullscreenPaneId: 'p-other' }))
    await vi.waitFor(() => expect(api.park).toHaveBeenLastCalledWith(id))

    act(() => windowPaneStore.setState({ fullscreenPaneId: 'p-this' }))
    await vi.waitFor(() => expect(api.unpark).toHaveBeenLastCalledWith(id, rect))
  })

  it('parks under an overlay-mode console and returns when it closes or pushes', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    act(() => useConsoleStore.setState({ open: true, mode: 'overlay' }))
    await vi.waitFor(() => expect(api.park).toHaveBeenLastCalledWith(id))

    act(() => useConsoleStore.setState({ mode: 'push' }))
    await vi.waitFor(() => expect(api.unpark).toHaveBeenLastCalledWith(id, rect))

    act(() => useConsoleStore.setState({ open: false, mode: 'overlay' }))
    await vi.waitFor(() => expect(api.unpark).toHaveBeenLastCalledWith(id, rect))
  })

  it('parks but does not close the webview on unmount: tab teardown owns closing', async () => {
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />).unmount()
    await vi.waitFor(() => expect(api.park).toHaveBeenLastCalledWith(id))
    expect(api.close).not.toHaveBeenCalled()
  })

  it('drives back, forward and reload', async () => {
    const user = userEvent.setup()
    const id = openBrowser('https://a.test')
    renderPane(<BrowserPane paneId="p-this" bufferId={id} visible />)

    await user.click(screen.getByRole('button', { name: 'Back' }))
    await user.click(screen.getByRole('button', { name: 'Forward' }))
    await user.click(screen.getByRole('button', { name: 'Reload' }))

    expect(api.back).toHaveBeenCalledWith(id)
    expect(api.forward).toHaveBeenCalledWith(id)
    expect(api.reload).toHaveBeenCalledWith(id)
  })
})

describe('a browser tab hosted by its pane', () => {
  const hosted = (id: string, dropTargeted: boolean) => (
    <PaneEditorViewContent
      paneId="p-this"
      wsId="ws-a"
      editorTabIds={[id]}
      activeEditorTabId={id}
      editorViewHidden={false}
      isActivePane
      showing
      dropTargeted={dropTargeted}
    />
  )

  it('opens in place for the active tab', () => {
    const id = openBrowser('https://a.test')
    renderPane(hosted(id, false))
    expect(api.open).toHaveBeenCalledWith(id, 'https://a.test', rect, 1, false)
    expect(api.park).not.toHaveBeenCalled()
  })

  it('parks the webview while its pane is a drop target, whose overlay would sit under it', async () => {
    const id = openBrowser('https://a.test')
    const { rerender } = renderPane(hosted(id, false))
    expect(api.park).not.toHaveBeenCalled()

    rerender(hosted(id, true))
    await vi.waitFor(() => expect(api.park).toHaveBeenLastCalledWith(id))
  })
})

describe('an agent op on a restored tab that was never shown', () => {
  it('creates it parked beside the visible tab, which stays selected and in place', async () => {
    const { paneActions, bufferActions } = windowPaneStore.getState()
    paneActions.openChat('chat-a', { workspaceId: 'ws-a' })
    const first = bufferActions.openContent({ type: 'browser', workspaceId: 'ws-test' })
    bufferActions.setBrowserLocation(first, { url: 'https://a.test/' })
    const second = bufferActions.openContent({ type: 'browser', workspaceId: 'ws-test' })
    bufferActions.setBrowserLocation(second, { url: 'https://b.test/' })

    function Host() {
      const pane = windowPaneStore.getState().paneActions.getPaneByEditorTabId(first)
      const active = useStore(windowPaneStore, (s) => s.panes[pane?.id ?? '']?.activeEditorTabId)
      return (
        <>
          <BrowserPane paneId="p-this" bufferId={first} visible={active === first} />
          <BrowserPane paneId="p-this" bufferId={second} visible={active === second} />
        </>
      )
    }
    renderPane(<Host />)
    expect(api.open).toHaveBeenCalledTimes(1)
    expect(api.open.mock.calls[0][0]).toBe(second)

    await runAgentOp({ chatId: 'chat-a', op: 'reload', tab: first, args: {} })

    expect(api.open.mock.calls.map((c) => [c[0], c[4]])).toEqual([
      [second, false],
      [first, true],
    ])
    expect(api.reload).toHaveBeenCalledWith(first)
    expect(api.unpark).not.toHaveBeenCalled()
    expect(windowPaneStore.getState().paneActions.getPaneByEditorTabId(first)).toMatchObject({
      activeEditorTabId: second,
    })
  })
})
