import { describe, expect, it } from 'vitest'
import {
  hideRowsForInFlightCreates,
  rowFromPending,
} from '@/components/sidebar/lib/rows-from-pending'
import type { PendingCreateEntry } from '@/lib/store/pending-creates'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const row = (id: string, parentId: string | null = null, over: Partial<SidebarRow> = {}) =>
  ({
    id,
    kind: 'chat',
    parentId,
    order: 0,
    label: id,
    ownsWorktree: false,
    workspaceId: 'ws',
    working: false,
    hasView: false,
    ...over,
  }) satisfies SidebarRow

const entry = (over: Partial<PendingCreateEntry> = {}): PendingCreateEntry => ({
  tempId: 'pending-1',
  kind: 'chat',
  projectId: 'p1',
  parentId: 'c-a',
  order: 1,
  workspaceId: 'ws',
  ownsWorktree: false,
  status: 'creating',
  label: '',
  rowIdsAtClick: ['c-a', 'existing'],
  ...over,
})

const ids = (rows: readonly SidebarRow[]) => rows.map((r) => r.id)

/**
 * The daemon's `created`/`placement_set` frames land the real row in the
 * tree while the POST that will carry its id is still open — so for the
 * whole CLI-spawn / worktree-provision round trip the panel drew the
 * spinner row AND an "Untitled chat" row, then the real one blinked out
 * when `realId` attached and back in when the entry cleared.
 */
describe('hideRowsForInFlightCreates', () => {
  it('hides a row the panel did not hold at click time while the create has no realId yet', () => {
    const pending = rowFromPending(entry())
    const rows = [row('c-a'), row('existing', 'c-a'), row('real-1', 'c-a'), pending]
    expect(ids(hideRowsForInFlightCreates(rows, [entry()], 'p1'))).toEqual([
      'c-a',
      'existing',
      'pending-1',
    ])
  })

  it('hides it wherever it reseeded first — a mint lands at root before its placement write', () => {
    const rows = [row('c-a'), row('existing', 'c-a'), row('real-1', null)]
    expect(ids(hideRowsForInFlightCreates(rows, [entry()], 'p1'))).toEqual(['c-a', 'existing'])
  })

  it('hands over to the realId filter once the POST has answered', () => {
    const rows = [row('c-a'), row('existing', 'c-a'), row('real-1', 'c-a'), row('other', 'c-a')]
    const attached = entry({ realId: 'real-1' })
    expect(ids(hideRowsForInFlightCreates(rows, [attached], 'p1'))).toEqual([
      'c-a',
      'existing',
      'other',
    ])
  })

  it('hides nothing for a naming or failed entry, nor for one without a snapshot', () => {
    const rows = [row('c-a'), row('real-1', 'c-a')]
    expect(ids(hideRowsForInFlightCreates(rows, [entry({ status: 'naming' })], 'p1'))).toEqual(
      ids(rows),
    )
    expect(ids(hideRowsForInFlightCreates(rows, [entry({ status: 'error' })], 'p1'))).toEqual(
      ids(rows),
    )
    expect(
      ids(hideRowsForInFlightCreates(rows, [entry({ rowIdsAtClick: undefined })], 'p1')),
    ).toEqual(ids(rows))
  })

  it("never lets another project's snapshot hide this panel's rows", () => {
    const rows = [row('c-a'), row('real-1', 'c-a')]
    const foreign = entry({ projectId: 'p2', rowIdsAtClick: ['unrelated'] })
    expect(ids(hideRowsForInFlightCreates(rows, [foreign], 'p1'))).toEqual(ids(rows))
  })

  it('returns the same array when there is nothing to hide', () => {
    const rows = [row('c-a')]
    expect(hideRowsForInFlightCreates(rows, [], 'p1')).toBe(rows)
  })
})
