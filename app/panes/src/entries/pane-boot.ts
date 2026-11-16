import '../index.css'
import '@/lib/transport/polyfill'
import { enableMapSet } from 'immer'
import { connectDaemonEvents } from '@/lib/events/connect'
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { installGlobalReporting } from '@/lib/errors/report'
import { startStream } from '@/core/stream/stream'
import { shellChannel } from '@/sdk/shell-channel'
import { PaneClient } from '@/sdk/pane-client'
import { followTheme } from '@/sdk/theme'
import type { BootedPane } from './booted-pane'
import { followShellAppearance } from './follow-shell-appearance'
import { followShellSettings } from './follow-shell-settings'
import { paintPaneSurface } from './paint-pane-surface'

/** Starts the runtime every pane page shares (appearance, reporting, the daemon stream and events, the shell's settings) and connects it to the shell as the `typeId` pane. The caller mounts its UI on `container` and then calls `client.register()`. */
export function bootPane(typeId: string): BootedPane {
  const channel = shellChannel()
  const container = document.getElementById('root')
  if (!channel || !container) throw new Error(`the ${typeId} pane runs inside the Crowbar shell`)

  ensureStartupAppearanceApplied()
  paintPaneSurface(document.body)
  enableMapSet()
  installGlobalReporting()
  startStream()
  connectDaemonEvents()
  initializeIconThemes()

  const client = new PaneClient(channel, typeId)
  followTheme(client)
  followShellAppearance(client)
  followShellSettings(client)
  return { client, container }
}
