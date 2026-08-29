import React from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SidebarCarousel } from '@/components/layout/sidebar-carousel'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

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

let mockMatch: object | null = null
vi.mock('@tanstack/react-router', () => ({
  useMatch: () => mockMatch,
}))

// @phosphor-icons/react ships pure ESM and gets its own React copy in the
// vitest/jsdom process, causing "Cannot read properties of null (reading
// 'useRef')". Mock it to plain SVG stubs, same pattern the retired
// sidebar-tab-bar.test.tsx used.
vi.mock('@phosphor-icons/react', () => ({
  FolderOpen: ({ size, weight }: { size?: number; weight?: string }) =>
    React.createElement('svg', {
      'data-icon': 'folder-open',
      'data-size': size,
      'data-weight': weight,
    }),
  GitBranch: ({ size, weight }: { size?: number; weight?: string }) =>
    React.createElement('svg', {
      'data-icon': 'git-branch',
      'data-size': size,
      'data-weight': weight,
    }),
}))

// @base-ui/react ships pure ESM (.mjs) and pnpm gives it its own React copy
// that diverges from react-dom's singleton in the vitest/jsdom process,
// causing the same "useRef" crash. Mock ui/tabs to plain markup that still
// forwards every prop a real caller cares about (value/onValueChange/variant/
// data-testid/className/aria-label), so the underline head's own props are
// exercised for real rather than by the mock's own tabs-list default.
interface TabsInjectedProps {
  _value?: string
  _onValueChange?: (value: string) => void
}
vi.mock('@/components/ui/tabs', () => ({
  Tabs: ({
    children,
    value,
    onValueChange,
  }: {
    children?: React.ReactNode
    value?: string
    onValueChange?: (value: string) => void
  }) => {
    const injected = React.Children.map(children, (child) =>
      React.isValidElement<TabsInjectedProps>(child)
        ? React.cloneElement(child, { _value: value, _onValueChange: onValueChange })
        : child,
    )
    return React.createElement('div', {}, injected)
  },
  TabsList: ({
    children,
    className,
    variant,
    _value,
    _onValueChange,
    ...rest
  }: {
    children?: React.ReactNode
    className?: string
    variant?: string
  } & TabsInjectedProps &
    Record<string, unknown>) => {
    const injected = React.Children.map(children, (child) =>
      React.isValidElement<TabsInjectedProps>(child)
        ? React.cloneElement(child, { _value, _onValueChange })
        : child,
    )
    return React.createElement('div', { className, 'data-variant': variant, ...rest }, injected)
  },
  TabsTab: ({
    children,
    value,
    className,
    _value,
    _onValueChange,
    ...rest
  }: {
    children?: React.ReactNode
    value?: string
    className?: string
  } & TabsInjectedProps &
    Record<string, unknown>) =>
    React.createElement(
      'button',
      {
        role: 'tab',
        'aria-selected': value === _value ? 'true' : 'false',
        className,
        onClick: () => _onValueChange?.(value ?? ''),
        ...rest,
      },
      children,
    ),
}))

