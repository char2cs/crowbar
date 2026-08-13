import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  resolvesToFirstChild,
  type DragSubjectBase,
  type DropMode,
  type DropPolicy,
  type DropTargetBase,
} from '@/components/tree-dnd/drop-core'
import {
  createDropHitTest,
  createDropRowDom,
  type DropRowSpec,
} from '@/components/tree-dnd/drop-dom'
import { PANE_DROP_ATTR } from '@/components/layout/drop-target-dom'
import type { ChatRowKind } from './chat-rows'

/**
 * What a drag may do in the Chats tree, and where a drop lands.
 *
 * The mechanics are the shared ones — `tree-dnd/drop-core.ts` owns the band
 * arithmetic and the before/after/into resolver, `drop-dom.ts` owns publishing a
 * row and hit-testing a pointer against it — so a chat row and a branch row feel
 * identical under the hand. What is HERE is only this tree's half: which
 * attributes its rows carry, and the single refusal its domain has.
 *
 * Those attribute names are the tree's identity. They are the sidebar's
 * neighbours on screen and carry none of the sidebar's names, so the sidebar's
 * hit test walks straight past a chat row and this one walks straight past a
 * branch — two trees on one page, neither able to see the other's rows.
 */

/** The two row kinds this tree moves. */
export type { ChatRowKind }

export interface ChatDragSubject extends DragSubjectBase {
  kind: ChatRowKind
}

export interface ChatDropRow extends DropTargetBase {
  kind: ChatRowKind
  /** This row's ancestor chain, `/a/b/` — how a refusal is tested. */
  path?: string
}

export type ResolvedChatDrop = ChatDropRow & { mode: DropMode }

export const CHAT_ROW_SPEC: DropRowSpec = {
  kinds: { chat: 'data-chat-drop', chatFolder: 'data-chat-folder-drop' },
  parentAttr: 'data-chat-parent',
  strings: { path: 'data-chat-path' },
  flags: { expanded: 'data-chat-expanded', hasChildren: 'data-chat-children' },
}

/**
 * Everything holds everything. The only refusal is structural.
 *
 * A chat holds threads and folders; a folder holds anything, including inside a
 * chat, because it carries no turns and so cannot make the indent ambiguous. The
 * one move that must not happen is a row into its own subtree, which would
 * detach that subtree from the tree entirely — the classic way to lose rows.
 *
 * Tested against the target's PUBLISHED ancestor chain rather than against the
 * store: this runs on every pointermove, and a tree walk per frame is the cost
 * this whole DOM-driven design exists to avoid. The ids are delimited on both
 * sides in the attribute, so a substring test is an exact ancestor test.
 */
export function chatAllowedModes(subjects: readonly ChatDragSubject[], target: ChatDropRow) {
  if (subjects.length === 0) return NO_MODES
  const path = target.path ?? ''
  for (const s of subjects) {
    if (s.id === target.id) return NO_MODES
    if (path.includes(`/${s.id}/`)) return NO_MODES
  }
  return ALL_MODES
}

/**
 * The outer band of a row reorders; the middle nests.
 *
 * A folder gets the narrow band: dropping into one changes where a chat is
 * FILED and nothing about what it reads, so it is the cheap, common move and
 * gets the easy target. A chat gets the wide one, because dropping into a chat
 * makes the row a THREAD of it and rewrites what that conversation sees — the
 * expensive miss, and so the harder thing to hit by accident.
 */
export function chatEdgeBandFor(kind: string): number {
  return kind === 'chatFolder' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY
}

export const CHAT_DROP_POLICY: DropPolicy<ChatDragSubject, ChatDropRow> = {
  allowedModes: chatAllowedModes,
  edgeBandFor: chatEdgeBandFor,
}

const rows = createDropRowDom<ChatDropRow>(CHAT_ROW_SPEC)

/** The attributes a droppable chat/folder row spreads onto its own element. */
export function chatRowProps(row: ChatDropRow): Record<string, string | undefined> {
  return rows.props(row)
}

/** The live row element for a subject, for cloning it into the drag ghost. */
export function chatRowElementFor(
  subject: { kind: string; id: string },
  root: ParentNode = document,
): HTMLElement | null {
  return rows.elementFor(subject, root)
}

