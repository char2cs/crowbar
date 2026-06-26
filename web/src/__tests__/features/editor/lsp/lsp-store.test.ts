import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { CompletionItem } from 'vscode-languageserver-protocol'
import type { useLspStore as useLspStoreHook } from '@/features/editor/lsp/lsp-store'
import type { useEditorUIStore as useEditorUIStoreHook } from '@/features/editor/stores/ui-store'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    show: vi.fn(),
    dismissByKey: vi.fn(),
  },
}))

import { useLspStore } from '@/features/editor/lsp/lsp-store'
import { toast } from '@/features/window/stores/toast-store'

describe('lspStore — toast decoupling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useLspStore.setState((state) => ({
      ...state,
      lspStatus: {
        ...state.lspStatus,
        lastError: undefined,
        status: 'disconnected' as const,
      },
    }))
  })

  it('setLspError updates lspStatus.lastError without calling toast.show', () => {
    useLspStore.getState().actions.setLspError('language server crashed')

    expect(useLspStore.getState().lspStatus.lastError).toBe('language server crashed')
    expect(useLspStore.getState().lspStatus.status).toBe('error')
    expect(toast.show).not.toHaveBeenCalled()
  })

  it('clearLspError clears lspStatus.lastError without calling toast.dismissByKey', () => {
    useLspStore.setState((state) => ({
      ...state,
      lspStatus: {
        ...state.lspStatus,
        lastError: 'some error',
        status: 'error' as const,
        activeWorkspaces: ['ws1'],
      },
    }))

    useLspStore.getState().actions.clearLspError()

    expect(useLspStore.getState().lspStatus.lastError).toBeUndefined()
    expect(toast.dismissByKey).not.toHaveBeenCalled()
  })
})

type LspStoreHook = typeof useLspStoreHook
type EditorUIStoreHook = typeof useEditorUIStoreHook

describe('lspStore — completions', () => {
  let useLspStoreDynamic: LspStoreHook
  let useEditorUIStore: EditorUIStoreHook

  beforeEach(async () => {
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
    ;({ useLspStore: useLspStoreDynamic } = await import('@/features/editor/lsp/lsp-store'))
    ;({ useEditorUIStore } = await import('@/features/editor/stores/ui-store'))
  })

  afterEach(() => {
    useEditorUIStore?.setState({
      filteredCompletions: [],
      isLspCompletionVisible: false,
      selectedLspIndex: 0,
      currentPrefix: '',
      isApplyingCompletion: false,
    })
    useLspStoreDynamic?.setState({
      getCompletions: undefined,
      isLanguageSupported: undefined,
      currentCompletionRequest: null,
      completionCache: {},
    })
    vi.unstubAllGlobals()
  })

  it('shows unfiltered completions for a manual empty-prefix request', async () => {
    const completions: CompletionItem[] = [{ label: 'alpha' }, { label: 'beta' }]
    const getCompletions = vi.fn(async () => completions)

    useLspStoreDynamic.getState().actions.setCompletionHandlers(getCompletions, () => true)

    await useLspStoreDynamic.getState().actions.performCompletionRequest({
      filePath: '/tmp/manual.ts',
      cursorPos: 0,
      value: '',
      editorRef: { current: {} as HTMLDivElement },
      manual: true,
    })

    expect(getCompletions).toHaveBeenCalledWith('/tmp/manual.ts', 0, 0)
    expect(useEditorUIStore.getState().isLspCompletionVisible).toBe(true)
    expect(useEditorUIStore.getState().filteredCompletions).toEqual([
      { item: { label: 'alpha' }, score: 1, indices: [] },
      { item: { label: 'beta' }, score: 1, indices: [] },
    ])
  })
})
