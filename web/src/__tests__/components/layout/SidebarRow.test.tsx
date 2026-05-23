import { render, screen } from '@testing-library/react'
import { ChatRow, RepoRow, WorkspaceRow, NewRow } from '@/components/layout/SidebarRow'

test('ChatRow renders title and age', () => {
  render(<ChatRow title="Architecture decisions" age="2h" active />)
  expect(screen.getByText('Architecture decisions')).toBeInTheDocument()
  expect(screen.getByText('2h')).toBeInTheDocument()
})

test('RepoRow renders repo name with avatar', () => {
  render(<RepoRow name="crowbar" avatarLabel="C" avatarColor="bg-primary" />)
  expect(screen.getByText('crowbar')).toBeInTheDocument()
  expect(screen.getByText('C')).toBeInTheDocument()
})

test('WorkspaceRow renders branch name and stats', () => {
  render(
    <WorkspaceRow
      num={3}
      branch="feature/app-design"
      added={5672}
      age="16h ago"
      active
    />,
  )
  expect(screen.getByText('feature/app-design')).toBeInTheDocument()
  expect(screen.getByText('+5672')).toBeInTheDocument()
  expect(screen.getByText('16h ago')).toBeInTheDocument()
})

test('WorkspaceRow renders deleted lines when provided', () => {
  render(
    <WorkspaceRow num={2} branch="feature/api-backend" added={27347} deleted={455} age="1d ago" />,
  )
  expect(screen.getByText('-455')).toBeInTheDocument()
})

test('NewRow renders label', () => {
  render(<NewRow label="New workspace" />)
  expect(screen.getByText('New workspace')).toBeInTheDocument()
})
