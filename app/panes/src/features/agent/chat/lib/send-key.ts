import type { KeyboardEvent } from 'react'
import { IS_MOBILE } from '@/utils/platform'

export function isSendKey(event: KeyboardEvent): boolean {
  return event.key === 'Enter' && !IS_MOBILE && !event.shiftKey && !event.nativeEvent.isComposing
}
