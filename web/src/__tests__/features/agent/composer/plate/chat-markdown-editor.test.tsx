import { createRef } from 'react'
import type { KeyboardEvent } from 'react'
import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import type { Value } from 'platejs'
import {
  ChatMarkdownEditor,
  insertAttachmentMarkdownInto,
  type CaretEdges,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'

// Captures the config `ChatMarkdownEditor` hands `createPlatePlugin` for its
// OWN key-handling plugin, unchanged by this task, so that real (unmodified)
// handler can be called directly. jsdom cannot deliver a real keydown through
// Slate's own selection-dependent dispatch — see agent-empty-document.test.tsx
// and agent-chat-view.test.tsx's own notes on this — so firing `fireEvent.
// keyDown` at the mounted editable would prove nothing about this handler.
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
    | { type: string; lang?: string; children: { children: { text: string }[] }[] }
    | undefined
}

describe('insertAttachmentMarkdownInto', () => {
  it('inserts markdown deserialized through the codec, producing the expected code_block node shape', () => {
    const editor = editorWith('hello', 'end')

    insertAttachmentMarkdownInto(editor, '```text-attachment:abc123\nhello world\n```')

    const inserted = codeBlockNode(editor)
    expect(inserted?.lang).toBe('text-attachment:abc123')
    expect(inserted?.children.map((line) => line.children.map((leaf) => leaf.text).join(''))).toEqual([
      'hello world',
    ])
  })

  // The fallback branch: a caller (a modal, a drop handler) can invoke the
  // handle before the box has ever taken a selection of its own.
  it('inserts at the end of the document when the editor has no selection', () => {
    const editor = editorWith('hello')
    expect(editor.selection).toBeNull()

    insertAttachmentMarkdownInto(editor, '```text-attachment:def456\nmore\n```')

    expect(codeBlockNode(editor)?.lang).toBe('text-attachment:def456')
  })

  // TestRegression: two back-to-back inserts previously landed the second
  // INSIDE the first fence's own code_line (Slate's default `mode: 'lowest'`
  // matches the nested code_line, not the top-level code_block, at the
  // selection a fence insert leaves behind) — silently dropping the second
  // attachment's content. This is exactly the excalidraw modal's fence-then-
  // image save sequence, and any other two-attachment-in-a-row case (e.g.
  // two dropped files).
  it('TestRegression_insertsSecondAttachmentAsASiblingNotInsideTheFirstFence', () => {
    const editor = editorWith('', 'end')

    insertAttachmentMarkdownInto(editor, '```excalidraw:abc\n{"elements":[]}\n```')
    insertAttachmentMarkdownInto(editor, '![diagram](chats/c1/attachments/x-diagram.png)')

    const [, fence, image] = editor.children as {
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

  // UNCONTROLLED / REMOUNT-VIA-KEY CONTRACT: the file's own note explains why
  // a controlled contenteditable is unacceptable here — a remount is how the
  // PARENT pushes text in from outside, not something an insert should ever
  // trigger on its own. Proven by the editable DOM node's identity surviving
  // the call: a remount tears the old node down and mounts a fresh one.
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

  // A real Slate document always resolves `api.start([])`/`api.end([])` — this
  // guard has no live trigger, only the defensive shape of "don't select onto
  // points that don't exist". Exercised with a stand-in editor, the only way
  // to make either resolve to nothing.
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
