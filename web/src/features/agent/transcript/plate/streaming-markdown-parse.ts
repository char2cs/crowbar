import type { Value } from 'platejs'

/** Where a streaming message's settled prefix ends. Derived from the text alone:
 *  the prefix's NODES are the editor's own `children`, never copied here. */
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

interface Fence {
  char: string
  length: number
  info: string
}

/** The fence open after `line`: one opens on an opening line and closes on a
 *  bare fence of its character at least as long. */
function fenceAfter(fence: Fence | null, line: string): Fence | null {
  const match = FENCE.exec(line)
  if (!fence)
    return match ? { char: match[1]![0]!, length: match[1]!.length, info: match[2]! } : null
  const closes =
    match &&
    match[1]!.startsWith(fence.char) &&
    match[1]!.length >= fence.length &&
    match[2]!.trim() === ''
  return closes ? null : fence
}

export type DiagramFence = 'mermaid' | 'excalidraw'
const DIAGRAMS: DiagramFence[] = ['mermaid', 'excalidraw']

// The diagram language of the fence still open at the end of `md`, scanning from a stream cursor
// (never inside a fence). An unterminated opening line counts once its info string prefixes a
// diagram language, so a half-typed ```merm is a diagram, not code.
export function openDiagramFence(md: string, from: number): DiagramFence | null {
  let fence: Fence | null = null
  let pos = from
  let partial = false
  while (pos < md.length) {
    const nl = md.indexOf('\n', pos)
    const line = md.slice(pos, nl === -1 ? md.length : nl).replace(/\r$/, '')
    const opened = !fence
    fence = fenceAfter(fence, line)
    partial = opened && fence !== null && nl === -1
    if (nl === -1) break
    pos = nl + 1
  }
  if (!fence) return null
  const lang = fence.info.trim().split(/\s/)[0]!.split(':')[0]!
  if (partial) return (lang && DIAGRAMS.find((d) => d.startsWith(lang))) || null
  return DIAGRAMS.find((d) => d === lang) ?? null
}

interface Scan {
  /** Start offsets of lines a settled prefix may end right before. */
  candidates: number[]
  backward: boolean
}

// Lines before which the text parses on its own exactly as in the whole message: column 0, after
// a blank line, outside a fence. A list marker qualifies only after a chunk that cannot be a
// list, since after a blank line it continues the list before it.
function scanFrom(md: string, from: number): Scan {
  const candidates: number[] = []
  let fence: Fence | null = null
  let afterBlank = false
  let kind: ChunkKind = 'plain'
  let pos = from
  let started = false
  while (pos < md.length) {
    const nl = md.indexOf('\n', pos)
    const end = nl === -1 ? md.length : nl
    const line = md.slice(pos, end).replace(/\r$/, '')
    const blank = line.trim() === ''
    if (fence) {
      fence = fenceAfter(fence, line)
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
      fence = fenceAfter(null, line)
    }
    if (nl === -1) break
    pos = nl + 1
  }
  return { candidates, backward: false }
}

// `md` as a Plate value, parsing only the unsettled tail; the prefix reuses `current`'s nodes so
// `applyStreamedValue` skips them by identity. Equals `parse(md)` up to node ids: anything the
// scan cannot prove safe falls back to the whole parse.
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
