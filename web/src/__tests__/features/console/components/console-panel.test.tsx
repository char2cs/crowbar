import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { stubLayout } from '@/__tests__/__mocks__/stub-layout'
import { ConsolePanel } from '@/features/console/components/console-panel'
import { useConsoleStore } from '@/features/console/stores/console-store'

let restoreLayout: () => void
beforeEach(() => {
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
})
