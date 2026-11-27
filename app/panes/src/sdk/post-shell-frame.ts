import type { PaneFrame } from './pane-frame'
import { SDK_MAJOR } from './sdk-limits'
import { shellChannel } from './shell-channel'

export function postShellFrame(frame: PaneFrame): boolean {
  const channel = shellChannel()
  if (!channel) return false
  channel.post(JSON.stringify({ sdk: SDK_MAJOR, ...frame }))
  return true
}
