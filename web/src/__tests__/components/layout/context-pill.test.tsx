import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ContextPill } from '@/components/layout/context-pill'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { success } from '@/lib/loadable'

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

// ── WorkspaceBranchIcon ──────────────────────────────────────────────────────
// Uses @phosphor-icons/react (.es.js, same ESM singleton issue).
vi.mock('@/components/layout/workspace-branch-icon', () => ({
  WorkspaceBranchIcon: () => null,
}))

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

beforeEach(() => {
  mockPathname = '/'
  useSidebarStore.setState({ repos, activeTab: 'files' })
  useProjectStore.setState({ activeProjectId: 'p1' })
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
