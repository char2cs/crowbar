import type { Value } from 'platejs'

export interface StreamCursor {
  offset: number
  blocks: number
  prefix: string
  disabled: boolean
}

export const FRESH_CURSOR: StreamCursor = { offset: 0, blocks: 0, prefix: '', disabled: false }

const FENCE = /^ {0,3}(`{3,}|~{3,})(.*)$/
const LIST_MARKER = /^\s*(?:[-+*]|\d{1,9}[.)])(?:\s|$)/
const BACKWARD =
  /^ {0,3}(?:\[[^\]]*\]:|<(?:script|pre|style|textarea|!--|\?|![A-Za-z]|!\[CDATA\[))/i

type ChunkKind = 'plain' | 'list' | 'other'

interface Fence {
  char: string
  length: number
  info: string
}

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
  candidates: number[]
  backward: boolean
}

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
