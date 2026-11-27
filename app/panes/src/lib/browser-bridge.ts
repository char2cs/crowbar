import { invoke as tauriInvoke } from '@tauri-apps/api/core'
import type { UnlistenFn } from '@tauri-apps/api/event'
import { isDesktopApp } from '@/lib/crowbar-bridge'

export interface BrowserRect {
  x: number
  y: number
  width: number
  height: number
}

type TabEvent<K extends string, V> = { id: string } & Record<K, V>

export interface BrowserScreenshot {
  png: string
  width: number
  height: number
}

function invoke<T = void>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  if (!isDesktopApp()) return Promise.reject(new Error(`${cmd}: needs the desktop app`))
  return tauriInvoke<T>(cmd, args)
}

async function listenTo<T>(event: string, handler: (payload: T) => void): Promise<UnlistenFn> {
  if (!isDesktopApp()) return () => {}
  const { listen } = await import('@tauri-apps/api/event')
  return listen<T>(event, (e) => handler(e.payload))
}

export const browserApi = {
  open: (id: string, url: string, rect: BrowserRect, zoom: number, parked: boolean) =>
    invoke('browser_open', { id, url, zoom, parked, ...rect }),
  setBounds: (id: string, rect: BrowserRect) => invoke('browser_set_bounds', { id, ...rect }),
  park: (id: string) => invoke('browser_park', { id }),
  unpark: (id: string, rect: BrowserRect) => invoke('browser_unpark', { id, ...rect }),
  navigate: (id: string, url: string) => invoke('browser_navigate', { id, url }),
  back: (id: string) => invoke('browser_back', { id }),
  forward: (id: string) => invoke('browser_forward', { id }),
  reload: (id: string) => invoke('browser_reload', { id }),
  close: (id: string) => invoke('browser_close', { id }),
  evaluate: (id: string, body: string, timeoutMs: number) =>
    invoke<string>('browser_eval', { id, body, timeoutMs }),
  screenshot: (id: string, maxWidth?: number) =>
    invoke<BrowserScreenshot>('browser_screenshot', { id, maxWidth }),
  copyText: (text: string) => invoke<void>('browser_copy_text', { text }),
  capabilities: () => invoke<{ clearData: boolean }>('browser_capabilities'),
  clearData: () => invoke<void>('browser_clear_data'),
  onNavigated: (handler: (e: TabEvent<'url', string>) => void): Promise<UnlistenFn> =>
    listenTo('browser://navigated', handler),
  onLoading: (handler: (e: TabEvent<'loading', boolean>) => void): Promise<UnlistenFn> =>
    listenTo('browser://loading', handler),
  onTitle: (handler: (e: TabEvent<'title', string>) => void): Promise<UnlistenFn> =>
    listenTo('browser://title', handler),
  onNewWindow: (handler: (e: TabEvent<'url', string>) => void): Promise<UnlistenFn> =>
    listenTo('browser://new-window', handler),
}
