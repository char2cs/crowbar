import { renderHook } from '@testing-library/react'
import type React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useTreeContainerEvents } from '@/features/file-explorer/file-explorer/hooks/use-tree-container-events'
import { useFileClipboardStore } from '@/features/file-explorer/stores/file-explorer-clipboard-store'
import { toast } from '@/features/window/stores/toast-store'

const rows = [
  { file: { path: 'src', name: 'src', isDir: true }, depth: 0, isExpanded: true },
  { file: { path: 'src/main.ts', name: 'main.ts', isDir: false }, depth: 1, isExpanded: false },
]

function treeEvents(workspaceId: string | null, keyboardPath = 'src/main.ts') {
  const { result } = renderHook(() =>
    useTreeContainerEvents({
      workspaceId,
      containerRef: { current: null },
      visibleRows: rows,
      rowVirtualizer: { scrollToIndex: vi.fn() },
      keyboardPath,
      setFocusedPath: vi.fn(),
      onFileSelect: vi.fn(),
      openSearch: vi.fn(),
      handleContextMenu: vi.fn(),
      closeContextMenu: vi.fn(),
      isDragging: false,
      startDrag: vi.fn(),
    }),
  )
  return result.current
}

function press(events: ReturnType<typeof treeEvents>, key: string) {
  events.onKeyDown({
    key,
    metaKey: true,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    target: document.body,
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
  } as unknown as React.KeyboardEvent)
}

beforeEach(() => useFileClipboardStore.setState({ clipboard: null }))
afterEach(() => vi.restoreAllMocks())

// The keyboard twin of the context menu's Copy/Cut: paths mean something only in the explorer's
// own workspace, so a paste anywhere else refuses before any request.
describe('the tree’s copy, cut and paste keys', () => {
  it.each([
    ['c', 'copy'],
    ['x', 'cut'],
  ] as const)('⌘%s records the focused entry with the explorer’s workspace', (key, operation) => {
    press(treeEvents('ws-explorer'), key)

    expect(useFileClipboardStore.getState().clipboard).toEqual({
      entries: [{ path: 'src/main.ts', is_dir: false }],
      operation,
      wsId: 'ws-explorer',
    })
  })

  it('⌘V refuses entries copied in another workspace, and says so', async () => {
    const error = vi.spyOn(toast, 'error').mockReturnValue('')
    press(treeEvents('ws-a'), 'c')

    press(treeEvents('ws-b', 'src'), 'v')

    await vi.waitFor(() =>
      expect(error).toHaveBeenCalledWith(
        'Paste failed',
        'Copied from another workspace — paste there, or copy again here',
      ),
    )
  })
})
