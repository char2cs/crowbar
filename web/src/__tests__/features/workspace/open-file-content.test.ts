import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))
vi.mock('@/components/ui/toast', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

import { apiFetch } from '@/lib/api'
import { toast } from '@/components/ui/toast'
import { openFileContent } from '@/features/workspace/lib/open-file-content'

beforeEach(() => { vi.clearAllMocks() })

describe('openFileContent', () => {
  it('opens an editor buffer with fetched content on success', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue('hello world')
    const openContent = vi.fn()
    await openFileContent('web/vite.config.ts', { openContent } as never, { preview: false })
    expect(openContent).toHaveBeenCalledWith(expect.objectContaining({
      type: 'editor', path: 'web/vite.config.ts', name: 'vite.config.ts', content: 'hello world',
    }))
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('shows an error toast and opens nothing on failure', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('500'))
    const openContent = vi.fn()
    await openFileContent('web/vite.config.ts', { openContent } as never, { preview: false })
    expect(openContent).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith('Failed to open file', expect.objectContaining({ description: 'vite.config.ts' }))
  })

  it('passes isPreview when preview=true', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue('x')
    const openContent = vi.fn()
    await openFileContent('a/b.ts', { openContent } as never, { preview: true })
    expect(openContent).toHaveBeenCalledWith(expect.objectContaining({ isPreview: true }))
  })
})
