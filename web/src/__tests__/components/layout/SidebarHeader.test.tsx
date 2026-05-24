import { render, screen, fireEvent } from '@testing-library/react'
import { beforeEach, vi } from 'vitest'
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

it('renders a settings button when onSettingsClick is provided', () => {
  render(<SidebarHeader userInitials="MU" onSettingsClick={vi.fn()} />)
  expect(screen.getByRole('button', { name: /settings/i })).toBeInTheDocument()
})

it('calls onSettingsClick when settings button is clicked', () => {
  const onSettingsClick = vi.fn()
  render(<SidebarHeader userInitials="MU" onSettingsClick={onSettingsClick} />)
  fireEvent.click(screen.getByRole('button', { name: /settings/i }))
  expect(onSettingsClick).toHaveBeenCalled()
})
