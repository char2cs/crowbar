import { act, createElement, createRef, forwardRef, useImperativeHandle } from 'react'
import type { Ref } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AgentEmptyDocument,
  type AgentEmptyDocumentHandle,
  lastLineTop,
} from '@/features/agent/chat/agent-empty-document'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { saveExcalidrawDesign } from '@/features/agent/composer/lib/excalidraw-design-persistence'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'

// Same stand-in agent-chat-view.test.tsx uses: jsdom never delivers a keydown
// to a real Slate editable, so these tests are not about the editor's own
// behaviour (verified live and in its own suite) — only about the handle ref
// this file adds. Upgraded to a real forwardRef so the attachment-wiring
// tests below can observe what `insertAttachmentMarkdown` was called with,
// without needing a real Slate document.
const insertedMarkdown = vi.hoisted(() => [] as string[])
const insertedPendingImages = vi.hoisted(() => [] as [string, string][])
const settledPendingImages = vi.hoisted(() => [] as [string, string | null][])
vi.mock('@/features/agent/composer/plate/chat-markdown-editor', () => ({
  ChatMarkdownEditor: forwardRef(
    (
      _props: unknown,
      ref: Ref<{
        insertAttachmentMarkdown: (md: string) => void
        insertPendingImage: (objectUrl: string, alt: string) => void
        settlePendingImage: (objectUrl: string, finalMarkdown: string | null) => void
      }>,
    ) => {
      useImperativeHandle(ref, () => ({
        insertAttachmentMarkdown: (md: string) => {
          insertedMarkdown.push(md)
        },
        insertPendingImage: (objectUrl: string, alt: string) => {
          insertedPendingImages.push([objectUrl, alt])
        },
        settlePendingImage: (objectUrl: string, finalMarkdown: string | null) => {
          settledPendingImages.push([objectUrl, finalMarkdown])
        },
      }))
      return createElement('div', { 'data-testid': 'editor-stub' })
    },
  ),
}))

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(),
}))

// Task 27's own hook is a no-op under jsdom (`isTauri()` is false) — stubbed
// so the Tauri-drop tests below can invoke its `onDrop` callback directly,
// same precedent as agent-composer.test.tsx.
vi.mock('@/features/file-system/lib/tauri-file-drop', () => ({
  useTauriFileDrop: vi.fn(),
}))

// The real canvas mounts `@excalidraw/excalidraw`, which jsdom can't host —
// mocked the same way agent-composer.test.tsx does, capturing `onSave` so a
// test can trigger it directly.
let latestExcalidrawOnSave: ((result: { sceneJson: string; pngFile: File }) => void) | null = null
let latestExcalidrawInitialScene:
  { elements: unknown[]; appState: Record<string, unknown> } | undefined
vi.mock('@/features/agent/composer/excalidraw-canvas', () => ({
  ExcalidrawCanvas: ({
    onSave,
    initialScene,
  }: {
    onSave: (result: { sceneJson: string; pngFile: File }) => void
    initialScene?: { elements: unknown[]; appState: Record<string, unknown> }
  }) => {
    latestExcalidrawOnSave = onSave
    latestExcalidrawInitialScene = initialScene
    return createElement('div', { 'data-testid': 'excalidraw-canvas-mock' })
  },
}))

const toastError = vi.hoisted(() => vi.fn())
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

beforeEach(() => {
  insertedMarkdown.length = 0
  insertedPendingImages.length = 0
  settledPendingImages.length = 0
  vi.mocked(uploadChatAttachment).mockReset()
  vi.mocked(useTauriFileDrop).mockClear()
  toastError.mockClear()
  latestExcalidrawOnSave = null
  latestExcalidrawInitialScene = undefined
  localStorage.clear()
})

afterEach(() => {
  latestExcalidrawOnSave = null
  latestExcalidrawInitialScene = undefined
})

function draw(overrides: Partial<Parameters<typeof AgentEmptyDocument>[0]> = {}) {
  const ref = createRef<AgentEmptyDocumentHandle>()
  const view = render(
    <AgentEmptyDocument
      ref={ref}
      draft=""
      draftSeed={0}
      hasText={false}
      onDraftChange={vi.fn()}
      onSubmit={vi.fn()}
      onKeyDown={vi.fn()}
      controls={null}
      working={false}
      canStop={false}
      sending={false}
      onStop={vi.fn()}
      {...overrides}
    />,
  )
  return { ...view, ref }
}

