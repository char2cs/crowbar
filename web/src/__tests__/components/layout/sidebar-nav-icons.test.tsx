import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { SidebarNavIcons } from '@/components/layout/sidebar-nav-icons'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('SidebarNavIcons', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('renders three icon buttons', () => {
    render(<SidebarNavIcons />)
    expect(screen.getByRole('button', { name: 'Workspaces' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Files' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Git' })).toBeInTheDocument()
  })

  it('active tab button has aria-pressed true', () => {
    render(<SidebarNavIcons />)
    expect(screen.getByRole('button', { name: 'Workspaces' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Files' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: 'Git' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('active button has bg-accent class', () => {
    render(<SidebarNavIcons />)
    expect(screen.getByRole('button', { name: 'Workspaces' })).toHaveClass('bg-accent')
    expect(screen.getByRole('button', { name: 'Files' })).not.toHaveClass('bg-accent')
  })

  it('clicking a button sets the active tab in the store', () => {
    render(<SidebarNavIcons />)
    fireEvent.click(screen.getByRole('button', { name: 'Files' }))
    expect(useSidebarStore.getState().activeTab).toBe('files')
  })

  it('clicking the git button sets active tab to git', () => {
    render(<SidebarNavIcons />)
    fireEvent.click(screen.getByRole('button', { name: 'Git' }))
    expect(useSidebarStore.getState().activeTab).toBe('git')
  })
})
