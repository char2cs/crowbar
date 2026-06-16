import { renderHook } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'

let activeBuffer: { id: string; type: string } | null = null
vi.mock('@/features/workspace/hooks/use-active-workspace-buffer', () => ({
  useActiveWorkspaceBuffer: () => activeBuffer,
}))

import { useActiveWebViewerNavigation } from '@/features/tabs/hooks/use-active-webviewer-navigation'
import { useWebViewerNavigationStore } from '@/features/web-viewer/stores/web-viewer-navigation-store'

beforeEach(() => {
  activeBuffer = null
  useWebViewerNavigationStore.setState({ navigationByBufferId: {} })
})

describe('useActiveWebViewerNavigation', () => {
  it('reports a non-webviewer active buffer as not using webviewer navigation', () => {
    activeBuffer = { id: 'b1', type: 'editor' }
    const { result } = renderHook(() => useActiveWebViewerNavigation())
    expect(result.current.usesWebViewerNavigation).toBe(false)
    expect(result.current.activeWebViewerNavigation).toBeUndefined()
  })

  it('returns the webviewer nav entry for a webViewer active buffer', () => {
    activeBuffer = { id: 'b2', type: 'webViewer' }
    useWebViewerNavigationStore.setState({
      navigationByBufferId: {
        b2: {
          url: 'https://x.com',
          canGoBack: true,
          canGoForward: false,
          goBack: () => {},
          goForward: () => {},
          reload: () => {},
        },
      },
    })
    const { result } = renderHook(() => useActiveWebViewerNavigation())
    expect(result.current.usesWebViewerNavigation).toBe(true)
    expect(result.current.activeWebViewerNavigation?.canGoBack).toBe(true)
  })

  it('returns undefined nav when there is no active buffer', () => {
    activeBuffer = null
    const { result } = renderHook(() => useActiveWebViewerNavigation())
    expect(result.current.usesWebViewerNavigation).toBe(false)
    expect(result.current.activeWebViewerNavigation).toBeUndefined()
  })
})
