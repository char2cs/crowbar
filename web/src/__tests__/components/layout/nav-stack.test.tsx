import React from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { NavStack } from '@/components/layout/nav-stack'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

beforeEach(() => {
  useSidebarNavStore.getState().reset()
})

describe('NavStack', () => {
  it('renders children (root screen) when stack is empty', () => {
    render(<NavStack><div data-testid="root">Root</div></NavStack>)
    expect(screen.getByTestId('root')).toBeTruthy()
  })

  it('renders a pushed screen on top of root', () => {
    useSidebarNavStore.getState().push({
      id: 'test',
      title: 'Test Screen',
      component: <div data-testid="pushed">Pushed</div>,
    })
    render(<NavStack><div data-testid="root">Root</div></NavStack>)
    expect(screen.getByTestId('pushed')).toBeTruthy()
    expect(screen.getByText('Test Screen')).toBeTruthy()
  })

  it('back button pops the screen', async () => {
    useSidebarNavStore.getState().push({
      id: 'test',
      title: 'Test Screen',
      component: <div>Content</div>,
    })
    render(<NavStack><div>Root</div></NavStack>)
    await userEvent.click(screen.getByRole('button', { name: /back/i }))
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })
})
