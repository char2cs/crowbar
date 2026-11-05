import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAttachmentUpload } from '@/features/agent/composer/lib/use-attachment-upload'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  UNWORDED,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'

vi.mock('@/features/agent/composer/lib/attachment-upload', () => ({
  uploadAttachmentMarkdown: vi.fn(),
}))

let toastError: ReturnType<typeof watchToasts>

beforeEach(() => {
  vi.mocked(uploadAttachmentMarkdown).mockReset()
  toastError?.mockRestore()
  toastError = watchToasts()
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

  it('reports the daemon’s reason and does not insert', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockRejectedValue(refusal('413 Payload Too Large'))
    const onInsert = vi.fn()
    const { result } = renderHook(() => useAttachmentUpload('w1', 'c1', onInsert))

    await act(async () => {
      await result.current.uploadAndInsert({ path: '/tmp/a.png' })
    })

    expectReported(toastError, 'Could not attach that file', '413 Payload Too Large')
    expect(onInsert).not.toHaveBeenCalled()
  })

  it('words an upload that threw no Error as an unreachable daemon', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockRejectedValue(UNWORDED)
    const { result } = renderHook(() => useAttachmentUpload('w1', 'c1', vi.fn(), vi.fn(), vi.fn()))

    await act(async () => {
      await result.current.uploadAndInsert({ path: '/tmp/a.png' })
      await result.current.uploadAndInsert({
        file: new File(['b'], 'a.png', { type: 'image/png' }),
      })
    })

    expect(toastError).toHaveBeenCalledTimes(2)
    expectReported(
      toastError,
      'Could not attach that file',
      'Crowbar could not reach the daemon — try again.',
    )
  })

  it('an upload refused while the daemon migrates shows the migrating state, not a toast', async () => {
    vi.mocked(uploadAttachmentMarkdown).mockRejectedValue(migratingRefusal())
    const { result } = renderHook(() => useAttachmentUpload('w1', 'c1', vi.fn()))

    await act(async () => {
      await result.current.uploadAndInsert({ path: '/tmp/a.png' })
    })

    expectMigratingShown(toastError)
  })
})

// An image File with both pending-image callbacks gets an instant local preview instead of
// waiting on the upload; a non-image, a Tauri `{path}` or no callbacks insert on resolve (above).
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
    vi.mocked(uploadAttachmentMarkdown).mockRejectedValue(refusal('413 Payload Too Large'))
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
    expectReported(toastError, 'Could not attach that file', '413 Payload Too Large')
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
