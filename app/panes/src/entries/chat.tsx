import '../index.css'
import '@/lib/transport/polyfill'
import { enableMapSet } from 'immer'
import { connectDaemonEvents } from '@/lib/events/connect'
import { initializeSettingsStore } from '@/features/settings/store'
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { installGlobalReporting } from '@/lib/errors/report'
import { startStream } from '@/core/stream/stream'
import { followChatLifecycle } from '@/features/agent/stores/chat-lifecycle'
import { holdShownChatFeeds } from '@/features/panes/lib/shown-chat-feeds'
import { shellChannel } from '@/sdk/shell-channel'
import { PaneClient } from '@/sdk/pane-client'
import { followTheme } from '@/sdk/theme'
import { ChatPaneMount } from './chat-pane-mount'

const CHAT_PANE_TYPE = 'core.chat'

const channel = shellChannel()
const container = document.getElementById('root')
if (!channel || !container) throw new Error('the chat pane runs inside the Crowbar shell')

ensureStartupAppearanceApplied()
enableMapSet()
installGlobalReporting()
startStream()
followChatLifecycle()
connectDaemonEvents()
initializeIconThemes()
void initializeSettingsStore()
void holdShownChatFeeds()

const client = new PaneClient(channel, CHAT_PANE_TYPE)
followTheme(client)
new ChatPaneMount(container, client).start()
client.register()
