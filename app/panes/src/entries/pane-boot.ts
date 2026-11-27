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
import { followBreakpoints } from '@/sdk/window-breakpoints'
import { installPreviewOpener } from '@/features/editor/documents/open-preview'
import { ShellViewedOwner } from '@/features/git/lib/shell-viewed-owner'
import { installViewedOwner } from '@/features/git/lib/viewed-owner'
import type { BootedPane } from './booted-pane'
import { followChatZoom } from './follow-chat-zoom'
import { followFileDrops } from './follow-file-drops'
import { followShellAppearance } from './follow-shell-appearance'
import { followShellHosts } from './follow-shell-hosts'
import { followShellSettings } from './follow-shell-settings'
import { forwardToastsToShell } from './forward-toasts-to-shell'
import { paintPaneSurface } from './paint-pane-surface'

export function bootPane(typeId: string): BootedPane {
  const channel = shellChannel()
  const container = document.getElementById('root')
  if (!channel || !container) throw new Error(`the ${typeId} pane runs inside the Crowbar shell`)

  ensureStartupAppearanceApplied()
  paintPaneSurface(document.body, typeId)
  enableMapSet()
  installGlobalReporting()
  followWorkspaceScopes()
  startStream()
  connectDaemonEvents()
  initializeIconThemes()

  const client = new PaneClient(channel, typeId)
  installViewedOwner(new ShellViewedOwner(client))
  followTheme(client)
  followBreakpoints(client)
  followChatZoom(client)
  followShellAppearance(client)
  followShellSettings(client)
  followShellHosts(client)
  forwardToastsToShell(client)
  followFileDrops(client)
  installPreviewOpener(async (doc) =>
    client.open('core.editor', {}, { file: doc.path, mode: 'preview' }),
  )
  return { client, container }
}
