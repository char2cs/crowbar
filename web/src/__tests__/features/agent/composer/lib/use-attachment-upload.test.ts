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
