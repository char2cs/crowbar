import { render, screen, fireEvent } from '@testing-library/react'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'
import { vi } from 'vitest'

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
]

test('renders repo select and branch input', () => {
  render(<WorkspaceCreationForm repos={REPOS} onSubmit={() => {}} />)
  expect(screen.getByLabelText('Repo')).toBeInTheDocument()
  expect(screen.getByLabelText('Branch')).toBeInTheDocument()
})

test('Create button is disabled when branch is empty', () => {
  render(<WorkspaceCreationForm repos={REPOS} onSubmit={() => {}} />)
  expect(screen.getByRole('button', { name: /create/i })).toBeDisabled()
})

test('calls onSubmit with selected values', () => {
  const onSubmit = vi.fn()
  render(<WorkspaceCreationForm repos={REPOS} onSubmit={onSubmit} />)
  fireEvent.change(screen.getByLabelText('Repo'), { target: { value: 'quiver-core' } })
  fireEvent.change(screen.getByLabelText('Branch'), { target: { value: 'feature/new-thing' } })
  fireEvent.click(screen.getByRole('button', { name: /create/i }))
  expect(onSubmit).toHaveBeenCalledWith({
    repoId: 'quiver-core',
    branch: 'feature/new-thing',
  })
})

test('Create button is enabled when branch is filled', () => {
  render(<WorkspaceCreationForm repos={REPOS} onSubmit={() => {}} />)
  fireEvent.change(screen.getByLabelText('Branch'), { target: { value: 'main' } })
  expect(screen.getByRole('button', { name: /create/i })).not.toBeDisabled()
})
