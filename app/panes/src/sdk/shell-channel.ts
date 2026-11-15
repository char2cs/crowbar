import type { PaneChannel } from './pane-channel'

interface ChannelWindow {
  crowbar?: PaneChannel
}

/** The shell's channel, or `null` in any page that is not hosted by the shell. */
export function shellChannel(): PaneChannel | null {
  const channel = (window as unknown as ChannelWindow).crowbar
  return channel && typeof channel.post === 'function' ? channel : null
}
