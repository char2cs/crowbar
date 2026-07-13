import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ProjectHomeRow } from '@/components/layout/project-home-row'
import { useProjectStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import type { WorkspaceDTO } from '@/lib/types'

// @phosphor-icons/react ships pure ESM and gets its own React copy in the
// vitest/jsdom process, causing "Cannot read properties of null (reading 'useContext')".
// Mock it to a plain SVG stub so tests can exercise component logic without the
// ESM singleton issue.
vi.mock('@phosphor-icons/react', () => ({
  House: ({ size, weight }: { size?: number; weight?: string }) =>
    React.createElement('svg', { 'data-icon': 'house', 'data-size': size, 'data-weight': weight }),
  FolderPlus: ({ size }: { size?: number }) =>
    React.createElement('svg', { 'data-icon': 'folder-plus', 'data-size': size }),
}))

const navigateMock = vi.fn()
let mockMatch: object | null = null

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
  useMatch: () => mockMatch,
}))

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
  navigateMock.mockClear()
  mockMatch = null
  useProjectStore.setState({ activeProjectId: 'p1', projects: [] })
  useHomeWorkspaceStore.setState({ workspace: null })
})

describe('ProjectHomeRow', () => {
  it('renders with label "Home"', () => {
    render(<ProjectHomeRow />)
    expect(screen.getByRole('button', { name: /Home/i })).toBeInTheDocument()
  })

  it('navigates to home route on click', () => {
    render(<ProjectHomeRow />)
    fireEvent.click(screen.getByRole('button', { name: /Home/i }))
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p1' },
    })
  })

  it('does not navigate when no active project', () => {
    useProjectStore.setState({ activeProjectId: '', projects: [] })
    render(<ProjectHomeRow />)
    fireEvent.click(screen.getByRole('button', { name: /Home/i }))
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('applies ROW_ACTIVE class when route matches', () => {
    mockMatch = { params: { projectId: 'p1' } }
    render(<ProjectHomeRow />)
    const btn = screen.getByRole('button', { name: /Home/i })
    // ROW_ACTIVE contains 'bg-background' — confirm presence
    expect(btn.className).toContain('bg-background')
  })

  it('applies ROW_INACTIVE class when route does not match', () => {
    mockMatch = null
    render(<ProjectHomeRow />)
    const btn = screen.getByRole('button', { name: /Home/i })
    expect(btn.className).toContain('border-transparent')
  })

  it('navigates on Enter key', () => {
    render(<ProjectHomeRow />)
    const btn = screen.getByRole('button', { name: /Home/i })
    fireEvent.keyDown(btn, { key: 'Enter' })
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p1' },
    })
  })

  it('navigates on Space key', () => {
    render(<ProjectHomeRow />)
    const btn = screen.getByRole('button', { name: /Home/i })
    fireEvent.keyDown(btn, { key: ' ' })
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p1' },
    })
  })
})

// The home row is the home workspace's tile: an agent working in project home
// must move its icon into the loading state, exactly as a worktree row's branch
// glyph becomes the spinner (WorkspaceBranchIcon).
describe('ProjectHomeRow working overlay', () => {
  it('renders the flicker spinner while the home workspace is working', () => {
    useHomeWorkspaceStore.setState({ workspace: homeDTO(true) })

    const { container } = render(<ProjectHomeRow />)

    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    // The real flicker spinner (self-animating SVG), theme-token colored.
    expect(container.querySelector('svg animate')).not.toBeNull()
    expect(container.querySelector('.text-foreground')).not.toBeNull()
    // The House glyph yields to it for the duration of the turn.
    expect(container.querySelector('[data-icon="house"]')).toBeNull()
  })

  it('renders the House glyph (no spinner) when the home workspace is idle', () => {
    useHomeWorkspaceStore.setState({ workspace: homeDTO(false) })

    const { container } = render(<ProjectHomeRow />)

    expect(screen.queryByRole('status', { name: 'Loading' })).toBeNull()
    expect(container.querySelector('[data-icon="house"]')).not.toBeNull()
  })

  it('renders the House glyph before the home workspace has been read', () => {
    useHomeWorkspaceStore.setState({ workspace: null })

    const { container } = render(<ProjectHomeRow />)

    expect(screen.queryByRole('status', { name: 'Loading' })).toBeNull()
    expect(container.querySelector('[data-icon="house"]')).not.toBeNull()
  })
})
