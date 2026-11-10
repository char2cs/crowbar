import type { Descendant, Value } from 'platejs'

/** One top-level markdown block: its byte range in the source and the Plate nodes it parsed to. */
export interface SourceBlock {
  start: number
  end: number
  /** Canonical keys of the block's nodes; empty for a block with no node (a link definition). */
  keys: string[]
}

/** The text a document was parsed from, split into its top-level blocks. */
export interface MarkdownSource {
  text: string
  blocks: SourceBlock[]
}

/** A run of the output: an original block written back verbatim, or nodes serialized afresh. */
type Piece = { block: number } | { nodes: Descendant[] }

const keyCache = new WeakMap<object, string>()

function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`
  if (value === null || typeof value !== 'object') return JSON.stringify(value) ?? 'null'
  const entries = Object.keys(value)
    .filter((key) => key !== 'id')
    .sort()
    .map((key) => `${JSON.stringify(key)}:${canonical((value as Record<string, unknown>)[key])}`)
  return `{${entries.join(',')}}`
}

/** Node identity survives every edit that does not touch the node, so a key is computed once. */
export function nodeKey(node: Descendant): string {
  let key = keyCache.get(node)
  if (key === undefined) {
    key = canonical(node)
    keyCache.set(node, key)
  }
  return key
}

function runMatches(keys: string[], at: number, run: string[]): boolean {
  if (at + run.length > keys.length) return false
  return run.every((key, i) => keys[at + i] === key)
}

/** Where each source block sits in the current value (or -1): prefix and suffix first, so a
 *  local edit aligns exactly, then a forward search through the edited middle. */
function alignBlocks(keys: string[], blocks: SourceBlock[]): number[] {
  const at = blocks.map(() => -1)
  let head = 0
  let first = 0
  while (first < blocks.length && runMatches(keys, head, blocks[first].keys)) {
    at[first] = head
    head += blocks[first].keys.length
    first++
  }
  let tail = keys.length
  let last = blocks.length - 1
  while (last >= first && tail - blocks[last].keys.length >= head) {
    const start = tail - blocks[last].keys.length
    if (!runMatches(keys, start, blocks[last].keys)) break
    at[last] = start
    tail = start
    last--
  }
  for (let b = first; b <= last; b++) {
    for (let i = head; i + blocks[b].keys.length <= tail; i++) {
      if (!runMatches(keys, i, blocks[b].keys)) continue
      at[b] = i
      head = i + blocks[b].keys.length
      break
    }
  }
  return at
}

// Markdown has no empty paragraph: Plate would write one as a lone U+200B into the file.
const isEmptyParagraph = (node: Descendant) =>
  (node as { type?: string }).type === 'p' &&
  !isListItem(node) &&
  (node as { children: Descendant[] }).children.every(
    (child) => (child as { text?: string }).text === '',
  )

const isListItem = (node: Descendant | undefined) =>
  !!node && typeof (node as { listStyleType?: unknown }).listStyleType === 'string'

/** A kept block must not border a re-serialized list item of the same list: written apart, the
 *  two halves would no longer parse as one list. */
function releaseSplitLists(value: Value, blocks: SourceBlock[], at: number[]): void {
  const owner = new Array<number>(value.length).fill(-1)
  at.forEach((pos, b) => {
    for (let i = 0; pos >= 0 && i < blocks[b].keys.length; i++) owner[pos + i] = b
  })
  const release = (b: number) => {
    for (let i = 0; i < blocks[b].keys.length; i++) owner[at[b] + i] = -1
    at[b] = -1
  }
  let changed = true
  while (changed) {
    changed = false
    at.forEach((pos, b) => {
      const n = blocks[b].keys.length
      if (pos < 0 || n === 0) return
      const before = pos - 1
      const after = pos + n
      const splitsBefore = before >= 0 && owner[before] === -1 && isListItem(value[before])
      const splitsAfter = after < value.length && owner[after] === -1 && isListItem(value[after])
      if (
        (splitsBefore && isListItem(value[pos])) ||
        (splitsAfter && isListItem(value[after - 1]))
      ) {
        release(b)
        changed = true
      }
    })
  }
}

function piecesOf(value: Value, blocks: SourceBlock[], at: number[]): Piece[] {
  const startsAt = new Map<number, number[]>()
  at.forEach((pos, b) => {
    if (pos >= 0) startsAt.set(pos, [...(startsAt.get(pos) ?? []), b])
  })
  const pieces: Piece[] = []
  let fresh: Descendant[] = []
  const flushFresh = () => {
    if (fresh.length > 0) pieces.push({ nodes: fresh })
    fresh = []
  }
  let i = 0
  while (i <= value.length) {
    let kept: number | undefined
    // A block with no node (a link definition) has nothing to edit, so it stays where it was.
    for (const b of startsAt.get(i) ?? []) {
      flushFresh()
      pieces.push({ block: b })
      if (blocks[b].keys.length > 0) {
        kept = b
        break
      }
    }
    if (kept !== undefined) {
      i += blocks[kept].keys.length
      continue
    }
    if (i < value.length && !isEmptyParagraph(value[i])) fresh.push(value[i])
    i++
  }
  flushFresh()
  return pieces
}

/** Writes `value` back over the text it was parsed from: every block the user left alone is
 *  copied byte for byte, with its original spacing; only changed blocks are re-serialized. */
export function serializeOverSource(
  value: Value,
  source: MarkdownSource,
  serialize: (nodes: Descendant[]) => string,
): string {
  const { text, blocks } = source
  const at = alignBlocks(value.map(nodeKey), blocks)
  releaseSplitLists(value, blocks, at)
  const pieces = piecesOf(value, blocks, at)
  const eol = text.includes('\r\n') ? '\r\n' : '\n'
  let out = ''
  let prev: Piece | undefined
  for (const piece of pieces) {
    if ('block' in piece) {
      const block = blocks[piece.block]
      const adjacent = prev && 'block' in prev && prev.block === piece.block - 1
      if (adjacent) out += text.slice(blocks[piece.block - 1].end, block.start)
      else if (!prev && piece.block === 0) out += text.slice(0, block.start)
      else if (prev) out += eol + eol
      out += text.slice(block.start, block.end)
    } else {
      if (prev) out += eol + eol
      out += serialize(piece.nodes)
        .replace(/\r?\n$/, '')
        .replace(/\r?\n/g, eol)
    }
    prev = piece
  }
  if (prev && 'block' in prev && prev.block === blocks.length - 1) {
    return out + text.slice(blocks[prev.block].end)
  }
  return out === '' ? out : out + eol
}
