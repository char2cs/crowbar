import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { RemovalConfirmDialog } from '@/components/layout/removal-confirm-dialog'
import type { RemovalEntry } from '@/lib/store/sidebar-removal'

/**
 * The confirm dialog is the only place a user can consent to a delete that
 * destroys work existing nowhere else — so it must say exactly what that work
 * is, per branch, before offering "Delete anyway".
 */

function entry(over: Partial<RemovalEntry> = {}): RemovalEntry {
  return {
    entryId: 'e1',
    kind: 'repo',
    id: 'r1',
    label: 'checkout',
    projectId: 'p1',
    repoId: 'r1',
    wsId: '',
    providerIcon: '',
    hiddenIds: ['r1'],
    extra: 0,
    fallbackWsId: null,
    deadlineAt: null,
    ...over,
  }
}

describe('RemovalConfirmDialog', () => {
  it('lists every branch and exactly what it would lose, and asks to delete anyway', () => {
    const onConfirm = vi.fn()
    const asked = entry({
      atRisk: [
        {
          workspaceId: 'w1',
          branch: 'feature/pricing-rounding',
          uncommittedFiles: 1,
          unmergedCommits: 3,
        },
        { workspaceId: 'w2', branch: 'feature/notes', uncommittedFiles: 4, unmergedCommits: 0 },
      ],
    })
    render(<RemovalConfirmDialog entry={asked} onCancel={vi.fn()} onConfirm={onConfirm} />)

    expect(screen.getByText('Delete repository “checkout” and lose work?')).toBeTruthy()
    const items = within(
      screen.getByRole('list', { name: 'Work that would be lost' }),
    ).getAllByRole('listitem')
    expect(items.map((li) => li.textContent)).toEqual([
      'feature/pricing-rounding1 uncommitted file · 3 unmerged commits',
      'feature/notes4 uncommitted files',
    ])

    fireEvent.click(screen.getByRole('button', { name: 'Delete anyway' }))
    expect(onConfirm).toHaveBeenCalledExactlyOnceWith(asked)
  })

  it('names the kind of row a workspace refusal is about', () => {
    render(
      <RemovalConfirmDialog
        entry={entry({
          kind: 'workspace',
          label: 'feature/one',
          atRisk: [
            { workspaceId: 'w1', branch: 'feature/one', uncommittedFiles: 0, unmergedCommits: 1 },
          ],
        })}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    )

    expect(screen.getByText('Delete workspace “feature/one” and lose work?')).toBeTruthy()
    expect(screen.getByText('1 unmerged commit')).toBeTruthy()
  })

  it('without work at risk, asks the ordinary cascade question', () => {
    render(<RemovalConfirmDialog entry={entry()} onCancel={vi.fn()} onConfirm={vi.fn()} />)

    expect(screen.getByText('Delete repository “checkout”?')).toBeTruthy()
    expect(screen.queryByRole('list')).toBeNull()
    expect(screen.getByRole('button', { name: 'Delete repository' })).toBeTruthy()
  })
})
