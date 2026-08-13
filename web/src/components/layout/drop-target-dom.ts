import { SIDEBAR_DROP_POLICY, type DragSubject, type DropKind, type DropMode } from './drop-rules'
import {
  createDropHitTest,
  createDropRowDom,
  type DropRowSpec,
  type RowHit,
} from '@/components/tree-dnd/drop-dom'
import {
  dragSubjectsFor as coreDragSubjectsFor,
  sameDrop as coreSameDrop,
} from '@/components/tree-dnd/drop-core'
import type { DropLineRect } from './drop-indicator'

/**
 * The sidebar's row table, and the hit test bound to it.
 *
 * The mechanics — publish these attributes, read them back, resolve a pointer
 * against the matrix — are `tree-dnd/drop-dom.ts` and are the same for any tree.
 * What is here is only the sidebar's half: which attribute names its rows carry,
 * and that a drop on the editor pane means removal.
 *
 * The attribute names are the tree's identity. A second tree publishes its own
 * and is therefore invisible to this hit test and this one invisible to it, so
 * two trees can be on screen at once without either having to know the other
 * exists.
 */

/** The attribute that marks a row droppable, one per movable class. */
export const DROP_KIND_ATTR: Record<DropKind, string> = {
  workspace: 'data-ws-drop',
  folder: 'data-folder-drop',
  repo: 'data-repo-drop',
  project: 'data-project-drop',
}

export const SIDEBAR_ROW_SPEC: DropRowSpec = {
  kinds: DROP_KIND_ATTR,
  parentAttr: 'data-drop-parent',
  strings: { repoId: 'data-drop-repo', label: 'data-row-label' },
  flags: {
    locked: 'data-drop-locked',
    expanded: 'data-drop-expanded',
    hasChildren: 'data-drop-children',
  },
}

/**
 * The editor pane, published as a drop target.
 *
 * Removal is the only thing a drop there means, so the pane carries no mode and
 * no rect — it is one flag, read by the same hit test every row goes through.
 */
export const PANE_DROP_ATTR = 'data-pane-drop'

/** What the multiselection declares itself as, and what a drag reads back. */
const SELECTED_SELECTOR = '[aria-selected="true"]'

/**
 * A row, as both a drag subject and a drop target — the same handful of facts
 * answer both questions, so they are published once.
 */
export interface DropRow {
  kind: DropKind
  id: string
  /** Repo scope, for the same-repo rule. Absent on projects. */
  repoId?: string
  /** The container this row sits in: '' is the repo root / the sidebar itself. */
  parentId?: string
  /** A protected branch: reorders among its own siblings and nothing else. */
  locked?: boolean
  expanded?: boolean
  hasChildren?: boolean
  /**
   * What the row reads as — the branch, folder, repo or project name.
   *
   * Published beside the drop facts rather than scraped from `textContent`,
   * which on a row also carries its change counts and its buttons' labels.
   * Type-to-jump and the removal tray both name rows from this, so they cannot
   * disagree about what a row is called.
   */
  label?: string
}

/** A row plus the decision the pointer's position in it resolves to. */
export interface ResolvedDrop extends DropRow {
  mode: DropMode
}

/**
 * Where a pointer is: over the removal zone, or over a row that accepts it.
 *
 * A row hit carries the rect the decision was made from. The hit test has
 * already measured it, so the indicator positions itself off this rather than
 * measuring the row a second time — and it is the only reason a drag needs no
 * DOM read of its own per pointermove.
 */
export type DropHit = { kind: 'pane' } | { kind: 'row'; row: ResolvedDrop; rect: DropLineRect }

const rows = createDropRowDom<DropRow>(SIDEBAR_ROW_SPEC)

const hitTest = createDropHitTest<DragSubject, DropRow, { kind: 'pane' }>(
  rows,
  SIDEBAR_DROP_POLICY,
  {
    attr: PANE_DROP_ATTR,
    hit: (subjects) => (isRemovableByDrag(subjects) ? { kind: 'pane' } : null),
  },
)

/**
 * The attributes a droppable row spreads onto its own element.
 *
 * Booleans are published as presence, not as "true"/"false", so a row that is
 * neither locked nor expanded carries no attribute at all.
 */
export function dropRowProps(row: DropRow): Record<string, string | undefined> {
  return rows.props(row)
}

/** Read back what {@link dropRowProps} wrote, or null if this is not a row. */
export function readDropRow(el: Element): DropRow | null {
  return rows.read(el)
}

/** Every sidebar row matching `selector`, top to bottom as they are drawn. */
export function readDropRows(selector: string, root: ParentNode = document): DropRow[] {
  return rows.readAll(selector, root)
}

/** The live row element for a subject, for cloning it into the drag ghost. */
export function rowElementFor(
  subject: DragSubject,
  root: ParentNode = document,
): HTMLElement | null {
  return rows.elementFor(subject, root)
}

/**
 * What a pointer at (x, y) would drop onto, with the before/after/into decision
 * already made.
 */
export function findDrop(x: number, y: number, subjects: readonly DragSubject[]): DropHit | null {
  return hitTest(x, y, subjects)
}

/**
 * Whether these rows may leave through the editor pane.
 *
 * A project MAY, now that it is removable at all: it goes to the tray like a
 * repo, with no clock and a confirmation naming everything inside it. It was
 * excluded here when the sidebar had no way to delete one by any gesture, and
 * leaving it excluded afterwards was worse than either answer — the pane's veil
 * is drawn from what a removal would PLAN, so it began offering to remove a
 * project and then refusing on release.
 *
 * A LOCKED row may not: its worktree is pinned to the branch and the daemon
 * refuses the delete, so offering the veil would be promising something that
 * cannot happen. Unlock it first — that is a gesture now too.
 */
export function isRemovableByDrag(subjects: readonly DragSubject[]): boolean {
  return subjects.length > 0 && subjects.every((s) => !s.locked)
}

/**
 * The multiselected rows, read off the tree.
 *
 * `aria-selected` is where a tree already has to declare this, so the drag
 * reads the same thing assistive tech does instead of shadowing it in a second
 * structure that can disagree.
 */
export function readSelectedSubjects(root: ParentNode = document): DragSubject[] {
  return rows.readAll(SELECTED_SELECTOR, root)
}

/**
 * The rows a drag carries.
 *
 * Grabbing a row that is part of the selection moves the whole selection;
 * grabbing one outside it moves that row alone and leaves the selection be —
 * pressing on an unselected row is not a way to extend a selection.
 *
 * Pinned to the sidebar's own types rather than re-exported generic: the two
 * callers hand it a `DropRow` read off the DOM and a `DragSubject[]` read off
 * the selection, and inference across those two would widen the result to a
 * union nothing downstream accepts.
 */
export function dragSubjectsFor(
  grabbed: DragSubject,
  selection: readonly DragSubject[],
): DragSubject[] {
  return coreDragSubjectsFor(grabbed, selection)
}

/** Whether two resolved drops would draw and commit the same thing. */
export function sameDrop(a: ResolvedDrop | null, b: ResolvedDrop | null): boolean {
  return coreSameDrop(a, b)
}

export type { RowHit }
