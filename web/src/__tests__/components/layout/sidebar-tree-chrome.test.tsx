/**
 * `SidebarTreeChrome` hoists `RemovalTray`/`RenameDialog`/`RepoImportDialog`/
 * `SidebarRowContextMenu` so they mount ONCE at the ide-shell level rather
 * than once per `SpaceScroller` project panel. "New Project" used to live
 * here too, as a tree-foot row; Task 5 relocated it to a trailing `+` mark
 * in `SidebarProjectHeader`'s window-chrome row (spec §4.1) and lifted its
 * modal state up to `IDEShell` — this component no longer owns any of that.
 */
import { createRef } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  // RemovalTray needs both.
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))

import { SidebarTreeChrome } from '@/components/layout/sidebar-tree-chrome'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
})

function renderChrome() {
  const treeRef = createRef<HTMLDivElement>()
  return render(<SidebarTreeChrome treeRef={treeRef} rows={[]} repos={[]} />)
}

describe('the leftover "New Project" row is gone', () => {
  it('no longer renders a "New Project" button', () => {
    renderChrome()
    expect(screen.queryByText('New Project')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /new project/i })).not.toBeInTheDocument()
  })

  it('does not mount ImportProjectModal itself', () => {
    renderChrome()
    expect(screen.queryByText('Import project')).not.toBeInTheDocument()
  })
})
