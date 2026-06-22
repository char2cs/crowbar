import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Button } from '@/components/ui/button'
import { CommitPopover } from '@/features/git/components/commit-popover'
import type { GitFile } from '@/features/git/types/git-types'

const { commitChanges, stagePaths, unstagePaths } = vi.hoisted(() => ({
  commitChanges: vi.fn().mockResolvedValue(true),
  stagePaths: vi.fn().mockResolvedValue(true),
  unstagePaths: vi.fn().mockResolvedValue(true),
}))
vi.mock('@/features/git/api/git-commits-api', () => ({ commitChanges }))
vi.mock('@/features/git/api/git-status-api', () => ({ stagePaths, unstagePaths }))

const files: GitFile[] = [
  { path: 'a.ts', status: 'modified', staged: false },
  { path: 'b.ts', status: 'modified', staged: false },
]

const renderPopover = (onCommitted = vi.fn()) =>
  render(
    <CommitPopover
      wsId="w1"
      files={files}
      onCommitted={onCommitted}
      trigger={<Button>Commit changes</Button>}
    />,
  )

beforeEach(() => {
  vi.clearAllMocks()
})

describe('CommitPopover', () => {
  it('opens from the trigger with the message box + a checkbox per file', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Commit changes' }))
    expect(await screen.findByPlaceholderText('Commit message…')).toBeDefined()
    expect(screen.getByText('a.ts')).toBeDefined()
    expect(screen.getByText('b.ts')).toBeDefined()
  })

  it('Commit is disabled until there is a message and at least one file', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Commit changes' }))
    const commit = await screen.findByRole('button', { name: 'Commit' })
    expect(commit).toBeDisabled()
    await user.type(screen.getByPlaceholderText('Commit message…'), 'msg')
    expect(commit).not.toBeDisabled()
  })

  it('stages the checked files, unstages the unchecked, then commits', async () => {
    const user = userEvent.setup()
    const onCommitted = vi.fn()
    renderPopover(onCommitted)
    await user.click(screen.getByRole('button', { name: 'Commit changes' }))
    // Uncheck b.ts so it gets unstaged.
    await user.click(await screen.findByText('b.ts'))
    await user.type(screen.getByPlaceholderText('Commit message…'), 'my commit')
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(stagePaths).toHaveBeenCalledWith('w1', ['a.ts'])
    expect(unstagePaths).toHaveBeenCalledWith('w1', ['b.ts'])
    expect(commitChanges).toHaveBeenCalledWith('w1', 'my commit')
    expect(onCommitted).toHaveBeenCalled()
  })

  it('does not commit when staging fails', async () => {
    stagePaths.mockResolvedValueOnce(false)
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Commit changes' }))
    await user.type(screen.getByPlaceholderText('Commit message…'), 'my commit')
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(commitChanges).not.toHaveBeenCalled()
    expect(await screen.findByText('Failed to stage changes')).toBeDefined()
  })
})
