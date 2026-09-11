import { describe, expect, it } from 'vitest'
import { NodeApi, type Value } from 'platejs'
import { createPlateEditor } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatComposerStructurePlugin } from '@/features/agent/composer/plate/chat-composer-structure'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { insertAttachmentMarkdownInto } from '@/features/agent/composer/plate/chat-markdown-editor'

/** The composer's own editor, plugin-for-plugin — `chat-markdown-editor.tsx`
 *  adds `chatComposerStructurePlugin` to `chatComposerPlugins` for the box
 *  only, never for the transcript's streaming bubble, which renders through
 *  the bare set. Headless, the same way chat-markdown-editor.test.tsx proves
 *  its own transforms: a mounted contenteditable cannot deliver a real
 *  keystroke through Slate's selection-dependent dispatch under jsdom. */
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

/** The trash button on every attachment kind ends in the same call — see
 *  `useAttachmentDraggable`'s `remove` (attachment-drag-handle.tsx). */
function trashFirst(editor: PlateEditor, type: string) {
  const [entry] = editor.api.nodes({
    at: [],
    match: (n) => (n as { type?: string }).type === type,
  })
  if (entry) editor.tf.removeNodes({ at: entry[1] })
}

/** What the person is left with: a box that renders, and that the very next
 *  keystroke lands in. `children: []` fails the first; a caret parked in a
 *  void or in an attachment fence's hidden code body fails the second. */
function expectReadyForInput(editor: PlateEditor) {
  expect(editor.children.length).toBeGreaterThan(0)
  expect(types(editor).at(-1)).toBe('p')
  editor.tf.insertText('typed')
  expect(chatValueToMarkdown(editor.children as Value)).toContain('typed')
}

describe('TestRegression_ComposerAttachmentDeletionCorruption', () => {
  // THE reported order. Attaching leaves `[img, p]`; backspacing the empty
  // line left `[img]` alone, and the trash button on that last attachment
  // then left `children: []` with `selection: null` — not a valid Slate
  // document. Nothing rendered, the pill collapsed to an empty oval, and no
  // keystroke could ever bring it back. `normalize({ force: true })` did not
  // repair it either: Slate restores a block only inside `deleteFragment`,
  // never after `removeNodes`.
  it('leaves a typable box after the empty line, then the attachment, is deleted', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, IMAGE)
    expect(types(editor)).toEqual(['img', 'p'])

    editor.tf.deleteBackward('character')
    trashFirst(editor, 'img')

    expect(editor.children).toHaveLength(1)
    expectReadyForInput(editor)
  })

  // The order that always worked, kept so a fix for the one above can never
  // be written in a way that breaks it.
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

  // Every kind, not just the photo the bug was reported on: the file card is
  // a paragraph wrapping a link, the text attachment and the excalidraw
  // diagram are fences (the latter a fence PLUS its own persisted-PNG
  // sibling), and the image is a void block.
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

  // Not a deletion gesture at all — the failed-upload path (`settlePending
  // ImageInto`, chat-markdown-editor.tsx) removes its placeholder the same
  // way, and so does anything else that ever calls `removeNodes`. The floor
  // has to be a rule, not a fix for one button.
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

  // REGRESSION, found while fixing the above and worse than a dead box: with
  // the trailing line restored by normalization but backspace still Slate's
  // own, the caret landed INSIDE the attachment it was next to. A fence's
  // code body is rendered `hidden` under its preview, not removed — so the
  // next thing typed went into the attachment's payload, invisibly, and was
  // sent that way.
  it('never types into an attachment fence after backspacing next to it', () => {
    const editor = composer()
    insertAttachmentMarkdownInto(editor, TEXT_ATTACHMENT)
    editor.tf.deleteBackward('character')
    editor.tf.insertText('typed')

    expect(chatValueToMarkdown(editor.children as Value)).not.toContain('hi theretyped')
  })

  // The excalidraw kind is one attachment held as TWO adjacent nodes (fence,
  // then its persisted PNG — see `removeExcalidrawAttachment`). Backspace has
  // to take both, or the fence is left rendering a preview whose image is
  // gone, still in the document and still sent.
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

  // A fence that is NOT an attachment (`text-attachment` with no id, an
  // ordinary ```js block) is a code block the person is editing — backspace
  // against it merges in, exactly as before, rather than deleting it.
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

  // A caret at the start of a table cell is at the start of a block too, just
  // not a top-level one — whatever Plate's own tables do there has to keep
  // happening, so this asserts against the SAME gesture on an editor without
  // the plugin rather than against a hand-written expectation.
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

  // The trailing line is structure, not content: it must never reach the
  // model as a blank line, or every prompt with an attachment in it would
  // arrive padded.
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
