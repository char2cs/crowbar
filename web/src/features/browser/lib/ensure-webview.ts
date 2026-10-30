import { bestEffort } from '@/lib/best-effort'
import { browserApi, type BrowserRect } from '@/lib/browser-bridge'
import { useSettingsStore } from '@/features/settings/store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { registerWebview, webviewCreated } from './webview-registry'

/**
 * The tab's webview, made on first call: at `rect` and on screen, or parked
 * there (see `browser_open`). Undefined while the tab has no page to load.
 * The pane and the agent both come through here, so a tab never gets two.
 */
export function ensureWebview(
  bufferId: string,
  rect: BrowserRect,
  parked: boolean,
): Promise<unknown> | undefined {
  const existing = webviewCreated(bufferId)
  if (existing) return existing
  const buffer = windowPaneStore.getState().bufferActions.getBufferById(bufferId)
  if (buffer?.type !== 'browser' || buffer.url === '') return undefined
  const zoom = useSettingsStore.getState().settings.browserZoom / 100
  const created = browserApi.open(bufferId, buffer.url, rect, zoom, parked)
  registerWebview(bufferId, created)
  bestEffort(created, 'open browser webview')
  return created
}
