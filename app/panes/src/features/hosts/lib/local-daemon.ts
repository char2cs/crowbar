import { IS_MOBILE } from '@/utils/platform'

/** Whether this install has a daemon of its own: the phone app reaches paired hosts only. */
export function hasLocalDaemon(): boolean {
  return !IS_MOBILE
}
