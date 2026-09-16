/**
 * Task 6: per-branch lock choice at import time. `RepoImportDialog` already
 * lists remote branches with a checkbox per row (multi-select — Import posts
 * the whole batch in one call, see the component's own doc comment); this
 * covers the lock toggle added alongside it and what `onImport` receives.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { RepoImportDialog } from '@/components/layout/repo-import-dialog'
import * as api from '@/lib/api'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof api>()),
  apiFetch: vi.fn(),
  getRepoPullRequests: vi.fn(),
}))

// The branch list is virtualised; jsdom has no layout engine, so a zero-sized
// scroll element renders nothing at all. Same fix changed-files-tree.test.tsx
// uses for the same virtualizer.
const VIEWPORT_WIDTH = 320
const VIEWPORT_HEIGHT = 600
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
  vi.clearAllMocks()
  const rect = {
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: VIEWPORT_HEIGHT,
    width: VIEWPORT_WIDTH,
    height: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
  }
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
  vi.mocked(api.getRepoPullRequests).mockResolvedValue([])
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
})

function mockBranches(
  branches: Array<{ name: string; isProtected?: boolean; hasWorkspace?: boolean }>,
) {
  vi.mocked(api.apiFetch).mockResolvedValue(
    branches.map((b) => ({ isProtected: false, hasWorkspace: false, ...b })),
  )
}

function renderDialog(onImport = vi.fn()) {
  const onOpenChange = vi.fn()
  render(
    <RepoImportDialog
      projectId="proj-1"
      repoId="repo-1"
      defaultBranch="main"
      open={true}
      onOpenChange={onOpenChange}
      onImport={onImport}
    />,
  )
  return { onImport, onOpenChange }
}

async function waitForRow(name: string) {
  return waitFor(() => screen.getByText(name))
}

describe('RepoImportDialog lock choice', () => {
  it('shows no lock control on an unselected branch row', async () => {
    mockBranches([{ name: 'feature-a' }])
    renderDialog()
    await waitForRow('feature-a')
    expect(screen.queryByRole('button', { name: /lock feature-a/i })).not.toBeInTheDocument()
  })

  it('shows a lock toggle once the branch is checked for import', async () => {
    const user = userEvent.setup()
    mockBranches([{ name: 'feature-a' }])
    renderDialog()
    await waitForRow('feature-a')
    await user.click(screen.getByRole('checkbox', { name: /^feature-a/ }))
    expect(screen.getByRole('button', { name: /lock feature-a after import/i })).toBeInTheDocument()
  })

  it('importing with the lock choice left at its default (untouched) sends no locked branches — identical to import before this task', async () => {
    const user = userEvent.setup()
    mockBranches([{ name: 'feature-a' }])
    const { onImport, onOpenChange } = renderDialog()
    await waitForRow('feature-a')
    await user.click(screen.getByRole('checkbox', { name: /^feature-a/ }))
    await user.click(screen.getByRole('button', { name: 'Import' }))
    expect(onImport).toHaveBeenCalledWith(['feature-a'], [])
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('toggling a branch’s lock control and importing sends it as locked', async () => {
    const user = userEvent.setup()
    mockBranches([{ name: 'feature-a' }])
    const { onImport } = renderDialog()
    await waitForRow('feature-a')
    await user.click(screen.getByRole('checkbox', { name: /^feature-a/ }))
    await user.click(screen.getByRole('button', { name: /lock feature-a after import/i }))
    await user.click(screen.getByRole('button', { name: 'Import' }))
    expect(onImport).toHaveBeenCalledWith(['feature-a'], ['feature-a'])
  })

  it('each branch carries its own independent lock choice across a multi-branch import', async () => {
    const user = userEvent.setup()
    mockBranches([{ name: 'feature-a' }, { name: 'feature-b' }])
    const { onImport } = renderDialog()
    await waitForRow('feature-a')
    await waitForRow('feature-b')
    await user.click(screen.getByRole('checkbox', { name: /^feature-a/ }))
    await user.click(screen.getByRole('checkbox', { name: /^feature-b/ }))
    // Lock only feature-a.
    await user.click(screen.getByRole('button', { name: /lock feature-a after import/i }))
    await user.click(screen.getByRole('button', { name: 'Import' }))
    const [branches, locked] = onImport.mock.calls[0] as [string[], string[]]
    expect(new Set(branches)).toEqual(new Set(['feature-a', 'feature-b']))
    expect(locked).toEqual(['feature-a'])
  })

  it('unchecking a branch drops it from onImport even if it had been marked locked', async () => {
    const user = userEvent.setup()
    mockBranches([{ name: 'feature-a' }])
    const { onImport } = renderDialog()
    await waitForRow('feature-a')
    const checkbox = screen.getByRole('checkbox', { name: /^feature-a/ })
    await user.click(checkbox)
    await user.click(screen.getByRole('button', { name: /lock feature-a after import/i }))
    await user.click(checkbox) // deselect
    // Import is disabled with nothing selected, so re-select without locking
    // to prove the lock choice did not survive the deselect.
    await user.click(checkbox)
    await user.click(screen.getByRole('button', { name: 'Import' }))
    expect(onImport).toHaveBeenCalledWith(['feature-a'], [])
  })

  it('the lock toggle reflects its pressed state via aria-pressed', async () => {
    const user = userEvent.setup()
    mockBranches([{ name: 'feature-a' }])
    renderDialog()
    await waitForRow('feature-a')
    await user.click(screen.getByRole('checkbox', { name: /^feature-a/ }))
    const lockButton = screen.getByRole('button', { name: /lock feature-a after import/i })
    expect(lockButton).toHaveAttribute('aria-pressed', 'false')
    await user.click(lockButton)
    expect(lockButton).toHaveAttribute('aria-pressed', 'true')
  })

  it('a protected branch row never gets a lock toggle (it is not importable at all)', async () => {
    mockBranches([{ name: 'main', isProtected: true }])
    renderDialog()
    await waitForRow('main')
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /lock main/i })).not.toBeInTheDocument()
  })
})
