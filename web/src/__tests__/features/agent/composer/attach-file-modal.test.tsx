import { act } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'
import {
  uploadChatAttachment,
  type UploadedChatAttachment,
} from '@/features/agent/api/upload-chat-attachment'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(async (_ws: string, _chat: string, input: { file: File } | { path: string }) => ({
    ref: 'path' in input ? `chats/c1/attachments/x-${input.path}` : `chats/c1/attachments/x-${input.file.name}`,
    filename: 'path' in input ? input.path : input.file.name,
    size: 10,
    contentType: 'file' in input ? input.file.type || 'application/octet-stream' : 'application/octet-stream',
  })),
}))

// Task 29 stubbed this same hook the same way, for the same reason: under
// jsdom `isTauri()` is false so the real effect never fires, and stubbing it
// out here lets this suite invoke the `onDrop` callback the modal wires into
// it directly — the hook's own subscribe/filter behaviour is covered in
// tauri-file-drop.test.ts.
vi.mock('@/features/file-system/lib/tauri-file-drop', () => ({
  useTauriFileDrop: vi.fn(),
}))

const toastError = vi.hoisted(() => vi.fn())
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

afterEach(cleanup)

beforeEach(() => {
  vi.mocked(uploadChatAttachment).mockClear()
  vi.mocked(useTauriFileDrop).mockClear()
  toastError.mockClear()
})

function draw(overrides: Partial<Parameters<typeof AttachFileModal>[0]> = {}) {
  const props = {
    wsId: 'w1',
    chatId: 'c1',
    open: true,
    onClose: vi.fn(),
    onInsertMarkdown: vi.fn(),
    ...overrides,
  }
  return { ...render(<AttachFileModal {...props} />), props }
}

