import { EditorState } from '@codemirror/state'
import { markdown } from '@codemirror/lang-markdown'
import {
  livePreview,
  hasLivePreviewDecoration,
} from '@/features/markdown-chat/extensions/live-preview'

function makeState(content: string) {
  return EditorState.create({
    doc: content,
    extensions: [markdown(), livePreview()],
  })
}

test('heading line gets heading decoration when cursor is not on it', () => {
  const state = makeState('# Hello World\nSome text')
  // cursor on line 2 (position 15), not on the heading
  const stateWithCursor = state.update({ selection: { anchor: 15 } }).state
  // "# " prefix is hidden (replaced); heading class starts at pos 2 ("H" of "Hello")
  expect(hasLivePreviewDecoration(stateWithCursor, 2, 'cm-live-heading-1')).toBe(true)
})

test('heading line does not get decoration when cursor is on it', () => {
  const state = makeState('# Hello World\nSome text')
  // cursor on position 2 (inside the heading line)
  const stateWithCursor = state.update({ selection: { anchor: 2 } }).state
  expect(hasLivePreviewDecoration(stateWithCursor, 0, 'cm-live-heading-1')).toBe(false)
})

test('bold syntax gets decoration outside cursor line', () => {
  const state = makeState('Some **bold** text\nAnother line')
  const stateWithCursor = state.update({ selection: { anchor: 20 } }).state
  // "**" markers are hidden (replaced); bold class applies to the inner content at pos 7 ("b" of "bold")
  expect(hasLivePreviewDecoration(stateWithCursor, 7, 'cm-live-bold')).toBe(true)
})
