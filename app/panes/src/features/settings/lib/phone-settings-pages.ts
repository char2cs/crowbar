import type { SettingsTab } from '@/features/window/stores/ui-state-store'

/** One row of the phone's native Settings list: `hosts` is drawn natively, every other id is the
 *  web tab the sheet hosts. `icon` is the Phosphor name the desktop's tab uses. */
interface PhoneSettingsPage {
  id: SettingsTab
  label: string
  icon: string
}

// The phone has no editor, file tree, browser or keyboard of its own to configure, and the
// sheet's webview lacks the IPC the Developer tools need.
export const PHONE_SETTINGS_PAGES: PhoneSettingsPage[] = [
  { id: 'hosts', label: 'Hosts', icon: 'HardDrives' },
  { id: 'providers', label: 'Agents', icon: 'Robot' },
  { id: 'terminal', label: 'Terminal', icon: 'TerminalWindow' },
  { id: 'git', label: 'Git', icon: 'GitBranch' },
  { id: 'appearance', label: 'Fonts & display', icon: 'PaintBrush' },
]
