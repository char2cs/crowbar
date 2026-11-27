import { BlockSelectionPlugin } from '@platejs/selection/react'
import { createPlateEditor } from 'platejs/react'
import { describe, expect, it } from 'vitest'
import { markdownPlugins } from '@/features/editor/markdown/plate/markdown-plugins'
import { parseMarkdown } from '@/features/editor/markdown/plate/markdown-serialization'

const DOC = ['# Title', '', 'First paragraph.', '', 'Second paragraph.'].join('\n')

const editorWith = (md: string) => {
  const editor = createPlateEditor({ plugins: markdownPlugins, value: parseMarkdown(md).value })
  return editor
}

describe('markdown editor select-all', () => {
  it('selects the whole document as a range', () => {
    const editor = editorWith(DOC)
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
    const editor = editorWith('')
    editor.tf.select({ anchor: { path: [0, 0], offset: 0 }, focus: { path: [0, 0], offset: 0 } })

    expect(editor.tf.selectAll()).toBe(true)
    expect(editor.selection).not.toBeNull()
  })

  it('reaches a document that ends in a void block', () => {
    const editor = editorWith('Only paragraph.\n\n---\n')
    editor.tf.select({ anchor: { path: [0, 0], offset: 0 }, focus: { path: [0, 0], offset: 0 } })

    expect(editor.tf.selectAll()).toBe(true)
    expect(editor.selection!.focus.path[0]).toBe(editor.children.length - 1)
  })

  it('scopes to the code block when the caret is inside one', () => {
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
