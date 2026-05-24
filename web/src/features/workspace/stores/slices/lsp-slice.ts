import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { CompletionItem } from 'vscode-languageserver-types'

export interface LspStatusInfo {
  status: 'idle' | 'starting' | 'running' | 'error'
  activeWorkspaces?: string[]
  lastError?: string
  supportedLanguages?: string[]
}

export type CompletionCache = Record<string, CompletionItem[]>

export interface CompletionHandlers {
  getCompletions?: (filePath: string, line: number, character: number) => Promise<CompletionItem[]>
  isLanguageSupported?: (filePath: string) => boolean
}

export interface LspActions {
  setWorkspaceRoot(root: string): void
  setCompletionHandlers(handlers: CompletionHandlers): void
  updateLspStatus(info: Partial<LspStatusInfo>): void
  updateCompletionCache(key: string, items: CompletionItem[]): void
  clearCompletionCache(): void
}

export interface LspSlice {
  workspaceRoot: string
  lspStatus: LspStatusInfo
  completionCache: CompletionCache
  currentCompletionRequest: AbortController | null
  getCompletions?: (filePath: string, line: number, character: number) => Promise<CompletionItem[]>
  isLanguageSupported?: (filePath: string) => boolean
  lspActions: LspActions
}

export const createLspSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  LspSlice
> = (set, _get) => ({
  workspaceRoot: '',
  lspStatus: { status: 'idle' },
  completionCache: {},
  currentCompletionRequest: null,
  getCompletions: undefined,
  isLanguageSupported: undefined,

  lspActions: {
    setWorkspaceRoot(root) {
      set(state => { state.workspaceRoot = root })
    },

    setCompletionHandlers({ getCompletions, isLanguageSupported }) {
      set(state => {
        if (getCompletions) state.getCompletions = getCompletions
        if (isLanguageSupported) state.isLanguageSupported = isLanguageSupported
      })
    },

    updateLspStatus(info) {
      set(state => { Object.assign(state.lspStatus, info) })
    },

    updateCompletionCache(key, items) {
      set(state => { state.completionCache[key] = items })
    },

    clearCompletionCache() {
      set(state => { state.completionCache = {} })
    },
  },
})
