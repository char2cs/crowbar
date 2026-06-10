import { create } from 'zustand'
import { browserPaneGoBack, browserPaneGoForward, browserPaneReload } from '@/lib/crowbar-bridge'

export interface WebViewerNavEntry {
  url: string
  canGoBack: boolean
  canGoForward: boolean
  goBack: () => void
  goForward: () => void
  reload: () => void
}

interface WebViewerNavState {
  navigationByBufferId: Record<string, WebViewerNavEntry>
  registerBuffer: (bufferId: string, initialUrl: string) => void
  updateNavState: (
    bufferId: string,
    state: { url: string; canGoBack: boolean; canGoForward: boolean },
  ) => void
  removeBuffer: (bufferId: string) => void
}

export const useWebViewerNavigationStore = create<WebViewerNavState>((set) => ({
  navigationByBufferId: {},

  registerBuffer(bufferId, initialUrl) {
    set((state) => ({
      navigationByBufferId: {
        ...state.navigationByBufferId,
        [bufferId]: {
          url: initialUrl,
          canGoBack: false,
          canGoForward: false,
          goBack: () => void browserPaneGoBack(bufferId),
          goForward: () => void browserPaneGoForward(bufferId),
          reload: () => void browserPaneReload(bufferId),
        },
      },
    }))
  },

  // Also handles the case where a Tauri nav event arrives before registerBuffer
  // (race at mount): creates the entry with fallback bridge-calling functions.
  updateNavState(bufferId, { url, canGoBack, canGoForward }) {
    set((state) => {
      const existing = state.navigationByBufferId[bufferId]
      return {
        navigationByBufferId: {
          ...state.navigationByBufferId,
          [bufferId]: {
            url,
            canGoBack,
            canGoForward,
            goBack: existing?.goBack ?? (() => void browserPaneGoBack(bufferId)),
            goForward: existing?.goForward ?? (() => void browserPaneGoForward(bufferId)),
            reload: existing?.reload ?? (() => void browserPaneReload(bufferId)),
          },
        },
      }
    })
  },

  removeBuffer(bufferId) {
    set((state) => {
      const next = { ...state.navigationByBufferId }
      delete next[bufferId]
      return { navigationByBufferId: next }
    })
  },
}))
