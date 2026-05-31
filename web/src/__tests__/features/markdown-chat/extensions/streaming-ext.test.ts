import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { streamingExt, appendStreamChunk, hasBlinkingCursor } from '@/features/markdown-chat/extensions/streaming-ext'

function makeView(content: string) {
  const state = EditorState.create({
    doc: content,
    extensions: [streamingExt()],
  })
  // EditorView requires a DOM node — use a detached div in jsdom
  const dom = document.createElement('div')
  return new EditorView({ state, parent: dom })
}

test('appendStreamChunk appends text at end of document', () => {
  const view = makeView('Hello')
  appendStreamChunk(view, ' world')
  expect(view.state.doc.toString()).toBe('Hello world')
  view.destroy()
})

test('appendStreamChunk can be called multiple times', () => {
  const view = makeView('')
  appendStreamChunk(view, 'a')
  appendStreamChunk(view, 'b')
  appendStreamChunk(view, 'c')
  expect(view.state.doc.toString()).toBe('abc')
  view.destroy()
})

test('hasBlinkingCursor returns true after appendStreamChunk', () => {
  const view = makeView('Hello')
  appendStreamChunk(view, ' world')
  expect(hasBlinkingCursor(view.state)).toBe(true)
  view.destroy()
})
