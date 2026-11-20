import '../index.css'
import '@/lib/transport/polyfill'
import { enableMapSet } from 'immer'
import { connectDaemonEvents } from '@/lib/events/connect'
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { installGlobalReporting } from '@/lib/errors/report'
import { startStream } from '@/core/stream/stream'
import { followWorkspaceScopes } from '@/lib/follow-workspace-scopes'
import { shellChannel } from '@/sdk/shell-channel'
import { PaneClient } from '@/sdk/pane-client'
import { followTheme } from '@/sdk/theme'
import { ShellViewedOwner } from '@/features/git/lib/shell-viewed-owner'
import { installViewedOwner } from '@/features/git/lib/viewed-owner'
import type { BootedPane } from './booted-pane'
import { followShellAppearance } from './follow-shell-appearance'
import { followShellSettings } from './follow-shell-settings'
import { paintPaneSurface } from './paint-pane-surface'

/** Starts the runtime every pane page shares (appearance, reporting, workspace scopes, the daemon stream and events, the shell's settings) and connects it to the shell as the `typeId` pane. The caller mounts its UI on `container` and then calls `client.register()`. */
export function bootPane(typeId: string): BootedPane {
  const channel = shellChannel()
  const container = document.getElementById('root')
  if (!channel || !container) throw new Error(`the ${typeId} pane runs inside the Crowbar shell`)

  ensureStartupAppearanceApplied()
  paintPaneSurface(document.body)
  enableMapSet()
  installGlobalReporting()
  followWorkspaceScopes()
  startStream()
  connectDaemonEvents()
  initializeIconThemes()

  const client = new PaneClient(channel, typeId)
  installViewedOwner(new ShellViewedOwner(client))
  followTheme(client)
  followShellAppearance(client)
  followShellSettings(client)
  return { client, container }
}