describe('AgentEmptyDocument handle', () => {
  it('reports the handle’s own on-screen rect', () => {
    const { ref, container } = draw()

    const handle = container.querySelector('.dochandle') as HTMLElement
    const rect = { top: 84, left: 0, right: 0, bottom: 0, width: 0, height: 0 } as DOMRect
    vi.spyOn(handle, 'getBoundingClientRect').mockReturnValue(rect)

    expect(ref.current?.getHandleRect()).toBe(rect)
  })

  // A caller reading this off an unmounted instance gets "nothing to arrive
  // from", never a thrown error or a stale rect.
  it('reports null once unmounted', () => {
    const { ref, unmount } = draw()
    unmount()

    expect(ref.current).toBeNull()
  })
})

// REGRESSION: `place()` used to position the handle at the CARET's line when
// one existed in the document, and only fell back to the last line once the
// document lost focus entirely — so clicking back into an earlier sentence
// to fix a word dragged the send button up the page with the caret. It must
// always sit under the last written line, full stop.
describe('lastLineTop', () => {
  function docWithParagraphs(bottoms: number[], docTop = 0): HTMLDivElement {
    const doc = document.createElement('div')
    vi.spyOn(doc, 'getBoundingClientRect').mockReturnValue({ top: docTop } as DOMRect)
    const editable = document.createElement('div')
    editable.setAttribute('data-slate-editor', 'true')
    for (const bottom of bottoms) {
      const p = document.createElement('p')
      p.textContent = 'text'
      vi.spyOn(p, 'getBoundingClientRect').mockReturnValue({ bottom } as DOMRect)
      editable.appendChild(p)
    }
    doc.appendChild(editable)
    return doc
  }

  it('sits under the LAST paragraph, not the first, when there is more than one', () => {
    expect(lastLineTop(docWithParagraphs([100, 250]))).toBe(250)
  })

  it('is measured relative to the doc, not the viewport', () => {
    expect(lastLineTop(docWithParagraphs([250], 40))).toBe(210)
  })

  it('falls back to the first-line position on a genuinely empty document', () => {
    expect(lastLineTop(docWithParagraphs([]))).toBeCloseTo(48 + 27.2)
  })

  it('falls back to the first-line position when the editor holds no text at all', () => {
    const doc = document.createElement('div')
    vi.spyOn(doc, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect)
    const editable = document.createElement('div')
    editable.setAttribute('data-slate-editor', 'true')
    editable.appendChild(document.createElement('p')) // present, but empty
    doc.appendChild(editable)

    expect(lastLineTop(doc)).toBeCloseTo(48 + 27.2)
  })

  it('ignores where the caret actually is — a selection anchored in an earlier line does not move it', () => {
    const doc = docWithParagraphs([100, 250])
    document.body.appendChild(doc)
    try {
      const editable = doc.querySelector('[data-slate-editor]') as HTMLElement
      const firstParagraph = editable.firstElementChild as HTMLElement
      const range = document.createRange()
      range.selectNodeContents(firstParagraph)
      const selection = window.getSelection()
      selection?.removeAllRanges()
      selection?.addRange(range)

      expect(lastLineTop(doc)).toBe(250)
    } finally {
      document.body.removeChild(doc)
    }
  })
})

describe('AgentEmptyDocument stop control', () => {
  // REGRESSION: this surface hand-duplicates composer-handle.tsx's own
  // send/stop button, and used to gate `stopping` on the document being
  // empty the same way — hiding the only way to interrupt a turn already
  // running (a background handoff can start one before anything is typed)
  // the instant a person started writing.
  it('stops a running turn even with text already in the document', () => {
    const onStop = vi.fn()
    draw({ draft: 'a follow-up thought', hasText: true, working: true, canStop: true, onStop })

    const button = screen.getByRole('button', { name: 'Stop this turn' })
    expect(button).toBeEnabled()
    expect(button.className).toMatch(/\bhalt\b/)
    fireEvent.click(button)
    expect(onStop).toHaveBeenCalled()
  })

  it('sends the document when there is no running turn to stop', () => {
    const onSubmit = vi.fn()
    draw({ draft: 'the whole plan', hasText: true, onSubmit })

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeEnabled()
    fireEvent.click(button)
    expect(onSubmit).toHaveBeenCalled()
  })
})

