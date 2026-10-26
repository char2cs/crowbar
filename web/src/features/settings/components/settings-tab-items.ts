import {
  Bug,
  CodeBlock,
  Globe,
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

const HOSTS_TAB: SettingsTabItem = { id: 'hosts', label: 'Hosts', icon: HardDrives }

const SHARED_TAB_ITEMS: SettingsTabItem[] = [
  { id: 'appearance', label: 'Appearance', icon: PaintBrush },
  { id: 'editor', label: 'Editor', icon: CodeBlock },
  { id: 'file-explorer', label: 'Files', icon: TreeStructure },
  { id: 'git', label: 'Git', icon: GitBranch },
  { id: 'terminal', label: 'Terminal', icon: TerminalWindow },
  { id: 'browser', label: 'Browser', icon: Globe },
  // USER-FACING RENAME ONLY (spec §11). The tab is called Agents; its id stays
  // `providers` because it is a persisted key and a route into the dialog.
  { id: 'providers', label: 'Agents', icon: Robot },
  { id: 'keybindings', label: 'Keybindings', icon: Keyboard },
]

export const SETTINGS_TAB_ITEMS: SettingsTabItem[] = [
  ...SHARED_TAB_ITEMS,
  // Remote hosts dial through the Tauri bridge, so a browser tab has nothing to show here.
  ...(isTauri() ? [HOSTS_TAB] : []),
  ...(import.meta.env.DEV
    ? [{ id: 'developer' as SettingsTabItem['id'], label: 'Developer', icon: Bug }]
    : []),
]

// The phone has no editor, file tree, browser or keyboard of its own to configure, and the
// sheet's webview lacks the IPC the Developer tools need; Hosts is always there.
const PHONE_TAB_IDS: SettingsTab[] = ['appearance', 'terminal', 'providers', 'git', 'hosts']

/** The phone's settings sheet: the desktop tabs that apply to it, in its own order. */
export const PHONE_SETTINGS_TAB_ITEMS: SettingsTabItem[] = PHONE_TAB_IDS.map((id) =>
  [...SHARED_TAB_ITEMS, HOSTS_TAB].find((tab) => tab.id === id)!,
)
