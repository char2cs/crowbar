import { createRef, Profiler } from 'react'
import type { KeyboardEvent } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import { createPlateEditor, PlateController, useEditorRef } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import type { Value } from 'platejs'
import {
  ChatMarkdownEditor,
  insertAttachmentMarkdownInto,
  insertPendingImageInto,
  settlePendingImageInto,
  type CaretEdges,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'

// Captures the config of the editor's own key plugin so its real handler can be
// called directly: jsdom cannot drive Slate's selection-dependent keydown.
const createPlatePluginSpy = vi.hoisted(() => vi.fn())

vi.mock('platejs/react', async (importOriginal) => {
  const actual = await importOriginal<typeof import('platejs/react')>()
  return {
    ...actual,
    createPlatePlugin: (config: unknown) => {
      createPlatePluginSpy(config)
      return actual.createPlatePlugin(config as Parameters<typeof actual.createPlatePlugin>[0])
    },
  }
})

afterEach(cleanup)
beforeEach(() => createPlatePluginSpy.mockClear())

type KeyDownHandler = (ctx: { editor: PlateEditor; event: KeyboardEvent<HTMLDivElement> }) => void

function capturedKeyDownHandler(): KeyDownHandler {
  const call = createPlatePluginSpy.mock.calls.find(
    ([config]) => (config as { key?: string }).key === 'agent-chat-keys',
  )
  if (!call) throw new Error('agent-chat-keys plugin was never created')
  return (call[0] as { handlers: { onKeyDown: KeyDownHandler } }).handlers.onKeyDown
}

type DropEventStub = {
  dataTransfer: { types: string[] } | null
  preventDefault: () => void
}
type DropGuardHandler = (ctx: { event: DropEventStub }) => boolean | undefined

function capturedDropGuardHandler(): DropGuardHandler {
  const call = createPlatePluginSpy.mock.calls.find(
    ([config]) => (config as { key?: string }).key === 'agent-chat-drop-guard',
  )
  if (!call) throw new Error('agent-chat-drop-guard plugin was never created')
  return (call[0] as { handlers: { onDrop: DropGuardHandler } }).handlers.onDrop
}

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

function keyEvent(
  overrides: Partial<{
    key: string
    metaKey: boolean
    ctrlKey: boolean
    shiftKey: boolean
    altKey: boolean
  }>,
): KeyboardEvent<HTMLDivElement> {
  return {
    key: 'a',
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    preventDefault: vi.fn(),
    ...overrides,
  } as unknown as KeyboardEvent<HTMLDivElement>
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

  // REGRESSION, reported live: after attaching something, there was nowhere
  // to keep typing except clicking below the block yourself.
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

describe('ChatMarkdownEditor imperative handle', () => {
  it('inserts a block-level node built from markdown, and reports it via onChange', () => {
    const ref = createRef<ChatMarkdownEditorHandle>()
    const onChange = vi.fn()
    render(
      <ChatMarkdownEditor
        ref={ref}
        wsId="w1"
        chatId="c1"
        initialValue=""
        placeholder=""
        ariaLabel="Message the agent"
        onChange={onChange}
        onKeyDown={vi.fn()}
      />,
    )

    ref.current?.insertAttachmentMarkdown('```text-attachment:abc123\nhello world\n```')

    const lastCall = onChange.mock.calls.at(-1)?.[0] as string
    expect(lastCall).toContain('text-attachment:abc123')
  })

  it('inserts a pending image at a local object URL, and reports it via onChange', () => {
    const ref = createRef<ChatMarkdownEditorHandle>()
    const onChange = vi.fn()
    render(
      <ChatMarkdownEditor
        ref={ref}
        initialValue=""
        placeholder=""
        ariaLabel="Message the agent"
        onChange={onChange}
        onKeyDown={vi.fn()}
      />,
    )

    ref.current?.insertPendingImage('blob:local-preview', 'photo.png')

    expect(onChange.mock.calls.at(-1)?.[0]).toContain('blob:local-preview')
  })

  it('settles a pending image to its real ref, and reports the swap via onChange', () => {
    const ref = createRef<ChatMarkdownEditorHandle>()
    const onChange = vi.fn()
    render(
      <ChatMarkdownEditor
        ref={ref}
        initialValue=""
        placeholder=""
        ariaLabel="Message the agent"
        onChange={onChange}
        onKeyDown={vi.fn()}
      />,
    )
    ref.current?.insertPendingImage('blob:local-preview', 'photo.png')

    ref.current?.settlePendingImage(
      'blob:local-preview',
      '![photo.png](chats/c1/attachments/photo.png)',
    )

    const lastCall = onChange.mock.calls.at(-1)?.[0] as string
    expect(lastCall).toContain('chats/c1/attachments/photo.png')
    expect(lastCall).not.toContain('blob:local-preview')
  })

  // wsId/chatId are accepted-but-unused this task (a later task's paste
  // plugin needs them) — proven here by the OTHER existing caller,
  // agent-empty-document.tsx, which doesn't pass them at all yet.
  it('renders without wsId/chatId, matching the one call site not updated by this task', () => {
    const ref = createRef<ChatMarkdownEditorHandle>()
    expect(() =>
      render(
        <ChatMarkdownEditor
          ref={ref}
          initialValue="hi"
          placeholder=""
          ariaLabel="Message the agent"
          onChange={vi.fn()}
          onKeyDown={vi.fn()}
        />,
      ),
    ).not.toThrow()
    expect(ref.current?.insertAttachmentMarkdown).toBeTypeOf('function')
  })

  // A remount is how the parent pushes text in; an insert must never cause one.
  it('does not remount the editable when inserting via the handle', () => {
    const ref = createRef<ChatMarkdownEditorHandle>()
    const { container } = render(
      <ChatMarkdownEditor
        ref={ref}
        wsId="w1"
        chatId="c1"
        initialValue="hello"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
      />,
    )
    const editableBefore = container.querySelector('[data-slate-editor]')
    expect(editableBefore).not.toBeNull()

    ref.current?.insertAttachmentMarkdown('```text-attachment:abc123\nmore\n```')

    expect(container.querySelector('[data-slate-editor]')).toBe(editableBefore)
  })
})

// Every extra commit per keystroke queues behind the next key; enough in a row
// is React's #185 "Maximum update depth exceeded".
describe('ChatMarkdownEditor commits', () => {
  const grabbed: { editor?: PlateEditor } = {}
  function GrabEditor() {
    grabbed.editor = useEditorRef()
    return null
  }
  async function renderCounted(onChange: (markdown: string) => void) {
    const counter = { commits: 0 }
    const view = (handler: (markdown: string) => void) => (
      <PlateController>
        <Profiler id="editor" onRender={() => counter.commits++}>
          <ChatMarkdownEditor
            initialValue="hi there"
            placeholder=""
            ariaLabel="Message the agent"
            onChange={handler}
            onKeyDown={vi.fn()}
          />
        </Profiler>
        <GrabEditor />
      </PlateController>
    )
    const { rerender } = render(view(onChange))
    await act(async () => {})
    counter.commits = 0
    return { counter, rerender: (handler: (markdown: string) => void) => rerender(view(handler)) }
  }

  it('commits an edit to the text once', async () => {
    const onChange = vi.fn()
    const { counter } = await renderCounted(onChange)
    const editor = grabbed.editor!

    await act(async () => editor.tf.insertText('!', { at: editor.api.end([]) }))

    expect(onChange).toHaveBeenLastCalledWith('hi there!')
    expect(counter.commits).toBe(1)
  })

  it('takes a new onChange in one commit, and reports to the newest one', async () => {
    const first = vi.fn()
    const second = vi.fn()
    const { counter, rerender } = await renderCounted(first)

    await act(async () => rerender(second))
    expect(counter.commits).toBe(1)

    const editor = grabbed.editor!
    await act(async () => editor.tf.insertText('!', { at: editor.api.end([]) }))
    expect(second).toHaveBeenLastCalledWith('hi there!')
    expect(first).not.toHaveBeenCalled()
  })
})

// The guard's two branches in isolation; the end-to-end drop at the real
// editable lives in agent-composer.test.tsx.
describe('ChatMarkdownEditor drop guard (Wave 6, Bug 1)', () => {
  function renderAndCapture(): DropGuardHandler {
    render(
      <ChatMarkdownEditor
        wsId="w1"
        chatId="c1"
        initialValue=""
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
      />,
    )
    return capturedDropGuardHandler()
  }

  it('prevents default and reports itself handled for a drop carrying Files', () => {
    const onDrop = renderAndCapture()
    const preventDefault = vi.fn()

    const handled = onDrop({ event: { dataTransfer: { types: ['Files'] }, preventDefault } })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(handled).toBe(true)
  })

  it('does nothing for a drop that carries no Files', () => {
    const onDrop = renderAndCapture()
    const preventDefault = vi.fn()

    const handled = onDrop({ event: { dataTransfer: { types: ['text/plain'] }, preventDefault } })

    expect(preventDefault).not.toHaveBeenCalled()
    expect(handled).toBeUndefined()
  })

  it('does nothing when the drop event carries no dataTransfer at all', () => {
    const onDrop = renderAndCapture()
    const preventDefault = vi.fn()

    const handled = onDrop({ event: { dataTransfer: null, preventDefault } })

    expect(preventDefault).not.toHaveBeenCalled()
    expect(handled).toBeUndefined()
  })
})

// Regression coverage for the box's OWN key-handling plugin: untouched by
// this task, but this is the first direct (unmocked) test of this component,
// so its behaviour has never been proven against real assertions before.
describe('ChatMarkdownEditor key handling (unchanged by this task)', () => {
  type OnKeyDownMock = Mock<
    (event: KeyboardEvent<HTMLDivElement>, readMarkdown: () => string, caret: CaretEdges) => void
  >

  function renderAndCapture(onKeyDown: OnKeyDownMock): KeyDownHandler {
    render(
      <ChatMarkdownEditor
        wsId="w1"
        chatId="c1"
        initialValue="hi"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={onKeyDown}
      />,
    )
    return capturedKeyDownHandler()
  }

  it('dispatches to the caller with the live markdown and atEnd true at a collapsed end selection', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hi', 'end')
    const event = keyEvent({ key: 'Enter' })

    handleKeyDown({ editor, event })

    expect(onKeyDown).toHaveBeenCalledTimes(1)
    const [passedEvent, readMarkdown, caret] = onKeyDown.mock.calls[0] as [
      unknown,
      () => string,
      CaretEdges,
    ]
    expect(passedEvent).toBe(event)
    expect(caret).toEqual({ atStart: false, atEnd: true })
    expect(readMarkdown()).toBe('hi')
  })

  it('reports atStart true at a collapsed selection at the very beginning', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hi', 'start')

    handleKeyDown({ editor, event: keyEvent({ key: 'ArrowLeft' }) })

    const [, , caret] = onKeyDown.mock.calls[0] as [unknown, unknown, CaretEdges]
    expect(caret).toEqual({ atStart: true, atEnd: false })
  })

  it('reports neither edge for a non-collapsed selection', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hello world')
    editor.selection = { anchor: editor.api.start([])!, focus: editor.api.end([])! }

    handleKeyDown({ editor, event: keyEvent({ key: 'Backspace' }) })

    const [, , caret] = onKeyDown.mock.calls[0] as [unknown, unknown, CaretEdges]
    expect(caret).toEqual({ atStart: false, atEnd: false })
  })

  it('reports neither edge when there is no selection at all', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hi')

    handleKeyDown({ editor, event: keyEvent({ key: 'Enter' }) })

    const [, , caret] = onKeyDown.mock.calls[0] as [unknown, unknown, CaretEdges]
    expect(caret).toEqual({ atStart: false, atEnd: false })
  })

  it('intercepts Cmd+A to select the whole document instead of dispatching to the caller', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hello world', 'start')
    const event = keyEvent({ key: 'a', metaKey: true })

    handleKeyDown({ editor, event })

    expect(event.preventDefault).toHaveBeenCalledTimes(1)
    expect(onKeyDown).not.toHaveBeenCalled()
    expect(editor.selection).toEqual({ anchor: editor.api.start([]), focus: editor.api.end([]) })
  })

  it('intercepts Ctrl+A the same way', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hello world', 'start')

    handleKeyDown({ editor, event: keyEvent({ key: 'A', ctrlKey: true }) })

    expect(onKeyDown).not.toHaveBeenCalled()
  })

  it.each([{ shiftKey: true }, { altKey: true }])(
    'does not treat Cmd+A as select-all when %o is also held',
    (modifier) => {
      const onKeyDown: OnKeyDownMock = vi.fn()
      const handleKeyDown = renderAndCapture(onKeyDown)
      const editor = editorWith('hi', 'end')

      handleKeyDown({ editor, event: keyEvent({ key: 'a', metaKey: true, ...modifier }) })

      expect(onKeyDown).toHaveBeenCalledTimes(1)
    },
  )

  // No live trigger: a real document always has edges. A stand-in editor is the
  // only way to make them resolve to nothing.
  it('still consumes Cmd+A without selecting when the document has no resolvable edges', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const select = vi.fn()
    const noEdgesEditor = {
      api: { start: () => undefined, end: () => undefined },
      tf: { select },
    } as unknown as PlateEditor
    const event = keyEvent({ key: 'a', metaKey: true })

    handleKeyDown({ editor: noEdgesEditor, event })

    expect(select).not.toHaveBeenCalled()
    expect(event.preventDefault).not.toHaveBeenCalled()
    expect(onKeyDown).not.toHaveBeenCalled()
  })
})

