import { afterEach, describe, expect, it } from 'vitest'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

describe('terminal store updateSession no-op skip', () => {
  afterEach(() => {
    useTerminalStore.setState({ sessions: new Map() })
  })

  it('does not reallocate the sessions map when every updated field is Object.is-equal to current', () => {
    useTerminalStore.getState().updateSession('session-a', {
      title: 'zsh',
      currentDirectory: '/Users/x',
    })
    const before = useTerminalStore.getState().sessions

    // Same OSC7 cwd + title reported again — the hot-path no-op case.
    useTerminalStore.getState().updateSession('session-a', {
      title: 'zsh',
      currentDirectory: '/Users/x',
    })

    expect(useTerminalStore.getState().sessions).toBe(before)
  })

  it('still reallocates and applies the update when a field actually changes', () => {
    useTerminalStore.getState().updateSession('session-a', { title: 'zsh' })
    const before = useTerminalStore.getState().sessions

    useTerminalStore.getState().updateSession('session-a', { title: 'bash' })

    expect(useTerminalStore.getState().sessions).not.toBe(before)
    expect(useTerminalStore.getState().sessions.get('session-a')?.title).toBe('bash')
  })

  it('reallocates on the very first update for a session that has no prior entry', () => {
    const before = useTerminalStore.getState().sessions

    useTerminalStore.getState().updateSession('session-new', { title: 'zsh' })

    expect(useTerminalStore.getState().sessions).not.toBe(before)
    expect(useTerminalStore.getState().sessions.get('session-new')?.title).toBe('zsh')
  })
})
