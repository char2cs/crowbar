import {
  anyModeAllowed,
  dropModeAt,
  type DragSubjectBase,
  type DropMode,
  type DropPolicy,
  type DropTargetBase,
  type RowRect,
} from './drop-core'

export interface DropRowSpec {
  kinds: Readonly<Record<string, string>>
  parentAttr: string
  strings?: Readonly<Record<string, string>>
  flags?: Readonly<Record<string, string>>
}

type Resolved<Row> = Row & { mode: DropMode }

export interface RowHit<Row> {
  kind: 'row'
  row: Resolved<Row>
  rect: RowRect
  axis: 'x' | 'y'
}

export interface DropRowDom<Row> {
  props(row: Row): Record<string, string | undefined>
  read(el: Element): Row | null
  readAll(selector: string, root?: ParentNode): Row[]
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
      if (!attr) return null
      return root.querySelector<HTMLElement>(`[${attr}="${subject.id}"]`)
    },
  }
}

export interface DropZone<S extends DragSubjectBase, Hit> {
  attr: string
  hit(subjects: readonly S[], el: Element, point: { x: number; y: number }): Hit | null
}

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
