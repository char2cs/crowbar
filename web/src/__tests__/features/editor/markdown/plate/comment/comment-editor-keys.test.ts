import { describe, expect, it } from 'vitest'
import { commentEditorKeyAction } from '@/features/editor/markdown/plate/comment/comment-editor-keys'

const key = (key: string, mods: { metaKey?: boolean; ctrlKey?: boolean } = {}) => ({
  key,
  metaKey: false,
  ctrlKey: false,
  ...mods,
})

const closed = { isLinkEditorOpen: false }
const linkOpen = { isLinkEditorOpen: true }

describe('commentEditorKeyAction', () => {
  it('posts on Cmd+Enter and on Ctrl+Enter', () => {
    expect(commentEditorKeyAction(key('Enter', { metaKey: true }), closed)).toBe('submit')
    expect(commentEditorKeyAction(key('Enter', { ctrlKey: true }), closed)).toBe('submit')
  })

  it('leaves a bare Enter alone — in a rich editor that is a new paragraph', () => {
    expect(commentEditorKeyAction(key('Enter'), closed)).toBeNull()
  })

  it('leaves Shift+Enter alone', () => {
    expect(commentEditorKeyAction({ ...key('Enter'), metaKey: false }, closed)).toBeNull()
  })

  it('cancels the draft on Escape', () => {
    expect(commentEditorKeyAction(key('Escape'), closed)).toBe('cancel')
  })

  it('yields Escape to the link editor rather than discarding the draft', () => {
    // Backing out of a link dialog must not throw away everything typed so far.
    expect(commentEditorKeyAction(key('Escape'), linkOpen)).toBeNull()
  })

  it('still posts on Cmd+Enter while the link editor is open', () => {
    expect(commentEditorKeyAction(key('Enter', { metaKey: true }), linkOpen)).toBe('submit')
  })

  it('ignores ordinary typing', () => {
    for (const k of ['a', '/', 'Backspace', 'Tab', 'ArrowUp']) {
      expect(commentEditorKeyAction(key(k), closed)).toBeNull()
    }
  })
})
