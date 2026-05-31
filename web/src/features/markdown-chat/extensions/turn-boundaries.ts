import {
  Annotation,
  EditorState,
  RangeSetBuilder,
  StateField,
  Text,
  Transaction,
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

// Annotation used by streaming-ext to bypass the read-only filter.
export const streamingAnnotation = Annotation.define<boolean>()

// Convert turns array to a single CM6 document string.
export function turnsToDocument(turns: MarkdownTurn[]): string {
  return turns
    .map((t) => `<!-- turn:${t.id} role:${t.role} -->\n${t.content}`)
    .join('\n\n')
}

// Parse turn boundary markers from a CM6 Text object.
// Returns an array of TurnRange with character positions.
export function parseTurnBoundaries(doc: Text): TurnRange[] {
  const ranges: TurnRange[] = []

  for (let i = 1; i <= doc.lines; i++) {
    const line = doc.line(i)
    const match = line.text.match(TURN_MARKER_RE)
    if (match) {
      // Close the previous range
      if (ranges.length > 0) {
        ranges[ranges.length - 1].to = line.from - 1
      }
      ranges.push({
        id: match[1],
        role: match[2] as TurnRole,
        from: line.from,
        to: doc.length, // updated when next marker found
      })
    }
  }

  return ranges
}

// StateField that tracks turn ranges in the current document.
const turnRangesField = StateField.define<TurnRange[]>({
  create(state) {
    return parseTurnBoundaries(state.doc)
  },
  update(ranges, tr) {
    if (!tr.docChanged) return ranges
    return parseTurnBoundaries(tr.newDoc)
  },
})

// Build decorations for turn tinting and hiding boundary markers.
function buildDecorations(state: EditorState): DecorationSet {
  const ranges = state.field(turnRangesField)
  const builder = new RangeSetBuilder<Decoration>()

  for (const range of ranges) {
    const markerLine = state.doc.lineAt(range.from)
    // Replace marker line + its trailing newline so no blank gap appears
    const lineEnd = markerLine.to + 1 <= state.doc.length ? markerLine.to + 1 : markerLine.to
    builder.add(
      markerLine.from,
      lineEnd,
      Decoration.replace({}),
    )
    // Tint the entire turn range
    const contentFrom = markerLine.to + 1
    if (contentFrom < range.to) {
      builder.add(
        contentFrom,
        range.to,
        Decoration.mark({ class: `cm-turn-${range.role}` }),
      )
    }
  }

  return builder.finish()
}

// StateField for decorations.
const turnDecorationsField = StateField.define<DecorationSet>({
  create(state) {
    return buildDecorations(state)
  },
  update(deco, tr) {
    if (!tr.docChanged) return deco
    return buildDecorations(tr.state)
  },
  provide: (f) => EditorView.decorations.from(f),
})

// Transaction filter: reject edits to completed (non-streaming) turns.
// Agent streaming bypasses this via streamingAnnotation.
function makeReadOnlyFilter() {
  return EditorState.transactionFilter.of((tr: Transaction) => {
    if (!tr.docChanged) return tr
    if (tr.annotation(streamingAnnotation)) return tr

    const ranges = tr.startState.field(turnRangesField)
    const lastRange = ranges[ranges.length - 1]
    if (!lastRange) return tr

    // Allow edits only in the last turn's range (current user input)
    let blocked = false
    tr.changes.iterChanges((fromA) => {
      if (fromA < lastRange.from) blocked = true
    })

    return blocked ? [] : tr
  })
}

// CSS for turn tinting — injected via EditorView.theme.
const turnTheme = EditorView.theme({
  '.cm-turn-user': { backgroundColor: 'hsl(var(--color-muted) / 0.4)' },
  '.cm-turn-agent': { backgroundColor: 'hsl(var(--color-accent) / 0.15)' },
})

// streamingTurnId is reserved for future use — bypass is handled globally via streamingAnnotation
export function turnBoundaries(_streamingTurnId: string | null = null) {
  return [
    turnRangesField,
    turnDecorationsField,
    makeReadOnlyFilter(),
    turnTheme,
  ]
}

// Selector to get current turn ranges from state.
export function getTurnRanges(state: EditorState): TurnRange[] {
  return state.field(turnRangesField)
}
