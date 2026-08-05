import { BlockSelectionPlugin } from '@platejs/selection/react'
import { createPlateEditor } from 'platejs/react'
import { describe, expect, it } from 'vitest'
import { markdownPlugins } from '@/features/editor/markdown/plate/markdown-plugins'
import { markdownToPlateValue } from '@/features/editor/markdown/plate/markdown-serialization'

/**
 * Cmd/Ctrl+A must produce a TEXT selection over the whole document, not a
 * Plate block selection.
 *
 * Block selection routes every key through a hidden input in `document.body`
 * whose keymap covers Escape, Enter, Backspace, up and down — and NOT left or
 * right. A document selected that way could not be collapsed with an arrow
 * key: pressing one did nothing at all. A plain range keeps arrows,
 * shift+arrows, typing-over and copy on the caret's own behaviour, which is
 * what Obsidian does.
 *
 * `SlateReactExtensionPlugin` routes the keystroke to `editor.tf.selectAll()`
 * and only calls `preventDefault()` on a truthy return, so these assert the
 * transform directly — that is the whole contract the keydown handler has.
 */

const DOC = ['# Title', '', 'First paragraph.', '', 'Second paragraph.'].join('\n')

const editorWith = (md: string) => {
  const editor = createPlateEditor({ plugins: markdownPlugins, value: markdownToPlateValue(md) })
  return editor
}

describe('markdown editor select-all', () => {
  it('selects the whole document as a range', () => {
    const editor = editorWith(DOC)
    // Caret parked mid-document: upstream's default would have selected only
    // this block on the first press.
    editor.tf.select({ anchor: { path: [1, 0], offset: 3 }, focus: { path: [1, 0], offset: 3 } })

    expect(editor.tf.selectAll()).toBe(true)

    const selection = editor.selection
    expect(selection).not.toBeNull()
    expect(editor.api.isCollapsed()).toBe(false)
    expect(editor.api.string(selection!)).toContain('Title')
    expect(editor.api.string(selection!)).toContain('Second paragraph.')
  })

  it('leaves block selection empty', () => {
    const editor = editorWith(DOC)
    editor.tf.select({ anchor: { path: [1, 0], offset: 0 }, focus: { path: [1, 0], offset: 0 } })

    editor.tf.selectAll()

    // Non-empty selectedIds is the state whose keymap swallows left/right.
    expect(editor.getOption(BlockSelectionPlugin, 'selectedIds')?.size ?? 0).toBe(0)
  })

  it('is idempotent — a second press does not escalate to block selection', () => {
    const editor = editorWith(DOC)
    editor.tf.select({ anchor: { path: [1, 0], offset: 0 }, focus: { path: [1, 0], offset: 0 } })

    editor.tf.selectAll()
    const first = editor.selection
    expect(editor.tf.selectAll()).toBe(true)

    expect(editor.selection).toEqual(first)
    expect(editor.getOption(BlockSelectionPlugin, 'selectedIds')?.size ?? 0).toBe(0)
  })

  it('survives an empty document', () => {
    // `editor.tf.select([])` resolves a path to that node's full range; on an
    // empty document that range is collapsed, which must not throw or leave a
    // null selection behind.
    const editor = editorWith('')
    editor.tf.select({ anchor: { path: [0, 0], offset: 0 }, focus: { path: [0, 0], offset: 0 } })

    expect(editor.tf.selectAll()).toBe(true)
    expect(editor.selection).not.toBeNull()
  })

  it('reaches a document that ends in a void block', () => {
    // A trailing `hr` has no text of its own; the document range still has to
    // end past it rather than at the last paragraph.
    const editor = editorWith('Only paragraph.\n\n---\n')
    editor.tf.select({ anchor: { path: [0, 0], offset: 0 }, focus: { path: [0, 0], offset: 0 } })

    expect(editor.tf.selectAll()).toBe(true)
    expect(editor.selection!.focus.path[0]).toBe(editor.children.length - 1)
  })

  it('scopes to the code block when the caret is inside one', () => {
    // CodeBlockPlugin claims selectAll first; Obsidian scopes to the fence too.
    const editor = editorWith('# Title\n\n```ts\nconst a = 1\nconst b = 2\n```\n')
    const codeBlock = editor.children.findIndex((node) => node.type === 'code_block')
    expect(codeBlock).toBeGreaterThan(-1)
    editor.tf.select({
      anchor: { path: [codeBlock, 0, 0], offset: 2 },
      focus: { path: [codeBlock, 0, 0], offset: 2 },
    })

    expect(editor.tf.selectAll()).toBe(true)

    const text = editor.api.string(editor.selection!)
    expect(text).toContain('const a = 1')
    expect(text).not.toContain('Title')
  })

  it('scopes to the table when the caret is inside a cell', () => {
    // TablePlugin claims selectAll before this plugin and takes the whole
    // TABLE, not the one cell. Pinned because the ordering comment in
    // block-selection-kit.tsx asserts it.
    const editor = editorWith('# Title\n\n| a | b |\n| --- | --- |\n| one | two |\n')
    const table = editor.children.findIndex((node) => node.type === 'table')
    expect(table).toBeGreaterThan(-1)
    const at = { path: [table, 1, 0, 0, 0], offset: 1 }
    editor.tf.select({ anchor: at, focus: at })

    expect(editor.tf.selectAll()).toBe(true)

    const text = editor.api.string(editor.selection!)
    expect(text).toContain('one')
    expect(text).toContain('two')
    expect(text).not.toContain('Title')
  })
})
