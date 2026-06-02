import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { SidebarTabBar } from '@/components/layout/sidebar-tab-bar'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('SidebarTabBar', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('renders all 4 tabs', () => {
    render(<SidebarTabBar />)
    expect(screen.getByRole('button', { name: /workspaces/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chats/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /files/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /git/i })).toBeInTheDocument()
  })

  it('marks the active tab as pressed', () => {
    useSidebarStore.setState({ activeTab: 'files' })
    render(<SidebarTabBar />)
    expect(screen.getByRole('button', { name: /files/i })).toHaveAttribute('data-active', 'true')
  })

  it('calls setActiveTab when a tab is clicked', () => {
    render(<SidebarTabBar />)
    fireEvent.click(screen.getByRole('button', { name: /chats/i }))
    expect(useSidebarStore.getState().activeTab).toBe('chats')
  })
})
