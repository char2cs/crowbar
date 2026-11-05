import { describe, expect, it, vi } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import type { Value } from 'platejs'
import {
  insertAttachmentMarkdownInto,
  insertPendingImageInto,
  settlePendingImageInto,
} from '@/features/agent/composer/plate/chat-attachment-insert'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'

/** A headless editor built from the SAME plugin set the real component uses —
 *  the `applyStreamedValue` tests' own pattern for exercising Plate transforms
 *  without a mounted DOM. */
function editorWith(markdown: string, autoSelect?: 'start' | 'end'): PlateEditor {
  return createPlateEditor({
    plugins: chatComposerPlugins,
    value: chatMarkdownToValue(markdown),
    ...(autoSelect ? { autoSelect } : {}),
  })
}

function codeBlockNode(editor: PlateEditor) {
  return editor.children.find((node) => (node as { type?: string }).type === 'code_block') as
    { type: string; lang?: string; children: { children: { text: string }[] }[] } | undefined
}

describe('insertAttachmentMarkdownInto', () => {
  it('inserts markdown deserialized through the codec, producing the expected code_block node shape', () => {
    const editor = editorWith('hello', 'end')

    insertAttachmentMarkdownInto(editor, '```text-attachment:abc123\nhello world\n```')

    const inserted = codeBlockNode(editor)
    expect(inserted?.lang).toBe('text-attachment:abc123')
    expect(
      inserted?.children.map((line) => line.children.map((leaf) => leaf.text).join('')),
    ).toEqual(['hello world'])
  })

  // The fallback branch: a caller (a modal, a drop handler) can invoke the
  // handle before the box has ever taken a selection of its own.
  it('inserts at the end of the document when the editor has no selection', () => {
    const editor = editorWith('hello')
    expect(editor.selection).toBeNull()

    insertAttachmentMarkdownInto(editor, '```text-attachment:def456\nmore\n```')

    expect(codeBlockNode(editor)?.lang).toBe('text-attachment:def456')
  })

  // Two inserts in a row once landed the second inside the first fence's
  // code_line, dropping its content (the excalidraw fence-then-image save).
  it('TestRegression_insertsSecondAttachmentAsASiblingNotInsideTheFirstFence', () => {
    const editor = editorWith('', 'end')

    insertAttachmentMarkdownInto(editor, '```excalidraw:abc\n{"elements":[]}\n```')
    insertAttachmentMarkdownInto(editor, '![diagram](chats/c1/attachments/x-diagram.png)')

    const [fence, image] = editor.children as {
      type?: string
      url?: string
      children?: { type?: string; children?: { text?: string }[] }[]
    }[]
    expect(fence.type).toBe('code_block')
    expect(fence.children?.every((line) => line.type === 'code_line')).toBe(true)
    expect(image.type).toBe('img')
    expect(image.url).toBe('chats/c1/attachments/x-diagram.png')
    expect(chatValueToMarkdown(editor.children as Value)).toBe(
      '```excalidraw:abc\n{"elements":[]}\n```\n\n![diagram](chats/c1/attachments/x-diagram.png)',
    )
  })

  it('TestRegression_insertsTwoFencedAttachmentsAsSeparateSiblingBlocks', () => {
    const editor = editorWith('', 'end')

    insertAttachmentMarkdownInto(editor, '```text-attachment:aaa\nfirst\n```')
    insertAttachmentMarkdownInto(editor, '```text-attachment:bbb\nsecond\n```')

    const fences = (editor.children as { type?: string; lang?: string }[]).filter(
      (node) => node.type === 'code_block',
    )
    expect(fences.map((f) => f.lang)).toEqual(['text-attachment:aaa', 'text-attachment:bbb'])
    expect(chatValueToMarkdown(editor.children as Value)).toBe(
      '```text-attachment:aaa\nfirst\n```\n\n```text-attachment:bbb\nsecond\n```',
    )
  })

  // A blanket `mode: 'highest'` would split a table when the caret sits in a
  // cell; only a code_block ancestor is special-cased.
  it('TestRegression_singleInsertWithCaretInATableCellDoesNotSplitTheTable', () => {
    const editor = editorWith('', 'end')
    insertAttachmentMarkdownInto(editor, '| a | b |\n| --- | --- |\n| 1 | 2 |')
    const table = (editor.children as { type?: string }[]).find((n) => n.type === 'table') as {
      children: { type: string; children: { type: string; children: unknown[] }[] }[]
    }
    const firstCellParagraphPath = [
      (editor.children as unknown[]).indexOf(table),
      0, // header row
      0, // first th
      0, // its paragraph
    ]
    editor.tf.select({
      anchor: { path: [...firstCellParagraphPath, 0], offset: 1 },
      focus: { path: [...firstCellParagraphPath, 0], offset: 1 },
    })

    insertAttachmentMarkdownInto(editor, '![diagram](chats/c1/attachments/x-diagram.png)')

    const tablesAfter = (editor.children as { type?: string; children?: unknown[] }[]).filter(
      (n) => n.type === 'table',
    )
    expect(tablesAfter).toHaveLength(1) // still ONE table, not split in two
    expect(tablesAfter[0]?.children).toHaveLength(2) // header row + body row, both intact
    expect(chatValueToMarkdown(editor.children as Value)).toContain(
      '![diagram](chats/c1/attachments/x-diagram.png)',
    )
  })

  // After an attachment there must be a line to keep typing on without clicking below it.
  it('lands the caret on a fresh empty line right after inserting a single attachment', () => {
    const editor = editorWith('', 'end')

    insertAttachmentMarkdownInto(editor, '```text-attachment:abc123\nhello world\n```')

    const last = editor.children.at(-1) as { type?: string; children?: { text?: string }[] }
    expect(last.type).toBe('p')
    expect(last.children?.map((c) => c.text).join('')).toBe('')
    expect(editor.selection).not.toBeNull()
    expect(editor.api.end([])).toEqual(editor.selection?.focus)
  })

  // The multi-file-drop/paste case: each upload resolves independently and
  // calls this separately — the SECOND file's insert must not stack a new
  // blank paragraph on top of the one the FIRST file's insert just made.
  it('reuses the same trailing empty line for a second attachment inserted right after the first', () => {
    const editor = editorWith('', 'end')

    insertAttachmentMarkdownInto(editor, '```text-attachment:aaa\nfirst\n```')
    insertAttachmentMarkdownInto(editor, '```text-attachment:bbb\nsecond\n```')

    const emptyParagraphs = (
      editor.children as { type?: string; children?: { text?: string }[] }[]
    ).filter((n) => n.type === 'p' && n.children?.every((c) => c.text === ''))
    expect(emptyParagraphs).toHaveLength(1)
    expect(chatValueToMarkdown(editor.children as Value)).toBe(
      '```text-attachment:aaa\nfirst\n```\n\n```text-attachment:bbb\nsecond\n```',
    )
  })
})

