import { forwardRef, useImperativeHandle, type Ref } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  uploadChatAttachment,
  type UploadedChatAttachment,
} from '@/features/agent/api/upload-chat-attachment'
import type {
  ExcalidrawCanvasHandle,
  ExcalidrawSaveResult,
} from '@/features/agent/composer/excalidraw-canvas'
import { ExcalidrawTakeover } from '@/features/agent/composer/excalidraw-takeover'
import { INLINE_ATTACHMENT_MAX_BYTES } from '@/features/agent/composer/lib/inline-attachment-cap'
import { loadExcalidrawDesign } from '@/features/agent/composer/lib/excalidraw-design-persistence'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'

// The mocked canvas exposes `save()` via the same imperative handle the real
// one does, invoking `onSave` with whatever this file's `saveResult` holds at
// the time — this file only exercises the TAKEOVER's own wiring (the header's
// Attach button, id-minting, upload, insert-markdown, local persistence,
// chrome), which is everything a real Attach click would trigger regardless
// of what's drawn.
let latestInitialScene: ParsedExcalidrawScene | undefined
let saveResult: ExcalidrawSaveResult = {
  sceneJson: '{"elements":[],"appState":{}}',
  pngFile: new File(['x'], 'diagram.png', { type: 'image/png' }),
}
// Every existing test in this file wants Attach immediately usable, matching
// the real component's own near-instant local init in practice — only the
// dedicated "not ready yet" test below sets this false.
let mockReady = true

vi.mock('@/features/agent/composer/excalidraw-canvas', () => ({
  ExcalidrawCanvas: forwardRef(
    (
      {
        onSave,
        initialScene,
        onReady,
      }: {
        onSave: (result: ExcalidrawSaveResult) => void | Promise<void>
        initialScene?: ParsedExcalidrawScene
        onReady?: () => void
      },
      ref: Ref<ExcalidrawCanvasHandle>,
    ) => {
      latestInitialScene = initialScene
      if (mockReady) onReady?.()
      useImperativeHandle(ref, () => ({
        save: async () => {
          await onSave(saveResult)
        },
      }))
      return <div data-testid="excalidraw-canvas" />
    },
  ),
}))

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(async () => ({
    ref: 'chats/c1/attachments/x-diagram.png',
    filename: 'x-diagram.png',
    size: 10,
    contentType: 'image/png',
  })),
}))

const toastError = vi.fn()
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...args: unknown[]) => toastError(...args) },
}))

beforeEach(() => {
  localStorage.clear()
  saveResult = {
    sceneJson: '{"elements":[],"appState":{}}',
    pngFile: new File(['x'], 'diagram.png', { type: 'image/png' }),
  }
  mockReady = true
})

afterEach(() => {
  cleanup()
  latestInitialScene = undefined
  vi.clearAllMocks()
})

async function renderOpenTakeover(
  props: Partial<{
    wsId: string
    chatId: string
    initialScene: ParsedExcalidrawScene
  }> = {},
) {
  const onInsertMarkdown = vi.fn()
  const onClose = vi.fn()
  render(
    <ExcalidrawTakeover
      wsId={props.wsId ?? 'w1'}
      chatId={props.chatId ?? 'c1'}
      open
      onClose={onClose}
      onInsertMarkdown={onInsertMarkdown}
      initialScene={props.initialScene}
    />,
  )
  await waitFor(() => expect(screen.getByTestId('excalidraw-canvas')).toBeInTheDocument())
  return { onInsertMarkdown, onClose }
}

function clickAttach() {
  fireEvent.click(screen.getByRole('button', { name: /^attach/i }))
}

