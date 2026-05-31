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

// Sentinel line that marks where user input starts. Everything before it is read-only.
export const INPUT_MARKER = '<!-- input -->'

// Annotation used by streaming-ext and insertTurnAndStartStreaming to bypass the read-only filter.
export const streamingAnnotation = Annotation.define<boolean>()

// Convert turns array to a CM6 document string ending with the user input area.
export function turnsToDocument(turns: MarkdownTurn[]): string {
  if (turns.length === 0) return INPUT_MARKER + '\n'
  return (
    turns
      .map((t) => `<!-- turn:${t.id} role:${t.role} -->\n${t.content}`)
      .join('\n\n') +
    '\n\n' +
    INPUT_MARKER +
    '\n'
  )
}

// Parse turn boundary markers. Stops at INPUT_MARKER so all ranges are fully read-only.
export function parseTurnBoundaries(doc: Text): TurnRange[] {
  const ranges: TurnRange[] = []

  for (let i = 1; i <= doc.lines; i++) {
    const line = doc.line(i)

    if (line.text === INPUT_MARKER) {
      if (ranges.length > 0) {
        // End the last range at the blank separator line before the input marker
        ranges[ranges.length - 1].to = i > 1 ? doc.line(i - 1).to : line.from
      }
      break
    }

    const match = line.text.match(TURN_MARKER_RE)
    if (match) {
      if (ranges.length > 0) {
        ranges[ranges.length - 1].to = line.from - 1
      }
      ranges.push({
        id: match[1],
        role: match[2] as TurnRole,
        from: line.from,
        to: doc.length,
      })
    }
  }

  return ranges
}

// Returns the position of the first character AFTER the <!-- input --> line.
// Everything at or after this position is the editable user input area.
export function getInputPos(doc: Text): number {
  for (let i = doc.lines; i >= 1; i--) {
    const line = doc.line(i)
    if (line.text === INPUT_MARKER) {
      return Math.min(line.to + 1, doc.length)
    }
  }
  return doc.length
}

// Replace the input area with user + agent turn markers, then let streaming fill in agent content.
// Uses streamingAnnotation to bypass the read-only filter.
export function insertTurnAndStartStreaming(
  view: EditorView,
  userId: string,
  userContent: string,
  agentId: string,
): void {
  const doc = view.state.doc
  let removeFrom = doc.length
  for (let i = doc.lines; i >= 1; i--) {
    const line = doc.line(i)
    if (line.text === INPUT_MARKER) {
      // Include the blank separator line before the marker in the removal range
      removeFrom = line.from > 0 ? line.from - 1 : line.from
      break
    }
  }

  const insert =
    `\n\n<!-- turn:${userId} role:user -->\n${userContent}` +
    `\n\n<!-- turn:${agentId} role:agent -->\n`

  view.dispatch({
    changes: { from: removeFrom, to: doc.length, insert },
    annotations: streamingAnnotation.of(true),
  })
}

// Appends the input marker back after streaming completes.
export function resetInputMarker(view: EditorView): void {
  view.dispatch({
    changes: { from: view.state.doc.length, insert: `\n\n${INPUT_MARKER}\n` },
    annotations: streamingAnnotation.of(true),
  })
}

const turnRangesField = StateField.define<TurnRange[]>({
  create(state) {
    return parseTurnBoundaries(state.doc)
  },
  update(ranges, tr) {
    if (!tr.docChanged) return ranges
    return parseTurnBoundaries(tr.newDoc)
  },
})

function buildDecorations(state: EditorState): DecorationSet {
  const ranges = state.field(turnRangesField)
  const builder = new RangeSetBuilder<Decoration>()

  for (const range of ranges) {
    const markerLine = state.doc.lineAt(range.from)
    const markerEnd = Math.min(markerLine.to + 1, state.doc.length)
    // Hide the turn boundary marker line (including its trailing newline)
    builder.add(markerLine.from, markerEnd, Decoration.replace({}))
    // Tint the turn content
    const contentFrom = markerLine.to + 1
    if (contentFrom < range.to) {
      builder.add(contentFrom, range.to, Decoration.mark({ class: `cm-turn-${range.role}` }))
    }
  }

  // Hide the input marker line (always at end of completed turns)
  for (let i = state.doc.lines; i >= 1; i--) {
    const line = state.doc.line(i)
    if (line.text === INPUT_MARKER) {
      const lineEnd = Math.min(line.to + 1, state.doc.length)
      builder.add(line.from, lineEnd, Decoration.replace({}))
      break
    }
  }

  return builder.finish()
}

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

// Only allow edits in the user input area (at/after the input marker position).
function makeReadOnlyFilter() {
  return EditorState.transactionFilter.of((tr: Transaction) => {
    if (!tr.docChanged) return tr
    if (tr.annotation(streamingAnnotation)) return tr

    const inputPos = getInputPos(tr.startState.doc)
    let blocked = false
    tr.changes.iterChanges((fromA) => {
      if (fromA < inputPos) blocked = true
    })
    return blocked ? [] : tr
  })
}

const turnTheme = EditorView.theme({
  // Subtle but distinct tinting using direct oklch values (the project uses oklch)
  '.cm-turn-user': { backgroundColor: 'oklch(0 0 0 / 2%)' },
  '.cm-turn-agent': { backgroundColor: 'oklch(0.55 0.12 250 / 7%)' },
})

export function turnBoundaries(_streamingTurnId: string | null = null) {
  return [
    turnRangesField,
    turnDecorationsField,
    makeReadOnlyFilter(),
    turnTheme,
  ]
}

export function getTurnRanges(state: EditorState): TurnRange[] {
  return state.field(turnRangesField)
}
