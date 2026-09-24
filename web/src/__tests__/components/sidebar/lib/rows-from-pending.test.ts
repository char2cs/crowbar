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

  /**
   * A branch import's pending entry carries `repoId` (row-actions.ts's
   * `startImportPendingRows`) because its 202 hands back no id for
   * `attachRealId` to narrow onto — it can sit in `rowIdsAtClick` for the
   * whole provisioning window. Unscoped, that would blank out every OTHER
   * repo's (and project home's) freshly-created rows for as long as the
   * import runs — worse than the ghost row this mechanism exists to hide.
   * `rowRepoId` is what keeps the suppression inside the ONE repo it's
   * actually about.
   */
  describe('a repo-scoped entry (a branch import)', () => {
    const scopedEntry = entry({
      repoId: 'repo-1',
      rowIdsAtClick: ['c-a', 'existing'],
    })

    it('still hides an unknown row that belongs to its OWN repo', () => {
      const rows = [row('c-a'), row('existing', 'c-a'), row('ghost', 'c-a')]
      const rowRepoId = new Map([
        ['c-a', 'repo-1'],
        ['existing', 'repo-1'],
        ['ghost', 'repo-1'],
      ])
      expect(ids(hideRowsForInFlightCreates(rows, [scopedEntry], 'p1', rowRepoId))).toEqual([
        'c-a',
        'existing',
      ])
    })

    it('never hides an unknown row that belongs to a DIFFERENT repo', () => {
      const rows = [row('c-a'), row('other-repo-chat')]
      const rowRepoId = new Map([
        ['c-a', 'repo-1'],
        ['other-repo-chat', 'repo-2'],
      ])
      expect(ids(hideRowsForInFlightCreates(rows, [scopedEntry], 'p1', rowRepoId))).toEqual([
        'c-a',
        'other-repo-chat',
      ])
    })

    it('never hides an unknown row that belongs to no repo at all (project home)', () => {
      const rows = [row('c-a'), row('home-chat')]
      // `rowRepoId` has no entry for a project-home row — a repo never
      // claims it (`rowRepoScope`'s own doc, rows-from-repo.ts).
      const rowRepoId = new Map([['c-a', 'repo-1']])
      expect(ids(hideRowsForInFlightCreates(rows, [scopedEntry], 'p1', rowRepoId))).toEqual([
        'c-a',
        'home-chat',
      ])
    })

    it('suppresses nothing when no scope map is given at all — fails open, never project-wide', () => {
      const rows = [row('c-a'), row('ghost', 'c-a')]
      expect(ids(hideRowsForInFlightCreates(rows, [scopedEntry], 'p1'))).toEqual(ids(rows))
    })

    it('a repo-scoped entry and an unscoped entry combine correctly', () => {
      // `unscoped` (no `repoId` — an ordinary fork/thread create) still hides
      // project-wide, same as before this fix, so it applies to 'other-repo'
      // even though that row is outside `scopedEntry`'s own repo; `scopedEntry`
      // only ever narrows ITS OWN repo's rows further (it ignores 'other-repo'
      // entirely, and still hides 'ghost' inside repo-1 even though `unscoped`
      // already knew it at click time).
      const unscoped = entry({
        tempId: 'pending-2',
        rowIdsAtClick: ['c-a', 'existing', 'other-repo', 'ghost'],
      })
      const rows = [row('c-a'), row('existing', 'c-a'), row('other-repo'), row('ghost', 'c-a')]
      const rowRepoId = new Map([
        ['c-a', 'repo-1'],
        ['existing', 'repo-1'],
        ['other-repo', 'repo-2'],
        ['ghost', 'repo-1'],
      ])
      expect(
        ids(hideRowsForInFlightCreates(rows, [scopedEntry, unscoped], 'p1', rowRepoId)),
      ).toEqual(['c-a', 'existing', 'other-repo'])
    })
  })
})
