import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))
vi.mock('@/components/ui/toast', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

import { apiFetch } from '@/lib/api'
import { toast } from '@/components/ui/toast'
import { openFileContent } from '@/features/workspace/lib/open-file-content'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('openFileContent', () => {
  it('fetches the workspace-scoped content route and opens a buffer', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue({ content: 'hello world' })
    const openContent = vi.fn()
    await openFileContent('ws-1', 'web/vite.config.ts', { openContent } as never, {
      preview: false,
    })
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/workspaces/ws-1/files/content?path=web%2Fvite.config.ts',
    )
    expect(openContent).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'editor',
        path: 'web/vite.config.ts',
        name: 'vite.config.ts',
        content: 'hello world',
      }),
    )
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('decodes base64-encoded content', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      content: btoa('binary'),
      encoding: 'base64',
    })
    const openContent = vi.fn()
    await openFileContent('ws-1', 'a/b.bin', { openContent } as never, {})
    expect(openContent).toHaveBeenCalledWith(expect.objectContaining({ content: 'binary' }))
  })

  it('shows an error toast and opens nothing on failure', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('500'))
    const openContent = vi.fn()
    await openFileContent('ws-1', 'web/vite.config.ts', { openContent } as never, {
      preview: false,
    })
    expect(openContent).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith('Failed to open file', 'vite.config.ts')
  })

  it('passes isPreview when preview=true', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue({ content: 'x' })
    const openContent = vi.fn()
    await openFileContent('ws-1', 'a/b.ts', { openContent } as never, { preview: true })
    expect(openContent).toHaveBeenCalledWith(expect.objectContaining({ isPreview: true }))
  })
})
