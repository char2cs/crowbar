import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { DEFAULT_SIZE } from '@/features/console/lib/dock-prefs'
import { stubLayout } from '@/__tests__/__mocks__/stub-layout'
import { ConsolePanel } from '@/features/console/components/console-panel'
import { useConsoleStore } from '@/features/console/stores/console-store'

let restoreLayout: () => void
beforeEach(() => {
  localStorage.clear()
  restoreLayout = stubLayout()
  useConsoleStore.setState(useConsoleStore.getInitialState())
})

afterEach(() => restoreLayout())

describe('ConsolePanel', () => {
  it('is inert while closed and reachable once open', () => {
    const { container } = render(<ConsolePanel />)
    const panel = container.querySelector('#console-panel')
    expect(panel?.hasAttribute('inert')).toBe(true)
    act(() => useConsoleStore.getState().setOpen(true))
    expect(container.querySelector('#console-panel')?.hasAttribute('inert')).toBe(false)
  })

  it('closes on Escape from inside the log', () => {
    useConsoleStore.getState().setOpen(true)
    render(<ConsolePanel />)
    fireEvent.keyDown(screen.getByRole('log'), { key: 'Escape' })
    expect(useConsoleStore.getState().open).toBe(false)
  })

  it('closes on a pointer-down anywhere outside the panel', () => {
    useConsoleStore.getState().setOpen(true)
    render(
      <>
        <button type="button">page behind</button>
        <ConsolePanel />
      </>,
    )
    fireEvent.pointerDown(screen.getByRole('log'))
    expect(useConsoleStore.getState().open).toBe(true)
    fireEvent.pointerDown(screen.getByText('page behind'))
    expect(useConsoleStore.getState().open).toBe(false)
  })

  it('keeps a pushed console open on an outside press; Escape still closes it', () => {
    useConsoleStore.setState({ open: true, mode: 'push' })
    render(
      <>
        <button type="button">page beside</button>
        <ConsolePanel />
      </>,
    )
    fireEvent.pointerDown(screen.getByText('page beside'))
    expect(useConsoleStore.getState().open).toBe(true)
    fireEvent.keyDown(screen.getByRole('log'), { key: 'Escape' })
    expect(useConsoleStore.getState().open).toBe(false)
  })

  it('leaves its own trigger to toggle, so the badge does not close then reopen', () => {
    useConsoleStore.getState().setOpen(true)
    render(
      <>
        <button type="button" aria-controls="console-panel">
          badge
        </button>
        <ConsolePanel />
      </>,
    )
    fireEvent.pointerDown(screen.getByText('badge'))
    expect(useConsoleStore.getState().open).toBe(true)
  })

  it('shows the daemon restarted note', () => {
    useConsoleStore.getState().setOpen(true)
    useConsoleStore.getState().ingest([{ type: 'ready', seq: 0, reset: true }])
    render(<ConsolePanel />)
    expect(screen.getByText('Daemon restarted')).toBeTruthy()
  })

  it('renders time, level, message and key=value fields, and reveals the record JSON on click', () => {
    useConsoleStore.getState().setOpen(true)
    useConsoleStore.getState().ingest([
      {
        type: 'log',
        record: {
          seq: 1,
          iso: '2026-10-04T14:02:14.390Z',
          level: 'warn',
          component: 'release',
          msg: 'channel lookup slow',
          fields: [{ key: 'took', value: '1.8s', kind: 'duration' }],
        },
      },
    ])
    const { container } = render(<ConsolePanel />)
    const row = container.querySelector('[data-slot="log-row"]')
    expect(row?.getAttribute('data-level')).toBe('warn')
    expect(screen.getByText('WARN')).toBeTruthy()
    expect(screen.getByText('channel lookup slow')).toBeTruthy()
    expect(screen.getByText('took')).toBeTruthy()
    expect(screen.getByText('1.8s')).toBeTruthy()
    expect(container.querySelector('[data-slot="log-json"]')).toBeNull()

    fireEvent.click(screen.getByRole('button', { expanded: false }))
    const json = container.querySelector('[data-slot="log-json"]')
    expect(json?.textContent).toContain('"release"')
    expect(json?.textContent).toContain('"channel lookup slow"')
  })

  describe('dock and mode', () => {
    const panel = (c: HTMLElement) => c.querySelector('#console-panel') as HTMLElement

    it('cycles the dock position and persists it', () => {
      const { container } = render(<ConsolePanel />)
      expect(panel(container).dataset.dock).toBe('top')
      fireEvent.click(screen.getByRole('button', { name: 'Dock position: Top' }))
      expect(panel(container).dataset.dock).toBe('right')
      fireEvent.click(screen.getByRole('button', { name: 'Dock position: Right' }))
      fireEvent.click(screen.getByRole('button', { name: 'Dock position: Bottom' }))
      expect(panel(container).dataset.dock).toBe('left')
      expect(localStorage.getItem('console-dock')).toBe('left')
    })

    it('toggles between overlay and push and persists it', () => {
      const { container } = render(<ConsolePanel />)
      expect(panel(container).dataset.mode).toBe('overlay')
      fireEvent.click(screen.getByRole('button', { name: 'Mode: Overlay' }))
      expect(panel(container).dataset.mode).toBe('push')
      expect(localStorage.getItem('console-mode')).toBe('push')
    })

    it('collapses to no space when a pushed console is closed', () => {
      useConsoleStore.setState({ mode: 'push', dock: 'left' })
      const { container } = render(<ConsolePanel />)
      expect(panel(container).className).toContain('w-0')
      act(() => useConsoleStore.getState().setOpen(true))
      expect(panel(container).className).not.toContain('w-0')
    })
  })

  describe('resize', () => {
    const handle = () => screen.getByRole('separator', { name: 'Resize console' })

    function drag(from: number, to: number, axis: 'clientX' | 'clientY') {
      fireEvent.pointerDown(handle(), { button: 0, [axis]: from })
      fireEvent.pointerMove(handle(), { [axis]: to })
      fireEvent.pointerUp(handle(), { [axis]: to })
    }

    it('grows a top console downward and commits only on release', () => {
      const { container } = render(<ConsolePanel />)
      const el = container.querySelector('#console-panel') as HTMLElement
      fireEvent.pointerDown(handle(), { button: 0, clientY: 100 })
      fireEvent.pointerMove(handle(), { clientY: 160 })
      expect(el.style.getPropertyValue('--console-size')).toBe(`${DEFAULT_SIZE + 60}px`)
      expect(useConsoleStore.getState().size).toBe(DEFAULT_SIZE)
      fireEvent.pointerUp(handle(), { clientY: 160 })
      expect(useConsoleStore.getState().size).toBe(DEFAULT_SIZE + 60)
      expect(localStorage.getItem('console-size')).toBe(String(DEFAULT_SIZE + 60))
    })

    it('grows a right console leftward', () => {
      useConsoleStore.setState({ dock: 'right' })
      render(<ConsolePanel />)
      drag(500, 450, 'clientX')
      expect(useConsoleStore.getState().size).toBe(DEFAULT_SIZE + 50)
    })

    it('clamps to the minimum and to the window', () => {
      render(<ConsolePanel />)
      drag(0, -5000, 'clientY')
      expect(useConsoleStore.getState().size).toBe(160)
      drag(0, 99999, 'clientY')
      expect(useConsoleStore.getState().size).toBe(Math.floor(window.innerHeight * 0.8))
    })

    it('resets on double-click', () => {
      useConsoleStore.setState({ size: 300 })
      render(<ConsolePanel />)
      fireEvent.doubleClick(handle())
      expect(useConsoleStore.getState().size).toBe(DEFAULT_SIZE)
    })
  })
})
