import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test, beforeEach } from 'vitest'

// §3: every repo-scoped route is hierarchical under the owning project. The
// panel threads projectId + repoId into repoBase = `/v0/projects/:p/repos/:r`
// and drives branches (GET), branch-import (POST .../workspaces 202+WS, NO
// refetch/merge of the workspace list — only a branch-list refresh), and the
// icon routes (PUT/DELETE/emoji/github under .../icon).
//
// We mock apiFetch (branches GET + icon routes) and postWorkspace (the 202
// branch-import) so the test can assert the exact hierarchical URLs and the
// drop of the workspace-list merge.
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiFetch: vi.fn(),
    postWorkspace: vi.fn(() => Promise.resolve()),
  }
})

vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

import * as api from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { RepoSettingsPanel } from '@/components/layout/repo-settings-panel'
import { useSidebarStore } from '@/lib/store/sidebar'

const PROJECT = 'proj-1'
const REPO = 'repo-9'
const BASE = `/v0/projects/${PROJECT}/repos/${REPO}`

interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

const BRANCHES: BranchEntry[] = [
  { name: 'main', isProtected: true, hasWorkspace: false },
  { name: 'feature/a', isProtected: false, hasWorkspace: false },
  { name: 'feature/b', isProtected: false, hasWorkspace: false },
  { name: 'already-imported', isProtected: false, hasWorkspace: true },
]

function mockBranchesFetch(branches: BranchEntry[] = BRANCHES): void {
  vi.mocked(api.apiFetch).mockImplementation((path: string) => {
    if (path === `${BASE}/branches`) return Promise.resolve(branches as never)
    return Promise.resolve(undefined as never)
  })
}

function renderPanel() {
  return render(<RepoSettingsPanel projectId={PROJECT} repoId={REPO} repoName="my-repo" />)
}

// A selectable branch is a <label> wrapping a checkbox + the branch-name span.
// base-ui's checkbox sets aria-labelledby to its own hidden input wrapper (not
// the span), so the row has no resolvable accessible name — we toggle it by
// clicking the row label found via its visible branch text.
async function findBranchRow(name: string): Promise<HTMLElement> {
  const span = await screen.findByText(name)
  const label = span.closest('label')
  if (!label) throw new Error(`no selectable row for branch ${name}`)
  return label
}

beforeEach(() => {
  vi.mocked(api.apiFetch).mockReset()
  vi.mocked(api.postWorkspace).mockReset()
  vi.mocked(api.postWorkspace).mockResolvedValue(undefined)
  vi.mocked(toast.error).mockReset()
  useSidebarStore.setState({
    repos: [
      {
        id: REPO,
        projectId: PROJECT,
        name: 'my-repo',
        avatarLabel: 'M',
        avatarColor: 'bg-indigo-700',
        avatarURL: undefined,
        workspaces: [],
      },
    ],
  })
})

test('loads branches from the hierarchical .../branches route on mount', async () => {
  mockBranchesFetch()
  renderPanel()
  await waitFor(() => expect(vi.mocked(api.apiFetch)).toHaveBeenCalledWith(`${BASE}/branches`))
  await screen.findByText('feature/a')
  expect(screen.getByText('main')).toBeInTheDocument()
})

test('branch import POSTs each selected branch via postWorkspace(projectId, repoId, branch) — no list refetch/merge', async () => {
  mockBranchesFetch()
  renderPanel()
  fireEvent.click(await findBranchRow('feature/a'))
  fireEvent.click(await findBranchRow('feature/b'))

  fireEvent.click(screen.getByRole('button', { name: /import 2 branches/i }))

  await waitFor(() => {
    expect(vi.mocked(api.postWorkspace)).toHaveBeenCalledWith(PROJECT, REPO, 'feature/a')
    expect(vi.mocked(api.postWorkspace)).toHaveBeenCalledWith(PROJECT, REPO, 'feature/b')
  })
  // The 202+WS path means the workspace list is NOT fetched here; only the
  // branch list is refreshed (GET .../branches), so apiFetch only ever hits
  // the branches route.
  const nonBranch = vi.mocked(api.apiFetch).mock.calls.filter(([p]) => p !== `${BASE}/branches`)
  expect(nonBranch).toHaveLength(0)
})

