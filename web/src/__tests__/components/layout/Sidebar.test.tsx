import { render, screen } from '@testing-library/react'
import { beforeEach } from 'vitest'
import { Sidebar } from '@/components/layout/Sidebar'
import { useProjectStore } from '@/lib/store/projects'

beforeEach(() => {
  useProjectStore.setState(useProjectStore.getInitialState())
})

const CHATS = [{ id: 'c1', title: 'Auth design', age: '1h' }]
const REPOS = [
  {
    id: 'r1', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-primary',
    workspaces: [{ id: 'ws1', branch: 'feature/x', age: '2h' }],
  },
]

test('renders active project name and chats', () => {
  render(
    <Sidebar userInitials="MU" chats={CHATS} repos={REPOS} />,
  )
  expect(screen.getByText('Rabbyte')).toBeInTheDocument()
  expect(screen.getByText('Auth design')).toBeInTheDocument()
})

test('renders repo name and workspace branch', () => {
  render(
    <Sidebar userInitials="MU" chats={CHATS} repos={REPOS} />,
  )
  expect(screen.getByText('crowbar')).toBeInTheDocument()
  expect(screen.getByText('feature/x')).toBeInTheDocument()
})

test('renders New chat and New workspace rows', () => {
  render(
    <Sidebar userInitials="MU" chats={CHATS} repos={REPOS} />,
  )
  expect(screen.getByText('New chat')).toBeInTheDocument()
  expect(screen.getByText('New workspace')).toBeInTheDocument()
})
