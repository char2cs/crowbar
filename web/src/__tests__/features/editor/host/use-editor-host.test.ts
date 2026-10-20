import { describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'

// Every load of the Monaco chunk fails, as it does offline or after a deploy
// replaced the asset hash.
const { loads } = vi.hoisted(() => ({ loads: { count: 0 } }))
vi.mock('@/features/editor/lib/monaco-adapters', () => {
  loads.count++
  throw new Error('chunk load failed')
})

import { useEditorHost } from '@/features/editor/host/use-editor-host'
import { toast } from '@/features/window/stores/toast-store'

describe('useEditorHost', () => {
  it('reports a failed load, and the next mount retries it', async () => {
    const show = vi.spyOn(toast, 'show')

    const first = renderHook(() => useEditorHost())
    await waitFor(() =>
      expect(show).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'error', message: 'Editor failed to load' }),
      ),
    )
    expect(first.result.current).toBeNull()
    first.unmount()

    renderHook(() => useEditorHost())
    await waitFor(() => expect(loads.count).toBe(2))
  })
})
