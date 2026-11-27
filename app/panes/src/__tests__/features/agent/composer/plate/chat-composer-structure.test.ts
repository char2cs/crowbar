import { describe, expect, it } from 'vitest'
import { NodeApi, type Value } from 'platejs'
import { createPlateEditor } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  chatComposerStructurePlugin,
  createDocumentTitlePlugin,
} from '@/features/agent/composer/plate/chat-composer-structure'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { insertAttachmentMarkdownInto } from '@/features/agent/composer/plate/chat-attachment-insert'

function composer(markdown = ''): PlateEditor {
  return createPlateEditor({
    plugins: [...chatComposerPlugins, chatComposerStructurePlugin],
    value: chatMarkdownToValue(markdown),
    autoSelect: 'end',
  })
}

const IMAGE = '![shot.png](crowbar-asset://a/b/shot.png)'
const SECOND_IMAGE = '![b.png](crowbar-asset://a/b/b.png)'
const TEXT_ATTACHMENT = '```text-attachment:abc123\nhi there\n```'
const SCENE = JSON.stringify({ type: 'excalidraw', elements: [], appState: {} })
const EXCALIDRAW = `\`\`\`excalidraw:def456\n${SCENE}\n\`\`\`\n\n![diagram](crowbar-asset://a/b/d.png)`
const FILE_CARD = '[notes.pdf](crowbar-asset://a/b/notes.pdf)'

const types = (editor: PlateEditor) => (editor.children as { type?: string }[]).map((n) => n.type)

function trashFirst(editor: PlateEditor, type: string) {
  const [entry] = editor.api.nodes({
    at: [],
    match: (n) => (n as { type?: string }).type === type,
  })
  if (entry) editor.tf.removeNodes({ at: entry[1] })
}

function expectReadyForInput(editor: PlateEditor) {
  expect(editor.children.length).toBeGreaterThan(0)
  expect(types(editor).at(-1)).toBe('p')
  editor.tf.insertText('typed')
  expect(chatValueToMarkdown(editor.children as Value)).toContain('typed')
}

describe('TestRegression_ComposerAttachmentDeletionCorruption', () => {
  it('leaves a typable box after the empty line, then the attachment, is deleted', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, IMAGE)
    expect(types(editor)).toEqual(['img', 'p'])

    editor.tf.deleteBackward('character')
    trashFirst(editor, 'img')

    expect(editor.children).toHaveLength(1)
    expectReadyForInput(editor)
  })

  it('leaves a typable box after the attachment, then the empty line, is deleted', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, IMAGE)
    trashFirst(editor, 'img')
    editor.tf.deleteBackward('character')

    expectReadyForInput(editor)
  })

  it('survives two attachments emptied in any order', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, IMAGE)
    insertAttachmentMarkdownInto(editor, SECOND_IMAGE)
    expect(types(editor)).toEqual(['img', 'img', 'p'])

    editor.tf.deleteBackward('character')
    trashFirst(editor, 'img')
    trashFirst(editor, 'img')

    expectReadyForInput(editor)
  })

  it('survives backspace held down past the last attachment', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, IMAGE)
    for (let i = 0; i < 5; i++) editor.tf.deleteBackward('character')

    expectReadyForInput(editor)
  })

  for (const [kind, markdown, nodeType] of [
    ['an image', IMAGE, 'img'],
    ['a text attachment', TEXT_ATTACHMENT, 'code_block'],
    ['an excalidraw diagram', EXCALIDRAW, 'code_block'],
    ['a file card', FILE_CARD, 'p'],
  ] as const) {
    it(`survives emptying the box of ${kind}`, () => {
      const editor = composer()
      insertAttachmentMarkdownInto(editor, markdown)

      editor.tf.deleteBackward('character')
      trashFirst(editor, nodeType)

      expectReadyForInput(editor)
    })
  }

  it('survives the last node being removed programmatically', () => {
    const editor = composer()
    editor.tf.removeNodes({ at: [0] })

    expect(editor.children).toHaveLength(1)
    expect(types(editor)).toEqual(['p'])
  })

  it('survives select-all then delete with an attachment in the box', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, IMAGE)
    editor.tf.select({ anchor: editor.api.start([])!, focus: editor.api.end([])! })
    editor.tf.deleteFragment()

    expectReadyForInput(editor)
  })

  it('never types into an attachment fence after backspacing next to it', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, TEXT_ATTACHMENT)
    editor.tf.deleteBackward('character')
    editor.tf.insertText('typed')

    expect(chatValueToMarkdown(editor.children as Value)).not.toContain('hi theretyped')
  })

  it('takes an excalidraw fence and its png together', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, EXCALIDRAW)
    expect(types(editor)).toEqual(['code_block', 'img', 'p'])

    editor.tf.deleteBackward('character')

    expect(types(editor)).toEqual(['p'])
    expect(chatValueToMarkdown(editor.children as Value)).toBe('')
  })

  it('opens a seeded draft that ends in an attachment with somewhere to type', () => {
    const editor = composer(`look at this\n\n${IMAGE}`)

    expect(types(editor).at(-1)).toBe('p')
    expect(chatValueToMarkdown(editor.children as Value)).toBe(`look at this\n\n${IMAGE}`)
  })
})

