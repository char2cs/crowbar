import { waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import type { PlateEditor } from 'platejs/react'
import { CodeBlockPlugin } from '@platejs/code-block/react'
import { createChatPastePlugin } from '@/features/agent/composer/plate/chat-paste-plugin'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { INLINE_ATTACHMENT_MAX_BYTES } from '@/features/agent/composer/lib/inline-attachment-cap'

const uploadChatAttachmentMock = vi.hoisted(() => vi.fn())
vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: uploadChatAttachmentMock,
}))

const uploadAttachmentMarkdownMock = vi.hoisted(() => vi.fn())
vi.mock('@/features/agent/composer/lib/attachment-upload', () => ({
  uploadAttachmentMarkdown: uploadAttachmentMarkdownMock,
}))

const toastError = vi.hoisted(() => vi.fn())
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

// Same pattern chat-markdown-editor.test.tsx uses for its own key-handling
// plugin, for the same reason: jsdom does not implement `isContentEditable`
// (it is `undefined`, not `true`/`false`), which is exactly what slate-dom's
// `hasEditableTarget` checks before delivering a real `paste`/`keydown` DOM
// event to ANY handler — Slate's own default paste handling included. A
// `fireEvent.paste` fired at the mounted editable therefore proves nothing
// here (verified directly: neither this plugin's handler nor Slate's own
// `ReactEditor.insertData` fallback ever ran). Capturing the raw config
// `createPlatePlugin` is handed and calling its `handlers` directly is the
// only reliable way to exercise this plugin under jsdom.
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
  toastError.mockClear()
})

type ClipboardItemStub = { type: string; kind: string; getAsFile: () => File | null }
type PasteEventStub = {
  clipboardData: { getData: (type: string) => string; items: ClipboardItemStub[] } | null
  preventDefault: () => void
}
type KeyEventStub = { key: string }
type PastePluginHandlers = {
  onKeyDown: (ctx: { event: KeyEventStub }) => void
  onKeyUp: (ctx: { event: KeyEventStub }) => void
  onPaste: (ctx: { editor: PlateEditor; event: PasteEventStub }) => void
}

/** Builds the plugin exactly as `chat-markdown-editor.tsx` does, and returns
 *  its REAL (unmodified) `handlers` — captured from the literal config object
 *  handed to `createPlatePlugin`, not re-implemented. */
function pastePluginHandlers(wsId = 'w1', chatId = 'c1'): PastePluginHandlers {
  createChatPastePlugin({ wsId, chatId })
  const call = createPlatePluginSpy.mock.calls.find(
    ([config]) => (config as { key?: string }).key === 'agent-chat-paste',
  )
  if (!call) throw new Error('agent-chat-paste plugin was never created')
  return (call[0] as { handlers: PastePluginHandlers }).handlers
}

/** A headless editor built from the SAME plugin set the real component uses
 *  — `chat-markdown-editor.test.tsx`'s own `editorWith` pattern. */
function editorWith(markdown: string): PlateEditor {
  return createPlateEditor({ plugins: chatComposerPlugins, value: chatMarkdownToValue(markdown) })
}

