import type { SettingsTab } from '@/features/window/stores/ui-state-store'

interface PhoneSettingsPage {
  id: SettingsTab
  label: string
  icon: string
}

export const PHONE_SETTINGS_PAGES: PhoneSettingsPage[] = [
  { id: 'hosts', label: 'Hosts', icon: 'HardDrives' },
  { id: 'providers', label: 'Agents', icon: 'Robot' },
  { id: 'terminal', label: 'Terminal', icon: 'TerminalWindow' },
  { id: 'git', label: 'Git', icon: 'GitBranch' },
  { id: 'appearance', label: 'Fonts & display', icon: 'PaintBrush' },
]
