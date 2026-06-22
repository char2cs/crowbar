import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { CommitDialog } from '@/features/git/components/commit-dialog'
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

beforeEach(() => vi.clearAllMocks())

describe('CommitDialog', () => {
  it('renders the message box and a checkbox per file (all checked by default)', () => {
    render(<CommitDialog open onOpenChange={vi.fn()} wsId="w1" files={files} onCommitted={vi.fn()} />)
    expect(screen.getByPlaceholderText('Commit message…')).toBeDefined()
    expect(screen.getByText('a.ts')).toBeDefined()
    expect(screen.getByText('b.ts')).toBeDefined()
  })

  it('Commit is disabled until there is a message and at least one file', async () => {
    const user = userEvent.setup()
    render(<CommitDialog open onOpenChange={vi.fn()} wsId="w1" files={files} onCommitted={vi.fn()} />)
    const commit = screen.getByRole('button', { name: 'Commit' })
    expect(commit).toBeDisabled()
    await user.type(screen.getByPlaceholderText('Commit message…'), 'msg')
    expect(commit).not.toBeDisabled()
  })

  it('committing stages the checked files, unstages the unchecked, commits, then closes', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    const onCommitted = vi.fn()
    render(
      <CommitDialog open onOpenChange={onOpenChange} wsId="w1" files={files} onCommitted={onCommitted} />,
    )
    // Uncheck b.ts (clicking the label toggles its checkbox).
    await user.click(screen.getByText('b.ts'))
    await user.type(screen.getByPlaceholderText('Commit message…'), 'my commit')
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(stagePaths).toHaveBeenCalledWith('w1', ['a.ts'])
    expect(unstagePaths).toHaveBeenCalledWith('w1', ['b.ts'])
    expect(commitChanges).toHaveBeenCalledWith('w1', 'my commit')
    expect(onCommitted).toHaveBeenCalled()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
