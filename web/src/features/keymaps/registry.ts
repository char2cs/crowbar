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
import type { Command, CommandCategory } from './types'

/** Pane commands — these are resolved from the registry by use-pane-keyboard. */
export const PANE_NAVIGATE_LEFT = 'panes.navigateLeft'
export const PANE_NAVIGATE_RIGHT = 'panes.navigateRight'
export const PANE_NAVIGATE_UP = 'panes.navigateUp'
export const PANE_NAVIGATE_DOWN = 'panes.navigateDown'
export const PANE_SPLIT_RIGHT = 'panes.splitRight'
export const PANE_SPLIT_DOWN = 'panes.splitDown'
export const TAB_NEW = 'tabs.new'
export const TAB_NEW_TERMINAL = 'tabs.newTerminal'
export const TAB_NEW_FILE = 'tabs.newFile'
export const AGENT_NEW_CHAT = 'agent.newChat'
export const AGENT_CYCLE_PROVIDER = 'agent.cycleProvider'
export const AGENT_TOGGLE_VIEW_MODE = 'agent.toggleViewMode'
export const TAB_REOPEN_CLOSED = 'tabs.reopenClosed'
export const TAB_CLOSE = 'tabs.closeActive'

export const EDITOR_SAVE = 'editor.save'
export const EDITOR_SAVE_ALL = 'editor.saveAll'

export const OPEN_WORKSPACE_SWITCHER = 'navigation.openWorkspaceSwitcher'

export const SIDEBAR_TAB_WORKSPACES = 'navigation.sidebarWorkspaces'
export const SIDEBAR_TAB_FILES = 'navigation.sidebarFiles'
export const SIDEBAR_TAB_GIT = 'navigation.sidebarGit'
export const SIDEBAR_TAB_CHATS = 'navigation.sidebarChats'

/**
 * The order the Keybindings settings tab renders categories in — and, because it
 * renders ONLY what is listed here, the set of categories that are visible at all.
 * It lives beside COMMANDS rather than in the settings component so that adding a
 * category to a command and forgetting to render it cannot silently hide the
 * command: a registry test asserts every category in use appears here.
 */
export const CATEGORY_ORDER: CommandCategory[] = ['Navigation', 'Panes', 'Tabs', 'Chats', 'Editor']

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
    id: TAB_NEW,
    label: 'New tab',
    category: 'Tabs',
    defaultChord: 'mod+t',
    liveEditable: true,
  },
  {
    // Moved off mod+t, which is now New Tab.
    id: TAB_NEW_TERMINAL,
    label: 'New terminal tab',
    category: 'Tabs',
    defaultChord: 'mod+j',
    liveEditable: true,
  },
  {
    // ⌘N is the chat, not the file: starting a conversation is the thing this
    // app is opened to do, so it takes the unshifted chord and New File takes
    // ⌘⇧N. (They were the other way round through the New Tab work.)
    id: AGENT_NEW_CHAT,
    label: 'New chat',
    category: 'Tabs',
    defaultChord: 'mod+n',
    liveEditable: true,
  },
  {
    id: TAB_NEW_FILE,
    label: 'New file',
    category: 'Tabs',
    defaultChord: 'mod+shift+n',
    liveEditable: true,
  },
  {
    // Toggles the OPEN chat between its Chat and Terminal surfaces — the same
    // pair ViewSwitcher's own tabs flip between. It only does anything while a
    // chat pane is the focused, visible one (agent-chat-pane gates it) —
    // elsewhere the chord is free for other uses.
    //
    // Deliberately NOT mod+`, the obvious chord for this. macOS reserves it for
    // "move focus to next window in application" and consumes it in AppKit before
    // WKWebView is reached, so it never arrives as a keydown at all — invisible to
    // this command AND to the settings rebind capture, which is how it presented:
    // a shortcut that did nothing and could not even be reassigned.
    //
    // mod+/ collides with "toggle comment" in editors, which is fine: this fires
    // only while a CHAT pane is focused, never over the editor.
    id: AGENT_TOGGLE_VIEW_MODE,
    label: 'Toggle chat / terminal view',
    category: 'Chats',
    defaultChord: 'mod+/',
    liveEditable: true,
  },
  {
    // Used to own mod+/ (cycles the open chat to the next enabled provider, the
    // way ⌘-tab cycles apps) — that chord now toggles the chat/terminal view
    // instead (AGENT_TOGGLE_VIEW_MODE above). The action itself is untouched and
    // still fully wired in agent-chat-pane; it just ships with no default chord
    // until a user assigns one in Settings → Keybindings.
    id: AGENT_CYCLE_PROVIDER,
    label: 'Cycle chat provider',
    category: 'Chats',
    defaultChord: '',
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
  // The digit IS the tab's position in the sidebar strip — mod+2 is whatever sits
  // second. Keep these four in strip order, and renumber them when the strip is
  // reordered: a number that doesn't match what the user is looking at is a
  // shortcut they will never find.
  {
    id: SIDEBAR_TAB_WORKSPACES,
    label: 'Sidebar: Workspaces',
    category: 'Navigation',
    defaultChord: 'mod+1',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_CHATS,
    label: 'Sidebar: Chats',
    category: 'Navigation',
    defaultChord: 'mod+2',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_FILES,
    label: 'Sidebar: Files',
    category: 'Navigation',
    defaultChord: 'mod+3',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_GIT,
    label: 'Sidebar: Git',
    category: 'Navigation',
    defaultChord: 'mod+4',
    liveEditable: true,
  },
]

const COMMAND_BY_ID = new Map(COMMANDS.map((c) => [c.id, c]))

export function getCommand(id: string): Command | undefined {
  return COMMAND_BY_ID.get(id)
}
