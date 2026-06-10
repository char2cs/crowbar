import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createLspSlice, type LspSlice } from '@/features/workspace/stores/slices/lsp-slice'

function makeStore() {
  return createStore<LspSlice>()(
    immer((set, get) => createLspSlice(set as any, get as any, {} as any)),
  )
}

describe('lsp-slice', () => {
  let store: ReturnType<typeof makeStore>
  beforeEach(() => {
    store = makeStore()
  })

  it('starts with empty workspace root and idle status', () => {
    expect(store.getState().workspaceRoot).toBe('')
    expect(store.getState().lspStatus.status).toBe('idle')
  })

  it('setWorkspaceRoot updates root', () => {
    store.getState().lspActions.setWorkspaceRoot('/home/user/project')
    expect(store.getState().workspaceRoot).toBe('/home/user/project')
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
    store.getState().lspActions.updateCompletionCache('key-1', [{ label: 'foo' } as any])
    store.getState().lspActions.clearCompletionCache()
    expect(Object.keys(store.getState().completionCache)).toHaveLength(0)
  })
})
