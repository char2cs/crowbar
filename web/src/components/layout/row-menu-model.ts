import type { DragSubject } from './drop-rules'

/**
 * What the row context menu offers, decided without rendering anything.
 *
 * The menu is where the bulk actions live. A selection action bar was the
 * alternative and it lost twice over: every action it carried already had a
 * home, and it claimed the foot of the sidebar, which is where held rows now
 * wait.
 */

export type RowMenuAction = 'group' | 'lock' | 'unlock' | 'remove'

export interface RowMenuEntry {
  id: RowMenuAction
  label: string
}

/**
 * The menu for `subjects` — the multiselection when the clicked row is part of
 * it, that row alone when it is not. Callers get that split from
 * `dragSubjectsFor()`, the same function the drag uses, so a right-click and a
 * drag can never disagree about what they are acting on.
 *
 * An empty list means no menu at all rather than a menu of disabled items:
 * opening an empty popup is a worse answer than the row simply not having one.
 */
export function rowMenuFor(subjects: readonly DragSubject[]): RowMenuEntry[] {
  if (subjects.length === 0) return []
  // A project never joins a multiselection, so it is either the only subject or
  // it is mixed in with rows from inside itself — and "remove this project and
  // also this branch within it" is not a coherent action to offer.
  if (subjects.some((s) => s.kind === 'project') && subjects.length > 1) return []

  const entries: RowMenuEntry[] = []
  const n = subjects.length

  // Repos are their own level; there is no folder for one to go into.
  if (subjects.every((s) => s.kind === 'workspace' || s.kind === 'folder')) {
    entries.push({ id: 'group', label: n > 1 ? `Group ${n} into a folder` : 'Group into a folder' })
  }

  entries.push(...lockEntries(subjects))

  if (subjects.every((s) => !s.locked)) {
    entries.push({ id: 'remove', label: removeLabel(subjects) })
  }

  return entries
}

/**
 * Lock and unlock, offered for whichever of the two would actually change
 * something.
 *
 * Locking used to be the provider's alone: a protected branch was created locked
 * and re-locked on every poll, and there was no way to disagree. The user's
 * decision now outranks that (domain.Workspace.LockOverride), in BOTH directions
 * — main can be unlocked, an ordinary fork child can be locked — and it survives
 * the next poll. Automatic locking is untouched: a protected branch still starts
 * locked, this is only the ability to overrule it afterwards.
 *
 * Only workspaces. A folder holds rows and has nothing to protect; a repo's
 * header row is its default workspace, which is the one branch that must stay
 * locked — it IS the repo's own checkout, and handing it out for editing under
 * the sidebar's rules is not what the lock is for.
 *
 * On a homogeneous selection exactly one entry appears. On a mixed one both do,
 * each acting on the whole selection — which is the honest offer, since either
 * verb genuinely applies to part of it.
 */
function lockEntries(subjects: readonly DragSubject[]): RowMenuEntry[] {
  const rows = subjects.filter((s) => s.kind === 'workspace')
  if (rows.length === 0 || rows.length !== subjects.length) return []

  const out: RowMenuEntry[] = []
  const n = rows.length
  if (rows.some((s) => !s.locked)) {
    out.push({ id: 'lock', label: n > 1 ? `Lock ${n} workspaces` : 'Lock workspace' })
  }
  if (rows.some((s) => s.locked)) {
    out.push({ id: 'unlock', label: n > 1 ? `Unlock ${n} workspaces` : 'Unlock workspace' })
  }
  return out
}

function removeLabel(subjects: readonly DragSubject[]): string {
  const n = subjects.length
  if (n === 1) {
    switch (subjects[0].kind) {
      case 'repo':
        return 'Remove repository'
      case 'project':
        return 'Delete project'
      case 'folder':
        return 'Delete folder'
      default:
        return 'Remove workspace'
    }
  }
  return subjects.every((s) => s.kind === 'workspace')
    ? `Remove ${n} workspaces`
    : `Remove ${n} rows`
}
