import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, renderHook, screen, waitFor } from '@testing-library/react'

// Every load of the Monaco chunk fails, as it does offline or after a deploy
// replaced the asset hash.
const { loads } = vi.hoisted(() => ({ loads: { count: 0 } }))
vi.mock('@/features/editor/lib/monaco-adapters', () => {
  loads.count++
  throw new Error('chunk load failed')
})

import { SidebarToastOverlay } from '@/components/layout/sidebar-toast-overlay'
import { useEditorHost } from '@/features/editor/host/use-editor-host'
import { toastManager } from '@/lib/toast-manager'
import { toast } from '@/features/window/stores/toast-store'

const LOAD_FAILED = expect.objectContaining({
  key: 'editor-host-load',
  type: 'error',
  message: 'Editor failed to load',
})

afterEach(() => {
  act(() => toastManager.close('editor-host-load'))
  cleanup()
  vi.restoreAllMocks()
})

describe('useEditorHost', () => {
  it('reports a failed load, and the next mount retries it', async () => {
    const show = vi.spyOn(toast, 'show')

    const first = renderHook(() => useEditorHost())
    await waitFor(() => expect(show).toHaveBeenCalledWith(LOAD_FAILED))
    expect(first.result.current).toBeNull()
    first.unmount()

    renderHook(() => useEditorHost())
    await waitFor(() => expect(loads.count).toBe(2))
  })

  it('two panes failing together raise one toast', async () => {
    const show = vi.spyOn(toast, 'show')
    render(createElement(SidebarToastOverlay, { sidebarOpen: true, sidebarSide: 'left' }))

    renderHook(() => useEditorHost())
    renderHook(() => useEditorHost())

    await waitFor(() => expect(show).toHaveBeenCalledTimes(2))
    expect(show).toHaveBeenNthCalledWith(2, LOAD_FAILED)
    expect(screen.getAllByText('Editor failed to load')).toHaveLength(1)
  })
})
