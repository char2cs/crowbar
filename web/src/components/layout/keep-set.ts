/**
 * Which rows stay on screen when their parent folds away.
 *
 * This is TWO independent states, and conflating them is the whole difficulty:
 *
 *   1. **The multiselection.** Transient. Built by cmd/shift-click, drawn
 *      exactly like the open workspace, and cleared by the next plain click.
 *      That clearing — not the styling — is what stops a crowd of lit rows
 *      accumulating.
 *   2. **The keep set.** A snapshot taken at the moment a parent collapses,
 *      from whatever was multiselected or open *at that moment*. It carries no
 *      styling of its own. Clearing the multiselection and navigating to an
 *      unrelated workspace must both leave it completely alone, or a row you
 *      deliberately kept evaporates the instant you look at something else.
 *
 * Kept here as pure functions over a tree index so both rules are testable
 * without rendering anything.
 */

/** The tree questions this module needs, so it does not depend on a node type. */
export interface TreeIndex {
  /** Every descendant id of `id`, at any depth, in any order. */
  descendantsOf(id: string): string[]
  /** True when `ancestorId` sits somewhere above `id`. */
  isAncestorOf(ancestorId: string, id: string): boolean
}

/**
 * The ids to add to the keep set as `parentId` collapses.
 *
 * Whatever you had selected or open under this parent is promoted here and
 * now. After this the only thing that dismisses a row is cmd-clicking it off
 * or the parent's own fold-away control — navigation cannot touch it.
 */
export function snapshotOnCollapse(
  parentId: string,
  index: TreeIndex,
  selection: ReadonlySet<string>,
  activeId: string | null,
): string[] {
  return index.descendantsOf(parentId).filter((id) => selection.has(id) || id === activeId)
}

/** The ids to drop from the keep set as `parentId` expands: all of its own. */
export function releaseOnExpand(parentId: string, index: TreeIndex): string[] {
  return index.descendantsOf(parentId)
}

/**
 * The rows to render beneath a collapsed `parentId`.
 *
 * Only the OUTERMOST kept rows: a kept row brings its whole subtree with it, so
 * a kept descendant of another kept row would otherwise be drawn twice — once
 * hoisted and once inside its parent.
 */
export function keptRootsUnder(
  parentId: string,
  index: TreeIndex,
  kept: ReadonlySet<string>,
): string[] {
  const marked = index.descendantsOf(parentId).filter((id) => kept.has(id))
  return marked.filter(
    (id) => !marked.some((other) => other !== id && index.isAncestorOf(other, id)),
  )
}

/** True when `parentId` is folded but still holding rows — the three-dot state. */
export function isHoldingRows(
  parentId: string,
  index: TreeIndex,
  kept: ReadonlySet<string>,
): boolean {
  return index.descendantsOf(parentId).some((id) => kept.has(id))
}

/** Build the set after a fold-away: this parent lets go of everything it holds. */
export function foldAway(
  parentId: string,
  index: TreeIndex,
  kept: ReadonlySet<string>,
): Set<string> {
  const next = new Set(kept)
  for (const id of index.descendantsOf(parentId)) next.delete(id)
  return next
}

/**
 * Build a tree index from a flat parent map. `parentOf` returning undefined
 * means the id is a root.
 *
 * Guards against a corrupt parent chain: a cycle terminates rather than
 * hanging, which matters because this runs on every render of a collapsed row.
 */
export function indexFromParents(parentOf: ReadonlyMap<string, string | undefined>): TreeIndex {
  const childrenOf = new Map<string, string[]>()
  for (const [id, parent] of parentOf) {
    if (!parent) continue
    const siblings = childrenOf.get(parent)
    if (siblings) siblings.push(id)
    else childrenOf.set(parent, [id])
  }

  const descendantCache = new Map<string, string[]>()

  const descendantsOf = (id: string): string[] => {
    const cached = descendantCache.get(id)
    if (cached) return cached
    const out: string[] = []
    const seen = new Set<string>([id])
    const stack = [...(childrenOf.get(id) ?? [])]
    while (stack.length) {
      const next = stack.pop()!
      if (seen.has(next)) continue // corrupt chain: stop rather than loop
      seen.add(next)
      out.push(next)
      const kids = childrenOf.get(next)
      if (kids) stack.push(...kids)
    }
    descendantCache.set(id, out)
    return out
  }

  const isAncestorOf = (ancestorId: string, id: string): boolean => {
    const seen = new Set<string>([id])
    let cursor = parentOf.get(id)
    while (cursor) {
      if (cursor === ancestorId) return true
      if (seen.has(cursor)) return false // corrupt chain
      seen.add(cursor)
      cursor = parentOf.get(cursor)
    }
    return false
  }

  return { descendantsOf, isAncestorOf }
}