// Photos preview at once from a local object URL; the real ref replaces it
// when the upload settles, or the placeholder goes on failure.
describe('insertPendingImageInto / settlePendingImageInto', () => {
  it('inserts an image node pointing at the local object URL immediately', () => {
    const editor = editorWith('', 'end')

    insertPendingImageInto(editor, 'blob:local-preview', 'photo.png')

    const [image] = editor.children as { type?: string; url?: string }[]
    expect(image.type).toBe('img')
    expect(image.url).toBe('blob:local-preview')
  })

  it('replaces the placeholder url with the real one once the upload settles, in place', () => {
    const editor = editorWith('', 'end')
    insertPendingImageInto(editor, 'blob:local-preview', 'photo.png')

    settlePendingImageInto(
      editor,
      'blob:local-preview',
      '![photo.png](chats/c1/attachments/photo.png)',
    )

    const [image] = editor.children as { type?: string; url?: string }[]
    expect(image.type).toBe('img')
    expect(image.url).toBe('chats/c1/attachments/photo.png')
  })

  // The person may have kept typing (or attached something else) while the
  // upload was in flight — the placeholder is found by its OWN url, not by
  // whatever position it happened to start at.
  it('finds the placeholder by its own url, not by position, if the document changed since', () => {
    const editor = editorWith('', 'end')
    insertPendingImageInto(editor, 'blob:first', 'first.png')
    insertAttachmentMarkdownInto(editor, '```text-attachment:aaa\nkeep typing\n```')
    insertPendingImageInto(editor, 'blob:second', 'second.png')

    settlePendingImageInto(editor, 'blob:first', '![first.png](chats/c1/attachments/first.png)')

    const images = (editor.children as { type?: string; url?: string }[]).filter(
      (n) => n.type === 'img',
    )
    expect(images).toMatchObject([
      { type: 'img', url: 'chats/c1/attachments/first.png' },
      { type: 'img', url: 'blob:second' },
    ])
  })

  it('removes the placeholder when the upload failed instead of leaving a broken image behind', () => {
    const editor = editorWith('', 'end')
    insertPendingImageInto(editor, 'blob:local-preview', 'photo.png')

    settlePendingImageInto(editor, 'blob:local-preview', null)

    const images = (editor.children as { type?: string }[]).filter((n) => n.type === 'img')
    expect(images).toHaveLength(0)
  })

  it('revokes the object URL once settled, win or lose', () => {
    const revoke = vi.spyOn(URL, 'revokeObjectURL')
    const editor = editorWith('', 'end')
    insertPendingImageInto(editor, 'blob:local-preview', 'photo.png')

    settlePendingImageInto(
      editor,
      'blob:local-preview',
      '![photo.png](chats/c1/attachments/photo.png)',
    )

    expect(revoke).toHaveBeenCalledWith('blob:local-preview')
    revoke.mockRestore()
  })

  it('is a safe no-op when the placeholder is no longer in the document at all', () => {
    const editor = editorWith('', 'end')

    expect(() =>
      settlePendingImageInto(editor, 'blob:never-inserted', '![x](chats/c1/attachments/x.png)'),
    ).not.toThrow()
  })

  // The server's sniffed type can disagree with the browser's guess (HEIC, a
  // corrupt image): a non-image result replaces the placeholder, never leaving a
  // void `img` on a revoked blob url that blocks Send forever.
  it('replaces the placeholder outright when the upload resolves to a non-image attachment', () => {
    const editor = editorWith('', 'end')
    insertPendingImageInto(editor, 'blob:local-preview', 'photo.heic')

    settlePendingImageInto(
      editor,
      'blob:local-preview',
      '[photo.heic](chats/c1/attachments/photo.heic)',
    )

    const images = (editor.children as { type?: string }[]).filter((n) => n.type === 'img')
    expect(images).toHaveLength(0)
    const markdown = chatValueToMarkdown(editor.children as never)
    expect(markdown).toContain('[photo.heic](chats/c1/attachments/photo.heic)')
    expect(markdown).not.toContain('blob:')
  })
})
