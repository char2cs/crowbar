import { act } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'
import {
  uploadChatAttachment,
  type UploadedChatAttachment,
} from '@/features/agent/api/upload-chat-attachment'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(
    async (_ws: string, _chat: string, input: { file: File } | { path: string }) => ({
      ref: `chats/c1/attachments/x-${'file' in input ? input.file.name : input.path}`,
      filename: 'file' in input ? input.file.name : input.path,
      size: 10,
      contentType: 'file' in input ? input.file.type || 'application/octet-stream' : 'x',
    }),
  ),
}))
vi.mock('@/features/file-system/lib/tauri-file-drop', () => ({ useTauriFileDrop: vi.fn() }))

const chrome = vi.hoisted(() => ({
  native: false,
  show: vi.fn(),
  listener: null as null | ((intent: Record<string, unknown>) => void),
}))
vi.mock('@/lib/native-chrome', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/native-chrome')>()),
  hasNativeChrome: () => chrome.native,
  setNativeAttachSheet: chrome.show,
  onNativeChromeIntent: (cb: typeof chrome.listener) => {
    chrome.listener = cb
    return () => {
      chrome.listener = null
    }
  },
}))

afterEach(() => {
  cleanup()
  chrome.native = false
  chrome.show.mockClear()
})

function draw() {
  const props = {
    wsId: 'w1',
    chatId: 'c1',
    open: true,
    onClose: vi.fn(),
    onInsertMarkdown: vi.fn(),
  }
  return { ...render(<AttachFileModal {...props} />), props }
}

const browse = (...files: File[]) =>
  fireEvent.change(screen.getByLabelText('Choose files'), { target: { files } })

describe('AttachFileModal with the native chrome', () => {
  const sent = () => chrome.show.mock.lastCall?.[0]
  const intent = (payload: Record<string, unknown>) => act(() => chrome.listener?.(payload))
  const drawNative = () => {
    chrome.native = true
    return draw()
  }

  it('hands the native sheet the sources and limits instead of drawing a dialog', () => {
    drawNative()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(sent()).toEqual({
      open: true,
      title: 'Attach files',
      note: 'Images, PDFs, text and code, up to 25 MB each.',
      sources: [
        { id: 'photos', title: 'Photos', icon: 'Images' },
        { id: 'camera', title: 'Camera', icon: 'Camera' },
        { id: 'files', title: 'Files', icon: 'Folder' },
      ],
      files: [],
      hint: null,
      attachLabel: 'Attach files',
      canAttach: false,
    })
  })

  it('pushes each file with its progress and enables Attach once everything landed', async () => {
    let finish: ((value: UploadedChatAttachment) => void) | undefined
    vi.mocked(uploadChatAttachment).mockImplementationOnce(
      () => new Promise((resolve) => (finish = resolve)),
    )
    drawNative()
    browse(new File(['x'], 'a.png', { type: 'image/png' }))

    await waitFor(() =>
      expect(sent()).toMatchObject({
        files: [{ name: 'a.png', status: 'uploading', percent: null, canRetry: false }],
        hint: 'Uploading 0 of 1…',
        canAttach: false,
      }),
    )
    await act(async () => {
      finish?.({
        ref: 'chats/c1/attachments/x-a.png',
        filename: 'a.png',
        size: 10,
        contentType: 'image/png',
      })
    })
    await waitFor(() =>
      expect(sent()).toMatchObject({
        files: [{ name: 'a.png', status: 'done', detail: '1 KB · Uploaded' }],
        hint: null,
        attachLabel: 'Attach 1 file',
        canAttach: true,
      }),
    )
  })

  it('attaches the finished files when the sheet reports Attach', async () => {
    const { props } = drawNative()
    browse(new File(['hello'], 'notes.txt', { type: 'text/plain' }))
    await waitFor(() => expect(sent()?.canAttach).toBe(true))

    intent({ type: 'attachFiles' })

    expect(props.onInsertMarkdown).toHaveBeenCalledWith(
      '[notes.txt](chats/c1/attachments/x-notes.txt)',
    )
    expect(props.onClose).toHaveBeenCalledTimes(1)
  })

  it('removes a file the sheet crosses out, and abandons the upload on Cancel', async () => {
    const { props } = drawNative()
    browse(new File(['x'], 'a.txt', { type: 'text/plain' }))
    await waitFor(() => expect(sent()?.files).toHaveLength(1))

    intent({ type: 'removeAttachFile', id: sent()?.files[0].id })
    await waitFor(() => expect(sent()?.files).toEqual([]))

    intent({ type: 'closeAttachSheet' })
    expect(props.onClose).toHaveBeenCalledTimes(1)
    expect(props.onInsertMarkdown).not.toHaveBeenCalled()
  })

  it('offers Retry for a failed upload and sends it again when asked', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce(new Error('offline'))
    drawNative()
    browse(new File(['x'], 'a.txt', { type: 'text/plain' }))
    await waitFor(() => expect(sent()?.files[0]).toMatchObject({ status: 'bad', canRetry: true }))

    intent({ type: 'retryAttachFile', id: sent()?.files[0].id })

    await waitFor(() => expect(sent()?.files[0]).toMatchObject({ status: 'done', canRetry: false }))
  })

  it('steps aside for the system picker and returns with what it chose, or when it is dismissed', () => {
    drawNative()
    const photos = screen.getByLabelText('Photos', { selector: 'input' })
    const click = vi.spyOn(photos, 'click')

    intent({ type: 'pickAttachSource', id: 'photos' })
    expect(click).toHaveBeenCalledTimes(1)
    expect(sent()).toMatchObject({ open: false })

    fireEvent(photos, new Event('cancel'))
    expect(sent()).toMatchObject({ open: true })

    intent({ type: 'pickAttachSource', id: 'files' })
    expect(sent()).toMatchObject({ open: false })
    browse(new File(['x'], 'a.txt', { type: 'text/plain' }))
    expect(sent()).toMatchObject({ open: true })
  })

  it('closes the native sheet when the modal goes away', () => {
    const { unmount } = drawNative()
    unmount()
    expect(sent()).toMatchObject({ open: false })
  })
})
