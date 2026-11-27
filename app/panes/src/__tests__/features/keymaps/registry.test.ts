import { describe, it, expect } from 'vitest'
import {
  COMMANDS,
  getCommand,
  TAB_NEW,
  TAB_NEW_TERMINAL,
  TAB_NEW_FILE,
  TAB_OPEN_FILE,
  AGENT_NEW_CHAT,
  AGENT_CYCLE_PROVIDER,
  AGENT_TOGGLE_VIEW_MODE,
  AGENT_ZOOM_IN,
  AGENT_ZOOM_OUT,
  AGENT_ZOOM_RESET,
  CATEGORY_ORDER,
  BROWSER_TOGGLE_PICK,
} from '@/features/keymaps/registry'

describe('keymap registry — New Tab commands', () => {
  it('binds mod+t to New Tab, not New Terminal', () => {
    expect(getCommand(TAB_NEW)?.defaultChord).toBe('mod+t')
  })

  it('moves New Terminal to mod+j', () => {
    expect(getCommand(TAB_NEW_TERMINAL)?.defaultChord).toBe('mod+j')
  })

  it('binds New Chat to mod+n and New File to mod+shift+n', () => {
    expect(getCommand(AGENT_NEW_CHAT)?.defaultChord).toBe('mod+n')
    expect(getCommand(TAB_NEW_FILE)?.defaultChord).toBe('mod+shift+n')
  })

  it('binds Open file… to mod+o under Tabs', () => {
    expect(getCommand(TAB_OPEN_FILE)).toMatchObject({
      label: 'Open file…',
      category: 'Tabs',
      defaultChord: 'mod+o',
    })
  })

  it('binds Toggle chat/terminal view to mod+/, a chord macOS actually delivers', () => {
    expect(getCommand(AGENT_TOGGLE_VIEW_MODE)?.defaultChord).toBe('mod+/')
  })

  it('files Toggle chat/terminal view under its own Chats category', () => {
    expect(getCommand(AGENT_TOGGLE_VIEW_MODE)?.category).toBe('Chats')
  })

  it('leaves Cycle chat provider unbound by default now that mod+/ toggles the view instead', () => {
    expect(getCommand(AGENT_CYCLE_PROVIDER)?.defaultChord).toBe('')
  })

  it('advertises no split shortcut that would do nothing', () => {
    expect(COMMANDS.filter((c) => /split/i.test(c.label))).toEqual([])
  })

  it('offers no workspace switcher shortcut', () => {
    expect(COMMANDS.map((c) => c.label)).not.toContain('Open workspace switcher')
  })

  it('renders every category in use — CATEGORY_ORDER covers them all', () => {
    for (const command of COMMANDS) {
      expect(CATEGORY_ORDER).toContain(command.category)
    }
  })

  it('makes every tab and chat command live-editable', () => {
    for (const id of [
      TAB_NEW,
      TAB_NEW_TERMINAL,
      TAB_NEW_FILE,
      TAB_OPEN_FILE,
      AGENT_NEW_CHAT,
      AGENT_CYCLE_PROVIDER,
      AGENT_TOGGLE_VIEW_MODE,
    ]) {
      expect(getCommand(id)?.liveEditable).toBe(true)
    }
  })

  it('has no duplicate default chords', () => {
    const chords = COMMANDS.map((c) => c.defaultChord)
    expect(new Set(chords).size).toBe(chords.length)
  })
})

describe('keymap registry — chat zoom commands', () => {
  it('binds zoom in/out/reset to mod+=, mod+- and mod+0', () => {
    expect(getCommand(AGENT_ZOOM_IN)?.defaultChord).toBe('mod+=')
    expect(getCommand(AGENT_ZOOM_OUT)?.defaultChord).toBe('mod+-')
    expect(getCommand(AGENT_ZOOM_RESET)?.defaultChord).toBe('mod+0')
  })

  it('files the zoom commands under Chats', () => {
    expect(getCommand(AGENT_ZOOM_IN)?.category).toBe('Chats')
    expect(getCommand(AGENT_ZOOM_OUT)?.category).toBe('Chats')
    expect(getCommand(AGENT_ZOOM_RESET)?.category).toBe('Chats')
  })

  it('makes the zoom commands live-editable', () => {
    expect(getCommand(AGENT_ZOOM_IN)?.liveEditable).toBe(true)
    expect(getCommand(AGENT_ZOOM_OUT)?.liveEditable).toBe(true)
    expect(getCommand(AGENT_ZOOM_RESET)?.liveEditable).toBe(true)
  })
})

describe('keymap registry — browser pick', () => {
  it('registers the pick toggle as a live-editable Tabs command on its own chord', () => {
    const command = getCommand(BROWSER_TOGGLE_PICK)
    expect(command).toMatchObject({ category: 'Tabs', liveEditable: true })
    expect(COMMANDS.filter((c) => c.defaultChord === command?.defaultChord)).toHaveLength(1)
  })
})
