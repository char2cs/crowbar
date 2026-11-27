import { act, cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import { CodeBlockPlugin } from '@platejs/code-block/react'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import { createChatPastePlugin } from '@/features/agent/composer/plate/chat-paste-plugin'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { createRef } from 'react'
import {
  ChatMarkdownEditor,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  UNWORDED,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'

const INLINE_ATTACHMENT_MAX_BYTES = 32 * 1024

const uploadChatAttachmentMock = vi.hoisted(() => vi.fn())
vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: uploadChatAttachmentMock,
}))

const uploadAttachmentMarkdownMock = vi.hoisted(() => vi.fn())
vi.mock('@/features/agent/composer/lib/attachment-upload', () => ({
  uploadAttachmentMarkdown: uploadAttachmentMarkdownMock,
}))

let toastError: ReturnType<typeof watchToasts>

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

beforeEach(() => {
  createPlatePluginSpy.mockClear()
  uploadChatAttachmentMock.mockReset()
  uploadAttachmentMarkdownMock.mockReset()
  toastError?.mockRestore()
  toastError = watchToasts()
})

afterEach(cleanup)

type ClipboardItemStub = { type: string; kind: string; getAsFile: () => File | null }
type PasteEventStub = {
  clipboardData: {
    getData: (type: string) => string
    items: ClipboardItemStub[]
    files: File[]
  } | null
  preventDefault: () => void
}
type KeyEventStub = { key: string }
type PastePluginHandlers = {
  onKeyDown: (ctx: { event: KeyEventStub }) => void
  onKeyUp: (ctx: { event: KeyEventStub }) => void
  onPaste: (ctx: { editor: PlateEditor; event: PasteEventStub }) => void
}

function pastePluginHandlers(wsId = 'w1', chatId = 'c1'): PastePluginHandlers {
  createChatPastePlugin({ wsId, chatId })
  const call = createPlatePluginSpy.mock.calls.find(
    ([config]) => (config as { key?: string }).key === 'agent-chat-paste',
  )
  if (!call) throw new Error('agent-chat-paste plugin was never created')
  return (call[0] as { handlers: PastePluginHandlers }).handlers
}

function editorWith(markdown: string): PlateEditor {
  return createPlateEditor({ plugins: chatComposerPlugins, value: chatMarkdownToValue(markdown) })
}

function pasteEvent(text: string, items: ClipboardItemStub[] = [], files: File[] = []) {
  const preventDefault = vi.fn()
  const event: PasteEventStub = {
    clipboardData: { getData: () => text, items, files },
    preventDefault,
  }
  return { event, preventDefault }
}

function imageItem(file: File | null): ClipboardItemStub {
  return { type: 'image/png', kind: 'file', getAsFile: () => file }
}

