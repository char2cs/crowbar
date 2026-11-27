import { IS_MOBILE } from '@/utils/platform'

export function hasLocalDaemon(): boolean {
  return !IS_MOBILE
}