// A controllable stand-in: jsdom has no ResizeObserver. `trigger()` is the
// browser calling back on a size change.
class FakeResizeObserver {
  static instances: FakeResizeObserver[] = []
  observed: Element | null = null
  constructor(private callback: ResizeObserverCallback) {
    FakeResizeObserver.instances.push(this)
  }
  observe(el: Element) {
    this.observed = el
  }
  disconnect() {
    this.observed = null
  }
  unobserve() {}
  trigger() {
    this.callback([], this as unknown as ResizeObserver)
  }
}

describe('ChatMarkdownEditor height reporting', () => {
  const realResizeObserver = globalThis.ResizeObserver

  beforeEach(() => {
    FakeResizeObserver.instances = []
    globalThis.ResizeObserver = FakeResizeObserver as unknown as typeof ResizeObserver
  })

  afterEach(() => {
    globalThis.ResizeObserver = realResizeObserver
  })

  it('reports nothing on mount while the box has no layout', () => {
    const onHeightChange = vi.fn()
    render(
      <ChatMarkdownEditor
        initialValue="hello"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
        onHeightChange={onHeightChange}
      />,
    )

    // jsdom reports an empty box, which is the absence of a box: not reported.
    expect(onHeightChange).not.toHaveBeenCalled()
  })

  // Reported during commit, so a box that mounts multi-line paints its first
  // frame at the right height.
  it('reports the height of the box it mounts into', () => {
    const rect = vi
      .spyOn(HTMLElement.prototype, 'getBoundingClientRect')
      .mockReturnValue({ height: 40, width: 0 } as DOMRect)
    const onHeightChange = vi.fn()
    render(
      <ChatMarkdownEditor
        initialValue="hello"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
        onHeightChange={onHeightChange}
      />,
    )
    rect.mockRestore()

    expect(onHeightChange).toHaveBeenCalledWith(40)
  })

  it('does not report a height once the box is gone', () => {
    const onHeightChange = vi.fn()
    const { container } = render(
      <ChatMarkdownEditor
        initialValue="hello"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
        onHeightChange={onHeightChange}
      />,
    )
    const editable = container.querySelector('[data-slate-editor]') as HTMLElement
    vi.spyOn(editable, 'getBoundingClientRect').mockReturnValue({ height: 0, width: 0 } as DOMRect)
    FakeResizeObserver.instances[0]!.trigger()

    expect(onHeightChange).not.toHaveBeenCalled()
  })

  it('reports a new height when the observed editable resizes', () => {
    const onHeightChange = vi.fn()
    const { container } = render(
      <ChatMarkdownEditor
        initialValue="hello"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
        onHeightChange={onHeightChange}
      />,
    )
    const editable = container.querySelector('[data-slate-editor]') as HTMLElement
    onHeightChange.mockClear()
    vi.spyOn(editable, 'getBoundingClientRect').mockReturnValue({ height: 64 } as DOMRect)

    expect(FakeResizeObserver.instances).toHaveLength(1)
    FakeResizeObserver.instances[0]!.trigger()

    expect(onHeightChange).toHaveBeenCalledWith(64)
  })

  // A remount gives the editable a fresh node: the old observer goes, a new one
  // watches the new node (an effect with stable deps kept watching a detached one).
  it('tears down the old observer and attaches a fresh one across a genuine remount', () => {
    const onHeightChange = vi.fn()
    const { container, unmount } = render(
      <ChatMarkdownEditor
        initialValue="hello"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
        onHeightChange={onHeightChange}
      />,
    )
    const firstEditable = container.querySelector('[data-slate-editor]')
    expect(FakeResizeObserver.instances).toHaveLength(1)
    const firstObserver = FakeResizeObserver.instances[0]!
    expect(firstObserver.observed).toBe(firstEditable)

    unmount()
    expect(firstObserver.observed).toBeNull()

    render(
      <ChatMarkdownEditor
        initialValue="hello again"
        placeholder=""
        ariaLabel="Message the agent"
        onChange={vi.fn()}
        onKeyDown={vi.fn()}
        onHeightChange={onHeightChange}
      />,
    )

    expect(FakeResizeObserver.instances).toHaveLength(2)
    expect(FakeResizeObserver.instances[1]!.observed).not.toBeNull()
    expect(FakeResizeObserver.instances[1]!.observed).not.toBe(firstEditable)
  })
})
