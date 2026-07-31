import type { DragSubject } from './drop-rules'

/**
 * What the row context menu offers, decided without rendering anything.
 *
 * The menu is where the bulk actions live. A selection action bar was the
 * alternative and it lost twice over: every action it carried already had a
 * home, and it claimed the foot of the sidebar, which is where held rows now
 * wait.
 */

export type RowMenuAction = 'group' | 'remove'

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
 * An empty list means no menu at all rather than a menu of disabled items: a
 * project row offers neither action, and opening an empty popup on it would be a
 * worse answer than not opening one.
 */
export function rowMenuFor(subjects: readonly DragSubject[]): RowMenuEntry[] {
  if (subjects.length === 0) return []
  if (subjects.some((s) => s.kind === 'project')) return []

  const entries: RowMenuEntry[] = []
  const n = subjects.length

  // Repos are their own level; there is no folder for one to go into.
  if (subjects.every((s) => s.kind === 'workspace' || s.kind === 'folder')) {
    entries.push({ id: 'group', label: n > 1 ? `Group ${n} into a folder` : 'Group into a folder' })
  }

  if (subjects.every((s) => !s.locked)) {
    entries.push({ id: 'remove', label: removeLabel(subjects) })
  }

  return entries
}

function removeLabel(subjects: readonly DragSubject[]): string {
  const n = subjects.length
  if (n === 1) {
    switch (subjects[0].kind) {
      case 'repo':
        return 'Remove repository'
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
