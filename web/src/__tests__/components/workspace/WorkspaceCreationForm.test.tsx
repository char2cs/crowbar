import { render, screen, fireEvent } from '@testing-library/react'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'
import { vi } from 'vitest'

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
]
const FLOWS = [
  { name: 'feature-development', description: 'Full feature development' },
]

test('renders repo select, branch input, and workflow select', () => {
  render(<WorkspaceCreationForm repos={REPOS} flows={FLOWS} onSubmit={() => {}} />)
  expect(screen.getByLabelText('Repo')).toBeInTheDocument()
  expect(screen.getByLabelText('Branch')).toBeInTheDocument()
  expect(screen.getByLabelText('Workflow')).toBeInTheDocument()
})

test('Create button is disabled when branch is empty', () => {
  render(<WorkspaceCreationForm repos={REPOS} flows={FLOWS} onSubmit={() => {}} />)
  expect(screen.getByRole('button', { name: /create/i })).toBeDisabled()
})

test('calls onSubmit with selected values', () => {
  const onSubmit = vi.fn()
  render(<WorkspaceCreationForm repos={REPOS} flows={FLOWS} onSubmit={onSubmit} />)
  fireEvent.change(screen.getByLabelText('Repo'), { target: { value: 'quiver-core' } })
  fireEvent.change(screen.getByLabelText('Branch'), { target: { value: 'feature/new-thing' } })
  fireEvent.change(screen.getByLabelText('Workflow'), { target: { value: 'feature-development' } })
  fireEvent.click(screen.getByRole('button', { name: /create/i }))
  expect(onSubmit).toHaveBeenCalledWith({
    repoId: 'quiver-core',
    branch: 'feature/new-thing',
    flowName: 'feature-development',
  })
})
