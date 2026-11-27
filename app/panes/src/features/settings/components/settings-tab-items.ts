import {
  Bug,
  DeviceMobile,
  CodeBlock,
  Globe,
  GitBranch,
  HardDrives,
  Keyboard,
  Pulse,
  PaintBrush,
  Robot,
  TerminalWindow,
  TreeStructure,
} from '@phosphor-icons/react'
import type * as React from 'react'
import { isTauri } from '@/lib/crowbar-bridge'
import type { SettingsTab } from '@/features/window/stores/ui-state-store'

export interface SettingsTabItem {
  id: SettingsTab
  label: string
  icon: React.ComponentType<{
    size?: string | number
    className?: string
    weight?: 'regular' | 'duotone'
  }>
}

const HOSTS_TAB: SettingsTabItem = { id: 'hosts', label: 'Hosts', icon: HardDrives }

const DEVICES_TAB: SettingsTabItem = { id: 'devices', label: 'Devices', icon: DeviceMobile }

const SHARED_TAB_ITEMS: SettingsTabItem[] = [
  { id: 'appearance', label: 'Appearance', icon: PaintBrush },
  { id: 'editor', label: 'Editor', icon: CodeBlock },
  { id: 'file-explorer', label: 'Files', icon: TreeStructure },
  { id: 'git', label: 'Git', icon: GitBranch },
  { id: 'terminal', label: 'Terminal', icon: TerminalWindow },
  { id: 'browser', label: 'Browser', icon: Globe },
  { id: 'providers', label: 'Agents', icon: Robot },
  { id: 'active-sessions', label: 'Active sessions', icon: Pulse },
  { id: 'keybindings', label: 'Keybindings', icon: Keyboard },
]

export const SETTINGS_TAB_ITEMS: SettingsTabItem[] = [
  ...SHARED_TAB_ITEMS,
  ...(isTauri() ? [HOSTS_TAB, DEVICES_TAB] : []),
  ...(import.meta.env.DEV
    ? [{ id: 'developer' as SettingsTabItem['id'], label: 'Developer', icon: Bug }]
    : []),
]