// REGRESSION: this surface's send button hand-duplicates composer-handle.tsx's
// own, but had no `sending` state at all — the FIRST message in a chat clears
// the document and shows nothing while its own dispatch is in flight, where
// every later message (composer-handle.tsx) gets a spinner. Live-verified via
// a MutationObserver on the real button: the dock's spinner appears and
// disappears cleanly; this surface's button never once changed class before
// being replaced.
// REGRESSION: `draft` only carries what the box was last OPENED with — a
// remount seed, not what's actually typed. The send button used to derive
// its enabled/disabled state from THAT prop directly, so on a chat's first
// message (this surface) the button stayed permanently disabled and stuck
// in its muted "off" styling through an entire ordinary send, because
// nothing ever pushes real keystrokes back into the seed. `hasText`, tracked
// live from the box's own onChange, is what the button must key off instead.
describe('AgentEmptyDocument hasText tracking', () => {
  it('stays disabled while the seed still holds stale text but nothing was actually typed', () => {
    draw({ draft: 'text pushed in from outside', hasText: false })

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeDisabled()
    expect(button.className).toMatch(/\boff\b/)
  })

  it('enables once real typing is reported via hasText, even though the seed itself is empty', () => {
    draw({ draft: '', hasText: true })

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeEnabled()
    expect(button.className).not.toMatch(/\boff\b/)
  })
})

