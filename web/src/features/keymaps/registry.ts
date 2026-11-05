// Every bindable command with its default chord. `liveEditable` commands resolve their binding
// from the registry; the rest are still hardcoded in a hook and shown read-only in settings.
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
export const TAB_OPEN_FILE = 'tabs.openFile'
export const AGENT_NEW_CHAT = 'agent.newChat'
export const AGENT_NEW_CHAT_TERMINAL = 'agent.newChatTerminal'
export const AGENT_CYCLE_PROVIDER = 'agent.cycleProvider'
export const AGENT_TOGGLE_VIEW_MODE = 'agent.toggleViewMode'
export const TAB_REOPEN_CLOSED = 'tabs.reopenClosed'
export const TAB_CLOSE = 'tabs.closeActive'
export const BROWSER_TOGGLE_PICK = 'browser.togglePick'

export const AGENT_ZOOM_IN = 'agent.zoomIn'
export const AGENT_ZOOM_OUT = 'agent.zoomOut'
export const AGENT_ZOOM_RESET = 'agent.zoomReset'

export const EDITOR_SAVE = 'editor.save'
export const EDITOR_SAVE_ALL = 'editor.saveAll'

export const OPEN_WORKSPACE_SWITCHER = 'navigation.openWorkspaceSwitcher'

export const TOGGLE_CONSOLE = 'navigation.toggleConsole'

export const SIDEBAR_TAB_FILES = 'navigation.sidebarFiles'
export const SIDEBAR_TAB_GIT = 'navigation.sidebarGit'
export const SIDEBAR_TAB_CHATS = 'navigation.sidebarChats'

// Render order of the Keybindings tab, which shows ONLY these categories. Kept beside COMMANDS
// so a registry test can assert every category in use is listed (none silently hidden).
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
    // Opens one new chat on Terminal without changing the sticky default
    // (`chatIsDefaultPresentation`, Settings → Chat) for the chats after it.
    id: AGENT_NEW_CHAT_TERMINAL,
    label: 'New chat (Terminal)',
    category: 'Tabs',
    defaultChord: 'mod+alt+n',
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
    id: TAB_OPEN_FILE,
    label: 'Open file…',
    category: 'Tabs',
    defaultChord: 'mod+o',
    liveEditable: true,
  },
  {
    // Flips the focused chat between Chat and Terminal (agent-chat-pane gates it to chat panes,
    // so mod+/ never shadows an editor's toggle-comment). Not mod+`: macOS consumes that in
    // AppKit before WKWebView, so neither this command nor rebind capture would ever see it.
    id: AGENT_TOGGLE_VIEW_MODE,
    label: 'Toggle chat / terminal view',
    category: 'Chats',
    defaultChord: 'mod+/',
    liveEditable: true,
  },
  {
    // Cycles the open chat to the next enabled provider. Wired in agent-chat-pane but ships
    // unbound: mod+/ belongs to AGENT_TOGGLE_VIEW_MODE.
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
  {
    id: AGENT_ZOOM_IN,
    label: 'Zoom chat in',
    category: 'Chats',
    defaultChord: 'mod+=',
    liveEditable: true,
  },
  {
    id: AGENT_ZOOM_OUT,
    label: 'Zoom chat out',
    category: 'Chats',
    defaultChord: 'mod+-',
    liveEditable: true,
  },
  {
    id: AGENT_ZOOM_RESET,
    label: 'Reset chat zoom',
    category: 'Chats',
    defaultChord: 'mod+0',
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
  {
    id: BROWSER_TOGGLE_PICK,
    label: 'Pick element in browser tab',
    category: 'Tabs',
    defaultChord: 'mod+shift+e',
    liveEditable: true,
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
    id: TOGGLE_CONSOLE,
    label: 'Toggle console',
    category: 'Navigation',
    defaultChord: 'mod+`',
    liveEditable: true,
  },
  // The digit IS the mode's position in the sidebar mode bar (Chats, Files,
  // Git). Project switching rides mod+shift+digit (use-space-switcher-keyboard.ts).
  {
    id: SIDEBAR_TAB_CHATS,
    label: 'Sidebar: Chats',
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
]

const COMMAND_BY_ID = new Map(COMMANDS.map((c) => [c.id, c]))

export function getCommand(id: string): Command | undefined {
  return COMMAND_BY_ID.get(id)
}
