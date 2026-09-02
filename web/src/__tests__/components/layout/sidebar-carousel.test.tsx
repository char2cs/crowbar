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
  // `RemovalTray` (now mounted inside SidebarCarousel — addendum §2 step 4)
  // calls both unconditionally before its own `entries.length === 0`
  // bailout; every test in this file leaves the removal tray empty, so
  // these are never actually exercised, just needed so the hook calls
  // themselves don't throw.
  useNavigate: () => vi.fn(),
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
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
  CaretDown: ({ className, ...rest }: { className?: string }) =>
    React.createElement('svg', { 'data-icon': 'caret-down', className, ...rest }),
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
    // The card's height persists across renders as a fraction of the rail
    // (sidebar-card-height.ts) — clear it so each test starts from the
    // spec's own one-third default, not a previous test's committed drag.
    localStorage.removeItem('sidebar-card-height-fraction')
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

    // Regression: getInitialState() used to default activeTab to 'workspaces',
    // a value TABS no longer contains post-narrowing — the Tabs `value` then
    // matched neither glyph and NEITHER tab underlined on a cold load, until
    // the user clicked one or the home-route effect below fired. This test
    // renders under the store's real, un-overridden default (beforeEach only
    // calls getInitialState(), no activeTab override) to catch that gap.
    it('the Files tab is active by default, before any click or route effect', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(screen.getByRole('tab', { name: 'Files' })).toHaveAttribute('aria-selected', 'true')
      expect(screen.getByRole('tab', { name: 'Git' })).toHaveAttribute('aria-selected', 'false')
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

    // Regression: a prior live-verification pass measured this head at 48px
    // (tab buttons at 32px) against the design spec's flat 28px (§6.1/§11:
    // "28px against the segmented bar's 36"). Root cause was two stacked
    // bugs — TabsTab's own `sm:h-8` beating a local `h-7` override in the
    // cascade, and TabsList's own underline-variant `py-1` doubling up with
    // this row's own vertical padding (32 + 8 + 8 = 48). Each assertion
    // below targets one of the three contributors so the whole head, not
    // just the buttons, lands at 28px.
    describe('the head is 28px flat, not 48px (spec §6.1/§11)', () => {
      it('the head row carries no vertical padding of its own', () => {
        render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
        const head = screen.getByTestId('carousel-head')
        expect(head).not.toHaveClass('py-1')
        expect(head).not.toHaveClass('py-0.5')
      })

      it("cancels the underline TabsList's own py-1 so it does not double the row's padding", () => {
        render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
        const list = screen.getByTestId('tabs-underline')
        expect(list).toHaveClass('data-[orientation=horizontal]:py-0')
      })

      it('locks each tab button at h-7 on every breakpoint, defeating sm:h-8', () => {
        render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
        const tab = screen.getByRole('tab', { name: 'Files' })
        expect(tab).toHaveClass('h-7')
        expect(tab).toHaveClass('sm:h-7')
      })
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

  describe('the floating card (spec §6)', () => {
    it('opens at one third of the sidebar height', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" sidebarHeight={900} />)
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '300px' })
    })

    it('the resize handle is the top 6px', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      const handle = screen.getByTestId('carousel-resize-handle')
      expect(handle).toHaveClass('h-1.5') // matches pane-sash.tsx's confirmed w-1.5/h-1.5 = 6px
    })

    it('renders with no explicit height before the rail has been measured', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(screen.getByTestId('carousel-card')).not.toHaveAttribute('style')
    })

    it('reports its own height so an ancestor can inset the tree by the same amount', () => {
      const onHeightChange = vi.fn()
      render(
        <SidebarCarousel
          activeWorkspaceRepoPath="/repo"
          sidebarHeight={900}
          onHeightChange={onHeightChange}
        />,
      )
      expect(onHeightChange).toHaveBeenCalledWith(300)
    })

    it('resizing the sidebar keeps the card at its own committed fraction, not a frozen pixel value', () => {
      const { rerender } = render(
        <SidebarCarousel activeWorkspaceRepoPath="/repo" sidebarHeight={900} />,
      )
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '300px' })
      rerender(<SidebarCarousel activeWorkspaceRepoPath="/repo" sidebarHeight={1200} />)
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '400px' })
    })

    it('dragging the top handle up grows the card and persists the new fraction', () => {
      const onHeightChange = vi.fn()
      const { unmount } = render(
        <SidebarCarousel
          activeWorkspaceRepoPath="/repo"
          sidebarHeight={900}
          onHeightChange={onHeightChange}
        />,
      )
      const handle = screen.getByTestId('carousel-resize-handle')
      fireEvent.pointerDown(handle, { button: 0, clientY: 500 })
      fireEvent.pointerMove(window, { clientY: 400 }) // dragged up 100px
      fireEvent.pointerUp(window, { clientY: 400 })

      // 300px (one third of 900) + 100px dragged up = 400px.
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '400px' })
      expect(onHeightChange).toHaveBeenLastCalledWith(400)
      unmount()

      // Persisted as a proportion of the rail — a fresh mount at the same
      // rail height opens back at the dragged size, not the one-third default.
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" sidebarHeight={900} />)
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '400px' })
    })

    it('a plain click on the handle (no movement) never commits a height change', () => {
      const onHeightChange = vi.fn()
      render(
        <SidebarCarousel
          activeWorkspaceRepoPath="/repo"
          sidebarHeight={900}
          onHeightChange={onHeightChange}
        />,
      )
      onHeightChange.mockClear()
      const handle = screen.getByTestId('carousel-resize-handle')
      fireEvent.pointerDown(handle, { button: 0, clientY: 500 })
      fireEvent.pointerUp(window, { clientY: 500 })
      expect(onHeightChange).not.toHaveBeenCalled()
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '300px' })
    })

    // Fix round 1: a live drag must cost zero React re-renders of anything
    // outside this component — sidebar-split-pane.tsx's own established
    // contract ("subsequent moves only update one CSS custom property").
    // `onHeightChange` (React state, one level up in ide-shell.tsx) is the
    // channel that used to fire every frame; the tree's own bottom inset now
    // comes from `--card-bottom-inset`, written directly onto `railRef`.
    describe('a live drag costs zero React re-renders (fix round 1)', () => {
      it('does not call onHeightChange for intermediate drag frames, even once their own animation frame has run', () => {
        vi.useFakeTimers()
        try {
          const onHeightChange = vi.fn()
          render(
            <SidebarCarousel
              activeWorkspaceRepoPath="/repo"
              sidebarHeight={900}
              onHeightChange={onHeightChange}
            />,
          )
          onHeightChange.mockClear()
          const handle = screen.getByTestId('carousel-resize-handle')

          fireEvent.pointerDown(handle, { button: 0, clientY: 500 })
          fireEvent.pointerMove(window, { clientY: 480 })
          vi.advanceTimersByTime(20) // flushes the coalesced animation frame
          fireEvent.pointerMove(window, { clientY: 440 })
          vi.advanceTimersByTime(20)
          fireEvent.pointerMove(window, { clientY: 400 })
          vi.advanceTimersByTime(20)

          // Every one of those frames actually ran (proven below) and none of
          // them called onHeightChange.
          expect(onHeightChange).not.toHaveBeenCalled()

          fireEvent.pointerUp(window, { clientY: 400 })
          expect(onHeightChange).toHaveBeenCalledTimes(1)
          expect(onHeightChange).toHaveBeenCalledWith(400)
        } finally {
          vi.useRealTimers()
        }
      })

      it("writes the live value straight onto railRef's --card-bottom-inset during the drag, with no onHeightChange call", () => {
        vi.useFakeTimers()
        try {
          const onHeightChange = vi.fn()
          const railRef = { current: document.createElement('div') }
          render(
            <SidebarCarousel
              activeWorkspaceRepoPath="/repo"
              sidebarHeight={900}
              railRef={railRef}
              onHeightChange={onHeightChange}
            />,
          )
          onHeightChange.mockClear()
          const handle = screen.getByTestId('carousel-resize-handle')

          fireEvent.pointerDown(handle, { button: 0, clientY: 500 })
          fireEvent.pointerMove(window, { clientY: 400 }) // dragged up 100px
          vi.advanceTimersByTime(20)

          expect(railRef.current.style.getPropertyValue('--card-bottom-inset')).toBe('400px')
          expect(onHeightChange).not.toHaveBeenCalled()
        } finally {
          vi.useRealTimers()
        }
      })

      it('a sibling with no props of its own is not re-rendered by any frame of the drag, only (at most) once on release', () => {
        let siblingRenders = 0
        function Sibling() {
          siblingRenders++
          return null
        }
        // Mirrors ide-shell.tsx's real shape: a shared rail ref passed to
        // SidebarCarousel, a piece of state updated by onHeightChange, and a
        // non-memoized sibling under the same parent — exactly the shape
        // the review found: SidebarTreeSurface/SpaceScroller/SpacePanel/
        // SidebarTree/SidebarRow are all such non-memoized siblings.
        function Harness() {
          const railRef = React.useRef<HTMLDivElement>(null)
          const [, setCommittedHeight] = React.useState(0)
          return (
            <div ref={railRef}>
              <Sibling />
              <SidebarCarousel
                activeWorkspaceRepoPath="/repo"
                sidebarHeight={900}
                railRef={railRef}
                onHeightChange={setCommittedHeight}
              />
            </div>
          )
        }

        vi.useFakeTimers()
        try {
          render(<Harness />)
          const rendersAfterMount = siblingRenders

          const handle = screen.getByTestId('carousel-resize-handle')
          fireEvent.pointerDown(handle, { button: 0, clientY: 500 })
          // `act()` around each flush forces React to actually commit any
          // state update synchronously before the next line runs — without
          // it, a state update from inside a fake-timer callback can sit
          // uncommitted past the very assertion meant to catch it, passing
          // for the wrong reason regardless of whether the fix is in place.
          fireEvent.pointerMove(window, { clientY: 480 })
          act(() => vi.advanceTimersByTime(20))
          fireEvent.pointerMove(window, { clientY: 440 })
          act(() => vi.advanceTimersByTime(20))
          fireEvent.pointerMove(window, { clientY: 400 })
          act(() => vi.advanceTimersByTime(20))

          // No frame of the drag re-rendered the sibling.
          expect(siblingRenders).toBe(rendersAfterMount)

          fireEvent.pointerUp(window, { clientY: 400 })

          // At most one additional render for the WHOLE drag, at release.
          expect(siblingRenders).toBeLessThanOrEqual(rendersAfterMount + 1)
        } finally {
          vi.useRealTimers()
        }
      })
    })
  })

  describe('folding the card (spec §6.4)', () => {
    function foldToggle(): HTMLElement {
      return screen.getByTestId('carousel-fold-toggle')
    }

    function caret(): HTMLElement {
      return screen.getByTestId('carousel-fold-caret')
    }

    function carousel(): HTMLElement {
      return screen.getByTestId('panel-files').closest('[data-sidebar-carousel]') as HTMLElement
    }

    it('renders the caret unrotated, body visible, before any click', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(caret()).not.toHaveClass('rotate-180')
      expect(carousel()).not.toHaveClass('hidden')
      expect(carousel()).toHaveClass('flex')
    })

    it('clicking the caret folds the card: hides the body, keeps the head and its tab selection', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      fireEvent.click(screen.getByRole('tab', { name: 'Git' }))
      expect(useSidebarStore.getState().activeTab).toBe('git')

      fireEvent.click(foldToggle())

      expect(carousel()).toHaveClass('hidden')
      expect(carousel()).not.toHaveClass('flex')
      // The head survives folding: both glyphs stay, and the tab you were on
      // is still the one showing as selected — folding doesn't lose it.
      expect(screen.getByRole('tab', { name: 'Files' })).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: 'Git' })).toHaveAttribute('aria-selected', 'true')
      expect(caret()).toHaveClass('rotate-180')
    })

    it('keeps both panels mounted while folded — not unmounted', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      fireEvent.click(foldToggle())
      expect(screen.getAllByTestId('carousel-panel')).toHaveLength(2)
    })

    it('clicking again unfolds it: same DOM node, scroll position not reset', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      const before = carousel()
      Object.defineProperty(before, 'scrollLeft', {
        value: 137,
        configurable: true,
        writable: true,
      })

      fireEvent.click(foldToggle()) // fold
      fireEvent.click(foldToggle()) // unfold

      const after = carousel()
      expect(after).toBe(before) // never unmounted/remounted
      expect(after.scrollLeft).toBe(137) // a remount would have reset this to 0
      expect(after).not.toHaveClass('hidden')
      expect(caret()).not.toHaveClass('rotate-180')
    })

    it('collapses the card height to just the head while folded, and restores it on unfold', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" sidebarHeight={900} />)
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '300px' })

      fireEvent.click(foldToggle())
      // React clears a dropped style prop by emptying `style.cssText`, which
      // jsdom reflects as an empty (not absent) `style` attribute — assert
      // the actual computed property rather than attribute presence.
      expect(screen.getByTestId('carousel-card').style.height).toBe('')

      fireEvent.click(foldToggle())
      expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '300px' })
    })

    it('hides the resize handle while folded — there is nothing to drag', () => {
      render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
      expect(screen.getByTestId('carousel-resize-handle')).toBeInTheDocument()

      fireEvent.click(foldToggle())
      expect(screen.queryByTestId('carousel-resize-handle')).not.toBeInTheDocument()

      fireEvent.click(foldToggle())
      expect(screen.getByTestId('carousel-resize-handle')).toBeInTheDocument()
    })
  })
})