describe('AgentEmptyDocument sending state', () => {
  it('shows a sending spinner once dispatched but not yet proven delivered, same as the dock composer', () => {
    const { container } = draw({ sending: true })

    const button = screen.getByRole('button', { name: 'Sending' })
    expect(button).toBeDisabled()
    expect(button.className).toMatch(/\boff\b/)
    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('prefers the stop control over the sending spinner when both apply', () => {
    draw({ working: true, canStop: true, sending: true })

    expect(screen.getByRole('button', { name: 'Stop this turn' })).toBeInTheDocument()
    expect(screen.queryByRole('status')).toBeNull()
  })
})

// Finding I5: the blank-chat surface gets the same plus-button + modals +
// drop wiring as the composer (agent-composer.tsx), via the shared
// `useAttachmentUpload` hook — not a third copy-paste.
describe('AgentEmptyDocument attachments', () => {
  it('renders no plus button when wsId/chatId are not supplied', () => {
    draw()

    expect(screen.queryByRole('button', { name: /add to this message/i })).toBeNull()
  })

  it('opens the attach-file modal from the plus button and inserts uploaded markdown into the box', async () => {
    const user = userEvent.setup()
    vi.mocked(uploadChatAttachment).mockResolvedValueOnce({
      ref: 'chats/c1/attachments/x-notes.txt',
      filename: 'notes.txt',
      size: 5,
      contentType: 'text/plain',
    })
    draw({ wsId: 'w1', chatId: 'c1' })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /attach file/i }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    fireEvent.change(input, {
      target: { files: [new File(['hi'], 'notes.txt', { type: 'text/plain' })] },
    })

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(insertedMarkdown).toContain('[notes.txt](chats/c1/attachments/x-notes.txt)')
  })

  it('opens the excalidraw modal from the plus button and inserts both markdown blocks', async () => {
    const user = userEvent.setup()
    vi.mocked(uploadChatAttachment).mockResolvedValueOnce({
      ref: 'chats/c1/attachments/x-diagram.png',
      filename: 'x-diagram.png',
      size: 10,
      contentType: 'image/png',
    })
    draw({ wsId: 'w1', chatId: 'c1' })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))
    await screen.findByTestId('excalidraw-canvas-mock')

    const pngFile = new File(['x'], 'diagram.png', { type: 'image/png' })
    await act(async () => {
      await latestExcalidrawOnSave?.({ sceneJson: '{"elements":[]}', pngFile })
    })

    await waitFor(() => expect(screen.queryByTestId('excalidraw-takeover')).toBeNull())
    expect(insertedMarkdown.some((md) => md.includes('excalidraw:'))).toBe(true)
    expect(insertedMarkdown.at(-1)).toContain('![diagram](chats/c1/attachments/x-diagram.png)')
  })

  it('preloads the excalidraw takeover with this chat’s previously saved local design', async () => {
    saveExcalidrawDesign('w1', 'c1', '{"elements":[{"id":"saved"}],"appState":{}}')
    const user = userEvent.setup()
    draw({ wsId: 'w1', chatId: 'c1' })

    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))
    await screen.findByTestId('excalidraw-canvas-mock')

    expect(latestExcalidrawInitialScene).toEqual({ elements: [{ id: 'saved' }], appState: {} })
  })

  it('uploads a browser-dropped file over the document and inserts markdown, toggling the drop-target class', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValueOnce({
      ref: 'chats/c1/attachments/x-a.png',
      filename: 'a.png',
      size: 10,
      contentType: 'image/png',
    })
    const { container } = draw({ wsId: 'w1', chatId: 'c1' })
    const wrap = container.querySelector('.docwrap')!
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    fireEvent.dragOver(wrap, { dataTransfer: { types: ['Files'] } })
    expect(wrap).toHaveClass('drop-target')

    fireEvent.drop(wrap, { dataTransfer: { types: ['Files'], files: [file] } })

    expect(wrap).not.toHaveClass('drop-target')
    // An image file takes the optimistic path (chat-markdown-image-node.tsx)
    // now — insertedMarkdown (the non-optimistic `insertAttachmentMarkdown`
    // call) never fires for this one.
    expect(insertedPendingImages.map(([, alt]) => alt)).toContain('a.png')
    await waitFor(() =>
      expect(settledPendingImages.map(([, markdown]) => markdown)).toContain(
        '![a.png](chats/c1/attachments/x-a.png)',
      ),
    )
  })

  it('clears the drop-target state on drag-leave, unless it lands on a still-contained child', () => {
    const { container } = draw({ wsId: 'w1', chatId: 'c1' })
    const wrap = container.querySelector('.docwrap')!

    fireEvent.dragOver(wrap, { dataTransfer: { types: ['Files'] } })
    expect(wrap).toHaveClass('drop-target')

    fireEvent.dragLeave(wrap)
    expect(wrap).not.toHaveClass('drop-target')

    // Same guard agent-composer.tsx's own pill drag-leave uses: a leave that
    // lands on a target the wrapper still contains is not really a leave.
    fireEvent.dragOver(wrap, { dataTransfer: { types: ['Files'] } })
    expect(wrap).toHaveClass('drop-target')
    const leave = new Event('dragleave', { bubbles: true, cancelable: false })
    Object.defineProperty(leave, 'relatedTarget', { value: wrap, configurable: true })
    fireEvent(wrap, leave)
    expect(wrap).toHaveClass('drop-target')
  })

  it('ignores a drag-over and a drop that carry no files', () => {
    const { container } = draw({ wsId: 'w1', chatId: 'c1' })
    const wrap = container.querySelector('.docwrap')!

    fireEvent.dragOver(wrap, { dataTransfer: { types: ['text/plain'] } })
    expect(wrap).not.toHaveClass('drop-target')

    fireEvent.drop(wrap, { dataTransfer: { types: ['text/plain'], files: [] } })
    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })

  // The real Tauri desktop path (Task 27): a native OS drop never reaches the
  // DOM as a `drop` event, only as this hook's own callback.
  it('uploads each Tauri-reported host path dropped on the document', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValueOnce({
      ref: 'chats/c1/attachments/z-c.txt',
      filename: 'c.txt',
      size: 5,
      contentType: 'text/plain',
    })
    draw({ wsId: 'w1', chatId: 'c1' })

    const onDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
    expect(onDrop).toBeTypeOf('function')
    await act(async () => {
      onDrop?.(['/Users/me/c.txt'])
    })

    expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', { path: '/Users/me/c.txt' })
    await waitFor(() => expect(insertedMarkdown).toContain('[c.txt](chats/c1/attachments/z-c.txt)'))
  })

  // Without both ids there is nothing safe to upload to — the drop handlers
  // must no-op rather than call the daemon with an empty wsId/chatId.
  it('does not upload a browser-dropped file when wsId/chatId are missing', () => {
    const { container } = draw()
    const wrap = container.querySelector('.docwrap')!
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    fireEvent.drop(wrap, { dataTransfer: { types: ['Files'], files: [file] } })

    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })

  // Finding C3: a well-formed, small CSV dropped here resolves inline as a
  // markdown table too — the same shared hook agent-composer.tsx and
  // attach-file-modal.tsx use.
  it('resolves a small, well-formed dropped CSV inline as a markdown table, without uploading', async () => {
    const { container } = draw({ wsId: 'w1', chatId: 'c1' })
    const wrap = container.querySelector('.docwrap')!
    const file = new File(['name,age\nAda,36\n'], 'people.csv', { type: 'text/csv' })

    fireEvent.drop(wrap, { dataTransfer: { types: ['Files'], files: [file] } })

    await waitFor(() =>
      expect(insertedMarkdown).toContain('| name | age |\n| --- | --- |\n| Ada | 36 |'),
    )
    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })
})
