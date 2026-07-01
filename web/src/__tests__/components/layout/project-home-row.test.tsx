import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ProjectHomeRow } from '@/components/layout/project-home-row'
import { useProjectStore } from '@/lib/store/projects'

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

beforeEach(() => {
  navigateMock.mockClear()
  mockMatch = null
  useProjectStore.setState({ activeProjectId: 'p1', projects: [] })
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
