// Stub
import { create } from "zustand"

export interface WebViewerNavEntry {
  url: string
  canGoBack: boolean
  canGoForward: boolean
  goBack?: () => void
  goForward?: () => void
  reload?: () => void
}

interface WebViewerNavState {
  url: string
  canGoBack: boolean
  canGoForward: boolean
  navigationByBufferId: Record<string, WebViewerNavEntry>
}

export const useWebViewerNavigationStore = create<WebViewerNavState>(() => ({
  url: "",
  canGoBack: false,
  canGoForward: false,
  navigationByBufferId: {},
}))
