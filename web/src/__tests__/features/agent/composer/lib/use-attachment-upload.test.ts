import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAttachmentUpload } from '@/features/agent/composer/lib/use-attachment-upload'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'

vi.mock('@/features/agent/composer/lib/attachment-upload', () => ({
  uploadAttachmentMarkdown: vi.fn(),
}))

const toastError = vi.hoisted(() => vi.fn())
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

beforeEach(() => {
  vi.mocked(uploadAttachmentMarkdown).mockReset()
  toastError.mockClear()
})

describe('useAttachmentUpload', () => {
  it('inserts the resolved markdown on a successful upload', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockResolvedValue('![a.png](chats/c1/attachments/x-a.png)')
    const onInsert = vi.fn()
    const { result } = renderHook(() => useAttachmentUpload('w1', 'c1', onInsert))
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    await act(async () => {
      await result.current.uploadAndInsert({ file })
    })

    expect(uploadAttachmentMarkdown).toHaveBeenCalledWith('w1', 'c1', { file })
    expect(onInsert).toHaveBeenCalledWith('![a.png](chats/c1/attachments/x-a.png)')
    expect(toastError).not.toHaveBeenCalled()
  })

  it('toasts an Error rejection message and does not insert', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockRejectedValue(new Error('413 Payload Too Large'))
    const onInsert = vi.fn()
    const { result } = renderHook(() => useAttachmentUpload('w1', 'c1', onInsert))

    await act(async () => {
      await result.current.uploadAndInsert({ path: '/tmp/a.png' })
    })

    expect(toastError).toHaveBeenCalledWith('Could not attach that file', '413 Payload Too Large')
    expect(onInsert).not.toHaveBeenCalled()
  })

  it('falls back to a generic description for a non-Error rejection', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockRejectedValue('boom')
    const { result } = renderHook(() => useAttachmentUpload('w1', 'c1', vi.fn()))

    await act(async () => {
      await result.current.uploadAndInsert({ path: '/tmp/a.png' })
    })

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        'Could not attach that file',
        'Crowbar could not reach the daemon — try again.',
      ),
    )
  })
})

// REGRESSION, reported live: "photos attachments are not loaded instantly...
// let's not wait for them." An image FILE, with both pending-image callbacks
// supplied, now gets an instant local preview instead of waiting on the
// network round trip — everything else (a non-image file, a Tauri `{path}`
// with no client-side bytes to preview, or no callbacks supplied at all)
// keeps the original insert-on-resolve behaviour, proven above.
describe('useAttachmentUpload — optimistic image preview', () => {
  it('inserts a pending image immediately, before the upload resolves, for an image file', async () => {
    let resolveUpload!: (markdown: string) => void
    vi.mocked(uploadAttachmentMarkdown).mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve
      }),
    )
    const onInsert = vi.fn()
    const onInsertPendingImage = vi.fn()
    const onSettlePendingImage = vi.fn()
    const { result } = renderHook(() =>
      useAttachmentUpload('w1', 'c1', onInsert, onInsertPendingImage, onSettlePendingImage),
    )
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    let uploadPromise!: Promise<void>
    act(() => {
      uploadPromise = result.current.uploadAndInsert({ file })
    })

    // Inserted synchronously, before `uploadAttachmentMarkdown` has resolved
    // at all — `onInsert` (the non-optimistic path) never fires for this one.
    expect(onInsertPendingImage).toHaveBeenCalledTimes(1)
    const [objectUrl, alt] = onInsertPendingImage.mock.calls[0]!
    expect(typeof objectUrl).toBe('string')
    expect(alt).toBe('a.png')
    expect(onInsert).not.toHaveBeenCalled()
    expect(onSettlePendingImage).not.toHaveBeenCalled()

    resolveUpload('![a.png](chats/c1/attachments/x-a.png)')
    await act(async () => {
      await uploadPromise
    })

    expect(onSettlePendingImage).toHaveBeenCalledWith(
      objectUrl,
      '![a.png](chats/c1/attachments/x-a.png)',
    )
  })

  it('settles the pending image to null (not a toast-only failure) when the upload fails', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockRejectedValue(new Error('413 Payload Too Large'))
    const onInsertPendingImage = vi.fn()
    const onSettlePendingImage = vi.fn()
    const { result } = renderHook(() =>
      useAttachmentUpload('w1', 'c1', vi.fn(), onInsertPendingImage, onSettlePendingImage),
    )
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    await act(async () => {
      await result.current.uploadAndInsert({ file })
    })

    const [objectUrl] = onInsertPendingImage.mock.calls[0]!
    expect(onSettlePendingImage).toHaveBeenCalledWith(objectUrl, null)
    expect(toastError).toHaveBeenCalledWith('Could not attach that file', '413 Payload Too Large')
  })

  it('does not use the optimistic path for a non-image file, even with callbacks supplied', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockResolvedValue(
      '[report.pdf](chats/c1/attachments/report.pdf)',
    )
    const onInsert = vi.fn()
    const onInsertPendingImage = vi.fn()
    const { result } = renderHook(() =>
      useAttachmentUpload('w1', 'c1', onInsert, onInsertPendingImage, vi.fn()),
    )
    const file = new File(['bytes'], 'report.pdf', { type: 'application/pdf' })

    await act(async () => {
      await result.current.uploadAndInsert({ file })
    })

    expect(onInsertPendingImage).not.toHaveBeenCalled()
    expect(onInsert).toHaveBeenCalledWith('[report.pdf](chats/c1/attachments/report.pdf)')
  })

  // A Tauri `{path}` drop has no client-side `File`/bytes to build a preview
  // from at all — the daemon reads the path itself.
  it('does not use the optimistic path for a Tauri path input, even with callbacks supplied', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockResolvedValue('![a.png](chats/c1/attachments/a.png)')
    const onInsert = vi.fn()
    const onInsertPendingImage = vi.fn()
    const { result } = renderHook(() =>
      useAttachmentUpload('w1', 'c1', onInsert, onInsertPendingImage, vi.fn()),
    )

    await act(async () => {
      await result.current.uploadAndInsert({ path: '/tmp/a.png' })
    })

    expect(onInsertPendingImage).not.toHaveBeenCalled()
    expect(onInsert).toHaveBeenCalledWith('![a.png](chats/c1/attachments/a.png)')
  })
})
