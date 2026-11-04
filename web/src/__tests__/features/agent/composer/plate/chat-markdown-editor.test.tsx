import { createRef, Profiler } from 'react'
import type { KeyboardEvent } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import { createPlateEditor, PlateController, useEditorRef } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import {
  ChatMarkdownEditor,
  type CaretEdges,
  type ChatMarkdownEditorHandle,
  type ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import * as serialization from '@/features/agent/composer/plate/chat-composer-serialization'

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

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

afterEach(() => {
  cleanup()
  platform.mobile = false
})
beforeEach(() => createPlatePluginSpy.mockClear())

describe('ChatMarkdownEditor — autoFocus', () => {
  const renderFocused = () =>
    render(
      <ChatMarkdownEditor
        placeholder=""
        ariaLabel="Message the agent"
        autoFocus
        onShapeChange={vi.fn()}
        onKeyDown={vi.fn()}
      />,
    )

  it('takes focus on mount', () => {
    const { getByRole } = renderFocused()
    expect(getByRole('textbox', { name: 'Message the agent' })).toHaveFocus()
  })

  // On the phone a focused field raises the keyboard over the chat just opened.
  it('leaves focus alone on the phone', () => {
    platform.mobile = true
    const { getByRole } = renderFocused()
    expect(getByRole('textbox', { name: 'Message the agent' })).not.toHaveFocus()
  })
})

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

function editorWith(markdown: string, autoSelect?: 'start' | 'end'): PlateEditor {
  return createPlateEditor({
    plugins: chatComposerPlugins,
    value: serialization.chatMarkdownToValue(markdown),
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

const grabbed: { editor?: PlateEditor } = {}
function GrabEditor() {
  grabbed.editor = useEditorRef()
  return null
}

async function renderEditor(onShapeChange: (shape: ComposerShape) => void = vi.fn()) {
  const ref = createRef<ChatMarkdownEditorHandle>()
  const counter = { commits: 0 }
  const view = (handler: (shape: ComposerShape) => void) => (
    <PlateController>
      <Profiler id="editor" onRender={() => counter.commits++}>
        <ChatMarkdownEditor
          ref={ref}
          wsId="w1"
          chatId="c1"
          placeholder=""
          ariaLabel="Message the agent"
          onShapeChange={handler}
          onKeyDown={vi.fn()}
        />
      </Profiler>
      <GrabEditor />
    </PlateController>
  )
  const { container, rerender } = render(view(onShapeChange))
  await act(async () => {})
  counter.commits = 0
  return {
    ref,
    counter,
    editable: () => container.querySelector('[data-slate-editor]'),
    type: (text: string) =>
      act(async () => grabbed.editor!.tf.insertText(text, { at: grabbed.editor!.api.end([]) })),
    rerender: (handler: (shape: ComposerShape) => void) => rerender(view(handler)),
  }
}

describe('ChatMarkdownEditor imperative handle', () => {
  it('inserts a block-level node built from markdown, and reports the box no longer empty', async () => {
    const onShapeChange = vi.fn()
    const { ref } = await renderEditor(onShapeChange)

    act(() => ref.current?.insertAttachmentMarkdown('```text-attachment:abc123\nhello world\n```'))

    expect(ref.current?.getMarkdown()).toContain('text-attachment:abc123')
    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: false, slashQuery: null }, true)
  })

  it('settles a pending image to its real ref, an image alone being something to send', async () => {
    const onShapeChange = vi.fn()
    const { ref } = await renderEditor(onShapeChange)
    act(() => ref.current?.insertPendingImage('blob:local-preview', 'photo.png'))
    expect(ref.current?.getMarkdown()).toContain('blob:local-preview')
    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: false, slashQuery: null }, true)

    act(() =>
      ref.current?.settlePendingImage(
        'blob:local-preview',
        '![photo.png](chats/c1/attachments/photo.png)',
      ),
    )

    expect(ref.current?.getMarkdown()).toContain('chats/c1/attachments/photo.png')
    expect(ref.current?.getMarkdown()).not.toContain('blob:local-preview')
  })

  // The handle is how text gets in from outside now; none of it may remount.
  it('loads restored markdown through setMarkdown, with nothing to undo back to', async () => {
    const onShapeChange = vi.fn()
    const { ref, editable, type } = await renderEditor(onShapeChange)
    const before = editable()
    await type('typed')

    await act(async () => ref.current?.setMarkdown('**never mind**\n\nsecond thought'))
    await act(async () => grabbed.editor!.undo())

    expect(editable()).toBe(before)
    expect(editable()).toHaveTextContent('never mindsecond thought')
    expect(ref.current?.getMarkdown()).toBe('**never mind**\n\nsecond thought')
    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: false, slashQuery: null }, false)
  })

  it('empties the box through clear, with nothing left to undo', async () => {
    const onShapeChange = vi.fn()
    const { ref } = await renderEditor(onShapeChange)
    act(() => ref.current?.setMarkdown('sent already'))

    act(() => ref.current?.clear())
    act(() => grabbed.editor!.undo())

    expect(ref.current?.getMarkdown()).toBe('')
    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: true, slashQuery: null }, false)
  })
})

