import { EditorState, StateField } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView } from '@codemirror/view'

const TODO_ITEM_RE = /^- \[[ x]\] /

export interface TodoRange {
  from: number
  to: number
}

export function findTodoBlockRange(state: EditorState): TodoRange | null {
  let from: number | null = null
  let to: number | null = null

  for (let i = 1; i <= state.doc.lines; i++) {
    const line = state.doc.line(i)
    if (TODO_ITEM_RE.test(line.text)) {
      if (from === null) from = line.from
      to = line.to
    } else if (from !== null && line.text.trim() === '') {
      // allow a blank line — handled by continuing
      continue
    } else if (from !== null) {
      break
    }
  }

  return from !== null && to !== null ? { from, to } : null
}

// The sticky class is applied externally based on streaming state.
// This extension only provides the decoration infrastructure.
const stickyDecoField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(_deco, _tr) {
    return Decoration.none
  },
  provide: (f) => EditorView.decorations.from(f),
})

export function todoStickyExt() {
  return [stickyDecoField]
}
