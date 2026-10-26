import { IDEShell } from './ide-shell'
import { MobileShell } from './mobile/mobile-shell'
import { IS_MOBILE } from '@/utils/platform'

/** The routed app's frame: the desktop IDE shell, or the phone's single-pane shell. */
export function AppShell() {
  return IS_MOBILE ? <MobileShell /> : <IDEShell />
}
