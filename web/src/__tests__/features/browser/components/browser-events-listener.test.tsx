import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BrowserEventsListener } from '@/features/browser/components/browser-events-listener'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'

const events = vi.hoisted(() => ['onNavigated', 'onTitle', 'onLoading', 'onNewWindow'])
const api = vi.hoisted(() => Object.fromEntries(events.map((on) => [on, vi.fn()])))
vi.mock('@/lib/browser-bridge', () => ({ browserApi: api }))

type Handler = (e: { id: string; url?: string; title?: string; loading?: boolean }) => void
/** What the listener subscribed to `event` with, once it has. */
const heard = (event: string) => api[event]?.mock.lastCall?.[0] as Handler | undefined
const off = vi.fn()
const tab = { type: 'browser', workspaceId: 'ws-test' } as const

beforeEach(() => {
  vi.clearAllMocks()
  for (const listen of Object.values(api)) listen.mockResolvedValue(off)
  resetWindowPaneStoreForTests()
  windowPaneStore.getState().paneActions.setActiveProject('p1')
})

describe('BrowserEventsListener', () => {
  it('writes webview reports into the store with no BrowserPane mounted', async () => {
    const { bufferActions } = windowPaneStore.getState()
    const id = bufferActions.openContent(tab)
    const next = { url: 'https://a.test/next', title: 'Next page', loading: true }
    render(<BrowserEventsListener />)
    await vi.waitFor(() => expect(heard('onTitle')).toBeDefined())

    act(() => heard('onNavigated')?.({ id, url: next.url }))
    act(() => heard('onTitle')?.({ id, title: next.title }))
    act(() => heard('onLoading')?.({ id, loading: next.loading }))
    act(() => heard('onNavigated')?.({ id: 'unknown', url: 'https://x.test' }))

    expect(bufferActions.getBufferById(id)).toMatchObject(next)
  })

  it('opens a page that asks for a new window as a tab beside its opener', async () => {
    const { bufferActions, activePaneId } = windowPaneStore.getState()
    const opener = bufferActions.openContent({ ...tab, url: 'https://a.test' })
    render(<BrowserEventsListener />)
    await vi.waitFor(() => expect(heard('onNewWindow')).toBeDefined())

    act(() => heard('onNewWindow')?.({ id: opener, url: 'https://b.test/popup' }))
    act(() => heard('onNewWindow')?.({ id: 'closed-tab', url: 'https://x.test' }))

    const tabs = windowPaneStore.getState().panes[activePaneId]?.editorTabIds ?? []
    const opened = tabs.filter((id) => id !== opener).map((id) => bufferActions.getBufferById(id))
    expect(opened).toEqual([expect.objectContaining({ ...tab, url: 'https://b.test/popup' })])
  })

  it('unsubscribes on unmount', async () => {
    const view = render(<BrowserEventsListener />)
    await vi.waitFor(() => expect(heard('onNewWindow')).toBeDefined())
    view.unmount()
    await vi.waitFor(() => expect(off).toHaveBeenCalledTimes(events.length))
  })
})
