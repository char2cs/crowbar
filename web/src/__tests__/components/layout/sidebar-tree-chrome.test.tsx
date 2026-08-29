/**
 * `SidebarTreeChrome` is what `sidebar-tree-panel.tsx`'s "New Project" entry
 * point (the app's only way to add a second project once past the
 * zero-project /oobe screen) moved to — hoisted so it mounts ONCE at the
 * ide-shell level rather than once per `SpaceScroller` project panel.
 */
import { createRef } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  // RemovalTray needs both.
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))

const { importProjectAndSync } = vi.hoisted(() => ({
  importProjectAndSync: vi.fn(),
}))
vi.mock('@/lib/store/projects', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/store/projects')>()),
  importProjectAndSync,
}))
// The real modal drives a Tauri native-dialog / postProject flow this file has
// no business exercising — SidebarTreeChrome's own contract is just "open it,
// and hand its result to importProjectAndSync", which this stub proves
// without any of that.
vi.mock('@/components/projects/import-project-modal', () => ({
  ImportProjectModal: ({
    open,
    onImport,
  }: {
    open: boolean
    onImport: (p: { id: string; name: string; path: string; lastActivity: Date }) => void
  }) =>
    open ? (
      <button
        type="button"
        onClick={() =>
          onImport({ id: 'p2', name: 'second-project', path: '/p2', lastActivity: new Date(0) })
        }
      >
        confirm-import
      </button>
    ) : null,
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

describe('New Project entry point', () => {
  it('renders a "New Project" row', () => {
    renderChrome()
    expect(screen.getByText('New Project')).toBeInTheDocument()
  })

  it('opens the import modal on click, and importing closes it and syncs the project', async () => {
    const user = userEvent.setup()
    renderChrome()

    expect(screen.queryByText('confirm-import')).not.toBeInTheDocument()
    await user.click(screen.getByText('New Project'))
    expect(screen.getByText('confirm-import')).toBeInTheDocument()

    await user.click(screen.getByText('confirm-import'))
    expect(importProjectAndSync).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ id: 'p2', name: 'second-project' }),
    )
    expect(screen.queryByText('confirm-import')).not.toBeInTheDocument()
  })
})

describe('mounting once, not per panel', () => {
  it('renders exactly one "New Project" row and one removal tray region regardless of caller', () => {
    // The whole point of hoisting: this renders once for however many
    // projects the sidebar has, never once per SpaceScroller panel.
    renderChrome()
    expect(screen.getAllByText('New Project')).toHaveLength(1)
  })
})