test('surfaces a toast when a branch import rejects', async () => {
  mockBranchesFetch()
  vi.mocked(api.postWorkspace).mockRejectedValueOnce(new Error('locked'))
  renderPanel()
  fireEvent.click(await findBranchRow('feature/a'))
  fireEvent.click(screen.getByRole('button', { name: /import 1 branch/i }))

  await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalled())
  expect(vi.mocked(toast.error).mock.calls[0][0]).toMatch(/failed to import 1 branch/i)
})

test('protected branches are shown locked and not selectable for import', async () => {
  mockBranchesFetch()
  renderPanel()
  await screen.findByText('main')
  // The protected branch has no checkbox (it is rendered as a locked row).
  expect(screen.queryByLabelText('main')).not.toBeInTheDocument()
  // An already-imported branch is also non-selectable.
  expect(screen.queryByLabelText('already-imported')).not.toBeInTheDocument()
})

test('icon upload PUTs FormData to the hierarchical .../icon route', async () => {
  mockBranchesFetch()
  const { container } = renderPanel()
  await screen.findByText('feature/a')

  const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File(['x'], 'icon.png', { type: 'image/png' })
  fireEvent.change(fileInput, { target: { files: [file] } })

  await waitFor(() => {
    const call = vi.mocked(api.apiFetch).mock.calls.find(([p]) => p === `${BASE}/icon`)
    expect(call).toBeTruthy()
    expect(call![1]?.method).toBe('PUT')
    expect(call![1]?.body).toBeInstanceOf(FormData)
  })
})

test('emoji submit PUTs JSON to the hierarchical .../icon/emoji route', async () => {
  mockBranchesFetch()
  renderPanel()
  await screen.findByText('feature/a')

  fireEvent.click(screen.getByRole('button', { name: /emoji/i }))
  const emojiInput = screen.getByPlaceholderText('Type an emoji…')
  fireEvent.change(emojiInput, { target: { value: '🚀' } })
  fireEvent.click(screen.getByRole('button', { name: /^set$/i }))

  await waitFor(() => {
    const call = vi.mocked(api.apiFetch).mock.calls.find(([p]) => p === `${BASE}/icon/emoji`)
    expect(call).toBeTruthy()
    expect(call![1]?.method).toBe('PUT')
    expect(JSON.parse(call![1]?.body as string)).toEqual({ emoji: '🚀' })
  })
})

test('github avatar PUTs to the hierarchical .../icon/github route', async () => {
  mockBranchesFetch()
  renderPanel()
  await screen.findByText('feature/a')

  fireEvent.click(screen.getByRole('button', { name: /github/i }))

  await waitFor(() => {
    const call = vi.mocked(api.apiFetch).mock.calls.find(([p]) => p === `${BASE}/icon/github`)
    expect(call).toBeTruthy()
    expect(call![1]?.method).toBe('PUT')
  })
})

test('reset icon DELETEs the hierarchical .../icon route when an avatar exists', async () => {
  mockBranchesFetch()
  useSidebarStore.setState({
    repos: [
      {
        id: REPO,
        projectId: PROJECT,
        name: 'my-repo',
        avatarLabel: 'M',
        avatarColor: 'bg-indigo-700',
        avatarURL: 'https://example.com/a.png',
        workspaces: [],
      },
    ],
  })
  renderPanel()
  await screen.findByText('feature/a')

  fireEvent.click(screen.getByRole('button', { name: /reset to default/i }))

  await waitFor(() => {
    const call = vi.mocked(api.apiFetch).mock.calls.find(([p]) => p === `${BASE}/icon`)
    expect(call).toBeTruthy()
    expect(call![1]?.method).toBe('DELETE')
  })
})

test('re-fetches branches from the new repoBase when projectId/repoId change', async () => {
  mockBranchesFetch()
  const { rerender } = renderPanel()
  await waitFor(() => expect(vi.mocked(api.apiFetch)).toHaveBeenCalledWith(`${BASE}/branches`))

  vi.mocked(api.apiFetch).mockImplementation((path: string) => {
    if (path === '/v0/projects/proj-2/repos/repo-2/branches') {
      return Promise.resolve(BRANCHES as never)
    }
    return Promise.resolve(undefined as never)
  })
  rerender(<RepoSettingsPanel projectId="proj-2" repoId="repo-2" repoName="other" />)

  await waitFor(() =>
    expect(vi.mocked(api.apiFetch)).toHaveBeenCalledWith(
      '/v0/projects/proj-2/repos/repo-2/branches',
    ),
  )
})
