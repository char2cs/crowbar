import type { KeyboardEvent } from 'react'
import { IS_MOBILE } from '@/utils/platform'

/** Return sends on desktop; on a phone it is a newline and the send button sends. */
export function isSendKey(event: KeyboardEvent): boolean {
  return event.key === 'Enter' && !IS_MOBILE && !event.shiftKey && !event.nativeEvent.isComposing
}
