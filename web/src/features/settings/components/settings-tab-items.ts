import {
  Bug,
  CodeBlock,
  GitBranch,
  Keyboard,
  PaintBrush,
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
  { id: 'keybindings', label: 'Keybindings', icon: Keyboard },
  ...(import.meta.env.DEV
    ? [{ id: 'developer' as SettingsTabItem['id'], label: 'Developer', icon: Bug }]
    : []),
]
