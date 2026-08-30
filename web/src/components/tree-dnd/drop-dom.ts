import {
  anyModeAllowed,
  dropModeAt,
  type DragSubjectBase,
  type DropMode,
  type DropPolicy,
  type DropTargetBase,
  type RowRect,
} from './drop-core'

/**
 * The DOM half of drag and drop, driven by a table each tree publishes.
 *
 * Every fact a matrix needs rides on the row element itself rather than being
 * looked up in a store at pointer rate. A hit test is then one
 * `elementsFromPoint`, one `getAttribute` sweep and one rect — no tree walk, no
 * store read, nothing that scales with how many rows you have.
 *
 * Which attributes those are is the one thing that differs per tree, so it is
 * the one thing supplied: a {@link DropRowSpec} names the attribute for each
 * row kind and for each extra fact, and {@link createDropRowDom} returns the
 * read/write pair bound to it. Two trees on one page therefore cannot see each
 * other's rows — a chat row carries none of the sidebar's attributes, so the
 * sidebar's `readRow` returns null for it and its hit test walks straight past.
 * That mutual invisibility is the reason this is a registry and not a union
 * that grew two more members.
 */

/**
 * What a tree's rows publish about themselves.
 *
 * Written and read back through one table so the two halves cannot disagree,
 * which is the failure mode a hand-rolled second tree would hit first: a row
 * that writes `data-chat-parent` and reads `data-drop-parent` is a row that is
 * always at the root and nobody notices until a drop lands in the wrong place.
 */
export interface DropRowSpec {
  /**
   * Row kind → the attribute carrying that row's id. An element is one of this
   * tree's rows exactly when it carries one of these, so the attributes must be
   * unique to the tree.
   */
  kinds: Readonly<Record<string, string>>
  /**
   * The container attribute. Always written, '' meaning the tree's own root, so
   * a row read back always has a container to reason about rather than an
   * `undefined` every caller has to remember to ground.
   */
  parentAttr: string
  /** Field → attribute. Written only when set; read back as `string | undefined`. */
  strings?: Readonly<Record<string, string>>
  /**
   * Field → attribute, published as PRESENCE rather than as "true"/"false", so
   * a row that is neither locked nor expanded carries no attribute at all.
   */
  flags?: Readonly<Record<string, string>>
}

/** A row plus the decision the pointer's position in it resolves to. */
export type Resolved<Row> = Row & { mode: DropMode }

/**
 * A row hit, carrying the rect the decision was made from. The hit test has
 * already measured it, so the indicator positions itself off this rather than
 * measuring the row a second time — and it is the only reason a drag needs no
 * DOM read of its own per pointermove.
 */
export interface RowHit<Row> {
  kind: 'row'
  row: Resolved<Row>
  rect: RowRect
}

/** The read/write pair for one tree's rows. */
export interface DropRowDom<Row> {
  /** The attributes a droppable row spreads onto its own element. */
  props(row: Row): Record<string, string | undefined>
  /** Read back what {@link DropRowDom.props} wrote, or null if this is not a row. */
  read(el: Element): Row | null
  /** Every row under `root` matching `selector`, in document order. */
  readAll(selector: string, root?: ParentNode): Row[]
  /** The live row element for a subject, for cloning it into the drag ghost. */
  elementFor(subject: { kind: string; id: string }, root?: ParentNode): HTMLElement | null
}

export function createDropRowDom<Row extends DragSubjectBase>(spec: DropRowSpec): DropRowDom<Row> {
  const kinds = Object.keys(spec.kinds)
  const strings = Object.entries(spec.strings ?? {})
  const flags = Object.entries(spec.flags ?? {})

  const props = (row: Row): Record<string, string | undefined> => {
    const fields = row as unknown as Record<string, unknown>
    const out: Record<string, string | undefined> = {
      [spec.kinds[row.kind]]: row.id,
      [spec.parentAttr]: row.parentId ?? '',
    }
    for (const [field, attr] of strings) out[attr] = fields[field] as string | undefined
    for (const [field, attr] of flags) out[attr] = fields[field] ? '' : undefined
    return out
  }

  const read = (el: Element): Row | null => {
    for (const kind of kinds) {
      const id = el.getAttribute(spec.kinds[kind])
      if (id === null) continue
      const row: Record<string, unknown> = {
        kind,
        id,
        parentId: el.getAttribute(spec.parentAttr) ?? '',
      }
      for (const [field, attr] of strings) row[field] = el.getAttribute(attr) ?? undefined
      for (const [field, attr] of flags) row[field] = el.hasAttribute(attr)
      return row as Row
    }
    return null
  }

  return {
    props,
    read,
    readAll: (selector: string, root: ParentNode = document): Row[] => {
      const out: Row[] = []
      for (const el of root.querySelectorAll(selector)) {
        const row = read(el)
        if (row) out.push(row)
      }
      return out
    },
    elementFor: (subject, root: ParentNode = document): HTMLElement | null => {
      const attr = spec.kinds[subject.kind]
      // A kind this tree does not publish has no element here by definition —
      // the other tree's rows are not this tree's to find.
      if (!attr) return null
      return root.querySelector<HTMLElement>(`[${attr}="${subject.id}"]`)
    },
  }
}

/**
 * A whole-region drop target that is not a row — the editor pane, standing for
 * removal.
 *
 * It carries no mode and no rect: it is one flag, read by the same hit test
 * every row goes through, and it answers with whatever the tree wants a drop
 * there to mean. Returning null is a refusal, and a refusal here stops the hit
 * test rather than falling through to the rows behind the region.
 */
export interface DropZone<S extends DragSubjectBase, Hit> {
  attr: string
  /**
   * `el`/`point` are the zone's own element and the pointer's viewport
   * position — extra facts a binary zone (the old dwell-to-remove pane) never
   * needed but a geometry-aware one does (Task 21's pane center/edge zones,
   * `getPaneDropZoneFromRect`). Additive: an existing `hit(subjects)` that
   * ignores them still satisfies this signature.
   */
  hit(subjects: readonly S[], el: Element, point: { x: number; y: number }): Hit | null
}

/**
 * Bind a row table and a policy into the pointer→decision function a drag runs
 * on every move.
 *
 * The topmost row under the pointer is the answer whether or not it accepts the
 * drag: a refusal returns null rather than falling through to whatever sits
 * behind it, because "this row says no" is the honest reading and drawing an
 * indicator on some ancestor would be the indicator lying.
 */
export function createDropHitTest<
  S extends DragSubjectBase,
  Row extends DropTargetBase,
  Hit = never,
>(
  dom: DropRowDom<Row>,
  policy: DropPolicy<S, Row>,
  zone?: DropZone<S, Hit>,
): (x: number, y: number, subjects: readonly S[]) => RowHit<Row> | Hit | null {
  return (x, y, subjects) => {
    for (const el of document.elementsFromPoint(x, y)) {
      if (zone && el.hasAttribute(zone.attr)) return zone.hit(subjects, el, { x, y })
      const row = dom.read(el)
      if (!row) continue
      const allowed = policy.allowedModes(subjects, row)
      if (!anyModeAllowed(allowed)) return null
      const rect = el.getBoundingClientRect()
      if (rect.height <= 0) return null
      const mode = dropModeAt((y - rect.top) / rect.height, allowed, policy.edgeBandFor(row.kind))
      return mode === null ? null : { kind: 'row', row: { ...row, mode }, rect }
    }
    return null
  }
}
