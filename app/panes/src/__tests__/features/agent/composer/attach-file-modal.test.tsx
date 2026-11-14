import { act } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'
import {
  uploadChatAttachment,
  type UploadedChatAttachment,
} from '@/features/agent/api/upload-chat-attachment'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import * as reports from '@/__tests__/__fixtures__/reports'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(
    async (_ws: string, _chat: string, input: { file: File } | { path: string }) => ({
      ref:
        'path' in input
          ? `chats/c1/attachments/x-${input.path}`
          : `chats/c1/attachments/x-${input.file.name}`,
      filename: 'path' in input ? input.path : input.file.name,
      size: 10,
      contentType:
        'file' in input
          ? input.file.type || 'application/octet-stream'
          : 'application/octet-stream',
    }),
  ),
}))

// Under jsdom `isTauri()` is false so the real hook never fires; stubbing it lets this suite call
// the modal's `onDrop` directly. Subscribe/filter behaviour is covered in tauri-file-drop.test.ts.
vi.mock('@/features/file-system/lib/tauri-file-drop', () => ({
  useTauriFileDrop: vi.fn(),
}))

afterEach(cleanup)

beforeEach(() => {
  vi.mocked(uploadChatAttachment).mockClear()
  vi.mocked(useTauriFileDrop).mockClear()
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

const browse = (...files: File[]) =>
  fireEvent.change(screen.getByLabelText('Choose files'), { target: { files } })

const attachButton = () => screen.getByRole('button', { name: /^Attach( \d+)? files?/ })

describe('AttachFileModal', () => {
  it('lists a browsed file as uploaded, and Attach puts its markdown in the message', async () => {
    const { props } = draw()
    browse(new File(['hello'], 'notes.txt', { type: 'text/plain' }))

    expect(await screen.findByText(/Uploaded/)).toBeInTheDocument()
    expect(props.onInsertMarkdown).not.toHaveBeenCalled()
    fireEvent.click(attachButton())

    expect(props.onInsertMarkdown).toHaveBeenCalledWith(
      '[notes.txt](chats/c1/attachments/x-notes.txt)',
    )
    expect(props.onClose).toHaveBeenCalledTimes(1)
  })

  it('attaches every finished file in the order chosen, an image as an image node', async () => {
    const { props } = draw()
    browse(
      new File(['x'], 'shot.png', { type: 'image/png' }),
      new File(['y'], 'spec.pdf', { type: 'application/pdf' }),
    )
    await waitFor(() => expect(screen.getAllByText(/Uploaded/)).toHaveLength(2))
    expect(attachButton()).toHaveTextContent('Attach 2 files')
    fireEvent.click(attachButton())

    expect(vi.mocked(props.onInsertMarkdown).mock.calls.map(([md]) => md)).toEqual([
      '![shot.png](chats/c1/attachments/x-shot.png)',
      '[spec.pdf](chats/c1/attachments/x-spec.pdf)',
    ])
  })

  // A small, well-formed CSV inlines as a table without touching the network.
  it('inlines a small browsed CSV as a markdown table, without uploading', async () => {
    const { props } = draw()
    browse(new File(['name,age\nAda,36\n'], 'people.csv', { type: 'text/csv' }))
    await screen.findByText(/Uploaded/)
    fireEvent.click(attachButton())

    expect(props.onInsertMarkdown).toHaveBeenCalledWith(
      '| name | age |\n| --- | --- |\n| Ada | 36 |',
    )
    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })

  it('shows a loading bar and a progress hint while an upload is in flight, Attach waiting', async () => {
    let finish: ((value: UploadedChatAttachment) => void) | undefined
    vi.mocked(uploadChatAttachment).mockImplementationOnce(
      () => new Promise((resolve) => (finish = resolve)),
    )
    draw()
    browse(new File(['x'], 'a.png', { type: 'image/png' }))

    const bar = await screen.findByRole('progressbar', { name: 'Uploading a.png' })
    expect(bar).not.toHaveAttribute('aria-valuenow')
    expect(screen.getByText('Uploading 0 of 1…')).toBeInTheDocument()
    expect(attachButton()).toBeDisabled()

    await act(async () => {
      finish?.({
        ref: 'chats/c1/attachments/x-a.png',
        filename: 'a.png',
        size: 10,
        contentType: 'image/png',
      })
    })
    expect(screen.queryByRole('progressbar')).toBeNull()
    expect(attachButton()).toBeEnabled()
  })

  it('fills the bar and says how much of the file has gone, from the upload progress', async () => {
    vi.mocked(uploadChatAttachment).mockImplementationOnce(
      (_ws, _chat, _input, _id, options) =>
        new Promise(() => options?.onProgress?.(512 * 1024, 2 * 1024 * 1024)),
    )
    draw()
    browse(new File([new Uint8Array(2 * 1024 * 1024)], 'big.png', { type: 'image/png' }))

    const bar = await screen.findByRole('progressbar', { name: 'Uploading big.png' })
    await waitFor(() => expect(bar).toHaveAttribute('aria-valuenow', '25'))
    expect(screen.getByText('512 KB of 2 MB · Uploading…')).toBeInTheDocument()
  })

  it('advances the bar with each acknowledged chunk until the file is uploaded', async () => {
    let report: ((loaded: number, total: number) => void) | undefined
    let finish: ((value: UploadedChatAttachment) => void) | undefined
    vi.mocked(uploadChatAttachment).mockImplementationOnce((_ws, _chat, _input, _id, options) => {
      report = (loaded, total) => options?.onProgress?.(loaded, total)
      return new Promise((resolve) => (finish = resolve))
    })
    draw()
    const total = 4 * 1024 * 1024
    browse(new File([new Uint8Array(total)], 'big.bin'))
    const bar = await screen.findByRole('progressbar', { name: 'Uploading big.bin' })
    expect(bar).not.toHaveAttribute('aria-valuenow')

    for (const [loaded, percent, note] of [
      [1024 * 1024, '25', '1 MB of 4 MB · Uploading…'],
      [3 * 1024 * 1024, '75', '3 MB of 4 MB · Uploading…'],
    ] as const) {
      act(() => report?.(loaded, total))
      await waitFor(() => expect(bar).toHaveAttribute('aria-valuenow', percent))
      expect(screen.getByText(note)).toBeInTheDocument()
    }

    await act(async () => {
      finish?.({ ref: 'r', filename: 'big.bin', size: total, contentType: 't' })
    })
    expect(screen.queryByRole('progressbar')).toBeNull()
    expect(screen.getByText('4 MB · Uploaded')).toBeInTheDocument()
  })

  it('offers Retry on a failed upload and sends it again under the same upload id', async () => {
    const ids: Array<string | undefined> = []
    vi.mocked(uploadChatAttachment).mockImplementationOnce(async (_ws, _chat, _input, id) => {
      ids.push(id)
      throw new Error('boom')
    })
    vi.mocked(uploadChatAttachment).mockImplementationOnce(async (_ws, _chat, input, id) => {
      ids.push(id)
      const name = 'file' in input ? input.file.name : input.path
      return { ref: `r/${name}`, filename: name, size: 1, contentType: 't' }
    })
    draw()
    browse(new File(['x'], 'a.bin'))

    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))

    expect(await screen.findByText(/Uploaded/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    expect(ids).toHaveLength(2)
    expect(ids[0]).toBeTruthy()
    expect(ids[1]).toBe(ids[0])
  })

  it('aborts an upload still on its way when its row is removed or the dialog is cancelled', async () => {
    const signals: AbortSignal[] = []
    const hang = (
      _ws: string,
      _chat: string,
      _input: unknown,
      _id?: string,
      options?: { signal?: AbortSignal },
    ) => {
      if (options?.signal) signals.push(options.signal)
      return new Promise<never>(() => {})
    }
    vi.mocked(uploadChatAttachment).mockImplementationOnce(hang).mockImplementationOnce(hang)
    const { props } = draw()
    browse(new File(['x'], 'a.png', { type: 'image/png' }), new File(['y'], 'b.png'))
    await screen.findByText('b.png')

    fireEvent.click(screen.getByRole('button', { name: 'Remove a.png' }))
    expect(signals.map((signal) => signal.aborted)).toEqual([true, false])
    fireEvent.click(screen.getByRole('button', { name: /^Cancel/ }))
    expect(signals[1]?.aborted).toBe(true)
    expect(props.onClose).toHaveBeenCalled()
  })

  it('refuses a file over the daemon limit on the row, without uploading it', () => {
    draw()
    const big = new File(['x'], 'cart-recording.mov')
    Object.defineProperty(big, 'size', { value: 64 * 1024 * 1024 })
    browse(big)

    expect(screen.getByText('cart-recording.mov')).toBeInTheDocument()
    expect(screen.getByText('64 MB · too large, the limit is 25 MB')).toBeInTheDocument()
    expect(uploadChatAttachment).not.toHaveBeenCalled()
    expect(attachButton()).toBeDisabled()
  })

  it('says why an upload failed on its row, and still attaches the files that went through', async () => {
    vi.mocked(uploadChatAttachment).mockRejectedValueOnce(reports.refusal('disk full'))
    const { props } = draw()
    browse(new File(['x'], 'bad.png', { type: 'image/png' }), new File(['y'], 'ok.txt'))

    expect(await screen.findByText(/disk full/)).toBeInTheDocument()
    await screen.findByText(/Uploaded/)
    fireEvent.click(attachButton())
    expect(props.onInsertMarkdown).toHaveBeenCalledTimes(1)
    expect(props.onInsertMarkdown).toHaveBeenCalledWith('[ok.txt](chats/c1/attachments/x-ok.txt)')
  })

  it('drops a removed row from what Attach inserts', async () => {
    const { props } = draw()
    browse(new File(['x'], 'a.txt'), new File(['y'], 'b.txt'))
    await waitFor(() => expect(screen.getAllByText(/Uploaded/)).toHaveLength(2))

    fireEvent.click(screen.getByRole('button', { name: 'Remove a.txt' }))
    fireEvent.click(attachButton())

    expect(props.onInsertMarkdown).toHaveBeenCalledTimes(1)
    expect(props.onInsertMarkdown).toHaveBeenCalledWith('[b.txt](chats/c1/attachments/x-b.txt)')
  })

  it('Cancel closes without inserting anything', async () => {
    const { props } = draw()
    browse(new File(['x'], 'a.txt'))
    await screen.findByText(/Uploaded/)

    fireEvent.click(screen.getByRole('button', { name: /^Cancel/ }))

    expect(props.onClose).toHaveBeenCalledTimes(1)
    expect(props.onInsertMarkdown).not.toHaveBeenCalled()
  })

  it('Enter attaches what is ready', async () => {
    const { props } = draw()
    browse(new File(['x'], 'a.txt'))
    await screen.findByText(/Uploaded/)

    fireEvent.keyDown(screen.getByTestId('attach-file-dropzone'), { key: 'Enter' })

    expect(props.onInsertMarkdown).toHaveBeenCalledWith('[a.txt](chats/c1/attachments/x-a.txt)')
  })

  it('opens the native picker from the "choose from your computer" link', () => {
    draw()
    const input = screen.getByLabelText('Choose files') as HTMLInputElement
    const click = vi.spyOn(input, 'click')

    fireEvent.click(screen.getByRole('button', { name: 'choose from your computer' }))

    expect(click).toHaveBeenCalledTimes(1)
  })

  it('does nothing when the browse input reports no FileList at all', () => {
    draw()
    const input = screen.getByLabelText('Choose files') as HTMLInputElement
    Object.defineProperty(input, 'files', { value: null, configurable: true })

    fireEvent.change(input)

    expect(screen.queryByTestId('attach-file-list')).toBeNull()
  })

  describe('drag and drop onto the dropzone', () => {
    it('marks the zone while files are dragged over it, and clears on leave', () => {
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')

      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })
      expect(zone).toHaveAttribute('data-drop-target')

      fireEvent.dragLeave(zone)
      expect(zone).not.toHaveAttribute('data-drop-target')
    })

    it('ignores a drag-over or a drop that carries no files', () => {
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')

      fireEvent.dragOver(zone, { dataTransfer: { types: ['text/plain'] } })
      fireEvent.drop(zone, { dataTransfer: { types: ['text/plain'], files: [] } })

      expect(zone).not.toHaveAttribute('data-drop-target')
      expect(uploadChatAttachment).not.toHaveBeenCalled()
    })

    it('lists a dropped file and clears the drag-over state', async () => {
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')
      const file = new File(['bytes'], 'a.png', { type: 'image/png' })

      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })
      fireEvent.drop(zone, { dataTransfer: { types: ['Files'], files: [file] } })

      expect(zone).not.toHaveAttribute('data-drop-target')
      expect(await screen.findByText(/Uploaded/)).toBeInTheDocument()
      expect(uploadChatAttachment).toHaveBeenCalledWith(
        'w1',
        'c1',
        { file },
        expect.any(String),
        expect.anything(),
      )
    })
  })

  // A native OS drop never reaches the DOM as a `drop` event, only as this hook's callback.
  describe('the Tauri host-path drop channel', () => {
    it('stages each reported host path by its file name and clears the drag-over state', async () => {
      const { props } = draw()
      const zone = screen.getByTestId('attach-file-dropzone')
      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })

      const onDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]
      await act(async () => {
        onDrop?.(['/Users/me/report.csv'])
      })

      expect(zone).not.toHaveAttribute('data-drop-target')
      expect(uploadChatAttachment).toHaveBeenCalledWith(
        'w1',
        'c1',
        { path: '/Users/me/report.csv' },
        expect.any(String),
        expect.anything(),
      )
      expect(await screen.findByText('report.csv')).toBeInTheDocument()
      fireEvent.click(attachButton())
      expect(props.onInsertMarkdown).toHaveBeenCalledWith(
        '[/Users/me/report.csv](chats/c1/attachments/x-/Users/me/report.csv)',
      )
    })

    // A fresh onDrop per dragOver/dragLeave render would re-subscribe Tauri's IPC mid-drag.
    it('keeps the same onDrop across a dragOver/dragLeave re-render', () => {
      draw()
      const zone = screen.getByTestId('attach-file-dropzone')
      const firstOnDrop = vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]

      fireEvent.dragOver(zone, { dataTransfer: { types: ['Files'] } })
      fireEvent.dragLeave(zone)

      expect(vi.mocked(useTauriFileDrop).mock.calls.at(-1)?.[1]).toBe(firstOnDrop)
    })
  })

  it('does not render a dialog when closed', () => {
    draw({ open: false })
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  // Base UI closes a dialog on Escape by firing onOpenChange(false).
  it('calls onClose when the dialog requests dismissal', async () => {
    const { props } = draw()

    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape', code: 'Escape' })

    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
  })
})
