import { beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(),
}))

beforeEach(() => {
  vi.mocked(uploadChatAttachment).mockReset()
})

describe('uploadAttachmentMarkdown', () => {
  it('resolves a small, well-formed CSV FILE inline as a markdown table, without uploading', async () => {
    const file = new File(['name,age\nAda,36\n'], 'people.csv', { type: 'text/csv' })

    const markdown = await uploadAttachmentMarkdown('w1', 'c1', { file })

    expect(markdown).toBe('| name | age |\n| --- | --- |\n| Ada | 36 |')
    expect(uploadChatAttachment).not.toHaveBeenCalled()
  })

  it('falls through to upload for a CSV FILE too large/malformed to inline', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/x-huge.csv',
      filename: 'huge.csv',
      size: 999,
      contentType: 'text/csv',
    })
    // Ragged rows -> resolveCsv rejects it as { kind: 'file' }.
    const file = new File(['a,b\n1\n'], 'huge.csv', { type: 'text/csv' })

    const markdown = await uploadAttachmentMarkdown('w1', 'c1', { file })

    expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', { file })
    expect(markdown).toBe('[huge.csv](chats/c1/attachments/x-huge.csv)')
  })

  it('uploads a non-CSV file and returns image markdown for an image contentType', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/x-a.png',
      filename: 'a.png',
      size: 10,
      contentType: 'image/png',
    })
    const file = new File(['bytes'], 'a.png', { type: 'image/png' })

    const markdown = await uploadAttachmentMarkdown('w1', 'c1', { file })

    expect(markdown).toBe('![a.png](chats/c1/attachments/x-a.png)')
  })

  it('uploads a non-CSV file and returns file-link markdown for a non-image contentType', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/x-b.pdf',
      filename: 'b.pdf',
      size: 20,
      contentType: 'application/pdf',
    })
    const file = new File(['bytes'], 'b.pdf', { type: 'application/pdf' })

    const markdown = await uploadAttachmentMarkdown('w1', 'c1', { file })

    expect(markdown).toBe('[b.pdf](chats/c1/attachments/x-b.pdf)')
  })

  // A Tauri host PATH has no client-side bytes to inspect before the daemon
  // reads it, so CSV resolution never applies — it always uploads, even for
  // a `.csv` path.
  it('always uploads a {path} input, never attempting CSV resolution', async () => {
    vi.mocked(uploadChatAttachment).mockResolvedValue({
      ref: 'chats/c1/attachments/x-report.csv',
      filename: 'report.csv',
      size: 30,
      contentType: 'text/csv',
    })

    const markdown = await uploadAttachmentMarkdown('w1', 'c1', { path: '/Users/me/report.csv' })

    expect(uploadChatAttachment).toHaveBeenCalledWith('w1', 'c1', {
      path: '/Users/me/report.csv',
    })
    expect(markdown).toBe('[report.csv](chats/c1/attachments/x-report.csv)')
  })
})
