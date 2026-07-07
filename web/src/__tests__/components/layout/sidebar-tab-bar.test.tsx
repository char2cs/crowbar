import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SidebarTabBar } from '@/components/layout/sidebar-tab-bar'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

// @base-ui/react ships pure ESM (.mjs) and pnpm gives it its own React copy
// that diverges from react-dom's singleton in the vitest/jsdom process, causing
// "Cannot read properties of null (reading 'useRef')". Mock Tabs component to
// a simple wrapper so tests can exercise the component's logic.
interface TabsInjectedProps {
  _tabsValue?: string
  _onValueChange?: (value: string) => void
}

interface TabsProps extends TabsInjectedProps {
  children?: React.ReactNode
  value?: string
  onValueChange?: (value: string) => void
  className?: string
}

interface TabsTabProps extends TabsInjectedProps {
  children?: React.ReactNode
  value?: string
  className?: string
}

vi.mock('@/components/ui/tabs', () => ({
  Tabs: ({ children, value, onValueChange, className }: TabsProps) => {
    // Pass value and onValueChange through context-like props to children
    const processedChildren = React.Children.map(children, (child) => {
      if (!React.isValidElement<TabsInjectedProps>(child)) return child
      return React.cloneElement(child, { _tabsValue: value, _onValueChange: onValueChange })
    })
    return React.createElement('div', { 'data-testid': 'tabs', className }, processedChildren)
  },
  TabsList: ({ children, className, _tabsValue, _onValueChange }: TabsProps) => {
    const processedChildren = React.Children.map(children, (child) => {
      if (!React.isValidElement<TabsInjectedProps>(child)) return child
      return React.cloneElement(child, { _tabsValue, _onValueChange })
    })
    return React.createElement('div', { 'data-testid': 'tabs-list', className }, processedChildren)
  },
  TabsTab: ({ children, value, className, _tabsValue, _onValueChange, ...props }: TabsTabProps) =>
    React.createElement(
      'button',
      {
        role: 'tab',
        'aria-selected': value === _tabsValue ? 'true' : 'false',
        className,
        onClick: () => _onValueChange?.(value ?? ''),
        ...props,
      },
      children,
    ),
}))

// @phosphor-icons/react ships pure ESM and gets its own React copy in the
// vitest/jsdom process, causing "Cannot read properties of null (reading 'useRef')".
// Mock it to a plain SVG stub so tests can exercise component logic without the
// ESM singleton issue.
vi.mock('@phosphor-icons/react', () => ({
  SquaresFour: ({ size, weight }: { size?: number; weight?: string }) =>
    React.createElement('svg', {
      'data-icon': 'squares-four',
      'data-size': size,
      'data-weight': weight,
    }),
  FolderOpen: ({ size, weight }: { size?: number; weight?: string }) =>
    React.createElement('svg', {
      'data-icon': 'folder-open',
      'data-size': size,
      'data-weight': weight,
    }),
  GitBranch: ({ size, weight }: { size?: number; weight?: string }) =>
    React.createElement('svg', {
      'data-icon': 'git-branch',
      'data-size': size,
      'data-weight': weight,
    }),
}))

let mockMatch: object | null = null

vi.mock('@tanstack/react-router', () => ({
  useMatch: () => mockMatch,
}))

describe('SidebarTabBar', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    mockMatch = null
  })

  it('renders all 3 tabs when not on home route', () => {
    render(<SidebarTabBar />)
    expect(screen.getByRole('tab', { name: /workspaces/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /files/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /git/i })).toBeInTheDocument()
  })

  it('marks the active tab as selected', () => {
    useSidebarStore.setState({ activeTab: 'files' })
    render(<SidebarTabBar />)
    expect(screen.getByRole('tab', { name: /files/i })).toHaveAttribute('aria-selected', 'true')
  })

  it('calls setActiveTab when a tab is clicked', () => {
    render(<SidebarTabBar />)
    fireEvent.click(screen.getByRole('tab', { name: /files/i }))
    expect(useSidebarStore.getState().activeTab).toBe('files')
  })

  it('hides the git tab on the home route', () => {
    mockMatch = { params: { projectId: 'p1' } }
    render(<SidebarTabBar />)
    expect(screen.queryByRole('tab', { name: /git/i })).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /workspaces/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /files/i })).toBeInTheDocument()
  })

  it('resets active tab to workspaces when navigating to home route with git active', () => {
    useSidebarStore.setState({ activeTab: 'git' })
    mockMatch = { params: { projectId: 'p1' } }
    render(<SidebarTabBar />)
    expect(useSidebarStore.getState().activeTab).toBe('workspaces')
  })
})
