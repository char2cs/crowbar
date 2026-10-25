import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BrowserEventsListener } from '@/features/browser/components/browser-events-listener'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

const api = vi.hoisted(() => ({ onNavigated: vi.fn(), onTitle: vi.fn(), onLoading: vi.fn() }))
vi.mock('@/lib/browser-bridge', () => ({ browserApi: api }))

type Handler = (e: { id: string; url?: string; title?: string; loading?: boolean }) => void
const handlers: { navigated?: Handler; title?: Handler; loading?: Handler } = {}
const off = { navigated: vi.fn(), title: vi.fn(), loading: vi.fn() }

beforeEach(() => {
  off.navigated.mockClear()
  off.title.mockClear()
  api.onNavigated.mockImplementation(async (cb: Handler) => {
    handlers.navigated = cb
    return off.navigated
  })
  api.onLoading.mockImplementation(async (cb: Handler) => {
    handlers.loading = cb
    return off.loading
  })
  api.onTitle.mockImplementation(async (cb: Handler) => {
    handlers.title = cb
    return off.title
  })
  resetWindowPaneStoreForTests()
  windowPaneStore.getState().paneActions.setActiveProject('p1')
})

describe('BrowserEventsListener', () => {
  it('writes webview reports into the store with no BrowserPane mounted', async () => {
    const id = windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'browser', workspaceId: 'ws-test' })
    render(<BrowserEventsListener />)
    await vi.waitFor(() => expect(handlers.title).toBeDefined())

    act(() => handlers.navigated?.({ id, url: 'https://a.test/next' }))
    act(() => handlers.title?.({ id, title: 'Next page' }))
    act(() => handlers.loading?.({ id, loading: true }))
    act(() => handlers.navigated?.({ id: 'unknown', url: 'https://x.test' }))

    expect(windowPaneStore.getState().bufferActions.getBufferById(id)).toMatchObject({
      url: 'https://a.test/next',
      title: 'Next page',
      loading: true,
    })
  })

  it('unsubscribes on unmount', async () => {
    const view = render(<BrowserEventsListener />)
    await vi.waitFor(() => expect(handlers.title).toBeDefined())
    view.unmount()
    await vi.waitFor(() => {
      expect(off.navigated).toHaveBeenCalled()
      expect(off.title).toHaveBeenCalled()
      expect(off.loading).toHaveBeenCalled()
    })
  })
})
