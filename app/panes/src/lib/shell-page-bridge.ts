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
      | 'toggleDir'
      | 'openFile'
      | 'dragRow'
      | 'moveRow'
  }
>
type RowMenu = {
  type: 'rowMenu'
  rowId: string
  rect: { x: number; y: number; width: number; height: number }
  actions: NativeRowAction[]
}
type RowDrag = { type: 'rowDrag'; active: boolean }
type PageMessage =
  | { type: 'sidebarReady' }
  | RowDrag
  | { type: 'agentsSettingsReady' }
  | { type: 'settingsTabReady' }
  | { type: 'filesReady' }
  | RowMenu
  | PageIntent

const CHANNELS = {
  sidebar: { handler: 'sidebar', receiver: '__crowbarSidebar', ready: 'sidebarReady' },
  agents: {
    handler: 'settings',
    receiver: '__crowbarAgentsSettings',
    ready: 'agentsSettingsReady',
  },
  tab: { handler: 'settings', receiver: '__crowbarSettingsTab', ready: 'settingsTabReady' },
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

export function receiveShellSnapshots<T>(channel: Channel, onSnapshot: (snapshot: T) => void) {
  const { receiver, ready } = CHANNELS[channel]
  shell()[receiver] = onSnapshot
  postToShell(channel, { type: ready })
  return () => {
    if (shell()[receiver] === onSnapshot) delete shell()[receiver]
  }
}
