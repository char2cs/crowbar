import type { PaneChannel } from './pane-channel'

interface ChannelWindow {
  crowbar?: PaneChannel
}

export function shellChannel(): PaneChannel | null {
  const channel = (window as unknown as ChannelWindow).crowbar
  return channel && typeof channel.post === 'function' ? channel : null
}
