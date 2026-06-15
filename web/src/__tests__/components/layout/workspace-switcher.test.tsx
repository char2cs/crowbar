import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { WorkspaceSwitcherMenu } from '@/components/layout/workspace-switcher'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'

const navigateMock = vi.fn()
let mockPathname = '/workspaces/ws3'
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => unknown }) =>
    select({ location: { pathname: mockPathname } }),
}))

const repos: Repo[] = [
  {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [{ id: 'ws1', branch: 'develop', status: 'pr-open', age: '1d' }],
  },
  {
    id: 'r2',
    name: 'quiver.desktop',
    avatarLabel: 'Q',
    avatarColor: 'bg-teal-700',
    workspaces: [{ id: 'ws3', branch: 'feature/quiver-shell', status: 'new', age: '3d' }],
  },
]

beforeEach(() => {
  navigateMock.mockClear()
  mockPathname = '/workspaces/ws3'
  useSidebarStore.setState({ repos })
})

describe('WorkspaceSwitcherMenu', () => {
  it('renders a row for every workspace', () => {
    render(<WorkspaceSwitcherMenu onClose={() => {}} />)
    expect(screen.getByText('develop')).toBeInTheDocument()
    expect(screen.getByText('feature/quiver-shell')).toBeInTheDocument()
  })

  it('filters rows as the query changes', () => {
    render(<WorkspaceSwitcherMenu onClose={() => {}} />)
    fireEvent.change(screen.getByPlaceholderText('Switch workspace…'), {
      target: { value: 'develop' },
    })
    expect(screen.getByText('develop')).toBeInTheDocument()
    expect(screen.queryByText('feature/quiver-shell')).not.toBeInTheDocument()
  })

  it('navigates to the chosen workspace and closes on select', () => {
    const onClose = vi.fn()
    render(<WorkspaceSwitcherMenu onClose={onClose} />)
    fireEvent.click(screen.getByText('develop'))
    expect(navigateMock).toHaveBeenCalledWith({ to: '/workspaces/$wsId', params: { wsId: 'ws1' } })
    expect(onClose).toHaveBeenCalled()
  })

  it('shows an empty state when nothing matches', () => {
    render(<WorkspaceSwitcherMenu onClose={() => {}} />)
    fireEvent.change(screen.getByPlaceholderText('Switch workspace…'), {
      target: { value: 'zzzzz' },
    })
    expect(screen.getByText(/no workspaces/i)).toBeInTheDocument()
  })
})
