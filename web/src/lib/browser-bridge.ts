import { invoke as tauriInvoke } from '@tauri-apps/api/core'
import type { UnlistenFn } from '@tauri-apps/api/event'
import { isDesktopApp } from '@/lib/crowbar-bridge'

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

// The webview commands exist only in the desktop build: elsewhere a call is refused, not sent.
function invoke<T = void>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  if (!isDesktopApp()) return Promise.reject(new Error(`${cmd}: needs the desktop app`))
  return tauriInvoke<T>(cmd, args)
}

// The event API loads with the first listener, keeping it out of the entry bundle.
async function listenTo<T>(event: string, handler: (payload: T) => void): Promise<UnlistenFn> {
  const { listen } = await import('@tauri-apps/api/event')
  return listen<T>(event, (e) => handler(e.payload))
}

/** The only module that talks to the native webviews (`browser-<id>` in Rust). */
export const browserApi = {
  // Must be idempotent per id. `zoom` is a factor (1 = 100%) and only applies
  // when the webview is created; `parked` creates it off screen at `rect`'s size.
  open: (id: string, url: string, rect: BrowserRect, zoom: number, parked: boolean) =>
    invoke('browser_open', { id, url, zoom, parked, ...rect }),
  /** Ignored while parked: only `unpark` brings a tab back. */
  setBounds: (id: string, rect: BrowserRect) => invoke('browser_set_bounds', { id, ...rect }),
  /** Moves the tab out of the window but keeps it live at its size, so its page is not throttled. */
  park: (id: string) => invoke('browser_park', { id }),
  unpark: (id: string, rect: BrowserRect) => invoke('browser_unpark', { id, ...rect }),
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
