import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import type { ExcalidrawSaveResult } from '@/features/agent/composer/excalidraw-canvas'
import { ExcalidrawModal } from '@/features/agent/composer/excalidraw-modal'

// The mocked canvas hands its `onCancel`/`onSave` props straight to the
// test via this closure, rather than mounting the real (heavy, canvas-
// rendering) `<Excalidraw>` tree — this file only exercises the MODAL's own
// wiring (id-minting, upload, insert-markdown, toast-on-failure), which is
// everything a real Save/Cancel click would trigger regardless of what's
// drawn.
let latestOnCancel: (() => void) | null = null
let latestOnSave: ((result: ExcalidrawSaveResult) => void) | null = null

vi.mock('@/features/agent/composer/excalidraw-canvas', () => ({
  ExcalidrawCanvas: ({
    onCancel,
    onSave,
  }: {
    onCancel: () => void
    onSave: (result: ExcalidrawSaveResult) => void
  }) => {
    latestOnCancel = onCancel
    latestOnSave = onSave
    return <div data-testid="excalidraw-canvas" />
  },
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

afterEach(() => {
  cleanup()
  latestOnCancel = null
  latestOnSave = null
  vi.clearAllMocks()
})

async function renderOpenModal(onInsertMarkdown = vi.fn(), onClose = vi.fn()) {
  render(
    <ExcalidrawModal
      wsId="w1"
      chatId="c1"
      open
      onClose={onClose}
      onInsertMarkdown={onInsertMarkdown}
    />,
  )
  await waitFor(() => expect(screen.getByTestId('excalidraw-canvas')).toBeInTheDocument())
  return { onInsertMarkdown, onClose }
}

describe('ExcalidrawModal', () => {
  it('lazily mounts the canvas only once opened', async () => {
    await renderOpenModal()
  })

  it('uses the SAME id for the fence-tag JSON and the uploaded PNG filename', async () => {
    const { onInsertMarkdown } = await renderOpenModal()

    // Simulate the canvas calling back with a scene + PNG, as a real Save
    // click on `excalidraw-canvas.tsx` would.
    const pngFile = new File(['x'], 'diagram.png', { type: 'image/png' })
    await latestOnSave?.({ sceneJson: '{"elements":[],"appState":{}}', pngFile })

    expect(uploadChatAttachment).toHaveBeenCalledTimes(1)
    const [, , , uploadedId] = vi.mocked(uploadChatAttachment).mock.calls[0]
    expect(uploadedId).toBeTruthy()

    expect(onInsertMarkdown).toHaveBeenCalledTimes(2)
    const [fenceMarkdown] = onInsertMarkdown.mock.calls[0]
    const [imageMarkdownCall] = onInsertMarkdown.mock.calls[1]
    expect(fenceMarkdown).toContain(`excalidraw:${uploadedId}`)
    expect(imageMarkdownCall).toContain('x-diagram.png')
  })

  it('shows a toast and inserts nothing when the upload fails', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('daemon unreachable'))
    const { onInsertMarkdown, onClose } = await renderOpenModal()

    const pngFile = new File(['x'], 'diagram.png', { type: 'image/png' })
    await latestOnSave?.({ sceneJson: '{"elements":[],"appState":{}}', pngFile })

    expect(toastError).toHaveBeenCalledTimes(1)
    expect(toastError.mock.calls[0][1]).toContain('daemon unreachable')
    expect(onInsertMarkdown).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('stringifies a non-Error rejection for the toast', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce('413')
    await renderOpenModal()

    const pngFile = new File(['x'], 'diagram.png', { type: 'image/png' })
    await latestOnSave?.({ sceneJson: '{"elements":[],"appState":{}}', pngFile })

    expect(toastError.mock.calls[0][1]).toBe('413')
  })

  it('closes without inserting anything when Cancel is invoked', async () => {
    const { onInsertMarkdown, onClose } = await renderOpenModal()

    latestOnCancel?.()

    expect(onClose).toHaveBeenCalledTimes(1)
    expect(onInsertMarkdown).not.toHaveBeenCalled()
    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })
})
