import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test, beforeEach } from 'vitest'

const BASE = '/v0/projects/proj-1/repos/repo-1'

const BRANCHES = [
  { name: 'dev', isProtected: true, hasWorkspace: false },
  { name: 'feat/base', isProtected: false, hasWorkspace: false },
  { name: 'feat/9324', isProtected: false, hasWorkspace: false },
  { name: 'already', isProtected: false, hasWorkspace: true },
]
const PR_LINKS = [
  { head: 'feat/9324', base: 'feat/base' },
  { head: 'feat/base', base: 'dev' },
]

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiFetch: vi.fn((path: string) =>
      path === `${BASE}/branches`
        ? Promise.resolve(BRANCHES as never)
        : Promise.resolve(undefined as never),
    ),
    getRepoPullRequests: vi.fn(() => Promise.resolve(PR_LINKS)),
  }
})

import { RepoImportDialog } from '@/components/layout/repo-import-dialog'

// jsdom has no layout, so every element reports a 0×0 rect and @tanstack/react-virtual
// windows to zero rows. Give elements a real viewport height so the virtualizer
// mounts the (few) rows this test asserts on.
beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    top: 0,
    left: 0,
    right: 400,
    bottom: 600,
    width: 400,
    height: 600,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect)
})

function renderDialog(onOpenChange = vi.fn(), onImport = vi.fn()) {
  return render(
    <RepoImportDialog
      projectId="proj-1"
      repoId="repo-1"
      defaultBranch="dev"
      open
      onOpenChange={onOpenChange}
      onImport={onImport}
    />,
  )
}

// base-ui's checkbox carries no resolvable accessible name, so a selectable row
// is toggled by clicking its wrapping <label> (found via the branch text).
async function selectBranch(name: string) {
  const span = await screen.findByText(name)
  const label = span.closest('label')
  if (!label) throw new Error(`branch ${name} is not selectable (no label)`)
  fireEvent.click(label)
}

test('renders branch rows; protected + already-imported are not selectable', async () => {
  renderDialog()
  await waitFor(() => expect(screen.getByText('feat/9324')).toBeInTheDocument())

  // Selectable rows are wrapped in a <label>; protected/imported rows are not.
  expect(screen.getByText('feat/9324').closest('label')).not.toBeNull()
  expect(screen.getByText('feat/base').closest('label')).not.toBeNull()
  expect(screen.getByText('dev').closest('label')).toBeNull()
  expect(screen.getByText('already').closest('label')).toBeNull()
})

test('selecting a branch with a missing PR base shows the "creates N parents" hint', async () => {
  renderDialog()
  await selectBranch('feat/9324')

  await waitFor(() => expect(screen.getByText(/creates 1 parent branch/i)).toBeInTheDocument())
})

test('Import hands the selection to onImport and closes the dialog', async () => {
  const onOpenChange = vi.fn()
  const onImport = vi.fn()
  renderDialog(onOpenChange, onImport)
  await selectBranch('feat/9324')

  fireEvent.click(screen.getByRole('button', { name: 'Import' }))

  expect(onImport).toHaveBeenCalledWith(['feat/9324'])
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

// Regression for `native/oracle/blocked/repo-import-dialog-duplicate-button-id.md`:
// this dialog's own "Import" button and the Dialog primitive's built-in close
// button both used to inherit `button.tsx`'s default `data-oracle-id="button"`,
// so a capture rooted at this dialog carried two `button`-id anchors and the
// oracle differ refused it outright (it matches by id and cannot say which of
// the two it compared). The close button now names itself `dialog-close`.
//
// Mutation actually run (`data-oracle-id="dialog-close"` deleted from
// `dialog.tsx`'s `DialogPopup` close button, then reverted):
//
//   AssertionError: expected …(2) to have a length of 1 but got 2
//   - Expected: 1
//   + Received: 2
//   at expect(document.querySelectorAll('[data-oracle-id="button"]')).toHaveLength(1)
//
// i.e. exactly the "two `button` anchors" collision the blocked doc names.
test('the close button and the Import button do not collide on data-oracle-id', () => {
  renderDialog()

  expect(document.querySelectorAll('[data-oracle-id="button"]')).toHaveLength(1)
  expect(document.querySelectorAll('[data-oracle-id="dialog-close"]')).toHaveLength(1)
})
