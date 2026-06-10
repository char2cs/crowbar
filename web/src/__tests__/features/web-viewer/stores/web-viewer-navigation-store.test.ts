import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock the bridge BEFORE importing the store (the store imports bridge at module level)
vi.mock('@/lib/crowbar-bridge', () => ({
  browserPaneGoBack: vi.fn().mockResolvedValue(undefined),
  browserPaneGoForward: vi.fn().mockResolvedValue(undefined),
  browserPaneReload: vi.fn().mockResolvedValue(undefined),
}))

import { useWebViewerNavigationStore } from '@/features/web-viewer/stores/web-viewer-navigation-store'
import { browserPaneGoBack, browserPaneGoForward, browserPaneReload } from '@/lib/crowbar-bridge'

beforeEach(() => {
  useWebViewerNavigationStore.setState({ navigationByBufferId: {} })
  vi.clearAllMocks()
})

describe('registerBuffer', () => {
  it('creates an entry with initial url and false flags', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    const entry = useWebViewerNavigationStore.getState().navigationByBufferId['buf-1']
    expect(entry.url).toBe('https://example.com')
    expect(entry.canGoBack).toBe(false)
    expect(entry.canGoForward).toBe(false)
  })

  it('goBack calls browserPaneGoBack with the bufferId', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().navigationByBufferId['buf-1'].goBack()
    expect(browserPaneGoBack).toHaveBeenCalledWith('buf-1')
  })

  it('goForward calls browserPaneGoForward with the bufferId', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().navigationByBufferId['buf-1'].goForward()
    expect(browserPaneGoForward).toHaveBeenCalledWith('buf-1')
  })

  it('reload calls browserPaneReload with the bufferId', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().navigationByBufferId['buf-1'].reload()
    expect(browserPaneReload).toHaveBeenCalledWith('buf-1')
  })
})

describe('updateNavState', () => {
  it('updates url, canGoBack, canGoForward without touching functions', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().updateNavState('buf-1', {
      url: 'https://new.example.com',
      canGoBack: true,
      canGoForward: false,
    })
    const entry = useWebViewerNavigationStore.getState().navigationByBufferId['buf-1']
    expect(entry.url).toBe('https://new.example.com')
    expect(entry.canGoBack).toBe(true)
    expect(entry.canGoForward).toBe(false)
    // goBack function still works
    entry.goBack()
    expect(browserPaneGoBack).toHaveBeenCalledWith('buf-1')
  })

  it('creates an entry if buffer was not registered', () => {
    useWebViewerNavigationStore.getState().updateNavState('buf-x', {
      url: 'https://surprise.com',
      canGoBack: false,
      canGoForward: true,
    })
    const entry = useWebViewerNavigationStore.getState().navigationByBufferId['buf-x']
    expect(entry.url).toBe('https://surprise.com')
  })

  it('goBack still works when updateNavState creates the entry from scratch', () => {
    useWebViewerNavigationStore.getState().updateNavState('buf-new', {
      url: 'https://surprise.com',
      canGoBack: true,
      canGoForward: false,
    })
    useWebViewerNavigationStore.getState().navigationByBufferId['buf-new'].goBack()
    expect(browserPaneGoBack).toHaveBeenCalledWith('buf-new')
  })
})

describe('removeBuffer', () => {
  it('deletes the buffer entry', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().removeBuffer('buf-1')
    expect(useWebViewerNavigationStore.getState().navigationByBufferId['buf-1']).toBeUndefined()
  })
})
