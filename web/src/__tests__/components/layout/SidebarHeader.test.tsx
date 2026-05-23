import { render, screen } from '@testing-library/react'
import { beforeEach } from 'vitest'
import { SidebarHeader } from '@/components/layout/SidebarHeader'
import { useProjectStore } from '@/lib/store/projects'

beforeEach(() => {
  useProjectStore.setState(useProjectStore.getInitialState())
})

test('renders active project name in breadcrumb dropdown', () => {
  render(<SidebarHeader userInitials="MU" />)
  expect(screen.getByText('Rabbyte')).toBeInTheDocument()
  expect(screen.getByText('Projects')).toBeInTheDocument()
})

test('renders user initials in avatar', () => {
  render(<SidebarHeader userInitials="MU" />)
  expect(screen.getByText('MU')).toBeInTheDocument()
})
