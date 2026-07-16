import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { useBufferDisplayName } from '@/features/tabs/hooks/use-buffer-display-name'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { parseOSC7 } from '@/features/terminal/utils/osc-parser'
import type {
  EditorContent,
  PaneContent,
  TerminalContent,
} from '@/features/panes/types/pane-content'

const editorBuffer: EditorContent = {
  id: 'buf-editor',
  type: 'editor',
  path: '/project/foo.ts',
  name: 'foo.ts',
  content: '',
  savedContent: '',
  isDirty: false,
  isVirtual: false,
  isPinned: false,
  isPreview: false,
  isActive: false,
  tokens: [],
}

function terminalBuffer(id: string, sessionId: string): TerminalContent {
  return {
    id,
    type: 'terminal',
    path: '',
    name: 'Terminal',
    isPinned: false,
    isPreview: false,
    isActive: false,
    sessionId,
  }
}

let renderCount = 0

function Probe({ buffers }: { buffers: PaneContent[] }) {
  renderCount++
  const getBufferDisplayName = useBufferDisplayName({ buffers, rootFolderPath: undefined })
  return <div data-testid="probe">{buffers.map((b) => getBufferDisplayName(b)).join('|')}</div>
}

describe('useBufferDisplayName session subscription', () => {
  afterEach(() => {
    // Wrapped in act(): the Probe from the just-finished test is still
    // mounted at this point (RTL's own unmount-on-cleanup afterEach runs
    // after this describe-local one), so resetting the store here is a real
    // subscriber-visible update.
    act(() => {
      useTerminalStore.setState({ sessions: new Map() })
    })
    renderCount = 0
  })

  it('does not re-render when only non-terminal buffers are shown and an unrelated terminal session updates', () => {
    render(<Probe buffers={[editorBuffer]} />)
    const before = renderCount

    act(() => {
      useTerminalStore.getState().updateSession('unrelated-session', { title: 'new title' })
    })

    expect(renderCount).toBe(before)
  })

  it('does not re-render a terminal-buffer consumer when a DIFFERENT session updates', () => {
    const bufA = terminalBuffer('buf-a', 'session-a')
    const bufB = terminalBuffer('buf-b', 'session-b')
    render(<Probe buffers={[bufA]} />)
    const before = renderCount

    act(() => {
      useTerminalStore.getState().updateSession('session-b', { title: 'other tab title' })
    })
    // touch bufB so the linter doesn't flag it as unused if refactored later
    void bufB

    expect(renderCount).toBe(before)
  })

  it('DOES re-render a terminal-buffer consumer when its own session title changes (projection is not over-narrow)', () => {
    const bufA = terminalBuffer('buf-a', 'session-a')
    render(<Probe buffers={[bufA]} />)
    const before = renderCount

    act(() => {
      // Deliberately space-containing values: proves the field separator can't
      // alias two projected fields onto each other.
      useTerminalStore.getState().updateSession('session-a', {
        title: 'my session',
        currentDirectory: '/Users/x/My Project',
      })
    })

    expect(renderCount).toBeGreaterThan(before)
    expect(screen.getByTestId('probe').textContent).toBe('my session')
  })

  it('distinguishes two directories that share a prefix up to a control byte (OSC7 paths arrive sanitized)', () => {
    const bufA = terminalBuffer('buf-a', 'session-a')
    render(<Probe buffers={[bufA]} />)

    // Simulate the real OSC7 → parseOSC7 → updateSession pipeline. The second
    // payload smuggles \x01 — the projection's field separator — right after a
    // prefix it shares with the first. If the control byte survived into the
    // store, the projected tuple would contain a rogue separator, the useMemo
    // split would truncate currentDirectory back to the shared prefix, and the
    // label would falsely stay 'app'.
    const benign = parseOSC7('\x1b]7;file://host/repo/app\x07')
    const hostile = parseOSC7('\x1b]7;file://host/repo/app\x01two\x07')
    expect(benign).toBe('/repo/app')
    expect(hostile).toBe('/repo/apptwo')

    act(() => {
      useTerminalStore.getState().updateSession('session-a', { currentDirectory: benign! })
    })
    expect(screen.getByTestId('probe').textContent).toBe('app')

    act(() => {
      useTerminalStore.getState().updateSession('session-a', { currentDirectory: hostile! })
    })
    expect(useTerminalStore.getState().getSession('session-a')?.currentDirectory).toBe(
      '/repo/apptwo',
    )
    expect(screen.getByTestId('probe').textContent).toBe('apptwo')
  })

  it('DOES re-render when the session gains a customName (own-session, different field)', () => {
    const bufA = terminalBuffer('buf-a', 'session-a')
    render(<Probe buffers={[bufA]} />)
    const before = renderCount

    act(() => {
      useTerminalStore.getState().updateSession('session-a', { customName: true, name: 'Renamed' })
    })

    expect(renderCount).toBeGreaterThan(before)
    expect(screen.getByTestId('probe').textContent).toBe('Renamed')
  })
})
