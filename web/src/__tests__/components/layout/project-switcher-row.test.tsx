// web/src/__tests__/components/layout/project-switcher-row.test.tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { ProjectSwitcherRow } from '@/components/layout/project-switcher-row'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { success } from '@/lib/loadable'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

beforeEach(() => {
  useSidebarNavStore.getState().reset()
  useProjectStore.setState({ activeProjectId: 'p1' })
  // The row reads the live project list (useProjectDataStore), not the
  // import-only useProjectStore.projects.
  useProjectDataStore.setState({
    data: success([{ id: 'p1', name: 'Rabbyte', path: '/a', lastActivity: new Date(0) }]),
  })
})

describe('ProjectSwitcherRow', () => {
  it('shows the active project name', () => {
    render(<ProjectSwitcherRow />)
    expect(screen.getByRole('button', { name: /Rabbyte/ })).toBeTruthy()
  })

  it('pushes the project switcher screen on click', async () => {
    render(<ProjectSwitcherRow />)
    await userEvent.click(screen.getByRole('button', { name: /Rabbyte/ }))
    const stack = useSidebarNavStore.getState().stack
    expect(stack).toHaveLength(1)
    expect(stack[0].id).toBe('project-switcher')
    expect(stack[0].title).toBe('Projects')
  })

  it('falls back to "Select project" when none is active', () => {
    useProjectStore.setState({ activeProjectId: '' })
    useProjectDataStore.setState({ data: success([]) })
    render(<ProjectSwitcherRow />)
    expect(screen.getByRole('button', { name: /Select project/ })).toBeTruthy()
  })
})
