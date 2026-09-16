import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { CommitBox } from '@/features/git/components/commit-box'
import type { GitFile } from '@/features/git/types/git-types'

const { commitChanges, stagePaths } = vi.hoisted(() => ({
  commitChanges: vi.fn().mockResolvedValue(true),
  stagePaths: vi.fn().mockResolvedValue(true),
}))
vi.mock('@/features/git/api/git-commits-api', () => ({ commitChanges }))
vi.mock('@/features/git/api/git-status-api', () => ({ stagePaths }))

const files: GitFile[] = [
  { path: 'a.ts', status: 'modified', staged: false },
  { path: 'b.ts', status: 'modified', staged: false },
]

const renderBox = (onCommitted = vi.fn(), theFiles = files) =>
  render(<CommitBox wsId="w1" files={theFiles} onCommitted={onCommitted} />)

beforeEach(() => {
  vi.clearAllMocks()
})

describe('CommitBox', () => {
  it('renders an always-visible describe field, Commit and Pull request buttons — no popover', () => {
    renderBox()
    expect(screen.getByPlaceholderText('Describe the change…')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Commit' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Pull request' })).toBeInTheDocument()
  })

  it('Pull request has no backend yet, so it stays a disabled placeholder', () => {
    renderBox()
    expect(screen.getByRole('button', { name: 'Pull request' })).toBeDisabled()
  })

  it('Commit is disabled until there is a message and at least one changed file', async () => {
    const user = userEvent.setup()
    renderBox()
    const commit = screen.getByRole('button', { name: 'Commit' })
    expect(commit).toBeDisabled()
    await user.type(screen.getByPlaceholderText('Describe the change…'), 'msg')
    expect(commit).not.toBeDisabled()
  })

  it('Commit stays disabled with a message but no changed files', async () => {
    const user = userEvent.setup()
    renderBox(vi.fn(), [])
    await user.type(screen.getByPlaceholderText('Describe the change…'), 'msg')
    expect(screen.getByRole('button', { name: 'Commit' })).toBeDisabled()
  })

  it('stages every changed file, then commits', async () => {
    const user = userEvent.setup()
    const onCommitted = vi.fn()
    renderBox(onCommitted)
    await user.type(screen.getByPlaceholderText('Describe the change…'), 'my commit')
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(stagePaths).toHaveBeenCalledWith('w1', ['a.ts', 'b.ts'])
    expect(commitChanges).toHaveBeenCalledWith('w1', 'my commit')
    expect(onCommitted).toHaveBeenCalled()
  })

  it('does not commit when staging fails', async () => {
    stagePaths.mockResolvedValueOnce(false)
    const user = userEvent.setup()
    renderBox()
    await user.type(screen.getByPlaceholderText('Describe the change…'), 'my commit')
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(commitChanges).not.toHaveBeenCalled()
    expect(await screen.findByText('Failed to stage changes')).toBeInTheDocument()
  })
})
