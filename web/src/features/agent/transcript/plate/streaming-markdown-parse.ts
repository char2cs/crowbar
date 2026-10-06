import type { Value } from 'platejs'

/**
 * Where a streaming message's settled prefix ends. Derived from the text alone:
 * the prefix's NODES are the editor's own `children`, never copied here.
 */
export interface StreamCursor {
  /** Text offset where the unsettled tail begins. */
  offset: number
  /** How many leading editor blocks the text before `offset` parses to. */
  blocks: number
  /** The text already settled (or, once `disabled`, last seen); a message that
   *  stops starting with it was rewritten and starts over. */
  prefix: string
  /** Set once a construct that reaches backwards (a reference definition, a raw
   *  HTML block ended by something other than a blank line) was seen. */
  disabled: boolean
}

export const FRESH_CURSOR: StreamCursor = { offset: 0, blocks: 0, prefix: '', disabled: false }

const FENCE = /^ {0,3}(`{3,}|~{3,})(.*)$/
const LIST_MARKER = /^\s*(?:[-+*]|\d{1,9}[.)])(?:\s|$)/
// What makes an earlier block parse differently once it is seen: a link
// reference or footnote definition, and the HTML blocks a blank line does not end.
const BACKWARD =
  /^ {0,3}(?:\[[^\]]*\]:|<(?:script|pre|style|textarea|!--|\?|![A-Za-z]|!\[CDATA\[))/i

type ChunkKind = 'plain' | 'list' | 'other'

interface Scan {
  /** Start offsets of lines a settled prefix may end right before. */
  candidates: number[]
  backward: boolean
}

/**
 * Lines at which the text before them parses on its own, exactly as it does in
 * the whole message: a column-0 line after a blank line, outside a fence. A
 * list marker qualifies only after a chunk that cannot be a list, since a
 * marker after a blank line continues the list before it.
 */
function scanFrom(md: string, from: number): Scan {
  const candidates: number[] = []
  let fence: { char: string; length: number } | null = null
  let afterBlank = false
  let kind: ChunkKind = 'plain'
  let pos = from
  let started = false
  while (pos < md.length) {
    const nl = md.indexOf('\n', pos)
    const end = nl === -1 ? md.length : nl
    const line = md.slice(pos, end).replace(/\r$/, '')
    const blank = line.trim() === ''
    const fenceMatch = FENCE.exec(line)
    if (fence) {
      const closes =
        fenceMatch &&
        fenceMatch[1]!.startsWith(fence.char) &&
        fenceMatch[1]!.length >= fence.length &&
        fenceMatch[2]!.trim() === ''
      if (closes) fence = null
    } else if (blank) {
      afterBlank = true
    } else {
      if (BACKWARD.test(line)) return { candidates, backward: true }
      const marker = LIST_MARKER.test(line)
      const col0 = !/^\s/.test(line)
      const complete = nl !== -1
      if (!started || afterBlank) {
        const startsChunk = started && col0 && complete && (!marker || kind === 'plain')
        if (startsChunk) candidates.push(pos)
        kind = !col0 ? 'other' : marker ? 'list' : 'plain'
      } else if (marker) {
        kind = 'list'
      }
      afterBlank = false
      started = true
      if (fenceMatch) fence = { char: fenceMatch[1]![0]!, length: fenceMatch[1]!.length }
    }
    if (nl === -1) break
    pos = nl + 1
  }
  return { candidates, backward: false }
}

/**
 * `md` as a Plate value, parsing only the unsettled tail. The settled prefix is
 * taken from `current` (the editor's children, which already hold it), so its
 * nodes keep their identity from delta to delta and `applyStreamedValue`
 * skips them without comparing.
 *
 * The result equals `parse(md)` up to node ids; anything the scan cannot prove
 * safe falls back to the whole parse.
 */
export function parseStreamingMarkdown(
  md: string,
  cursor: StreamCursor,
  current: Value,
  parse: (md: string) => Value,
): { value: Value; cursor: StreamCursor } {
  let at = cursor
  if (!md.startsWith(at.prefix) || current.length < at.blocks) at = FRESH_CURSOR
  if (at.disabled) return { value: parse(md), cursor: { ...at, prefix: md } }

  const scan = scanFrom(md, at.offset)
  if (scan.backward) {
    return { value: parse(md), cursor: { ...FRESH_CURSOR, disabled: true, prefix: md } }
  }

  const tail = parse(at.offset === 0 ? md : md.slice(at.offset))
  const value = at.blocks === 0 ? tail : [...current.slice(0, at.blocks), ...tail]

  // Keep one complete chunk in the tail: the line a boundary ends before must
  // itself be known to be complete, so only the second-to-last candidate settles.
  const boundary = scan.candidates.at(-2)
  if (boundary === undefined) return { value, cursor: at }
  const settledBlocks = at.blocks + parse(md.slice(at.offset, boundary)).length
  return {
    value,
    cursor: {
      offset: boundary,
      blocks: settledBlocks,
      prefix: md.slice(0, boundary),
      disabled: false,
    },
  }
}