/** Read a chat row back off an element, or null if this is not one. */
export function readChatRow(el: Element): ChatDropRow | null {
  return rows.read(el)
}

/**
 * The EDITOR PANE is where a chat goes to be removed — the same target, read
 * through the same attribute, that a workspace row is dragged onto.
 *
 * Not a bin in this panel's footer. That bin was here, and it was wrong twice
 * over: it made the sidebar's one destructive gesture mean two different things
 * depending on which panel you were looking at, and it put the target at the
 * bottom of a scrolling list instead of on the biggest surface in the window.
 * The pane's veil, its arming dwell and the removal tray behind it are all the
 * sidebar's, unchanged (editor-removal-overlay.tsx, removal-tray.tsx).
 *
 * Every row this tree carries can leave — there is no lock here and no kind the
 * daemon refuses — so the zone never returns null, and a pointer over the pane
 * never falls through to whatever is drawn behind it.
 */
const hitTest = createDropHitTest<ChatDragSubject, ChatDropRow, { kind: 'pane' }>(
  rows,
  CHAT_DROP_POLICY,
  { attr: PANE_DROP_ATTR, hit: () => ({ kind: 'pane' }) },
)

export type ChatDropHit =
  | { kind: 'pane' }
  | {
      kind: 'row'
      row: ResolvedChatDrop
      rect: { top: number; bottom: number; left: number; width: number }
    }

/** What a pointer at (x, y) would drop onto, with the decision already made. */
export function findChatDrop(
  x: number,
  y: number,
  subjects: readonly ChatDragSubject[],
): ChatDropHit | null {
  return hitTest(x, y, subjects) as ChatDropHit | null
}

/** One row's new place: which container, and at which dense index inside it. */
export interface ChatMove {
  id: string
  parentId: string
  order: number
}

/**
 * What a drop means, worked out before anything is written or sent.
 *
 * `calls` is what goes to the daemon, in order — each `order` indexes the level
 * as it stands once the previous call has landed, so firing them together would
 * leave a multi-row move in an arbitrary arrangement. `writes` is the whole
 * destination level renumbered: the rows that moved AND the siblings they
 * displaced, so the optimistic paint is the arrangement the server will end up
 * with rather than a splice that the next sort undoes.
 */
export interface ChatDropPlan {
  calls: ChatMove[]
  writes: ChatMove[]
}

const NOTHING: ChatDropPlan = { calls: [], writes: [] }

/**
 * Where a drop lands, against the REAL sibling list rather than the drawn one.
 *
 * The subtle case is `after` on an expanded row that has children: the gap under
 * such a row is not the slot after its subtree, it is the slot before its first
 * child — which is where the indicator was drawn. So "after" there is a
 * re-parent, and pretending otherwise would drop the row somewhere the line
 * never pointed.
 */
export function chatDropPlan(
  subjects: readonly ChatDragSubject[],
  target: ResolvedChatDrop,
  siblings: ReadonlyMap<string, readonly string[]>,
): ChatDropPlan {
  if (subjects.length === 0) return NOTHING

  const firstChild = target.mode !== 'into' && resolvesToFirstChild(target, target.mode)
  const container = target.mode === 'into' || firstChild ? target.id : (target.parentId ?? '')

  const current = siblings.get(container) ?? []
  const moving = subjects.map((s) => s.id)
  const lifted = new Set(moving)
  const rest = current.filter((id) => !lifted.has(id))

  let at: number
  if (target.mode === 'into') at = rest.length
  else if (firstChild) at = 0
  else {
    const i = rest.indexOf(target.id)
    // An anchor that is not in this level is a race — the row moved out from
    // under the pointer. Landing at the end beats swallowing the drag.
    at = i < 0 ? rest.length : target.mode === 'after' ? i + 1 : i
  }

  const next = [...rest.slice(0, at), ...moving, ...rest.slice(at)]
  // A drop that changes nothing sends nothing. Dropping a row onto its own edge
  // is the commonest accident in a list, and a write per accident is a write the
  // daemon has to serialise behind the ones that mean something.
  if (next.length === current.length && next.every((id, i) => id === current[i])) return NOTHING

  return {
    calls: moving.map((id, i) => ({ id, parentId: container, order: at + i })),
    writes: next.map((id, order) => ({ id, parentId: container, order })),
  }
}
