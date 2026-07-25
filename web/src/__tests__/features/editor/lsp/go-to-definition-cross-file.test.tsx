import type React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'

// Cmd+Click into a DIFFERENT file did nothing at all — no jump, no tab, no
// error. The hook took the LSP target as an ABSOLUTE path (it "stripped" the
// workspace root using a store field that was provably always ''), so the
// open-buffer lookup could never match and `readWorkspaceFile` was handed an
// absolute path that safepath rejects with ErrPathEscapesWorkspace. The
// rejection landed in a catch that only called `logger.error`, so the user saw
// nothing. The daemon now answers with workspace-relative paths and this locks
// both halves: the jump works, and every dead end is spoken aloud.

const { readWorkspaceFile } = vi.hoisted(() => ({
  readWorkspaceFile: vi.fn(async () => 'target file contents'),
}))
const { toast } = vi.hoisted(() => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))
const { pushEntry } = vi.hoisted(() => ({ pushEntry: vi.fn() }))
const { openContent, addBufferToPane, activatePaneBuffer, getPaneByBufferId } = vi.hoisted(() => ({
  openContent: vi.fn(() => 'buffer-new'),
  addBufferToPane: vi.fn(),
  activatePaneBuffer: vi.fn(),
  getPaneByBufferId: vi.fn(() => null as { id: string } | null),
}))

interface TestBuffer {
  id: string
  path: string
  type: 'editor'
}

const wsState = {
  workspaceId: 'ws-1',
  activePaneId: 'pane-1',
  panes: { 'pane-1': { id: 'pane-1', activeBufferId: 'buffer-app' } } as Record<
    string,
    { id: string; activeBufferId: string }
  >,
  buffers: [] as TestBuffer[],
  paneActions: { getPaneByBufferId, addBufferToPane, activatePaneBuffer },
  bufferActions: { openContent },
}

vi.mock('@/features/workspace/stores/workspace-store-ref', () => ({
  getActiveWorkspaceStoreRef: () => ({ getState: () => wsState }),
}))
vi.mock('@/features/file-system/controllers/platform', () => ({ readWorkspaceFile }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast }))
vi.mock('@/features/editor/stores/jump-list-store', () => ({
  useJumpListStore: { getState: () => ({ actions: { pushEntry } }) },
}))
vi.mock('@/features/editor/stores/state-store', () => ({
  useEditorStateStore: {
    getState: () => ({
      cursorPosition: { line: 1, column: 2, offset: 3 },
      scrollTop: 0,
      scrollLeft: 0,
    }),
  },
}))
vi.mock('@/features/editor/hooks/use-center-cursor', () => ({
  useCenterCursor: () => ({ centerCursorInViewport: vi.fn() }),
}))
vi.mock('@/features/editor/extensions/api', () => ({
  editorAPI: { getContent: () => '', setCursorPosition: vi.fn() },
}))
vi.mock('@/features/editor/utils/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), warn: vi.fn(), error: vi.fn() },
}))

import { useGoToDefinition } from '@/features/editor/lsp/use-go-to-definition'
import type { EditorResolvedPosition } from '@/features/editor/view-model/view-layout'

const RANGE = { start: { line: 7, character: 4 }, end: { line: 7, character: 12 } }

function cmdClick(): React.MouseEvent<HTMLDivElement> {
  const editor = document.createElement('div')
  return {
    metaKey: true,
    ctrlKey: false,
    preventDefault: vi.fn(),
    currentTarget: editor,
    clientX: 100,
    clientY: 50,
  } as unknown as React.MouseEvent<HTMLDivElement>
}

async function clickWithDefinition(filePath: string) {
  const { result } = renderHook(() =>
    useGoToDefinition({
      getDefinition: async () => [{ filePath, range: RANGE }],
      isLanguageSupported: () => true,
      filePath: 'src/app.ts',
      lineHeight: 18,
      charWidth: 8,
      // The hook reads only line/column off the resolved position; the rest of
      // the layout record is irrelevant to which file it jumps to.
      resolveEditorPosition: () => ({ line: 5, column: 3 }) as EditorResolvedPosition,
    }),
  )

  await act(async () => {
    await result.current.handleClick(cmdClick())
    // The hook defers cursor placement (setTimeout -> requestAnimationFrame,
    // use-go-to-definition.ts). Flush it inside the test: left pending, that
    // timer fires after the environment is torn down, and the rAF call throws
    // `requestAnimationFrame is not defined` as an unhandled error that fails
    // the whole run — intermittently, since whether it fires before teardown
    // depends on machine load.
    await vi.runAllTimersAsync()
  })
}

describe('go to definition across files', () => {
  beforeEach(() => {
    // Fake only the timers the hook actually uses, so React Testing Library's
    // own async `act` scheduling keeps its real timers.
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'requestAnimationFrame', 'cancelAnimationFrame'],
    })
    vi.clearAllMocks()
    readWorkspaceFile.mockResolvedValue('target file contents')
    openContent.mockReturnValue('buffer-new')
    getPaneByBufferId.mockReturnValue(null)
    wsState.buffers = [{ id: 'buffer-app', path: 'src/app.ts', type: 'editor' }]
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('opens the target file through its workspace-relative path', async () => {
    await clickWithDefinition('src/lib/target.ts')

    expect(readWorkspaceFile).toHaveBeenCalledWith('ws-1', 'src/lib/target.ts')
    expect(openContent).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'editor',
        path: 'src/lib/target.ts',
        name: 'target.ts',
        content: 'target file contents',
      }),
    )
    expect(activatePaneBuffer).toHaveBeenCalledWith('pane-1', 'buffer-new')
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('reveals a target already open in another pane instead of re-reading it', async () => {
    wsState.buffers = [
      { id: 'buffer-app', path: 'src/app.ts', type: 'editor' },
      { id: 'buffer-target', path: 'src/lib/target.ts', type: 'editor' },
    ]
    getPaneByBufferId.mockReturnValue({ id: 'pane-2' })

    await clickWithDefinition('src/lib/target.ts')

    expect(readWorkspaceFile).not.toHaveBeenCalled()
    expect(openContent).not.toHaveBeenCalled()
    expect(addBufferToPane).not.toHaveBeenCalled()
    expect(activatePaneBuffer).toHaveBeenCalledWith('pane-2', 'buffer-target')
  })

  it('records the departure point in the jump list before navigating', async () => {
    await clickWithDefinition('src/lib/target.ts')

    expect(pushEntry).toHaveBeenCalledWith(
      expect.objectContaining({
        bufferId: 'buffer-app',
        workspaceId: 'ws-1',
        filePath: 'src/app.ts',
      }),
    )
  })

  it('tells the user when the target cannot be read instead of failing silently', async () => {
    readWorkspaceFile.mockRejectedValue(new Error('path escapes the workspace'))

    await clickWithDefinition('src/lib/target.ts')

    expect(openContent).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalled()
  })

  it('reports an out-of-workspace target rather than issuing a read that cannot succeed', async () => {
    await clickWithDefinition('/usr/local/go/src/fmt/print.go')

    expect(readWorkspaceFile).not.toHaveBeenCalled()
    expect(openContent).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalled()
  })
})
