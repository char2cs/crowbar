import type { Path, Value } from 'platejs'

export type Node = { text?: string; children?: Node[] } & Record<string, unknown>

// Props a plugin derives rather than the markdown carrying, so a fresh parse of
// the same text never matches them: `NodeIdPlugin` stamps a random `id`, and
// `@platejs/list` renumbers `listStart` (deleting it from the first item).
const IGNORED_KEYS = new Set(['id', 'listStart'])

export function nodesEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (typeof a !== 'object' || a === null || typeof b !== 'object' || b === null) return false
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false
    return a.every((item, i) => nodesEqual(item, b[i]))
  }
  const ao = a as Record<string, unknown>
  const bo = b as Record<string, unknown>
  const keysA = Object.keys(ao).filter((k) => !IGNORED_KEYS.has(k))
  const keysB = Object.keys(bo).filter((k) => !IGNORED_KEYS.has(k))
  if (keysA.length !== keysB.length) return false
  return keysA.every((k) => k in bo && nodesEqual(ao[k], bo[k]))
}

// Excludes `text` too: on a leaf, text is allowed to differ — only the marks
// must match for a text change to be a pure append.
function ownPropsEqual(a: Node, b: Node): boolean {
  const keysA = Object.keys(a).filter(
    (k) => !IGNORED_KEYS.has(k) && k !== 'children' && k !== 'text',
  )
  const keysB = Object.keys(b).filter(
    (k) => !IGNORED_KEYS.has(k) && k !== 'children' && k !== 'text',
  )
  if (keysA.length !== keysB.length) return false
  return keysA.every((k) => nodesEqual(a[k], b[k]))
}

/** One leaf whose marks and/or text differ, at the path it holds in both trees. */
export interface LeafDiff {
  path: Path
  prev: Node
  next: Node
}

/**
 * Collects every differing leaf when both trees share the same shape, returning
 * false for a structural change — tells a mark completing in place apart from a
 * block whose shape changed and must be replaced whole.
 */
export function leafDivergences(prev: Node, next: Node, path: Path, out: LeafDiff[]): boolean {
  const prevIsText = typeof prev.text === 'string'
  const nextIsText = typeof next.text === 'string'
  if (prevIsText !== nextIsText) return false
  if (prevIsText && nextIsText) {
    if (prev.text !== next.text || !ownPropsEqual(prev, next)) out.push({ path, prev, next })
    return true
  }
  if (!ownPropsEqual(prev, next)) return false
  const prevChildren = prev.children ?? []
  const nextChildren = next.children ?? []
  if (prevChildren.length !== nextChildren.length) return false
  for (let i = 0; i < prevChildren.length; i++) {
    if (!leafDivergences(prevChildren[i]!, nextChildren[i]!, [...path, i], out)) return false
  }
  return true
}

export function markKeys(node: Node): string[] {
  return Object.keys(node).filter((k) => !IGNORED_KEYS.has(k) && k !== 'children' && k !== 'text')
}

/** The rightmost text leaf of a node — where a trailing append always lands. */
export function lastLeaf(node: Node): Node {
  if (typeof node.text === 'string') return node
  const children = node.children ?? []
  return lastLeaf(children[children.length - 1] ?? {})
}

export function flattenText(node: Node): string {
  if (typeof node.text === 'string') return node.text
  return (node.children ?? []).map(flattenText).join('')
}

// Mark-blind on purpose: marks are exactly what a completing span changes, and
// this measures how much text was already visible regardless of them.
export function commonPrefixLength(a: string, b: string): number {
  const max = Math.min(a.length, b.length)
  let i = 0
  while (i < max && a[i] === b[i]) i++
  return i
}

/** Paired with `commonPrefixLength` so a mark completing mid-block leaves the
 *  untouched tail un-faded. Callers clamp against the prefix: an identical
 *  string matches fully from both ends. */
export function commonSuffixLength(a: string, b: string): number {
  const max = Math.min(a.length, b.length)
  let i = 0
  while (i < max && a[a.length - 1 - i] === b[b.length - 1 - i]) i++
  return i
}

/** How many leading top-level blocks `prev` and `next` already agree on. */
export function stableBlockCount(prev: Value, next: Value): number {
  const max = Math.min(prev.length, next.length)
  let i = 0
  while (i < max && nodesEqual(prev[i], next[i])) i++
  return i
}

export interface TextDivergence {
  /** How many of `prev`'s trailing leaf characters stay untouched. */
  keep: number
  /** What replaces the rest of that leaf; empty when `next` is a strict prefix. */
  replacement: string
}

/**
 * How `next`'s trailing leaf diverges from `prev`'s: a pure append, or a
 * partial prefix when turn reconciliation rewrites the tail. Null for any
 * structural change (marks, a non-trailing leaf, child count, block type).
 */
export function trailingTextDivergence(prev: Node, next: Node): TextDivergence | null {
  const prevIsText = typeof prev.text === 'string'
  const nextIsText = typeof next.text === 'string'
  if (prevIsText !== nextIsText) return null
  if (prevIsText && nextIsText) {
    if (!ownPropsEqual(prev, next)) return null
    const prevText = prev.text as string
    const nextText = next.text as string
    const max = Math.min(prevText.length, nextText.length)
    let keep = 0
    while (keep < max && prevText[keep] === nextText[keep]) keep++
    return { keep, replacement: nextText.slice(keep) }
  }
  if (!ownPropsEqual(prev, next)) return null
  const prevChildren = prev.children ?? []
  const nextChildren = next.children ?? []
  if (prevChildren.length !== nextChildren.length) return null
  for (let i = 0; i < prevChildren.length; i++) {
    if (nodesEqual(prevChildren[i], nextChildren[i])) continue
    // Divergence behind the tail is structural, not a stream growing in place.
    if (i !== prevChildren.length - 1) return null
    return trailingTextDivergence(prevChildren[i], nextChildren[i])
  }
  return { keep: (lastLeaf(prev).text as string).length, replacement: '' }
}
