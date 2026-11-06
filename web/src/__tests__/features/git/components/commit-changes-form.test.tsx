import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CommitChangesForm } from '@/features/git/components/commit-changes-form'
import { useGitRefreshStore } from '@/features/git/stores/git-refresh'
import { ApiError } from '@/lib/errors/api-error'
import { setWorkspaceScope } from '@/lib/workspace-scope'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api/fetch', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  apiFetch,
}))

const WS = 'ws-commit'
const calls = () =>
  apiFetch.mock.calls.map(([url, init]: [string, RequestInit]) => [
    url.split('/git/')[1],
    JSON.parse(String(init.body)),
  ])

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: WS, owningChatId: 'chat-1' })
  apiFetch.mockResolvedValue({})
})

describe('CommitChangesForm', () => {
  it('stages every change and commits it with the subject and body, then refreshes git', async () => {
    const user = userEvent.setup()
    const before = useGitRefreshStore.getState().requested[WS] ?? 0
    render(<CommitChangesForm wsId={WS} changes={2} />)

    await user.type(screen.getByLabelText('Commit message'), 'Add div{Enter}{Enter}Raises on zero')
    await user.click(screen.getByRole('button', { name: 'Commit 2 changes' }))

    await waitFor(() =>
      expect(calls()).toEqual([
        ['stage', { paths: ['.'] }],
        ['commit', { subject: 'Add div', body: 'Raises on zero' }],
      ]),
    )
    expect(useGitRefreshStore.getState().requested[WS]).toBe(before + 1)
    expect(screen.getByLabelText('Commit message')).toHaveValue('')
  })

  it('offers no commit without a message', () => {
    render(<CommitChangesForm wsId={WS} changes={1} />)
    expect(screen.getByRole('button', { name: 'Commit 1 change' })).toBeDisabled()
  })

  it('says why a commit failed and keeps the message', async () => {
    const user = userEvent.setup()
    apiFetch.mockResolvedValueOnce({}).mockRejectedValueOnce(new ApiError('nothing to commit', 409))
    render(<CommitChangesForm wsId={WS} changes={1} />)

    await user.type(screen.getByLabelText('Commit message'), 'Fix')
    await user.click(screen.getByRole('button', { name: 'Commit 1 change' }))

    expect(await screen.findByText('nothing to commit')).toBeInTheDocument()
    expect(screen.getByLabelText('Commit message')).toHaveValue('Fix')
  })
})