describe('the structural floor leaves ordinary editing alone', () => {
  it('still merges a line into the paragraph above it', () => {
    const editor = composer('one\n\ntwo')
    editor.tf.select(editor.api.start([1])!)
    editor.tf.deleteBackward('character')

    expect(chatValueToMarkdown(editor.children as Value)).toBe('onetwo')
  })

  it('still backspaces from an empty line into the heading above it', () => {
    const editor = composer('# Title')
    editor.tf.insertNodes({ type: 'p', children: [{ text: '' }] }, { at: [1] })
    editor.tf.select(editor.api.start([1])!)
    editor.tf.deleteBackward('character')
    editor.tf.insertText('!')

    expect(chatValueToMarkdown(editor.children as Value)).toBe('# Title!')
  })

  it('still merges into a plain code block', () => {
    const editor = composer('```js\nlet a = 1\n```\n\ntail')
    editor.tf.select(editor.api.start([1])!)
    editor.tf.deleteBackward('character')

    expect(types(editor)[0]).toBe('code_block')
    expect(NodeApi.string(editor.children[0])).toBe('let a = 1tail')
  })

  it('still backspaces inside a word', () => {
    const editor = composer('hello')
    editor.tf.deleteBackward('character')

    expect(chatValueToMarkdown(editor.children as Value)).toBe('hell')
  })

  it('leaves a caret at the start of a table cell alone', () => {
    const table = '| a | b |\n| - | - |\n| 1 | 2 |'
    const backspaceInSecondCell = (editor: PlateEditor) => {
      const cells = Array.from(
        editor.api.nodes({ at: [], match: (n) => (n as { type?: string }).type === 'td' }),
      )
      editor.tf.select(editor.api.start(cells[1][1])!)
      editor.tf.deleteBackward('character')
      return chatValueToMarkdown(editor.children as Value)
    }
    const bare = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(table),
      autoSelect: 'end',
    })

    expect(backspaceInSecondCell(composer(table))).toBe(backspaceInSecondCell(bare))
  })

  it('never changes the markdown a prompt is sent as', () => {
    for (const markdown of [
      'hello',
      '# Title',
      '- one\n- two',
      '> quoted',
      '| a | b |\n| - | - |\n| 1 | 2 |',
      TEXT_ATTACHMENT,
      EXCALIDRAW,
      `hello\n\n${IMAGE}`,
    ]) {
      expect(chatValueToMarkdown(composer(markdown).children as Value)).toBe(markdown)
    }
  })
})

describe('Tab in the composer', () => {
  it('leaves a plain line alone, both ways, so focus can move on', () => {
    const editor = composer('hello')
    expect(editor.tf.tab({ reverse: false })).toBeFalsy()
    expect(editor.tf.tab({ reverse: true })).toBeFalsy()
    expect(chatValueToMarkdown(editor.children as Value)).toBe('hello')
  })

  it('still nests a list item', () => {
    const editor = composer('- one\n- two')
    expect(editor.tf.tab({ reverse: false })).toBe(true)
    expect(chatValueToMarkdown(editor.children as Value)).toMatch(/- one\n\s+- two/)
  })

  describe('a box with nothing in it', () => {
    it('is one empty line however the blank lines got there', () => {
      const editor = composer()
      editor.tf.insertBreak()
      editor.tf.insertBreak()
      expect(editor.children).toHaveLength(1)
      expect(NodeApi.string(editor.children[0]!)).toBe('')
    })

    it('holds no text at all when only soft breaks or spaces were typed', () => {
      const editor = composer()
      editor.tf.insertText('\n\n \n')
      expect(editor.children).toHaveLength(1)
      expect(NodeApi.string(editor.children[0]!)).toBe('')
    })

    it('keeps blank lines that sit between text', () => {
      const editor = composer('a')
      editor.tf.insertBreak()
      editor.tf.insertBreak()
      expect(editor.children.length).toBeGreaterThan(1)
    })
  })
})

describe('the blank chat document title', () => {
  function document(markdown = '', isDocument = true): PlateEditor {
    return createPlateEditor({
      plugins: [
        ...chatComposerPlugins,
        chatComposerStructurePlugin,
        createDocumentTitlePlugin(() => isDocument),
      ],
      value: chatMarkdownToValue(markdown),
      autoSelect: 'end',
    })
  }

  it('makes the first typed line a title and keeps it one while typing on', () => {
    const editor = document()
    editor.tf.insertText('F')
    expect(types(editor)[0]).toBe('h1')
    editor.tf.insertText('ix it')
    expect(types(editor)[0]).toBe('h1')
    expect(chatValueToMarkdown(editor.children as Value)).toBe('# Fix it')
  })

  it('starts body text after a break in the title', () => {
    const editor = document()
    editor.tf.insertText('Fix it')
    editor.tf.insertBreak()
    editor.tf.insertText('details')
    expect(types(editor)).toEqual(['h1', 'p'])
    expect(chatValueToMarkdown(editor.children as Value)).toBe('# Fix it\n\ndetails')
  })

  it('is one empty line again when the title is deleted', () => {
    const editor = document()
    editor.tf.insertText('a')
    editor.tf.deleteBackward('character')
    expect(types(editor)).toEqual(['p'])
    expect(NodeApi.string(editor.children[0]!)).toBe('')
  })

  it('leaves a leading slash command as a plain line', () => {
    const editor = document()
    editor.tf.insertText('/compact')
    expect(types(editor)[0]).toBe('p')
  })

  it('does nothing outside the document', () => {
    const editor = document('', false)
    editor.tf.insertText('hello')
    expect(types(editor)[0]).toBe('p')
  })
})
