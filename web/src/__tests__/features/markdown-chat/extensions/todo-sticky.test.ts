import { EditorState } from '@codemirror/state'
import { markdown } from '@codemirror/lang-markdown'
import { todoStickyExt, findTodoBlockRange } from '@/features/markdown-chat/extensions/todo-sticky'
import { describe, test, expect } from 'vitest'

function makeState(content: string) {
  return EditorState.create({
    doc: content,
    extensions: [markdown(), todoStickyExt()],
  })
}

describe('todo-sticky', () => {
  test('findTodoBlockRange returns null when no checklist present', () => {
    const state = makeState('Just some text\nNo checklist here')
    expect(findTodoBlockRange(state)).toBeNull()
  })

  test('findTodoBlockRange returns range when checklist is present', () => {
    const state = makeState('- [ ] Task one\n- [x] Task two\n- [ ] Task three')
    const range = findTodoBlockRange(state)
    expect(range).not.toBeNull()
    expect(range!.from).toBe(0)
    expect(range!.to).toBeGreaterThan(0)
  })

  test('findTodoBlockRange finds checklist in middle of content', () => {
    const state = makeState('Preamble text\n\n- [ ] Task one\n- [x] Task two\n\nTrailing text')
    const range = findTodoBlockRange(state)
    expect(range).not.toBeNull()
    // Range should start at the first checklist item
    const line = state.doc.lineAt(range!.from)
    expect(line.text.trim()).toBe('- [ ] Task one')
  })
})
