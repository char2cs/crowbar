import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createLspSlice, type LspSlice } from '@/features/workspace/stores/slices/lsp-slice'

function makeStore() {
  return createStore<LspSlice>()(
    immer((set, get) =>
      createLspSlice(...([set, get, {}] as unknown as Parameters<typeof createLspSlice>)),
    ),
  )
}

describe('lsp-slice', () => {
  let store: ReturnType<typeof makeStore>
  beforeEach(() => {
    store = makeStore()
  })

  it('starts with idle status', () => {
    expect(store.getState().lspStatus.status).toBe('idle')
  })

  // The slice deliberately holds NO workspace root. It used to carry a
  // `workspaceRoot` field with no production writer, so it was always '' while
  // go-to-definition believed it was stripping a real prefix with it. The
  // daemon owns the worktree and returns workspace-relative paths, so nothing
  // here needs the absolute root.
  it('does not carry a workspace root', () => {
    expect(store.getState()).not.toHaveProperty('workspaceRoot')
    expect(store.getState().lspActions).not.toHaveProperty('setWorkspaceRoot')
  })

  it('updateLspStatus updates status fields', () => {
    store
      .getState()
      .lspActions.updateLspStatus({ status: 'running', supportedLanguages: ['typescript'] })
    expect(store.getState().lspStatus.status).toBe('running')
    expect(store.getState().lspStatus.supportedLanguages).toEqual(['typescript'])
  })

  it('setCompletionHandlers stores handlers', () => {
    const isSupported = () => true
    store.getState().lspActions.setCompletionHandlers({ isLanguageSupported: isSupported })
    expect(store.getState().isLanguageSupported).toBe(isSupported)
  })

  it('clearCompletionCache empties the cache', () => {
    store.getState().lspActions.updateCompletionCache('key-1', [{ label: 'foo' }])
    store.getState().lspActions.clearCompletionCache()
    expect(Object.keys(store.getState().completionCache)).toHaveLength(0)
  })
})
