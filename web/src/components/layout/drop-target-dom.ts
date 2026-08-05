import {
  allowedModes,
  resolveDropMode,
  type DragSubject,
  type DropKind,
  type DropMode,
  type DropTarget,
} from './drop-rules'
import type { DropLineRect } from './drop-indicator'

/**
 * The DOM half of drag and drop: what a row publishes about itself, and what a
 * pointer position resolves to.
 *
 * Every fact the drop matrix needs rides on the row element itself rather than
 * being looked up in a store at pointer rate. A hit test is then one
 * `elementsFromPoint`, one `getAttribute` sweep and one rect — no tree walk, no
 * store read, nothing that scales with how many workspaces you have.
 */

/** The attribute that marks a row droppable, one per movable class. */
export const DROP_KIND_ATTR: Record<DropKind, string> = {
  workspace: 'data-ws-drop',
  folder: 'data-folder-drop',
  repo: 'data-repo-drop',
  project: 'data-project-drop',
}

const DROP_KINDS = Object.keys(DROP_KIND_ATTR) as DropKind[]

const REPO_ATTR = 'data-drop-repo'
const PARENT_ATTR = 'data-drop-parent'
const EXPANDED_ATTR = 'data-drop-expanded'
const CHILDREN_ATTR = 'data-drop-children'
const LOCKED_ATTR = 'data-drop-locked'
const LABEL_ATTR = 'data-row-label'

/**
 * The editor pane, published as a drop target.
 *
 * Removal is the only thing a drop there means, so the pane carries no mode and
 * no rect — it is one flag, read by the same hit test every row goes through.
 */
export const PANE_DROP_ATTR = 'data-pane-drop'

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

/**
 * The attributes a droppable row spreads onto its own element.
 *
 * Booleans are published as presence, not as "true"/"false", so a row that is
 * neither locked nor expanded carries no attribute at all.
 */
export function dropRowProps(row: DropRow): Record<string, string | undefined> {
  return {
    [DROP_KIND_ATTR[row.kind]]: row.id,
    [REPO_ATTR]: row.repoId,
    [PARENT_ATTR]: row.parentId ?? '',
    [LABEL_ATTR]: row.label,
    [EXPANDED_ATTR]: row.expanded ? '' : undefined,
    [CHILDREN_ATTR]: row.hasChildren ? '' : undefined,
    [LOCKED_ATTR]: row.locked ? '' : undefined,
  }
}

/** Read back what {@link dropRowProps} wrote, or null if this is not a row. */
export function readDropRow(el: Element): DropRow | null {
  for (const kind of DROP_KINDS) {
    const id = el.getAttribute(DROP_KIND_ATTR[kind])
    if (id === null) continue
    return {
      kind,
      id,
      repoId: el.getAttribute(REPO_ATTR) ?? undefined,
      parentId: el.getAttribute(PARENT_ATTR) ?? '',
      label: el.getAttribute(LABEL_ATTR) ?? undefined,
      locked: el.hasAttribute(LOCKED_ATTR),
      expanded: el.hasAttribute(EXPANDED_ATTR),
      hasChildren: el.hasAttribute(CHILDREN_ATTR),
    }
  }
  return null
}

/**
 * What a pointer at (x, y) would drop onto, with the before/after/into decision
 * already made.
 *
 * The topmost row under the pointer is the answer whether or not it accepts the
 * drag: a refusal returns null rather than falling through to whatever sits
 * behind it, because "this row says no" is the honest reading and drawing an
 * indicator on some ancestor would be the indicator lying.
 */
export function findDrop(x: number, y: number, subjects: readonly DragSubject[]): DropHit | null {
  for (const el of document.elementsFromPoint(x, y)) {
    if (el.hasAttribute(PANE_DROP_ATTR)) {
      return isRemovableByDrag(subjects) ? { kind: 'pane' } : null
    }
    const row = readDropRow(el)
    if (!row) continue
    const allowed = allowedModes(subjects as DragSubject[], row as DropTarget)
    if (!allowed.before && !allowed.after && !allowed.into) return null
    const rect = el.getBoundingClientRect()
    if (rect.height <= 0) return null
    const mode = resolveDropMode((y - rect.top) / rect.height, row as DropTarget, allowed)
    return mode === null ? null : { kind: 'row', row: { ...row, mode }, rect }
  }
  return null
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

/** Whether two resolved drops would draw and commit the same thing. */
export function sameDrop(a: ResolvedDrop | null, b: ResolvedDrop | null): boolean {
  if (a === null || b === null) return a === b
  return a.id === b.id && a.kind === b.kind && a.mode === b.mode
}

/**
 * The rows a drag carries.
 *
 * Grabbing a row that is part of the selection moves the whole selection;
 * grabbing one outside it moves that row alone and leaves the selection be —
 * pressing on an unselected row is not a way to extend a selection.
 */
export function dragSubjectsFor(
  grabbed: DragSubject,
  selection: readonly DragSubject[],
): DragSubject[] {
  return selection.some((s) => s.id === grabbed.id) ? [...selection] : [grabbed]
}

/**
 * The multiselected rows, read off the tree.
 *
 * `aria-selected` is where a tree already has to declare this, so the drag
 * reads the same thing assistive tech does instead of shadowing it in a second
 * structure that can disagree.
 */
export function readSelectedSubjects(root: ParentNode = document): DragSubject[] {
  const out: DragSubject[] = []
  for (const el of root.querySelectorAll('[aria-selected="true"]')) {
    const row = readDropRow(el)
    if (row) out.push(row)
  }
  return out
}

/** The live row element for a subject, for cloning it into the drag ghost. */
export function rowElementFor(
  subject: DragSubject,
  root: ParentNode = document,
): HTMLElement | null {
  return root.querySelector<HTMLElement>(`[${DROP_KIND_ATTR[subject.kind]}="${subject.id}"]`)
}