describe('AttachFileModal', () => {
  it('uploads a browsed file and inserts a file-link markdown node', async () => {
    const onInsertMarkdown = vi.fn()
    draw({ onInsertMarkdown })
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() =>
      expect(onInsertMarkdown).toHaveBeenCalledWith('[notes.txt](chats/c1/attachments/x-notes.txt)'),
    )
  })

  it('inserts an image node for an image file', async () => {
    const onInsertMarkdown = vi.fn()
    draw({ onInsertMarkdown })
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    const file = new File(['x'], 'shot.png', { type: 'image/png' })
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() =>
      expect(onInsertMarkdown).toHaveBeenCalledWith('![shot.png](chats/c1/attachments/x-shot.png)'),
    )
  })

  // Successful uploads close the modal — both browse and drop insert THEN
  // close, since nothing left to do once the markdown is handed back.
  it('closes the modal once the browsed upload resolves', async () => {
    const onClose = vi.fn()
    draw({ onClose })
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(['x'], 'a.txt')] } })

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  })

  it('opens the native file picker when the Choose a file button is clicked', () => {
    draw()
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    const clickSpy = vi.spyOn(input, 'click')

    fireEvent.click(screen.getByRole('button', { name: 'Choose a file' }))

    expect(clickSpy).toHaveBeenCalledTimes(1)
  })

  // Defensive fallback: a real `<input type="file">` always reports at least
  // an empty FileList, but the type is `FileList | null` — this proves the
  // `?? []` guard rather than leaving it an unexercised assumption.
  it('does nothing when the browse input reports no FileList at all', () => {
    const onInsertMarkdown = vi.fn()
    draw({ onInsertMarkdown })
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    Object.defineProperty(input, 'files', { value: null, configurable: true })

    fireEvent.change(input)

    expect(uploadChatAttachment).not.toHaveBeenCalled()
    expect(onInsertMarkdown).not.toHaveBeenCalled()
  })

  it('shows the uploading state while an upload is in flight and disables the browse button', async () => {
    let resolveUpload: ((value: UploadedChatAttachment) => void) | undefined
    vi.mocked(uploadChatAttachment).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveUpload = resolve
        }),
    )
    draw()
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(['x'], 'a.png', { type: 'image/png' })] } })

    expect(await screen.findByText('Uploading…')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Choose a file' })).toBeDisabled()
    expect(input).toBeDisabled()

    await act(async () => {
      resolveUpload?.({
        ref: 'chats/c1/attachments/x-a.png',
        filename: 'a.png',
        size: 10,
        contentType: 'image/png',
      })
    })
  })

  describe('drag and drop onto the dropzone', () => {
    it('shows a drag-over state while a file is dragged over the zone, clearing on drag-leave', () => {
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')

      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })
      expect(zone).toHaveClass('border-ring')

      fireEvent.dragLeave(zone)
      expect(zone).not.toHaveClass('border-ring')
    })

    it('ignores a drag-over that carries no files', () => {
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')

      fireEvent.dragOver(zone, { dataTransfer: { types: ['text/plain'] } })

      expect(zone).not.toHaveClass('border-ring')
    })

    it('uploads a dropped image and inserts image markdown, clearing the drag-over state', async () => {
      const onInsertMarkdown = vi.fn()
      draw({ onInsertMarkdown })
      const zone = screen.getByTestId('attach-file-dropzone')
      const file = new File(['bytes'], 'a.png', { type: 'image/png' })

      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })
      fireEvent.drop(zone, { dataTransfer: { types: ['Files'], files: [file] } })

      expect(zone).not.toHaveClass('border-ring')
      await waitFor(() =>
        expect(onInsertMarkdown).toHaveBeenCalledWith('![a.png](chats/c1/attachments/x-a.png)'),
      )
    })

    it('uploads a dropped non-image file and inserts link markdown', async () => {
      const onInsertMarkdown = vi.fn()
      draw({ onInsertMarkdown })
      const zone = screen.getByTestId('attach-file-dropzone')
      const file = new File(['bytes'], 'notes.pdf', { type: 'application/pdf' })

      fireEvent.drop(zone, { dataTransfer: { types: ['Files'], files: [file] } })

      await waitFor(() =>
        expect(onInsertMarkdown).toHaveBeenCalledWith('[notes.pdf](chats/c1/attachments/x-notes.pdf)'),
      )
      expect(onInsertMarkdown).not.toHaveBeenCalledWith(expect.stringMatching(/^!/))
    })

    it('ignores a drop that carries no files', () => {
      const onInsertMarkdown = vi.fn()
      draw({ onInsertMarkdown })
      const zone = screen.getByTestId('attach-file-dropzone')

      fireEvent.drop(zone, { dataTransfer: { types: ['text/plain'], files: [] } })

      expect(uploadChatAttachment).not.toHaveBeenCalled()
      expect(onInsertMarkdown).not.toHaveBeenCalled()
    })
  })

  // The real Tauri desktop path (Task 27): a native OS drop never reaches the
  // DOM as a `drop` event at all, only as this hook's own callback — see
  // useTauriFileDrop's own note on why. Exercised via the stubbed hook above,
  // same as agent-composer.test.tsx does for the pill's own drop handler.
  describe('the Tauri host-path drop channel', () => {
    it('uploads each Tauri-reported host path, clears the drag-over state, and closes', async () => {
      const onInsertMarkdown = vi.fn()
      const onClose = vi.fn()
      draw({ onInsertMarkdown, onClose })
      const zone = screen.getByTestId('attach-file-dropzone')

      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })
      expect(zone).toHaveClass('border-ring')

      const onDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
      expect(onDrop).toBeTypeOf('function')
      await act(async () => {
        onDrop?.(['/Users/me/report.csv'])
      })

      expect(zone).not.toHaveClass('border-ring')
      expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', { path: '/Users/me/report.csv' })
      await waitFor(() =>
        expect(onInsertMarkdown).toHaveBeenCalledWith(
          '[/Users/me/report.csv](chats/c1/attachments/x-/Users/me/report.csv)',
        ),
      )
      expect(onClose).toHaveBeenCalledTimes(1)
    })

    // The proactive fix this task called out: an inline arrow passed to
    // `useTauriFileDrop` would get a fresh identity on every dragOver/
    // dragLeave re-render (each toggles `dropTarget`, a state update), tearing
    // down and re-establishing Tauri's `onDragDropEvent` IPC subscription
    // continuously for the whole span of an active drag. Proven directly
    // against the stubbed hook rather than inferred from the fix.
    it('passes the same onDrop identity to useTauriFileDrop across a dragOver/dragLeave re-render', () => {
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')
      const callsBefore = vi.mocked(useTauriFileDrop).mock.calls.length
      const firstOnDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
      expect(firstOnDrop).toBeTypeOf('function')

      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })
      fireEvent.dragLeave(zone)

      expect(vi.mocked(useTauriFileDrop).mock.calls.length).toBeGreaterThan(callsBefore)
      const lastOnDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
      expect(lastOnDrop).toBe(firstOnDrop)
    })
  })

  describe('a failed upload', () => {
    it('surfaces a failed upload as a toast, without inserting or closing', async () => {
      vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('413 Payload Too Large'))
      const onInsertMarkdown = vi.fn()
      const onClose = vi.fn()
      draw({ onInsertMarkdown, onClose })
      const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
      const file = new File(['bytes'], 'huge.png', { type: 'image/png' })

      fireEvent.change(input, { target: { files: [file] } })

      await waitFor(() =>
        expect(toastError).toHaveBeenCalledWith("Couldn't attach that file", '413 Payload Too Large'),
      )
      expect(onInsertMarkdown).not.toHaveBeenCalled()
      expect(onClose).not.toHaveBeenCalled()
    })

    // A failure that isn't an `Error` instance (e.g. a plain thrown string)
    // still gets a description, not a crash.
    it('falls back to a generic description for a non-Error rejection', async () => {
      vi.mocked(uploadChatAttachment).mockRejectedValueOnce('boom')
      draw()
      const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
      fireEvent.change(input, { target: { files: [new File(['x'], 'huge.png')] } })

      await waitFor(() => expect(toastError).toHaveBeenCalledWith("Couldn't attach that file", 'boom'))
    })

    // The upload state must clear on failure too, or the picker would stay
    // permanently disabled after one rejected upload.
    it('clears the uploading state after a failed upload', async () => {
      vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('offline'))
      draw()
      const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
      fireEvent.change(input, { target: { files: [new File(['x'], 'a.png', { type: 'image/png' })] } })

      await waitFor(() => expect(toastError).toHaveBeenCalled())
      expect(screen.getByRole('button', { name: 'Choose a file' })).not.toBeDisabled()
    })

    it('surfaces a failed dropped upload the same way', async () => {
      vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('disk full'))
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')
      const file = new File(['bytes'], 'huge.png', { type: 'image/png' })

      fireEvent.drop(zone, { dataTransfer: { types: ['Files'], files: [file] } })

      await waitFor(() => expect(toastError).toHaveBeenCalledWith("Couldn't attach that file", 'disk full'))
    })
  })

  it('does not render a dialog when closed', () => {
    draw({ open: false })
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  // Base UI closes a dialog on Escape by firing onOpenChange(false) — same
  // precedent as primitive-dialog-service.test.tsx's own pressEscape().
  it('calls onClose when the dialog requests dismissal', async () => {
    const onClose = vi.fn()
    draw({ onClose })

    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape', code: 'Escape' })

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  })
})
