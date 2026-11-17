import { report } from '@/lib/errors/report'
import { browserApi, type BrowserRect } from '@/lib/browser-bridge'
import { useSettingsStore } from '@/features/settings/store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { registerWebview, webviewCreated } from './webview-registry'

/**
 * The tab's webview, made on first call at `rect` and on screen. Undefined while the tab has
 * no page to load; a second call for the same tab returns the first's creation.
 */
export function ensureWebview(bufferId: string, rect: BrowserRect): Promise<unknown> | undefined {
  const existing = webviewCreated(bufferId)
  if (existing) return existing
  const buffer = windowPaneStore.getState().bufferActions.getBufferById(bufferId)
  if (buffer?.type !== 'browser' || buffer.url === '') return undefined
  const zoom = useSettingsStore.getState().settings.browserZoom / 100
  const created = browserApi.open(bufferId, buffer.url, rect, zoom, false)
  registerWebview(bufferId, created)
  void created.catch((err: unknown) => report(err, { action: 'Open the page' }))
  return created
}
