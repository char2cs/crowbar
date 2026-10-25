import { invoke } from '@tauri-apps/api/core'
import type { UnlistenFn } from '@tauri-apps/api/event'

/** Logical px relative to the window content area. */
export interface BrowserRect {
  x: number
  y: number
  width: number
  height: number
}

interface NavigatedEvent {
  id: string
  url: string
}

interface TitleEvent {
  id: string
  title: string
}

interface LoadingEvent {
  id: string
  loading: boolean
}

export interface BrowserScreenshot {
  /** Base64, no data: prefix. */
  png: string
  width: number
  height: number
}

// The event API loads with the first listener, keeping it out of the entry bundle.
async function listenTo<T>(event: string, handler: (payload: T) => void): Promise<UnlistenFn> {
  const { listen } = await import('@tauri-apps/api/event')
  return listen<T>(event, (e) => handler(e.payload))
}

/** The only module that talks to the native webviews (`browser-<id>` in Rust). */
export const browserApi = {
  // Must be idempotent per id: a pane that remounts the same tab calls it again.
  // `zoom` is a factor (1 = 100%) and only applies when the webview is created.
  open: (id: string, url: string, rect: BrowserRect, zoom: number) =>
    invoke('browser_open', { id, url, zoom, ...rect }),
  setBounds: (id: string, rect: BrowserRect) => invoke('browser_set_bounds', { id, ...rect }),
  setVisible: (id: string, visible: boolean) => invoke('browser_set_visible', { id, visible }),
  navigate: (id: string, url: string) => invoke('browser_navigate', { id, url }),
  back: (id: string) => invoke('browser_back', { id }),
  forward: (id: string) => invoke('browser_forward', { id }),
  reload: (id: string) => invoke('browser_reload', { id }),
  close: (id: string) => invoke('browser_close', { id }),
  /** Runs `body` (an async function body) in the page; resolves to its JSON text. */
  evaluate: (id: string, body: string, timeoutMs: number) =>
    invoke<string>('browser_eval', { id, body, timeoutMs }),
  screenshot: (id: string, maxWidth?: number) =>
    invoke<BrowserScreenshot>('browser_screenshot', { id, maxWidth }),
  /** Writes to the OS pasteboard: a pick finishes inside the native webview, so the app document has no user activation for navigator.clipboard. */
  copyText: (text: string) => invoke<void>('browser_copy_text', { text }),
  capabilities: () => invoke<{ clearData: boolean }>('browser_capabilities'),
  clearData: () => invoke<void>('browser_clear_data'),
  onNavigated: (handler: (e: NavigatedEvent) => void): Promise<UnlistenFn> =>
    listenTo<NavigatedEvent>('browser://navigated', handler),
  onLoading: (handler: (e: LoadingEvent) => void): Promise<UnlistenFn> =>
    listenTo<LoadingEvent>('browser://loading', handler),
  onTitle: (handler: (e: TitleEvent) => void): Promise<UnlistenFn> =>
    listenTo<TitleEvent>('browser://title', handler),
}
