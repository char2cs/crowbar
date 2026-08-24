/**
 * Filtering the Chats panel by title.
 *
 * A hit is drawn with the rows above it, dimmed, rather than alone: a chat that
 * turns up with no folder and no parent chat around it has lost the two facts
 * that say what it is. Descendants of a hit come along whole, because a matched
 * parent is usually what you were looking for and its threads are the answer.
 */

/** A node as the search sees it: an id, something to match, and children. */
export interface SearchNode {
  id: string
  title: string
  children: readonly SearchNode[]
}

export interface SearchSets {
  /** Titles containing the query. */
  match: Set<string>
  /** Everything rendered: matches, their ancestors, their descendants. */
  keep: Set<string>
  /** Kept ONLY as an ancestor of a match — drawn dimmed. */
  ctx: Set<string>
}

export function searchChats(roots: readonly SearchNode[], query: string): SearchSets {
  const q = query.trim().toLowerCase()
  const match = new Set<string>()
  const keep = new Set<string>()
  const ctx = new Set<string>()
  if (!q) return { match, keep, ctx }

  const keepSubtree = (node: SearchNode): void => {
    keep.add(node.id)
    for (const c of node.children) keepSubtree(c)
  }

  const walk = (nodes: readonly SearchNode[], trail: readonly string[]): void => {
    for (const node of nodes) {
      if (node.title.toLowerCase().includes(q)) {
        match.add(node.id)
        keepSubtree(node)
        for (const ancestor of trail) {
          keep.add(ancestor)
          ctx.add(ancestor)
        }
      }
      walk(node.children, [...trail, node.id])
    }
  }
  walk(roots, [])

  // A row that matched is a HIT, whatever else it is. Resolved after the walk
  // rather than during it: a match can be discovered only after it has already
  // been added as some earlier hit's ancestor, so subtracting as we go would
  // depend on document order.
  for (const id of match) ctx.delete(id)

  return { match, keep, ctx }
}

/**
 * The three pieces a matched title renders as. Case-insensitive on the query,
 * but the `hit` is sliced from the ORIGINAL title so the row keeps its own
 * casing under the mark.
 */
export function splitHighlight(
  title: string,
  query: string,
): { before: string; hit: string; after: string } {
  const q = query.trim().toLowerCase()
  const i = q ? title.toLowerCase().indexOf(q) : -1
  if (i < 0) return { before: title, hit: '', after: '' }
  return {
    before: title.slice(0, i),
    hit: title.slice(i, i + q.length),
    after: title.slice(i + q.length),
  }
}
