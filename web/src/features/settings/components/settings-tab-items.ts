import {
  Bug,
  CodeBlock,
  GitBranch,
  HardDrives,
  Keyboard,
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

export const SETTINGS_TAB_ITEMS: SettingsTabItem[] = [
  { id: 'appearance', label: 'Appearance', icon: PaintBrush },
  { id: 'editor', label: 'Editor', icon: CodeBlock },
  { id: 'file-explorer', label: 'Files', icon: TreeStructure },
  { id: 'git', label: 'Git', icon: GitBranch },
  { id: 'terminal', label: 'Terminal', icon: TerminalWindow },
  // USER-FACING RENAME ONLY (spec §11). The tab is called Agents; its id stays
  // `providers` because it is a persisted key and a route into the dialog.
  { id: 'providers', label: 'Agents', icon: Robot },
  { id: 'keybindings', label: 'Keybindings', icon: Keyboard },
  // Desktop-only: remote hosts dial through the Tauri bridge, so this surface
  // has nothing to show in a browser tab.
  ...(isTauri()
    ? [{ id: 'hosts' as SettingsTabItem['id'], label: 'Hosts', icon: HardDrives }]
    : []),
  ...(import.meta.env.DEV
    ? [{ id: 'developer' as SettingsTabItem['id'], label: 'Developer', icon: Bug }]
    : []),
]
