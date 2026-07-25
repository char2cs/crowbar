import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test } from 'vitest'

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiFetch: vi.fn(() => Promise.resolve(undefined as never)) }
})
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: () => false }))

import { RepoIconPopover } from '@/components/layout/repo-icon-popover'
import type { Repo } from '@/lib/store/sidebar'

const REPO: Repo = {
  id: 'repo-1',
  projectId: 'proj-1',
  name: 'zen',
  avatarLabel: 'ZE',
  avatarColor: 'bg-blue-500',
} as Repo

test('avatar renders as an editable trigger, popup hidden until clicked', async () => {
  render(<RepoIconPopover repo={REPO} />)

  const trigger = screen.getByRole('button', { name: /edit zen icon/i })
  expect(trigger).toBeInTheDocument()
  // Icon controls are not mounted until the popover opens.
  expect(screen.queryByRole('button', { name: 'Upload' })).not.toBeInTheDocument()

  fireEvent.click(trigger)

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /upload/i })).toBeInTheDocument()
  })
  expect(screen.getByRole('button', { name: /emoji/i })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /github/i })).toBeInTheDocument()
})

test('clicking the avatar does not bubble to the row (navigation is stopped)', () => {
  const onRowClick = vi.fn()
  render(
    <div onClick={onRowClick}>
      <RepoIconPopover repo={REPO} />
    </div>,
  )
  fireEvent.click(screen.getByRole('button', { name: /edit zen icon/i }))
  expect(onRowClick).not.toHaveBeenCalled()
})

test('a working repo shows the spinner and is not editable', () => {
  render(<RepoIconPopover repo={{ ...REPO, defaultWorking: true }} />)
  expect(screen.queryByRole('button', { name: /edit zen icon/i })).not.toBeInTheDocument()
})
