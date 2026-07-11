/**
 * Command registry.
 *
 * A finite, knowable set of commands extracted from the keyboard hooks that
 * already exist in the codebase. Each command records its default chord and
 * whether the live binding is resolved from the registry (`liveEditable`) or
 * still hardcoded in a hook (display-only, shown read-only in settings).
 *
 * Source hooks:
 *  - features/panes/hooks/use-pane-keyboard.ts   (Panes — live-editable)
 *  - features/keymaps/hooks/use-save-keyboard.ts (Editor — display-only)
 */
import type { Command } from './types'

/** Pane commands — these are resolved from the registry by use-pane-keyboard. */
export const PANE_NAVIGATE_LEFT = 'panes.navigateLeft'
export const PANE_NAVIGATE_RIGHT = 'panes.navigateRight'
export const PANE_NAVIGATE_UP = 'panes.navigateUp'
export const PANE_NAVIGATE_DOWN = 'panes.navigateDown'
export const PANE_SPLIT_RIGHT = 'panes.splitRight'
export const PANE_SPLIT_DOWN = 'panes.splitDown'
export const TAB_NEW_TERMINAL = 'tabs.newTerminal'
export const TAB_REOPEN_CLOSED = 'tabs.reopenClosed'
export const TAB_CLOSE = 'tabs.closeActive'

export const EDITOR_SAVE = 'editor.save'
export const EDITOR_SAVE_ALL = 'editor.saveAll'

export const OPEN_WORKSPACE_SWITCHER = 'navigation.openWorkspaceSwitcher'

export const SIDEBAR_TAB_WORKSPACES = 'navigation.sidebarWorkspaces'
export const SIDEBAR_TAB_FILES = 'navigation.sidebarFiles'
export const SIDEBAR_TAB_GIT = 'navigation.sidebarGit'
export const SIDEBAR_TAB_CHATS = 'navigation.sidebarChats'

export const COMMANDS: Command[] = [
  // --- Panes (live-editable, resolved from registry) ---
  {
    id: PANE_SPLIT_RIGHT,
    label: 'Split editor right',
    category: 'Panes',
    defaultChord: 'mod+\\',
    liveEditable: true,
  },
  {
    id: PANE_SPLIT_DOWN,
    label: 'Split editor down',
    category: 'Panes',
    defaultChord: 'mod+shift+\\',
    liveEditable: true,
  },
  {
    id: PANE_NAVIGATE_LEFT,
    label: 'Focus pane left',
    category: 'Panes',
    defaultChord: 'mod+alt+arrowleft',
    liveEditable: true,
  },
  {
    id: PANE_NAVIGATE_RIGHT,
    label: 'Focus pane right',
    category: 'Panes',
    defaultChord: 'mod+alt+arrowright',
    liveEditable: true,
  },
  {
    id: PANE_NAVIGATE_UP,
    label: 'Focus pane up',
    category: 'Panes',
    defaultChord: 'mod+alt+arrowup',
    liveEditable: true,
  },
  {
    id: PANE_NAVIGATE_DOWN,
    label: 'Focus pane down',
    category: 'Panes',
    defaultChord: 'mod+alt+arrowdown',
    liveEditable: true,
  },
  // --- Tabs (live-editable, resolved from registry) ---
  {
    id: TAB_NEW_TERMINAL,
    label: 'New terminal tab',
    category: 'Tabs',
    defaultChord: 'mod+t',
    liveEditable: true,
  },
  {
    id: TAB_REOPEN_CLOSED,
    label: 'Reopen last closed tab',
    category: 'Tabs',
    defaultChord: 'mod+shift+t',
    liveEditable: true,
  },
  {
    id: TAB_CLOSE,
    label: 'Close active tab',
    category: 'Tabs',
    defaultChord: 'mod+w',
    liveEditable: true,
  },
  // --- Editor (display-only: still hardcoded in use-save-keyboard.ts) ---
  {
    id: EDITOR_SAVE,
    label: 'Save',
    category: 'Editor',
    defaultChord: 'mod+s',
    liveEditable: false,
  },
  {
    id: EDITOR_SAVE_ALL,
    label: 'Save all',
    category: 'Editor',
    defaultChord: 'mod+shift+s',
    liveEditable: false,
  },
  // --- Navigation (live-editable) ---
  {
    id: OPEN_WORKSPACE_SWITCHER,
    label: 'Open workspace switcher',
    category: 'Navigation',
    defaultChord: 'mod+k',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_WORKSPACES,
    label: 'Sidebar: Workspaces',
    category: 'Navigation',
    defaultChord: 'mod+1',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_FILES,
    label: 'Sidebar: Files',
    category: 'Navigation',
    defaultChord: 'mod+2',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_GIT,
    label: 'Sidebar: Git',
    category: 'Navigation',
    defaultChord: 'mod+3',
    liveEditable: true,
  },
  // Chats sits second in the sidebar strip but takes the next FREE number: the
  // 1/2/3 bindings above already exist (and may be user-overridden in a persisted
  // keymap), so re-numbering them to match strip order would silently move
  // bindings out from under people. mod+4 is unused elsewhere in the registry.
  {
    id: SIDEBAR_TAB_CHATS,
    label: 'Sidebar: Chats',
    category: 'Navigation',
    defaultChord: 'mod+4',
    liveEditable: true,
  },
]

const COMMAND_BY_ID = new Map(COMMANDS.map((c) => [c.id, c]))

export function getCommand(id: string): Command | undefined {
  return COMMAND_BY_ID.get(id)
}

/** Commands whose live binding is resolved from the effective keymap. */
export const LIVE_EDITABLE_COMMAND_IDS = COMMANDS.filter((c) => c.liveEditable).map((c) => c.id)