describe('createChatPastePlugin', () => {
  it('wraps an over-threshold plain-text paste as a text-attachment fence', () => {
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const { event, preventDefault } = pasteEvent('x'.repeat(500))

    const handled = onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(chatValueToMarkdown(editor.children as never)).toContain('```text-attachment:')
    expect(handled).toBe(true)
  })

  it('uploads an over-cap pasted text as a file instead of wrapping it inline', async () => {
    uploadAttachmentMarkdownMock.mockResolvedValue(
      '[pasted.txt](chats/c1/attachments/x-pasted.txt)',
    )
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const hugeText = 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1)
    const { event, preventDefault } = pasteEvent(hugeText)

    const handled = onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(handled).toBe(true)
    expect(uploadAttachmentMarkdownMock).toHaveBeenCalledTimes(1)
    const [, , input] = uploadAttachmentMarkdownMock.mock.calls[0] as [
      string,
      string,
      { file: File },
    ]
    expect(input.file.name).toBe('pasted.txt')
    expect(input.file.type).toBe('text/plain')
    await waitFor(() => {
      expect(chatValueToMarkdown(editor.children as never)).toContain(
        '[pasted.txt](chats/c1/attachments/x-pasted.txt)',
      )
    })
    expect(chatValueToMarkdown(editor.children as never)).not.toContain('text-attachment:')
  })

  it('surfaces a failed over-cap text upload as a toast instead of silently doing nothing', async () => {
    uploadAttachmentMarkdownMock.mockRejectedValueOnce(refusal('413 Payload Too Large'))
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const hugeText = 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1)
    const { event } = pasteEvent(hugeText)

    onPaste({ editor, event })

    await waitFor(() =>
      expectReported(toastError, 'Could not attach that text', '413 Payload Too Large'),
    )
  })

  it('an over-cap text upload refused while migrating shows the migrating state', async () => {
    uploadAttachmentMarkdownMock.mockRejectedValueOnce(migratingRefusal())
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const hugeText = 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1)
    const { event } = pasteEvent(hugeText)

    onPaste({ editor, event })

    await waitFor(() => expectMigratingShown(toastError))
  })

  it('does nothing when the paste event carries no clipboardData at all', () => {
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const before = editor.children
    const preventDefault = vi.fn()

    onPaste({ editor, event: { clipboardData: null, preventDefault } })

    expect(preventDefault).not.toHaveBeenCalled()
    expect(uploadChatAttachmentMock).not.toHaveBeenCalled()
    expect(editor.children).toBe(before)
  })

  it('leaves a short plain-text paste to Slate’s own default handling', () => {
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const before = editor.children
    const { event, preventDefault } = pasteEvent('hello there')

    const handled = onPaste({ editor, event })

    expect(preventDefault).not.toHaveBeenCalled()
    expect(editor.children).toBe(before)
    expect(handled).not.toBe(true)
  })

  it('lets Shift+paste bypass interception entirely, even for an over-threshold paste', () => {
    const { onKeyDown, onKeyUp, onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const before = editor.children

    onKeyDown({ event: { key: 'Shift' } })
    const { event, preventDefault } = pasteEvent('x'.repeat(500))
    onPaste({ editor, event })
    onKeyUp({ event: { key: 'Shift' } })

    expect(preventDefault).not.toHaveBeenCalled()
    expect(editor.children).toBe(before)
  })

  it('still intercepts after an unrelated key is pressed and released', () => {
    const { onKeyDown, onKeyUp, onPaste } = pastePluginHandlers()
    const editor = editorWith('')

    onKeyDown({ event: { key: 'a' } })
    onKeyUp({ event: { key: 'a' } })
    const { event, preventDefault } = pasteEvent('x'.repeat(500))
    onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(chatValueToMarkdown(editor.children as never)).toContain('```text-attachment:')
  })

  it('resumes intercepting after Shift is released', () => {
    const { onKeyDown, onKeyUp, onPaste } = pastePluginHandlers()
    const editor = editorWith('')

    onKeyDown({ event: { key: 'Shift' } })
    onKeyUp({ event: { key: 'Shift' } })
    const { event, preventDefault } = pasteEvent('x'.repeat(500))
    onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
  })

  it('uploads a pasted image and inserts it as markdown', async () => {
    uploadChatAttachmentMock.mockResolvedValue({
      ref: 'chats/c1/attachments/x-pasted-image.png',
      filename: 'pasted-image.png',
      size: 10,
      contentType: 'image/png',
    })
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'pasted-image.png', { type: 'image/png' })
    const { event, preventDefault } = pasteEvent('', [imageItem(file)])

    const handled = onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(handled).toBe(true)
    expect(uploadChatAttachmentMock).toHaveBeenCalledWith('w1', 'c1', { file })
    await waitFor(() => {
      expect(chatValueToMarkdown(editor.children as never)).toContain(
        '![pasted-image.png](chats/c1/attachments/x-pasted-image.png)',
      )
    })
  })

  it('inserts a local preview immediately, before the upload resolves', () => {
    let resolveUpload!: (value: {
      ref: string
      filename: string
      size: number
      contentType: string
    }) => void
    uploadChatAttachmentMock.mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve
      }),
    )
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'pasted-image.png', { type: 'image/png' })
    const { event } = pasteEvent('', [imageItem(file)])

    onPaste({ editor, event })

    const markdown = chatValueToMarkdown(editor.children as never)
    expect(markdown).toMatch(/!\[pasted-image\.png\]\(blob:/)

    resolveUpload({
      ref: 'chats/c1/attachments/x-pasted-image.png',
      filename: 'pasted-image.png',
      size: 10,
      contentType: 'image/png',
    })
  })

  it('does nothing further when an image clipboard item yields no file', () => {
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const before = editor.children
    const { event, preventDefault } = pasteEvent('', [imageItem(null)])

    onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(uploadChatAttachmentMock).not.toHaveBeenCalled()
    expect(editor.children).toBe(before)
  })

  it('surfaces a failed image upload as a toast instead of silently doing nothing', async () => {
    uploadChatAttachmentMock.mockRejectedValueOnce(refusal('413 Payload Too Large'))
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'huge.png', { type: 'image/png' })
    const { event } = pasteEvent('', [imageItem(file)])

    onPaste({ editor, event })

    await waitFor(() =>
      expectReported(toastError, 'Could not attach that image', '413 Payload Too Large'),
    )
  })

  it('an image upload refused while migrating shows the migrating state', async () => {
    uploadChatAttachmentMock.mockRejectedValueOnce(migratingRefusal())
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'huge.png', { type: 'image/png' })
    const { event } = pasteEvent('', [imageItem(file)])

    onPaste({ editor, event })

    await waitFor(() => expectMigratingShown(toastError))
  })

  it('uploads a pasted non-image file through the shared upload/markdown resolver', async () => {
    uploadAttachmentMarkdownMock.mockResolvedValue(
      '[report.pdf](chats/c1/attachments/x-report.pdf)',
    )
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'report.pdf', { type: 'application/pdf' })
    const { event, preventDefault } = pasteEvent('', [], [file])

    const handled = onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(handled).toBe(true)
    expect(uploadAttachmentMarkdownMock).toHaveBeenCalledWith('w1', 'c1', { file })
    await waitFor(() => {
      expect(chatValueToMarkdown(editor.children as never)).toContain(
        '[report.pdf](chats/c1/attachments/x-report.pdf)',
      )
    })
  })

  it('surfaces a failed non-image file upload as a toast instead of silently doing nothing', async () => {
    uploadAttachmentMarkdownMock.mockRejectedValueOnce(refusal('413 Payload Too Large'))
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'huge.pdf', { type: 'application/pdf' })
    const { event } = pasteEvent('', [], [file])

    onPaste({ editor, event })

    await waitFor(() =>
      expectReported(toastError, 'Could not attach that file', '413 Payload Too Large'),
    )
  })

  it('a non-image file upload refused while migrating shows the migrating state', async () => {
    uploadAttachmentMarkdownMock.mockRejectedValueOnce(migratingRefusal())
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'huge.pdf', { type: 'application/pdf' })
    const { event } = pasteEvent('', [], [file])

    onPaste({ editor, event })

    await waitFor(() => expectMigratingShown(toastError))
  })

  it('words a paste whose upload threw no Error as an unreachable daemon', async () => {
    uploadChatAttachmentMock.mockRejectedValue(UNWORDED)
    uploadAttachmentMarkdownMock.mockRejectedValue(UNWORDED)
    const { onPaste } = pastePluginHandlers()
    const image = new File(['bytes'], 'a.png', { type: 'image/png' })
    const pdf = new File(['bytes'], 'a.pdf', { type: 'application/pdf' })
    onPaste({ editor: editorWith(''), event: pasteEvent('', [imageItem(image)]).event })
    onPaste({ editor: editorWith(''), event: pasteEvent('', [], [pdf]).event })
    const huge = 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1)
    onPaste({ editor: editorWith(''), event: pasteEvent(huge).event })

    await waitFor(() => {
      for (const what of ['image', 'file', 'text'])
        expectReported(
          toastError,
          `Could not attach that ${what}`,
          'Crowbar could not reach the daemon — try again.',
        )
    })
  })

  it('prefers an image clipboard item over a files entry when both are present', () => {
    uploadChatAttachmentMock.mockResolvedValue({
      ref: 'chats/c1/attachments/x-pasted-image.png',
      filename: 'pasted-image.png',
      size: 10,
      contentType: 'image/png',
    })
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const imageFile = new File(['bytes'], 'pasted-image.png', { type: 'image/png' })
    const { event } = pasteEvent('', [imageItem(imageFile)], [imageFile])

    onPaste({ editor, event })

    expect(uploadChatAttachmentMock).toHaveBeenCalledTimes(1)
    expect(uploadAttachmentMarkdownMock).not.toHaveBeenCalled()
  })

  it('leaves a non-image file paste alone with the caret inside a real code block', () => {
    const { onPaste } = pastePluginHandlers()
    const editor = (() => {
      const e = editorWith('```js\nconsole.log(1)\n```')
      const idx = e.children.findIndex(
        (node) => (node as { type?: string }).type === CodeBlockPlugin.key,
      )
      const point = e.api.end([idx])
      e.selection = { anchor: point!, focus: point! }
      return e
    })()
    const file = new File(['bytes'], 'report.pdf', { type: 'application/pdf' })
    const { event, preventDefault } = pasteEvent('', [], [file])

    onPaste({ editor, event })

    expect(preventDefault).not.toHaveBeenCalled()
    expect(uploadAttachmentMarkdownMock).not.toHaveBeenCalled()
  })

  it('TestRegression_twoConsecutivePastesWithNoInterveningKeystrokeProduceTwoSeparateFences', () => {
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')

    const first = pasteEvent('x'.repeat(500))
    onPaste({ editor, event: first.event })
    const second = pasteEvent('y'.repeat(500))
    onPaste({ editor, event: second.event })

    expect(first.preventDefault).toHaveBeenCalledTimes(1)
    expect(second.preventDefault).toHaveBeenCalledTimes(1)

    const fences = (editor.children as { type?: string; lang?: string }[]).filter(
      (node) => node.type === 'code_block',
    )
    expect(fences).toHaveLength(2)
    const markdown = chatValueToMarkdown(editor.children as never)
    expect((markdown.match(/```text-attachment:/g) ?? []).length).toBe(2)
    expect(markdown).toContain('x'.repeat(500))
    expect(markdown).toContain('y'.repeat(500))
    expect(markdown).not.toContain('x'.repeat(500) + 'y')
    expect(markdown).not.toContain('y'.repeat(500) + 'x')
  })

  describe('caret inside an existing code block', () => {
    function editorInsideCodeBlock(): PlateEditor {
      const editor = editorWith('```js\nconsole.log(1)\n```')
      const idx = editor.children.findIndex(
        (node) => (node as { type?: string }).type === CodeBlockPlugin.key,
      )
      expect(idx).toBeGreaterThanOrEqual(0)
      const point = editor.api.end([idx])
      expect(point).toBeDefined()
      editor.selection = { anchor: point!, focus: point! }
      return editor
    }

    it('leaves an over-threshold text paste alone', () => {
      const { onPaste } = pastePluginHandlers()
      const editor = editorInsideCodeBlock()
      const before = editor.children
      const { event, preventDefault } = pasteEvent('x'.repeat(500))

      onPaste({ editor, event })

      expect(preventDefault).not.toHaveBeenCalled()
      expect(editor.children).toBe(before)
    })

    it('leaves an image paste alone', () => {
      const { onPaste } = pastePluginHandlers()
      const editor = editorInsideCodeBlock()
      const file = new File(['bytes'], 'a.png', { type: 'image/png' })
      const { event, preventDefault } = pasteEvent('', [imageItem(file)])

      onPaste({ editor, event })

      expect(preventDefault).not.toHaveBeenCalled()
      expect(uploadChatAttachmentMock).not.toHaveBeenCalled()
    })

    it('still treats a language-less fenced code block as real code, not an attachment fence', () => {
      const { onPaste } = pastePluginHandlers()
      const editor = editorWith('```\nplain\n```')
      const idx = editor.children.findIndex(
        (node) => (node as { type?: string }).type === CodeBlockPlugin.key,
      )
      expect(idx).toBeGreaterThanOrEqual(0)
      expect((editor.children[idx] as { lang?: unknown }).lang).toBeUndefined()
      const point = editor.api.end([idx])
      editor.selection = { anchor: point!, focus: point! }
      const { event, preventDefault } = pasteEvent('x'.repeat(500))

      onPaste({ editor, event })

      expect(preventDefault).not.toHaveBeenCalled()
    })
  })
})

describe('createChatPastePlugin, registered through the real ChatMarkdownEditor', () => {
  function renderWithDnd(ui: React.ReactElement) {
    return render(<DndScope>{ui}</DndScope>)
  }

  function firePaste(editable: HTMLElement, clipboardData: unknown) {
    Object.defineProperty(editable, 'isContentEditable', { value: true, configurable: true })
    const event = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'clipboardData', { value: clipboardData })
    act(() => {
      editable.dispatchEvent(event)
    })
  }

  it('uploads and inserts a real pasted image through the actually-mounted composer', async () => {
    let resolveUpload!: (value: {
      ref: string
      filename: string
      size: number
      contentType: string
    }) => void
    uploadChatAttachmentMock.mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve
      }),
    )
    const ref = createRef<ChatMarkdownEditorHandle>()
    const { container } = renderWithDnd(
      <ChatMarkdownEditor
        ref={ref}
        wsId="w1"
        chatId="c1"
        placeholder="Message the agent…"
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
        onKeyDown={() => {}}
      />,
    )
    const editable = container.querySelector('[data-slate-editor]') as HTMLElement
    expect(editable).toBeTruthy()
    await act(async () => {})

    const file = new File(['bytes'], 'pasted-image.png', { type: 'image/png' })
    firePaste(editable, {
      items: [{ type: 'image/png', kind: 'file', getAsFile: () => file }],
      files: [] as File[],
      getData: () => '',
    })

    expect(uploadChatAttachmentMock).toHaveBeenCalledWith('w1', 'c1', { file })

    await waitFor(() => {
      const preview = container.querySelector('img[alt="pasted-image.png"]') as HTMLImageElement
      expect(preview?.src).toMatch(/^blob:/)
    })

    resolveUpload({
      ref: 'chats/c1/attachments/x-pasted-image.png',
      filename: 'pasted-image.png',
      size: 10,
      contentType: 'image/png',
    })
    await waitFor(() => {
      const settled = container.querySelector('img[alt="pasted-image.png"]') as HTMLImageElement
      expect(settled.src).toContain('chats/c1/attachments/x-pasted-image.png')
    })
    expect(ref.current?.getMarkdown()).toBe(
      '![pasted-image.png](chats/c1/attachments/x-pasted-image.png)',
    )
  })

  it('does not intercept when wsId/chatId are missing, per the conditional in chat-markdown-editor.tsx', async () => {
    const { container } = renderWithDnd(
      <ChatMarkdownEditor
        placeholder="Message the agent…"
        ariaLabel="Message the agent"
        onShapeChange={vi.fn()}
        onKeyDown={() => {}}
      />,
    )
    const editable = container.querySelector('[data-slate-editor]') as HTMLElement
    expect(editable).toBeTruthy()
    await act(async () => {})

    const file = new File(['bytes'], 'pasted-image.png', { type: 'image/png' })
    firePaste(editable, {
      items: [{ type: 'image/png', kind: 'file', getAsFile: () => file }],
      files: [] as File[],
      getData: () => '',
    })

    expect(uploadChatAttachmentMock).not.toHaveBeenCalled()
  })
})
