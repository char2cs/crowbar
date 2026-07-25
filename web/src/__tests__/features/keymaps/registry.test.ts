import { describe, it, expect } from 'vitest'
import {
  COMMANDS,
  getCommand,
  TAB_NEW,
  TAB_NEW_TERMINAL,
  TAB_NEW_FILE,
  AGENT_NEW_CHAT,
  AGENT_CYCLE_PROVIDER,
  CATEGORY_ORDER,
} from '@/features/keymaps/registry'

describe('keymap registry — New Tab commands', () => {
  it('binds mod+t to New Tab, not New Terminal', () => {
    expect(getCommand(TAB_NEW)?.defaultChord).toBe('mod+t')
  })

  it('moves New Terminal to mod+j', () => {
    expect(getCommand(TAB_NEW_TERMINAL)?.defaultChord).toBe('mod+j')
  })

  // The unshifted chord belongs to the chat — starting a conversation is the
  // thing this app is opened to do — and New File takes the shifted one. They
  // shipped the other way round; this pins the swap.
  it('binds New Chat to mod+n and New File to mod+shift+n', () => {
    expect(getCommand(AGENT_NEW_CHAT)?.defaultChord).toBe('mod+n')
    expect(getCommand(TAB_NEW_FILE)?.defaultChord).toBe('mod+shift+n')
  })

  // Cycles the open chat to the next enabled provider, the way ⌘-tab cycles apps.
  // Registered like every other chord so it is rebindable rather than a hardcoded
  // literal buried in the chat pane.
  //
  // NOT mod+` — macOS reserves that for "move focus to next window in
  // application" and consumes it in AppKit before WKWebView ever sees it, so the
  // chord was invisible to the handler AND to the rebind capture. Pinned here so
  // nobody restores it.
  it('binds Cycle chat provider to mod+/, a chord macOS actually delivers', () => {
    expect(getCommand(AGENT_CYCLE_PROVIDER)?.defaultChord).toBe('mod+/')
  })

  it('binds no command to mod+` — macOS swallows it', () => {
    expect(COMMANDS.filter((c) => c.defaultChord === 'mod+`')).toEqual([])
  })

  it('files Cycle chat provider under its own Chats category', () => {
    expect(getCommand(AGENT_CYCLE_PROVIDER)?.category).toBe('Chats')
  })

  // The Keybindings tab renders ONLY the categories listed in CATEGORY_ORDER, so
  // a command in a category missing from it is invisible in settings — bound and
  // working, but unfindable and unrebindable. Every category in use must appear.
  it('renders every category in use — CATEGORY_ORDER covers them all', () => {
    for (const command of COMMANDS) {
      expect(CATEGORY_ORDER).toContain(command.category)
    }
  })

  // Every chord the New Tab surface draws must be rebindable, or the badge
  // it renders can never go out of sync with reality by design.
  it('makes all five live-editable', () => {
    for (const id of [
      TAB_NEW,
      TAB_NEW_TERMINAL,
      TAB_NEW_FILE,
      AGENT_NEW_CHAT,
      AGENT_CYCLE_PROVIDER,
    ]) {
      expect(getCommand(id)?.liveEditable).toBe(true)
    }
  })

  it('has no duplicate default chords', () => {
    const chords = COMMANDS.map((c) => c.defaultChord)
    expect(new Set(chords).size).toBe(chords.length)
  })
})
