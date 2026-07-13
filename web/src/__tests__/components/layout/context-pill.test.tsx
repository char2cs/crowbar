import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ContextPill } from '@/components/layout/context-pill'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { success } from '@/lib/loadable'
import type { WorkspaceDTO } from '@/lib/types'

// @base-ui/react ships pure ESM (.mjs) and pnpm gives it its own React copy
// that diverges from react-dom's singleton in the vitest/jsdom process, causing
// "Cannot read properties of null (reading 'useContext')".  Rather than fighting
// the toolchain, we mock the three UI surfaces that pull in @base-ui/react so
// the tests can exercise ContextPill's own state-management and hook-wiring
// without any @base-ui dependency at test time.

// ── CommandDialog / CommandDialogTrigger / CommandDialogPopup ────────────────
// Thin stand-ins that propagate open state via a plain React context.
vi.mock('@/components/ui/command', async () => {
  const React = await import('react')
  type Ctx = { open: boolean; onOpenChange: (v: boolean) => void }
  const Ctx = React.createContext<Ctx>({ open: false, onOpenChange: () => {} })

  function CommandDialog({
    open,
    onOpenChange,
    children,
  }: {
    open: boolean
    onOpenChange: (v: boolean) => void
    children: React.ReactNode
  }) {
    return React.createElement(Ctx.Provider, { value: { open, onOpenChange } }, children)
  }

  function CommandDialogTrigger({
    children,
    render: renderEl,
  }: {
    children: React.ReactNode
    render: React.ReactElement<{ onClick?: () => void }>
  }) {
    const { onOpenChange } = React.useContext(Ctx)
    return React.cloneElement(renderEl, { onClick: () => onOpenChange(true) }, children)
  }

  function CommandDialogPopup({ children }: { children: React.ReactNode }) {
    const { open } = React.useContext(Ctx)
    return open ? React.createElement(React.Fragment, null, children) : null
  }

  return { CommandDialog, CommandDialogTrigger, CommandDialogPopup }
})

// ── Button ───────────────────────────────────────────────────────────────────
// @base-ui/react's useRender (used by Button) also fails with the React-
// singleton mismatch; replace with a minimal <button> wrapper.
vi.mock('@/components/ui/button', async () => {
  const React = await import('react')
  const Button = React.forwardRef<HTMLButtonElement, React.ComponentPropsWithoutRef<'button'>>(
    function Button(
      { children, 'aria-label': ariaLabel, onClick, disabled, className, ...rest },
      ref,
    ) {
      return React.createElement(
        'button',
        { 'aria-label': ariaLabel, onClick, disabled, className, ref, ...rest },
        children,
      )
    },
  )
  Button.displayName = 'Button'
  return { Button }
})

// ── WorkspaceSwitcherMenu ────────────────────────────────────────────────────
// The real menu uses Command/Autocomplete (also @base-ui/react).  A stub with
// the placeholder text is enough for "opens the switcher" assertions.
vi.mock('@/components/layout/workspace-switcher', () => ({
  WorkspaceSwitcherMenu: () => React.createElement('input', { placeholder: 'Switch workspace…' }),
}))

// NB: WorkspaceBranchIcon (and the FlickerSpinner it renders while `working`) is
// deliberately NOT mocked — the spinner IS what these tests assert, so the real
// component must render. Its own suite proves it works unmocked in jsdom.

// ── Platform + keymap ────────────────────────────────────────────────────────
// On macOS (where IS_MAC=true), "mod" is metaKey — tests fire ctrlKey, so
// stub IS_MAC=false so that ctrlKey is treated as the modifier, matching how
// the dedicated keyboard-hook tests work.
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'navigation.openWorkspaceSwitcher': 'mod+k',
  }),
}))

let mockPathname = '/'
vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => unknown }) =>
    select({ location: { pathname: mockPathname } }),
  useNavigate: () => vi.fn(),
}))

const repos: Repo[] = [
  {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [{ id: 'ws1', branch: 'ide-polish', status: 'pr-open', age: '1d' }],
  },
]

function homeDTO(working: boolean): WorkspaceDTO {
  return {
    id: 'home-1',
    repoId: '',
    projectId: 'p1',
    branch: 'home',
    status: 'new',
    working,
  } as WorkspaceDTO
}