function pasteEvent(text: string, items: ClipboardItemStub[] = []) {
  const preventDefault = vi.fn()
  const event: PasteEventStub = {
    clipboardData: { getData: () => text, items },
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

    onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(chatValueToMarkdown(editor.children as never)).toContain('```text-attachment:')
  })

  // Finding I4: an over-threshold paste that ALSO exceeds the shared inline
  // size cap must not become an unsendable inline fence — it uploads as a
  // `.txt` file instead.
  it('uploads an over-cap pasted text as a file instead of wrapping it inline', async () => {
    uploadAttachmentMarkdownMock.mockResolvedValue(
      '[pasted.txt](chats/c1/attachments/x-pasted.txt)',
    )
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const hugeText = 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1)
    const { event, preventDefault } = pasteEvent(hugeText)

    onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
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
    // The oversized raw text never lands as an inline fence.
    expect(chatValueToMarkdown(editor.children as never)).not.toContain('text-attachment:')
  })

  it('surfaces a failed over-cap text upload as a toast instead of silently doing nothing', async () => {
    uploadAttachmentMarkdownMock.mockRejectedValueOnce(new Error('413 Payload Too Large'))
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const hugeText = 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1)
    const { event } = pasteEvent(hugeText)

    onPaste({ editor, event })

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        'Could not attach that text',
        '413 Payload Too Large',
      ),
    )
  })

  it('falls back to a generic description for a non-Error over-cap upload rejection', async () => {
    uploadAttachmentMarkdownMock.mockRejectedValueOnce('boom')
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const hugeText = 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES + 1)
    const { event } = pasteEvent(hugeText)

    onPaste({ editor, event })

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        'Could not attach that text',
        'Crowbar could not reach the daemon — try again.',
      ),
    )
  })

  // Defensive-only fallback (`clipboard?.items ?? []`, `clipboard?.getData
  // (...) ?? ''`): some paste events genuinely carry no `clipboardData` at
  // all. Neither an image nor an over-threshold paste can be detected from
  // nothing, so this falls through to Slate's own default handling exactly
  // like a short plain-text paste does.
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

    onPaste({ editor, event })

    expect(preventDefault).not.toHaveBeenCalled()
    expect(editor.children).toBe(before)
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

  // The key handlers gate on the KEY, not merely on "a key happened" — an
  // unrelated key must not leave `shiftHeld` somehow stuck on.
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

  // Releasing Shift re-arms interception — proves `onKeyUp` actually clears
  // the flag rather than it staying "sticky" for the rest of the session.
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

    onPaste({ editor, event })

    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(uploadChatAttachmentMock).toHaveBeenCalledWith('w1', 'c1', { file })
    await waitFor(() => {
      expect(chatValueToMarkdown(editor.children as never)).toContain(
        '![pasted-image.png](chats/c1/attachments/x-pasted-image.png)',
      )
    })
  })

  // The brief's own reference guard: a clipboard item can claim `image/*`
  // and still yield no File (a mid-drag clipboard, a revoked permission).
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

  // Risk class 1 (this task's brief): an unhandled upload rejection used to
  // be a silent no-op mid-typing. Caught and toasted instead, the same
  // precedent agent-composer.tsx's own `uploadAndInsert` already set.
  it('surfaces a failed image upload as a toast instead of silently doing nothing', async () => {
    uploadChatAttachmentMock.mockRejectedValueOnce(new Error('413 Payload Too Large'))
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'huge.png', { type: 'image/png' })
    const { event } = pasteEvent('', [imageItem(file)])

    onPaste({ editor, event })

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        'Could not attach that image',
        '413 Payload Too Large',
      ),
    )
  })

  it('falls back to a generic description for a non-Error rejection', async () => {
    uploadChatAttachmentMock.mockRejectedValueOnce('boom')
    const { onPaste } = pastePluginHandlers()
    const editor = editorWith('')
    const file = new File(['bytes'], 'huge.png', { type: 'image/png' })
    const { event } = pasteEvent('', [imageItem(file)])

    onPaste({ editor, event })

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        'Could not attach that image',
        'Crowbar could not reach the daemon — try again.',
      ),
    )
  })

  // Wave 6, Bug 2 (live-reproduced): two over-threshold pastes back to back,
  // with no intervening keystroke, merged into a SINGLE text-attachment
  // fence instead of producing two. Root cause: `insertAttachmentMarkdownInto`
  // leaves the selection sitting inside the fence's own `code_line` right
  // after inserting it, so this plugin's OWN "caret inside a code block"
  // bypass (previously a bare `{ type: CodeBlockPlugin.key }` match) treated
  // the second paste as "the user is editing real code, leave it alone" and
  // let it fall through to Slate's default handling, which appended the
  // second paste's text into the SAME fence. Fixed by `isRealCodeBlock`
  // narrowing that match to exclude the plugin's own attachment fences
  // (identified by their `lang` prefix) — a caret left inside one of THOSE
  // now falls through to this plugin's normal threshold logic instead, whose
  // existing `insertAttachmentMarkdownInto` redirect (proven by
  // `TestRegression_insertsTwoFencedAttachmentsAsSeparateSiblingBlocks` in
  // chat-markdown-editor.test.tsx) correctly appends the new fence as a
  // sibling of the first rather than corrupting it.
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
    // Each fence holds ONLY its own paste — no merged/duplicated content.
    expect(markdown).toContain('x'.repeat(500))
    expect(markdown).toContain('y'.repeat(500))
    expect(markdown).not.toContain('x'.repeat(500) + 'y')
    expect(markdown).not.toContain('y'.repeat(500) + 'x')
  })

  describe('caret inside an existing code block', () => {
    /** A headless editor whose selection sits inside a real `code_block`
     *  node, built from a genuine fenced-code paste rather than a hand-built
     *  node shape. */
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

    // `isRealCodeBlock`'s `lang` read has to fall back for a fence with NO
    // language marker at all (a bare ``` ``` ```, `lang: undefined`) — a
    // genuine, unremarkable real code block, and one Plate's own markdown
    // deserializer produces routinely. Must bypass exactly like a `js` fence
    // does, not be mistaken for one of THIS plugin's own (always-`lang`-
    // prefixed) attachment fences.
    it('still treats a language-less fenced code block as real code, not an attachment fence', () => {
      const { onPaste } = pastePluginHandlers()
      const editor = editorWith('```\nplain\n```')
      const idx = editor.children.findIndex(
        (node) => (node as { type?: string }).type === CodeBlockPlugin.key,
      )
      expect(idx).toBeGreaterThanOrEqual(0)
      // The deserializer leaves a language-less fence's `lang` unset —
      // `isRealCodeBlock` must not choke on that, only on an ACTUAL
      // attachment-fence prefix.
      expect((editor.children[idx] as { lang?: unknown }).lang).toBeUndefined()
      const point = editor.api.end([idx])
      editor.selection = { anchor: point!, focus: point! }
      const { event, preventDefault } = pasteEvent('x'.repeat(500))

      onPaste({ editor, event })

      expect(preventDefault).not.toHaveBeenCalled()
    })
  })
})
