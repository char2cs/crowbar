import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  initViewStoreSubscription,
  _resetViewStoreUnsubscribeForTesting,
} from '@/features/editor/stores/view-store'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

const createMockStorage = () => {
  const storage = new Map<string, string>()

  return {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      storage.set(key, value)
    },
    removeItem: (key: string) => {
      storage.delete(key)
    },
    clear: () => {
      storage.clear()
    },
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() {
      return storage.size
    },
  }
}

describe('editor view store large files', () => {
  let wsStore: WorkspaceStore

  beforeEach(() => {
    initViewStoreSubscription()
    vi.stubGlobal('localStorage', createMockStorage())
    vi.stubGlobal('window', {
      __TAURI_INTERNALS__: {
        invoke: vi.fn().mockResolvedValue([]),
        metadata: {
          currentWindow: { label: 'main' },
          currentWebview: { label: 'main' },
        },
      },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
    wsStore = createWorkspaceStore('test-ws')
    setActiveWorkspaceStoreRef(wsStore)
  })

  afterEach(async () => {
    _resetViewStoreUnsubscribeForTesting()
    setActiveWorkspaceStoreRef(null)
    const { useEditorViewStore } = await import('@/features/editor/stores/view-store')
    useEditorViewStore.setState({
      lines: [''],
      lineCount: 1,
    })
    vi.unstubAllGlobals()
  })

  it('tracks large active buffers by line count without storing every line', async () => {
    const { useEditorViewStore } = await import('@/features/editor/stores/view-store')
    const { bufferActions, paneActions } = wsStore.getState()
    const content = Array.from({ length: 50_000 }, (_, index) => `line ${index}`).join('\n')

    const bufferId = bufferActions.openContent({
      type: 'editor',
      path: '/workspace/sqlite.c',
      name: 'sqlite.c',
      content: '',
    })

    paneActions.addBufferToPane(ROOT_PANE_ID, bufferId)
    paneActions.activatePaneBuffer(ROOT_PANE_ID, bufferId)

    wsStore.setState((state) => ({
      ...state,
      buffers: state.buffers.map((b) =>
        b.id === bufferId && b.type === 'editor' ? { ...b, content, isDirty: true } : b,
      ),
    }))

    const viewState = useEditorViewStore.getState()
    expect(viewState.lineCount).toBe(50_000)
    expect(viewState.lines).toHaveLength(0)
    expect(useEditorViewStore.getState().actions.getLines()).toHaveLength(50_000)
  })

  it('updates cached lines incrementally for small typing edits', async () => {
    const { applyIncrementalLineEdit } = await import('@/features/editor/stores/view-store')
    const previousContent = 'first line\nsecond line\nthird line'
    const previousLines = previousContent.split('\n')

    expect(
      applyIncrementalLineEdit(
        previousContent,
        'first line\nsecond fast line\nthird line',
        previousLines,
      ),
    ).toEqual(['first line', 'second fast line', 'third line'])

    expect(
      applyIncrementalLineEdit(
        previousContent,
        'first line\nsecond line\ninserted\nthird line',
        previousLines,
      ),
    ).toEqual(['first line', 'second line', 'inserted', 'third line'])

    expect(
      applyIncrementalLineEdit(previousContent, 'first line\nthird line', previousLines),
    ).toEqual(['first line', 'third line'])

    expect(
      applyIncrementalLineEdit(previousContent, `x${'.'.repeat(1001)}`, previousLines),
    ).toBeNull()
  })

  it('matches full line rebuild for boundary edits', async () => {
    const { applyIncrementalLineEdit } = await import('@/features/editor/stores/view-store')
    const cases = [
      ['alpha\nbeta\ngamma', 'xalpha\nbeta\ngamma'],
      ['alpha\nbeta\ngamma', 'alpha\nxbeta\ngamma'],
      ['alpha\nbeta\ngamma', 'alpha\nbeta\ngammax'],
      ['alpha\nbeta\ngamma', 'alpha\nbeta\n\ngamma'],
      ['alpha\nbeta\ngamma', 'alpha\nbe\nta\ngamma'],
      ['alpha\nbeta\ngamma\n', 'alpha\nbeta\ngamma\nx'],
      ['alpha\nbeta\ngamma', 'alpha\nbeta'],
    ]

    for (const [previousContent, nextContent] of cases) {
      expect(
        applyIncrementalLineEdit(previousContent, nextContent, previousContent.split('\n')),
      ).toEqual(nextContent.split('\n'))
    }
  })
})
