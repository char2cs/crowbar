import { render, screen } from '@testing-library/react'
import { SidebarHeader } from '@/components/layout/SidebarHeader'

test('renders project name in breadcrumb', () => {
  render(<SidebarHeader projectName="Rabbyte" userInitials="MU" />)
  expect(screen.getByText('Rabbyte')).toBeInTheDocument()
  expect(screen.getByText('Projects')).toBeInTheDocument()
})

test('renders user initials in avatar', () => {
  render(<SidebarHeader projectName="Rabbyte" userInitials="MU" />)
  expect(screen.getByText('MU')).toBeInTheDocument()
})
