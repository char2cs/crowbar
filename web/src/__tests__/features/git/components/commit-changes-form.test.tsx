import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CommitChangesForm } from '@/features/git/components/commit-changes-form'
import { useGitRefreshStore } from '@/features/git/stores/git-refresh'
import { ApiError } from '@/lib/errors/api-error'
import { setWorkspaceScope } from '@/lib/workspace-scope'
import type { GitFile } from '@/features/git/types/git-types'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api/fetch', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  apiFetch,
}))

const WS = 'ws-commit'
const calls = () =>
  (apiFetch.mock.calls as [string, RequestInit][]).map(([url, init]) => [
    url.split('/git/')[1],
    JSON.parse(String(init.body)),
  ])

const file = (path: string, status: GitFile['status'] = 'modified', staged = false): GitFile => ({
  path,
  status,
  staged,
})
const TWO = [file('calc.py'), file('test_calc.py', 'untracked')]

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: WS, owningChatId: 'chat-1' })
  apiFetch.mockResolvedValue({})
})

describe('CommitChangesForm', () => {
  it('stages the files it shows and commits them with the subject and body, then refreshes git', async () => {
    const user = userEvent.setup()
    const before = useGitRefreshStore.getState().requested[WS] ?? 0
    render(<CommitChangesForm wsId={WS} files={TWO} />)

    await user.type(screen.getByLabelText('Commit message'), 'Add div{Enter}{Enter}Raises on zero')
    await user.click(screen.getByRole('button', { name: 'Commit 2 changes' }))

    await waitFor(() =>
      expect(calls()).toEqual([
        ['stage', { paths: ['calc.py', 'test_calc.py'] }],
        ['commit', { subject: 'Add div', body: 'Raises on zero' }],
      ]),
    )
    expect(useGitRefreshStore.getState().requested[WS]).toBe(before + 1)
    expect(screen.getByLabelText('Commit message')).toHaveValue('')
  })

  it('offers no commit without a message', () => {
    render(<CommitChangesForm wsId={WS} files={[file('calc.py')]} />)
    expect(screen.getByRole('button', { name: 'Commit 1 change' })).toBeDisabled()
  })

  // Untracked junk (a __pycache__ the agent's run left) is shown and stays out once unticked.
  it('commits only the files left ticked', async () => {
    const user = userEvent.setup()
    render(<CommitChangesForm wsId={WS} files={[...TWO, file('__pycache__/', 'untracked')]} />)
    expect(screen.getByRole('checkbox', { name: '__pycache__/' })).toBeChecked()

    await user.click(screen.getByRole('checkbox', { name: '__pycache__/' }))
    await user.type(screen.getByLabelText('Commit message'), 'Add tests')
    await user.click(screen.getByRole('button', { name: 'Commit 2 changes' }))

    await waitFor(() =>
      expect(calls()).toEqual([
        ['stage', { paths: ['calc.py', 'test_calc.py'] }],
        ['commit', { subject: 'Add tests', body: '' }],
      ]),
    )
  })

  it('takes an unticked file that was already staged out of the commit', async () => {
    const user = userEvent.setup()
    render(
      <CommitChangesForm wsId={WS} files={[file('calc.py'), file('notes.txt', 'added', true)]} />,
    )

    await user.click(screen.getByRole('checkbox', { name: 'notes.txt' }))
    await user.type(screen.getByLabelText('Commit message'), 'Fix')
    await user.click(screen.getByRole('button', { name: 'Commit 1 change' }))

    await waitFor(() =>
      expect(calls()).toEqual([
        ['stage', { paths: ['calc.py'] }],
        ['unstage', { paths: ['notes.txt'] }],
        ['commit', { subject: 'Fix', body: '' }],
      ]),
    )
  })

  it('offers no commit with every file unticked', async () => {
    const user = userEvent.setup()
    render(<CommitChangesForm wsId={WS} files={[file('calc.py')]} />)

    await user.type(screen.getByLabelText('Commit message'), 'Fix')
    await user.click(screen.getByRole('checkbox', { name: 'calc.py' }))

    expect(screen.getByRole('button', { name: 'Commit 0 changes' })).toBeDisabled()
  })

  it('says why a commit failed and keeps the message', async () => {
    const user = userEvent.setup()
    apiFetch.mockResolvedValueOnce({}).mockRejectedValueOnce(new ApiError('nothing to commit', 409))
    render(<CommitChangesForm wsId={WS} files={[file('calc.py')]} />)

    await user.type(screen.getByLabelText('Commit message'), 'Fix')
    await user.click(screen.getByRole('button', { name: 'Commit 1 change' }))

    expect(await screen.findByText('nothing to commit')).toBeInTheDocument()
    expect(screen.getByLabelText('Commit message')).toHaveValue('Fix')
  })
})
