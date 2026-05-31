import {
  Annotation,
  EditorState,
  RangeSetBuilder,
  StateField,
  Text,
} from '@codemirror/state'
import {
  Decoration,
  DecorationSet,
  EditorView,
} from '@codemirror/view'
import type { MarkdownTurn, TurnRole } from '../types'
import { TURN_MARKER_RE } from '../types'

export interface TurnRange {
  id: string
  role: TurnRole
  from: number
  to: number
}

// Used by appendTurnToHistory and appendStreamChunk to bypass any future filters.
export const streamingAnnotation = Annotation.define<boolean>()

export function turnsToDocument(turns: MarkdownTurn[]): string {
  if (turns.length === 0) return ''
  return turns
    .map((t) => `<!-- turn:${t.id} role:${t.role} -->\n${t.content}`)
    .join('\n\n')
}

export function parseTurnBoundaries(doc: Text): TurnRange[] {
  const ranges: TurnRange[] = []
  for (let i = 1; i <= doc.lines; i++) {
    const line = doc.line(i)
    const match = line.text.match(TURN_MARKER_RE)
    if (match) {
      if (ranges.length > 0) ranges[ranges.length - 1].to = line.from - 1
      ranges.push({ id: match[1], role: match[2] as TurnRole, from: line.from, to: doc.length })
    }
  }
  return ranges
}

// Append a completed turn (user or agent) to the history viewer.
// Used by handleSubmit in the view after input CM6 submits.
export function appendTurnToHistory(
  view: EditorView,
  id: string,
  role: TurnRole,
  content: string,
): void {
  const sep = view.state.doc.length === 0 ? '' : '\n\n'
  const insert = `${sep}<!-- turn:${id} role:${role} -->\n${content}`
  const end = view.state.doc.length + insert.length
  view.dispatch({
    changes: { from: view.state.doc.length, insert },
    annotations: streamingAnnotation.of(true),
    // Scroll the freshly appended turn into view.
    effects: EditorView.scrollIntoView(end, { y: 'end' }),
  })
}

const turnRangesField = StateField.define<TurnRange[]>({
  create: (state) => parseTurnBoundaries(state.doc),
  update: (ranges, tr) => tr.docChanged ? parseTurnBoundaries(tr.newDoc) : ranges,
})

function buildDecorations(state: EditorState): DecorationSet {
  const ranges = state.field(turnRangesField)
  const builder = new RangeSetBuilder<Decoration>()

  for (const range of ranges) {
    const markerLine = state.doc.lineAt(range.from)
    // Hide the boundary marker line (including trailing newline)
    const markerEnd = Math.min(markerLine.to + 1, state.doc.length)

    if (range.role === 'user') {
      // Decoration.replace spans the marker line AND its newline, causing CM6 to
      // merge the marker line into the first content line's visual .cm-line element.
      // That merged visual line's "from" is markerLine.from (not doc.line(N+1).from),
      // so Decoration.line must be placed at markerLine.from to target it correctly.
      builder.add(markerLine.from, markerLine.from, Decoration.line({ class: 'cm-turn-user cm-turn-head' }))
      builder.add(markerLine.from, markerEnd, Decoration.replace({}))

      // Tint remaining content lines (not merged with the replace block).
      const firstLineNum = markerLine.number + 2  // +2: skip marker(+1) and first content(+1)
      let lastLineNum = state.doc.lineAt(range.to).number
      // Don't tint the trailing blank line(s) — the "\n\n" separator before the
      // next turn falls inside this range and would show as an empty tinted line.
      while (lastLineNum >= firstLineNum && state.doc.line(lastLineNum).text.trim() === '') {
        lastLineNum--
      }
      for (let ln = firstLineNum; ln <= lastLineNum; ln++) {
        const l = state.doc.line(ln)
        builder.add(l.from, l.from, Decoration.line({ class: 'cm-turn-user' }))
      }
    } else {
      // Agent turns: hide marker line, no tint. The head class reserves the gap
      // for the sticky metadata label (turn-meta.ts). The line deco at
      // markerLine.from targets the merged first visual line (the replace below
      // spans the marker's newline), so it must precede the replace at this pos.
      builder.add(markerLine.from, markerLine.from, Decoration.line({ class: 'cm-turn-head' }))
      builder.add(markerLine.from, markerEnd, Decoration.replace({}))
    }
  }

  return builder.finish()
}

const turnDecorationsField = StateField.define<DecorationSet>({
  create: (state) => buildDecorations(state),
  update: (deco, tr) => tr.docChanged ? buildDecorations(tr.state) : deco,
  provide: (f) => EditorView.decorations.from(f),
})

// Full-width user-turn tinting via CSS.
// .cm-content has no horizontal padding; .cm-line carries the column padding.
// User lines get background that fills the full .cm-line block width.
const turnTheme = EditorView.theme({
  // Shared text metrics — must match the input editor (markdown-chat-input.tsx).
  '&': { height: '100%', width: '100%', fontSize: '15px' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': {
    overflow: 'auto',
    fontFamily: 'var(--font-sans, system-ui)',
    // Reserve scrollbar space so the centered text column lines up with the
    // input editor whether or not a scrollbar is present.
    scrollbarGutter: 'stable',
    scrollbarWidth: 'thin',
    scrollbarColor: 'var(--app-scrollbar-thumb) var(--app-scrollbar-track)',
  },
  '.cm-scroller::-webkit-scrollbar': { width: '6px' },
  '.cm-scroller::-webkit-scrollbar-track': { background: 'var(--app-scrollbar-track)' },
  '.cm-scroller::-webkit-scrollbar-thumb': {
    background: 'var(--app-scrollbar-thumb)',
    borderRadius: 'var(--app-scrollbar-radius)',
  },
  '.cm-content': {
    padding: '40px 0 32px',
    minWidth: '100%',
  },
  '.cm-line': {
    padding: '0 max(48px, calc((100% - 680px) / 2 + 48px))',
    lineHeight: '1.75',
  },
  '.cm-turn-user': {
    background: 'color-mix(in srgb, var(--primary) 5%, transparent)',
  },
  // Reserve space at the start of each turn for the sticky metadata label
  // rendered by turn-meta.ts, so the label never overlaps the first line.
  '.cm-turn-head': {
    paddingTop: '26px',
  },
})

export function turnBoundaries() {
  return [turnRangesField, turnDecorationsField, turnTheme]
}

export function getTurnRanges(state: EditorState): TurnRange[] {
  return state.field(turnRangesField)
}
