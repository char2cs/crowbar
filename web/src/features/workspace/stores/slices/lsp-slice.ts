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
  setCompletionHandlers(handlers: CompletionHandlers): void
  updateLspStatus(info: Partial<LspStatusInfo>): void
  updateCompletionCache(key: string, items: CompletionItem[]): void
  clearCompletionCache(): void
}

// The workspace's absolute disk root deliberately does NOT live here. The
// frontend never learns it: the daemon owns the LSP session and the worktree,
// and it relativizes every path it returns. A `workspaceRoot` field used to sit
// in this slice with no production writer, so it was permanently '' and the one
// reader (go-to-definition) silently "stripped" nothing.
export interface LspSlice {
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
  lspStatus: { status: 'idle' },
  completionCache: {},
  currentCompletionRequest: null,
  getCompletions: undefined,
  isLanguageSupported: undefined,

  lspActions: {
    setCompletionHandlers({ getCompletions, isLanguageSupported }) {
      set((state) => {
        if (getCompletions) state.getCompletions = getCompletions
        if (isLanguageSupported) state.isLanguageSupported = isLanguageSupported
      })
    },

    updateLspStatus(info) {
      set((state) => {
        Object.assign(state.lspStatus, info)
      })
    },

    updateCompletionCache(key, items) {
      set((state) => {
        state.completionCache[key] = items
      })
    },

    clearCompletionCache() {
      set((state) => {
        state.completionCache = {}
      })
    },
  },
})