describe('ExcalidrawTakeover', () => {
  it('lazily mounts the canvas only once opened', async () => {
    await renderOpenTakeover()
  })

  it('renders nothing when closed', () => {
    render(
      <ExcalidrawTakeover
        wsId="w1"
        chatId="c1"
        open={false}
        onClose={vi.fn()}
        onInsertMarkdown={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('excalidraw-canvas')).not.toBeInTheDocument()
  })

  // The whole point of trading the modal for this: a full-pane takeover, not
  // a centered box floating over the still-visible transcript.
  it('renders as a full-pane takeover, not a centered dialog', async () => {
    await renderOpenTakeover()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByTestId('excalidraw-takeover')).toBeInTheDocument()
  })

  it('closes when the close button is clicked, without saving/uploading/inserting anything', async () => {
    const { onClose, onInsertMarkdown } = await renderOpenTakeover()
    fireEvent.click(screen.getByRole('button', { name: /close/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(uploadChatAttachment).not.toHaveBeenCalled()
    expect(onInsertMarkdown).not.toHaveBeenCalled()
  })

  // REGRESSION, reported live: a global Escape listener closed the takeover
  // out from under Excalidraw's OWN use of the same key (deselecting a
  // shape, leaving text-edit mode) — Escape now does nothing here at all.
  it('does nothing on Escape', async () => {
    const { onClose } = await renderOpenTakeover()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
    expect(screen.getByTestId('excalidraw-takeover')).toBeInTheDocument()
  })

  it('preloads the canvas with a given initialScene', async () => {
    const scene: ParsedExcalidrawScene = { elements: [{ type: 'ellipse' }], appState: {} }
    await renderOpenTakeover({ initialScene: scene })
    expect(latestInitialScene).toEqual(scene)
  })

  it('uses the SAME id for the fence-tag JSON and the uploaded PNG filename', async () => {
    const { onInsertMarkdown } = await renderOpenTakeover()

    clickAttach()

    await waitFor(() => expect(uploadChatAttachment).toHaveBeenCalledTimes(1))
    const [, , , uploadedId] = vi.mocked(uploadChatAttachment).mock.calls[0]
    expect(uploadedId).toBeTruthy()

    await waitFor(() => expect(onInsertMarkdown).toHaveBeenCalledTimes(2))
    const [fenceMarkdown] = onInsertMarkdown.mock.calls[0]
    const [imageMarkdownCall] = onInsertMarkdown.mock.calls[1]
    expect(fenceMarkdown).toContain(`excalidraw:${uploadedId}`)
    expect(imageMarkdownCall).toContain('x-diagram.png')
  })

  it('uploads the scene as a file once it exceeds the shared inline size cap, but still inserts the PNG preview', async () => {
    vi.mocked(uploadChatAttachment)
      .mockResolvedValueOnce({
        ref: 'chats/c1/attachments/x-diagram.png',
        filename: 'x-diagram.png',
        size: 10,
        contentType: 'image/png',
      })
      .mockResolvedValueOnce({
        ref: 'chats/c1/attachments/y-diagram.excalidraw.json',
        filename: 'diagram.excalidraw.json',
        size: 99999,
        contentType: 'application/json',
      })
    saveResult = {
      sceneJson: JSON.stringify({
        elements: [],
        appState: {},
        pad: 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES),
      }),
      pngFile: new File(['x'], 'diagram.png', { type: 'image/png' }),
    }
    const { onInsertMarkdown } = await renderOpenTakeover()

    clickAttach()

    await waitFor(() => expect(uploadChatAttachment).toHaveBeenCalledTimes(2))
    const [, , secondInput] = vi.mocked(uploadChatAttachment).mock.calls[1]
    expect((secondInput as { file: File }).file.name).toBe('diagram.excalidraw.json')

    await waitFor(() => expect(onInsertMarkdown).toHaveBeenCalledTimes(2))
    const [sceneMarkdownCall] = onInsertMarkdown.mock.calls[0]
    const [imageMarkdownCall] = onInsertMarkdown.mock.calls[1]
    expect(sceneMarkdownCall).toBe(
      '[diagram.excalidraw.json](chats/c1/attachments/y-diagram.excalidraw.json)',
    )
    expect(sceneMarkdownCall).not.toContain('excalidraw:')
    expect(imageMarkdownCall).toContain('x-diagram.png')
  })

  it('keeps the inline excalidraw fence for a scene exactly at the size cap', async () => {
    const overhead = JSON.stringify({ elements: [], appState: {}, pad: '' }).length
    const sceneJson = JSON.stringify({
      elements: [],
      appState: {},
      pad: 'x'.repeat(INLINE_ATTACHMENT_MAX_BYTES - overhead),
    })
    expect(new TextEncoder().encode(sceneJson).byteLength).toBe(INLINE_ATTACHMENT_MAX_BYTES)
    saveResult = { sceneJson, pngFile: new File(['x'], 'diagram.png', { type: 'image/png' }) }
    const { onInsertMarkdown } = await renderOpenTakeover()

    clickAttach()

    await waitFor(() => expect(uploadChatAttachment).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(onInsertMarkdown).toHaveBeenCalled())
    const [fenceMarkdown] = onInsertMarkdown.mock.calls[0]
    expect(fenceMarkdown).toContain('excalidraw:')
  })

  it('shows a toast and inserts nothing when the upload fails', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('daemon unreachable'))
    const { onInsertMarkdown, onClose } = await renderOpenTakeover()

    clickAttach()

    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1))
    expect(toastError.mock.calls[0][1]).toContain('daemon unreachable')
    expect(onInsertMarkdown).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('stringifies a non-Error rejection for the toast', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce('413')
    await renderOpenTakeover()

    clickAttach()

    await waitFor(() => expect(toastError).toHaveBeenCalled())
    expect(toastError.mock.calls[0][1]).toBe('413')
  })

  // REGRESSION: this is the actual point of saving locally — the next time
  // either entry point opens for this chat, it should resume this design
  // instead of starting blank.
  it('saves the scene to this chat’s local design slot on a successful attach', async () => {
    saveResult = {
      sceneJson: '{"elements":["a"],"appState":{}}',
      pngFile: new File(['x'], 'diagram.png', { type: 'image/png' }),
    }
    await renderOpenTakeover({ wsId: 'w1', chatId: 'c1' })

    clickAttach()

    await waitFor(() =>
      expect(loadExcalidrawDesign('w1', 'c1')).toBe('{"elements":["a"],"appState":{}}'),
    )
  })

  it('does not touch the local design slot when the upload fails', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('daemon unreachable'))
    await renderOpenTakeover({ wsId: 'w1', chatId: 'c1' })

    clickAttach()

    await waitFor(() => expect(toastError).toHaveBeenCalled())
    expect(loadExcalidrawDesign('w1', 'c1')).toBeNull()
  })

  // The header replaces a bottom toolbar the canvas used to own (removed —
  // reported live as space the canvas should have had instead), so the
  // in-flight guard that used to live there moves here with it.
  it('disables Attach while a save is in flight, so a fast second click cannot fire it twice', async () => {
    let resolveUpload!: (value: UploadedChatAttachment) => void
    vi.mocked(uploadChatAttachment).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveUpload = resolve
        }),
    )
    const { onClose } = await renderOpenTakeover()

    clickAttach()

    await waitFor(() => expect(screen.getByRole('button', { name: /attaching/i })).toBeDisabled())
    fireEvent.click(screen.getByRole('button', { name: /attaching/i }))
    expect(uploadChatAttachment).toHaveBeenCalledTimes(1)

    resolveUpload({
      ref: 'chats/c1/attachments/x-diagram.png',
      filename: 'x-diagram.png',
      size: 10,
      contentType: 'image/png',
    })
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  })

  // REGRESSION, reported by review: the canvas's own async init (real,
  // since it's lazy-loaded) can leave a window where its imperative save()
  // silently no-ops (no api yet) — a click there looked like a hang, not a
  // disabled control. Attach must stay disabled until the canvas actually
  // signals it has something to export.
  it('disables Attach until the canvas signals it is ready, so a click cannot silently no-op', async () => {
    mockReady = false
    await renderOpenTakeover()

    expect(screen.getByRole('button', { name: /^attach/i })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /^attach/i }))
    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })

  it('also disables Close while a save is in flight', async () => {
    let resolveUpload!: (value: UploadedChatAttachment) => void
    vi.mocked(uploadChatAttachment).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveUpload = resolve
        }),
    )
    const { onClose } = await renderOpenTakeover()

    clickAttach()

    await waitFor(() => expect(screen.getByRole('button', { name: /close/i })).toBeDisabled())
    resolveUpload({
      ref: 'chats/c1/attachments/x-diagram.png',
      filename: 'x-diagram.png',
      size: 10,
      contentType: 'image/png',
    })
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  })
})
