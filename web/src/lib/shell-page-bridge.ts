// The phone's chat list, settings and file drawer pages run in extra webviews with no Tauri IPC: Swift
// hands each the main webview's snapshot through a global, and carries what it posts back
// to the main webview as intents.

import type { NativeChromeIntent, NativeRowAction } from '@/lib/native-chrome'

type PageIntent = Extract<
  NativeChromeIntent,
  {
    type:
      | 'selectRow'
      | 'selectProject'
      | 'toggleFold'
      | 'openSettings'
      | 'toggleProvider'
      | 'toggleProviderTools'
      | 'reorderProviders'
      | 'setDefaultPermission'
      | 'addHost'
      | 'revokeHost'
      | 'toggleDir'
      | 'openFile'
      | 'dragRow'
      | 'keepRemoval'
      | 'moveRow'
  }
>
/** A row was held still: Swift presents its menu natively and forwards only a pick. */
type RowMenu = {
  type: 'rowMenu'
  rowId: string
  rect: { x: number; y: number; width: number; height: number }
  actions: NativeRowAction[]
}
/** A row drag began or ended: Swift gives a haptic. */
type RowDrag = { type: 'rowDrag'; active: boolean }
type PageMessage =
  | { type: 'sidebarReady' }
  | RowDrag
  | { type: 'agentsSettingsReady' }
  | { type: 'hostsSettingsReady' }
  | { type: 'filesReady' }
  | RowMenu
  | PageIntent

/** The message handler Swift listens on, and the global it calls with each snapshot. */
const CHANNELS = {
  sidebar: { handler: 'sidebar', receiver: '__crowbarSidebar', ready: 'sidebarReady' },
  agents: {
    handler: 'settings',
    receiver: '__crowbarAgentsSettings',
    ready: 'agentsSettingsReady',
  },
  hosts: { handler: 'settings', receiver: '__crowbarHostsSettings', ready: 'hostsSettingsReady' },
  files: { handler: 'files', receiver: '__crowbarFiles', ready: 'filesReady' },
} as const
type Channel = keyof typeof CHANNELS
type Handler = (typeof CHANNELS)[Channel]['handler']

type Handlers = Partial<Record<Handler, { postMessage: (message: PageMessage) => void }>>
interface ShellWindow {
  webkit?: { messageHandlers?: Handlers }
  [receiver: string]: unknown
}

const shell = () => window as unknown as ShellWindow

export function postToShell(channel: Channel, message: PageMessage): void {
  shell().webkit?.messageHandlers?.[CHANNELS[channel].handler]?.postMessage(message)
}

/** Takes every snapshot the shell sends on `channel`, starting with the latest one, which it
 *  replays on the ready message. Returns the teardown. */
export function receiveShellSnapshots<T>(channel: Channel, onSnapshot: (snapshot: T) => void) {
  const { receiver, ready } = CHANNELS[channel]
  shell()[receiver] = onSnapshot
  postToShell(channel, { type: ready })
  return () => {
    if (shell()[receiver] === onSnapshot) delete shell()[receiver]
  }
}