describe('SidebarCarousel', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    mockMatch = null
    // jsdom does not implement scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
  })

  it('mounts both panels: Files and Git', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    expect(screen.getByTestId('panel-files')).toBeInTheDocument()
    expect(screen.getByTestId('panel-git')).toBeInTheDocument()
  })

  it('the card has exactly two panels', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    expect(screen.getAllByTestId('carousel-panel')).toHaveLength(2)
  })

  it('renders the panels in Files, Git order (index math must not be off-by-one)', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    const container = screen.getByTestId('panel-files').closest('[data-sidebar-carousel]')
    const testIds = Array.from(container?.querySelectorAll('[data-testid^="panel-"]') ?? []).map(
      (el) => el.getAttribute('data-testid'),
    )
    expect(testIds).toEqual(['panel-files', 'panel-git'])
  })

  it('scrolls to the Git panel index (1) when activeTab is git', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    const container = screen
      .getByTestId('panel-files')
      .closest('[data-sidebar-carousel]') as HTMLElement
    Object.defineProperty(container, 'clientWidth', { value: 400, configurable: true })
    const scrollToSpy = HTMLElement.prototype.scrollTo as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    act(() => {
      useSidebarStore.setState({ activeTab: 'git' })
    })

    expect(scrollToSpy).toHaveBeenCalledWith(
      expect.objectContaining({ left: 400, behavior: 'smooth' }),
    )
  })

  describe('the head', () => {
    it('uses the underline tabs variant, icon only', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(screen.queryByText('Files')).not.toBeInTheDocument()
      expect(screen.queryByText('Git')).not.toBeInTheDocument()
      expect(screen.getByTestId('tabs-underline')).toBeInTheDocument()
    })

    it('holds exactly two glyphs — Files and Git — off the home route', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(screen.getAllByRole('tab')).toHaveLength(2)
      expect(screen.getByRole('tab', { name: 'Files' })).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: 'Git' })).toBeInTheDocument()
    })

    it('clicking the Git glyph switches activeTab', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      fireEvent.click(screen.getByRole('tab', { name: 'Git' }))
      expect(useSidebarStore.getState().activeTab).toBe('git')
    })

    // Git has no meaning without a repo, and the project-home route has no
    // active workspace — carried over from the retired SidebarTabBar.
    it('hides the Git glyph on the home route', () => {
      mockMatch = { params: { projectId: 'p1' } }
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(screen.getAllByRole('tab')).toHaveLength(1)
      expect(screen.getByRole('tab', { name: 'Files' })).toBeInTheDocument()
    })

    it('resets activeTab to files when navigating to the home route with git active', () => {
      useSidebarStore.setState({ activeTab: 'git' })
      mockMatch = { params: { projectId: 'p1' } }
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(useSidebarStore.getState().activeTab).toBe('files')
    })
  })

  // Regression: hiding and re-showing the sidebar while the Files panel was
  // active landed on a neighbouring panel. Collapsing the panel drives the
  // carousel's width to 0, the browser clamps scrollLeft, and the offsets that
  // arrive while the sidebar reopens do not correspond to the width the
  // container ends up with — reading them back through Math.round() picked a
  // neighbouring panel and reassigned activeTab. Only a real scroll gesture may
  // move the tab.
  describe('activeTab is only derived from scroll offsets the user caused', () => {
    function carousel(): HTMLElement {
      return screen.getByTestId('panel-files').closest('[data-sidebar-carousel]') as HTMLElement
    }

    function setGeometry(el: HTMLElement, clientWidth: number, scrollLeft: number) {
      Object.defineProperty(el, 'clientWidth', { value: clientWidth, configurable: true })
      Object.defineProperty(el, 'scrollLeft', { value: scrollLeft, configurable: true })
    }

    // Select a tab and let its programmatic scroll finish, as the browser does.
    // Without the scrollend the carousel is still mid-animation and would have
    // ignored the scroll below for a reason that has nothing to do with gestures.
    function selectGitAndSettle(el: HTMLElement) {
      act(() => {
        useSidebarStore.setState({ activeTab: 'git' })
      })
      fireEvent(el, new Event('scrollend'))
    }

    it('ignores a settled scroll offset that no gesture produced', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
      const el = carousel()
      selectGitAndSettle(el)

      // An offset that rounds back to panel index 1 (Git, still the active
      // one) — what a collapse/expand cycle leaves behind, since the offset
      // was written for a width the container no longer has.
      setGeometry(el, 400, 400)
      fireEvent.scroll(el)

      expect(useSidebarStore.getState().activeTab).toBe('git')
    })

    it('ignores scrolls while the sidebar is collapsed to zero width', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
      const el = carousel()
      selectGitAndSettle(el)

      // Guards the zero-width early return: with no width there is no offset
      // that identifies a panel, so not even an armed gesture may be read back.
      fireEvent.wheel(el)
      setGeometry(el, 0, 0)
      fireEvent.scroll(el)

      expect(useSidebarStore.getState().activeTab).toBe('git')
    })

    it('still follows a wheel/swipe gesture onto the neighbouring panel', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
      const el = carousel()
      // Start from Files (index 0) this time — Git (index 1) is the last panel,
      // so the neighbouring-panel gesture here must move left, onto Files.
      fireEvent.wheel(el)
      setGeometry(el, 400, 0)
      fireEvent.scroll(el)

      expect(useSidebarStore.getState().activeTab).toBe('files')
    })
  })
})
