import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: vi.fn(),
  browserPaneSync: vi.fn().mockResolvedValue(undefined),
  browserPaneClose: vi.fn().mockResolvedValue(undefined),
  browserPaneNavigate: vi.fn().mockResolvedValue(undefined),
  browserPaneReload: vi.fn().mockResolvedValue(undefined),
  browserPaneGoBack: vi.fn().mockResolvedValue(undefined),
  browserPaneGoForward: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/features/web-viewer/hooks/use-browser-pane-anchor', () => ({
  useBrowserPaneAnchor: vi.fn().mockReturnValue({ isTauri: false }),
}))

vi.mock('@/features/web-viewer/stores/web-viewer-navigation-store', () => ({
  useWebViewerNavigationStore: Object.assign(vi.fn().mockReturnValue(undefined), {
    getState: vi.fn().mockReturnValue({
      registerBuffer: vi.fn(),
      removeBuffer: vi.fn(),
    }),
  }),
}))

import { isTauri } from '@/lib/crowbar-bridge'
import { useBrowserPaneAnchor } from '@/features/web-viewer/hooks/use-browser-pane-anchor'
import { WebViewer } from '@/features/web-viewer/components/web-viewer'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('WebViewer — non-Tauri', () => {
  it('shows the requires-desktop message', () => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(false)
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: false })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(screen.getByText(/requires the desktop app/i)).toBeInTheDocument()
  })

  it('renders the nav bar even in non-Tauri mode', () => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(false)
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: false })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(screen.getByRole('textbox')).toBeInTheDocument()
  })
})

describe('WebViewer — Tauri', () => {
  it('does not show the requires-desktop message', () => {
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: true })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(screen.queryByText(/requires the desktop app/i)).not.toBeInTheDocument()
  })

  it('renders an anchor div (data-anchor attribute)', () => {
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: true })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(document.querySelector('[data-browser-anchor]')).toBeInTheDocument()
  })
})
