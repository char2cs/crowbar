import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ContextPill } from '@/components/layout/context-pill'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'

let mockPathname = '/'
vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => unknown }) =>
    select({ location: { pathname: mockPathname } }),
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
  useProjectStore.setState({
    projects: [{ id: 'p1', name: 'Crowbar', path: '/x', lastActivity: new Date(0) }],
    activeProjectId: 'p1',
  })
})

describe('ContextPill', () => {
  it('renders reponame/branchname in workspace mode', () => {
    mockPathname = '/workspaces/ws1'
    render(<ContextPill />)
    expect(screen.getByText('crowbar')).toBeInTheDocument()
    expect(screen.getByText('ide-polish')).toBeInTheDocument()
  })

  it('renders the project name when no workspace is active', () => {
    mockPathname = '/'
    render(<ContextPill />)
    expect(screen.getByText('Crowbar')).toBeInTheDocument()
  })

  it('switches the sidebar to the workspaces tab on click', () => {
    mockPathname = '/workspaces/ws1'
    render(<ContextPill />)
    fireEvent.click(screen.getByRole('button'))
    expect(useSidebarStore.getState().activeTab).toBe('workspaces')
  })

  it('renders nothing when nothing resolves', () => {
    mockPathname = '/'
    useProjectStore.setState({ projects: [], activeProjectId: '' })
    const { container } = render(<ContextPill />)
    expect(container).toBeEmptyDOMElement()
  })
})
