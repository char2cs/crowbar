import React from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SidebarCarousel } from '@/components/layout/sidebar-carousel'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/components/layout/workspace-tree', () => ({
  WorkspaceTree: () => <div data-testid="panel-workspaces" />,
}))
vi.mock('@/features/agent/tree/agent-chats-panel', () => ({
  AgentChatsPanel: () => <div data-testid="panel-chats" />,
}))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="panel-files" />,
}))
vi.mock('@/features/git/components/git-panel', () => ({
  GitPanel: () => <div data-testid="panel-git" />,
}))
vi.mock('@/components/error-boundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: Object.assign(
    (sel: (state: unknown) => unknown) =>
      sel({ files: [], handleFileOpen: null, handleFileSelect: null }),
    { use: { handleFileOpen: () => null, handleFileSelect: () => null } },
  ),
}))
vi.mock('@/features/file-explorer/stores/file-explorer-tree-store', () => ({
  useFileTreeStore: { getState: () => ({ toggleFolder: vi.fn() }) },
}))
vi.mock('@/lib/store/sidebar', async () => {
  const actual = await vi.importActual('@/lib/store/sidebar')
  return actual
})

describe('SidebarCarousel', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    // jsdom does not implement scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
  })

  it('mounts all 4 panels', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    expect(screen.getByTestId('panel-workspaces')).toBeInTheDocument()
    expect(screen.getByTestId('panel-chats')).toBeInTheDocument()
    expect(screen.getByTestId('panel-files')).toBeInTheDocument()
    expect(screen.getByTestId('panel-git')).toBeInTheDocument()
  })

  it('renders the panels in Workspaces, Chats, Files, Git order (index math must not be off-by-one)', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    const container = screen.getByTestId('panel-workspaces').closest('[data-sidebar-carousel]')
    const testIds = Array.from(container?.querySelectorAll('[data-testid^="panel-"]') ?? []).map(
      (el) => el.getAttribute('data-testid'),
    )
    expect(testIds).toEqual(['panel-workspaces', 'panel-chats', 'panel-files', 'panel-git'])
  })

  it('scrolls to the Chats panel index (1) when activeTab is chats', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    const container = screen
      .getByTestId('panel-workspaces')
      .closest('[data-sidebar-carousel]') as HTMLElement
    Object.defineProperty(container, 'clientWidth', { value: 400, configurable: true })
    const scrollToSpy = HTMLElement.prototype.scrollTo as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    act(() => {
      useSidebarStore.setState({ activeTab: 'chats' })
    })

    expect(scrollToSpy).toHaveBeenCalledWith(
      expect.objectContaining({ left: 400, behavior: 'smooth' }),
    )
  })

  // Regression: hiding and re-showing the sidebar while the Files panel was
  // active landed on Chats. Collapsing the panel drives the carousel's width to
  // 0, the browser clamps scrollLeft, and the offsets that arrive while the
  // sidebar reopens do not correspond to the width the container ends up with —
  // reading them back through Math.round() picked a neighbouring panel and
  // reassigned activeTab. Only a real scroll gesture may move the tab.
  describe('activeTab is only derived from scroll offsets the user caused', () => {
    function carousel(): HTMLElement {
      return screen
        .getByTestId('panel-workspaces')
        .closest('[data-sidebar-carousel]') as HTMLElement
    }

    function setGeometry(el: HTMLElement, clientWidth: number, scrollLeft: number) {
      Object.defineProperty(el, 'clientWidth', { value: clientWidth, configurable: true })
      Object.defineProperty(el, 'scrollLeft', { value: scrollLeft, configurable: true })
    }

    // Select a tab and let its programmatic scroll finish, as the browser does.
    // Without the scrollend the carousel is still mid-animation and would have
    // ignored the scroll below for a reason that has nothing to do with gestures.
    function selectFilesAndSettle(el: HTMLElement) {
      act(() => {
        useSidebarStore.setState({ activeTab: 'files' })
      })
      fireEvent(el, new Event('scrollend'))
    }

    it('ignores a settled scroll offset that no gesture produced', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
      const el = carousel()
      selectFilesAndSettle(el)

      // An offset that rounds to panel index 1 (Chats) — what a collapse/expand
      // cycle leaves behind, since the offset was written for a width the
      // container no longer has.
      setGeometry(el, 400, 400)
      fireEvent.scroll(el)

      expect(useSidebarStore.getState().activeTab).toBe('files')
    })

    it('ignores scrolls while the sidebar is collapsed to zero width', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
      const el = carousel()
      selectFilesAndSettle(el)

      // Guards the zero-width early return: with no width there is no offset
      // that identifies a panel, so not even an armed gesture may be read back.
      fireEvent.wheel(el)
      setGeometry(el, 0, 0)
      fireEvent.scroll(el)

      expect(useSidebarStore.getState().activeTab).toBe('files')
    })

    it('still follows a wheel/swipe gesture onto the neighbouring panel', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
      const el = carousel()
      selectFilesAndSettle(el)

      fireEvent.wheel(el)
      setGeometry(el, 400, 400)
      fireEvent.scroll(el)

      expect(useSidebarStore.getState().activeTab).toBe('chats')
    })
  })
})
