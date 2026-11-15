import { followChatLifecycle } from '@/features/agent/stores/chat-lifecycle'
import { holdShownChatFeeds } from '@/features/panes/lib/shown-chat-feeds'
import { bootPane } from './pane-boot'
import { ChatPaneMount } from './chat-pane-mount'

const { client, container } = bootPane('core.chat')
followChatLifecycle()
void holdShownChatFeeds()
new ChatPaneMount(container, client).start()
client.register()