// Every extra commit per keystroke queues behind the next key; enough in a row
// is React's #185 "Maximum update depth exceeded".
describe('ChatMarkdownEditor shape and commits', () => {
  it('reports empty and the slash query without producing markdown', async () => {
    const serialize = vi.spyOn(serialization, 'chatValueToMarkdown')
    const onShapeChange = vi.fn()
    const { type } = await renderEditor(onShapeChange)

    await type('/')
    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: false, slashQuery: '' }, true)
    await type('Rev')
    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: false, slashQuery: 'Rev' }, true)
    await act(async () => {
      const editor = grabbed.editor!
      editor.tf.delete({ at: { anchor: editor.api.start([])!, focus: editor.api.end([])! } })
    })
    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: true, slashQuery: null }, true)
    expect(serialize).not.toHaveBeenCalled()
  })

  it('commits an edit to the text once', async () => {
    const onShapeChange = vi.fn()
    const { counter, type } = await renderEditor(onShapeChange)

    await type('hi')

    expect(onShapeChange).toHaveBeenLastCalledWith({ empty: false, slashQuery: null }, true)
    expect(counter.commits).toBe(1)
  })

  it('takes a new shape handler in one commit, and reports to the newest one', async () => {
    const first = vi.fn()
    const second = vi.fn()
    const { counter, rerender, type } = await renderEditor(first)

    await act(async () => rerender(second))
    expect(counter.commits).toBe(1)

    await type('!')
    expect(second).toHaveBeenCalled()
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
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
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

describe('ChatMarkdownEditor key handling', () => {
  type OnKeyDownMock = Mock<
    (
      event: KeyboardEvent<HTMLDivElement>,
      readMarkdown: () => string,
      caret: CaretEdges,
      shape: ComposerShape,
    ) => void
  >

  function renderAndCapture(onKeyDown: OnKeyDownMock): KeyDownHandler {
    render(
      <ChatMarkdownEditor
        wsId="w1"
        chatId="c1"
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
        onKeyDown={onKeyDown}
      />,
    )
    return capturedKeyDownHandler()
  }

  it('dispatches to the caller with the live markdown, its shape, and atEnd at a collapsed end', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hi', 'end')
    const event = keyEvent({ key: 'Enter' })

    handleKeyDown({ editor, event })

    expect(onKeyDown).toHaveBeenCalledTimes(1)
    const [passedEvent, readMarkdown, caret, shape] = onKeyDown.mock.calls[0]!
    expect(passedEvent).toBe(event)
    expect(caret).toEqual({ atStart: false, atEnd: true })
    expect(shape).toEqual({ empty: false, slashQuery: null })
    expect(readMarkdown()).toBe('hi')
  })

  it('reports atStart true at a collapsed selection at the very beginning', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hi', 'start')

    handleKeyDown({ editor, event: keyEvent({ key: 'ArrowLeft' }) })

    const [, , caret] = onKeyDown.mock.calls[0]!
    expect(caret).toEqual({ atStart: true, atEnd: false })
  })

  it('reports neither edge for a non-collapsed selection', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hello world')
    editor.selection = { anchor: editor.api.start([])!, focus: editor.api.end([])! }

    handleKeyDown({ editor, event: keyEvent({ key: 'Backspace' }) })

    const [, , caret] = onKeyDown.mock.calls[0]!
    expect(caret).toEqual({ atStart: false, atEnd: false })
  })

  it('reports neither edge when there is no selection at all', () => {
    const onKeyDown: OnKeyDownMock = vi.fn()
    const handleKeyDown = renderAndCapture(onKeyDown)
    const editor = editorWith('hi')

    handleKeyDown({ editor, event: keyEvent({ key: 'Enter' }) })

    const [, , caret] = onKeyDown.mock.calls[0]!
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
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
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
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
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
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
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
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
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
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
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
        placeholder=""
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
        onKeyDown={vi.fn()}
        onHeightChange={onHeightChange}
      />,
    )

    expect(FakeResizeObserver.instances).toHaveLength(2)
    expect(FakeResizeObserver.instances[1]!.observed).not.toBeNull()
    expect(FakeResizeObserver.instances[1]!.observed).not.toBe(firstEditable)
  })
})
