import {
  Bug,
  CodeBlock,
  GitBranch,
  Keyboard,
  PaintBrush,
  Robot,
  TerminalWindow,
  TreeStructure,
} from '@phosphor-icons/react'
import type * as React from 'react'
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
  ...(import.meta.env.DEV
    ? [{ id: 'developer' as SettingsTabItem['id'], label: 'Developer', icon: Bug }]
    : []),
]