beforeEach(() => {
  mockPathname = '/'
  useSidebarStore.setState({ repos, activeTab: 'files' })
  useProjectStore.setState({ activeProjectId: 'p1' })
  useHomeWorkspaceStore.setState({ workspace: null })
  // The pill reads the live project list (useProjectDataStore) for the name.
  useProjectDataStore.setState({
    data: success([{ id: 'p1', name: 'Crowbar', path: '/x', lastActivity: new Date(0) }]),
  })
})

describe('ContextPill', () => {
  it('renders reponame/branchname in workspace mode', () => {
    mockPathname = '/ide/p1/r1/ws1'
    render(<ContextPill />)
    expect(screen.getByText('crowbar')).toBeInTheDocument()
    expect(screen.getByText('ide-polish')).toBeInTheDocument()
  })

  it('renders the project name when no workspace is active', () => {
    mockPathname = '/'
    render(<ContextPill />)
    expect(screen.getByText('Crowbar')).toBeInTheDocument()
  })

  it('opens the workspace switcher on click', async () => {
    mockPathname = '/ide/p1/r1/ws1'
    render(<ContextPill />)
    fireEvent.click(screen.getByRole('button', { name: 'Switch workspace' }))
    expect(await screen.findByPlaceholderText('Switch workspace…')).toBeInTheDocument()
  })

  it('opens the workspace switcher on Ctrl+K keydown', async () => {
    mockPathname = '/ide/p1/r1/ws1'
    render(<ContextPill />)

    fireEvent.keyDown(window, { key: 'k', ctrlKey: true })

    expect(await screen.findByPlaceholderText('Switch workspace…')).toBeInTheDocument()
  })

  it('renders nothing when nothing resolves', () => {
    mockPathname = '/'
    useProjectStore.setState({ activeProjectId: '' })
    useProjectDataStore.setState({ data: success([]) })
    const { container } = render(<ContextPill />)
    expect(container).toBeEmptyDOMElement()
  })
})

// The pill shows the ACTIVE workspace's icon, and that icon must move into its
// loading state while an agent works — for every workspace kind.
describe('ContextPill working overlay', () => {
  const spinner = () => screen.queryByRole('status', { name: 'Loading' })

  it('spins the icon on a WORKTREE workspace (no regression)', () => {
    mockPathname = '/ide/p1/r1/ws1'
    useSidebarStore.setState({
      repos: [
        {
          ...repos[0],
          workspaces: [
            { id: 'ws1', branch: 'ide-polish', status: 'pr-open', age: '1d', working: true },
          ],
        },
      ],
    })

    render(<ContextPill />)

    expect(spinner()).toBeInTheDocument()
  })

  it('shows the status glyph (no spinner) on an idle worktree workspace', () => {
    mockPathname = '/ide/p1/r1/ws1'
    render(<ContextPill />)
    expect(spinner()).toBeNull()
  })

  it('spins the icon on PROJECT HOME when the home workspace is working', () => {
    mockPathname = '/ide/p1/home'
    useHomeWorkspaceStore.setState({ workspace: homeDTO(true) })

    const { container } = render(<ContextPill />)

    expect(spinner()).toBeInTheDocument()
    // Real flicker spinner, theme-token colored — never a hardcoded color.
    expect(container.querySelector('svg animate')).not.toBeNull()
    expect(container.querySelector('.text-foreground')).not.toBeNull()
  })

  it('shows the House glyph (no spinner) on an idle project home', () => {
    mockPathname = '/ide/p1/home'
    useHomeWorkspaceStore.setState({ workspace: homeDTO(false) })

    render(<ContextPill />)

    expect(screen.getByText('home')).toBeInTheDocument()
    expect(spinner()).toBeNull()
  })

  it('spins the icon on REPO HOME (the default workspace), replacing the repo avatar', () => {
    mockPathname = '/ide/p1/r1/default-ws'
    useSidebarStore.setState({
      repos: [{ ...repos[0], defaultWorkspaceId: 'default-ws', defaultWorking: true }],
    })

    render(<ContextPill />)

    expect(spinner()).toBeInTheDocument()
  })

  it('shows the repo avatar (no spinner) on an idle repo home', () => {
    mockPathname = '/ide/p1/r1/default-ws'
    useSidebarStore.setState({
      repos: [{ ...repos[0], defaultWorkspaceId: 'default-ws', defaultWorking: false }],
    })

    render(<ContextPill />)

    expect(spinner()).toBeNull()
    expect(screen.getByText('default')).toBeInTheDocument()
  })
})
