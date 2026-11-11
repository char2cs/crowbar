import {
  anyModeAllowed,
  dropModeAt,
  type DragSubjectBase,
  type DropMode,
  type DropPolicy,
  type DropTargetBase,
  type RowRect,
} from './drop-core'

// The DOM half of drag and drop. Every fact a hit test needs rides on the row element (one
// `elementsFromPoint`, one attribute sweep, one rect). Each tree supplies its own attribute table,
// so two trees on one page cannot see each other's rows — why this is a registry, not a union.

/** What a tree's rows publish about themselves, written and read through ONE table so the halves
 *  cannot disagree (a row writing one attribute and reading another silently lands at root). */
export interface DropRowSpec {
  /** Row kind → the attribute carrying its id; carrying one is what makes an element this tree's
   *  row, so the attributes must be unique to the tree. */
  kinds: Readonly<Record<string, string>>
  /** The container attribute, always written ('' = root) so a read-back row never has an
   *  `undefined` container to ground. */
  parentAttr: string
  /** Field → attribute. Written only when set; read back as `string | undefined`. */
  strings?: Readonly<Record<string, string>>
  /** Field → attribute, published as PRESENCE, not "true"/"false". */
  flags?: Readonly<Record<string, string>>
}

/** A row plus the decision the pointer's position in it resolves to. */
export type Resolved<Row> = Row & { mode: DropMode }

/** A row hit with the rect the decision was made from, so the indicator reuses it and a drag
 *  needs no DOM read of its own per pointermove. */
export interface RowHit<Row> {
  kind: 'row'
  row: Resolved<Row>
  rect: RowRect
  /** 'x' inside a `data-drop-axis="x"` strip (tabs side by side), else 'y'. */
  axis: 'x' | 'y'
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

// A whole-region drop target that is not a row (the editor pane), read by the same hit test.
// Null is a refusal, and it stops the hit test rather than falling through to rows behind it.
export interface DropZone<S extends DragSubjectBase, Hit> {
  attr: string
  /** `el`/`point` (the zone element and pointer) serve geometry-aware zones such as the pane's
   *  center/edge split; a `hit(subjects)` that ignores them still fits. */
  hit(subjects: readonly S[], el: Element, point: { x: number; y: number }): Hit | null
}

// Binds a row table and a policy into the per-move pointer→decision function. The topmost row is
// the answer even when it refuses (null, never an ancestor's indicator). Zones are checked per
// element in order, so a zone wins only when it is the TOPMOST thing painted at the point.
export function createDropHitTest<
  S extends DragSubjectBase,
  Row extends DropTargetBase,
  Hit = never,
>(
  dom: DropRowDom<Row>,
  policy: DropPolicy<S, Row>,
  zone?: DropZone<S, Hit> | ReadonlyArray<DropZone<S, Hit>>,
): (x: number, y: number, subjects: readonly S[]) => RowHit<Row> | Hit | null {
  const zones: ReadonlyArray<DropZone<S, Hit>> =
    zone === undefined ? [] : Array.isArray(zone) ? zone : [zone]
  return (x, y, subjects) => {
    for (const el of document.elementsFromPoint(x, y)) {
      // The ghost is a deep clone carrying real drop attributes under the cursor. `pointer-events:
      // none` should hide it from `elementsFromPoint`; this zero-cost check makes sure it is never
      // a hit.
      if (el.closest('[data-drag-ghost]')) continue
      for (const z of zones) {
        if (el.hasAttribute(z.attr)) return z.hit(subjects, el, { x, y })
      }
      const row = dom.read(el)
      if (!row) continue
      const allowed = policy.allowedModes(subjects, row)
      if (!anyModeAllowed(allowed)) return null
      const rect = el.getBoundingClientRect()
      if (rect.height <= 0 || rect.width <= 0) return null
      const axis = el.closest('[data-drop-axis="x"]') ? 'x' : 'y'
      const ratio = axis === 'x' ? (x - rect.left) / rect.width : (y - rect.top) / rect.height
      const mode = dropModeAt(ratio, allowed, policy.edgeBandFor(row.kind))
      return mode === null ? null : { kind: 'row', row: { ...row, mode }, rect, axis }
    }
    return null
  }
}
