import { Annotation, EditorState, StateField } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView, WidgetType } from '@codemirror/view'
import { streamingAnnotation } from './turn-boundaries'

class BlinkingCursorWidget extends WidgetType {
  toDOM() {
    const span = document.createElement('span')
    span.className = 'cm-streaming-cursor'
    return span
  }
  eq() { return true }
}

const blinkingCursorDecoration = Decoration.widget({
  widget: new BlinkingCursorWidget(),
  side: 1,
})

export const streamingDoneAnnotation = Annotation.define<boolean>()

const cursorActiveField = StateField.define<boolean>({
  create: () => false,
  update(active, tr) {
    if (tr.annotation(streamingAnnotation)) return true
    if (tr.annotation(streamingDoneAnnotation)) return false
    return active
  },
})

function buildCursorDeco(state: EditorState): DecorationSet {
  if (!state.field(cursorActiveField)) return Decoration.none
  const pos = state.doc.length
  return Decoration.set([blinkingCursorDecoration.range(pos)])
}

const cursorDecoField = StateField.define<DecorationSet>({
  create: (state) => buildCursorDeco(state),
  update(_deco, tr) {
    return buildCursorDeco(tr.state)
  },
  provide: (f) => EditorView.decorations.from(f),
})

const streamingTheme = EditorView.theme({
  '.cm-streaming-cursor': {
    display: 'inline-block',
    width: '2px',
    height: '1em',
    backgroundColor: 'var(--foreground)',
    verticalAlign: 'text-bottom',
    animation: 'cm-blink 1s step-start infinite',
  },
  '@keyframes cm-blink': {
    '0%, 100%': { opacity: '1' },
    '50%': { opacity: '0' },
  },
})

export function appendStreamChunk(view: EditorView, text: string): void {
  const pos = view.state.doc.length
  view.dispatch({
    changes: { from: pos, insert: text },
    annotations: streamingAnnotation.of(true),
    // Keep the latest streamed content in view as the agent writes.
    effects: EditorView.scrollIntoView(pos + text.length, { y: 'end' }),
  })
}

export function finalizeStreaming(view: EditorView): void {
  view.dispatch({ annotations: streamingDoneAnnotation.of(true) })
}

export function hasBlinkingCursor(state: EditorState): boolean {
  return state.field(cursorActiveField)
}

export function streamingExt() {
  return [cursorActiveField, cursorDecoField, streamingTheme]
}
