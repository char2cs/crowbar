import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { HostStatusDot, HostStatusIcon } from '@/features/hosts/components/host-status-icon'

describe('HostStatusIcon', () => {
  // One tone per state, said in words too: a host that can't be reached is red, not grey.
  it.each([
    ['up', 'online', 'Online'],
    ['no-live-stream', 'online', 'Online — no live stream'],
    ['down', 'offline', 'Offline'],
    ['not-available', 'offline', 'Host not available'],
    ['needs-repair', 'needs-attention', 'Needs pairing again'],
    ['unknown', 'unsettled', 'Status unknown'],
    ['probing', 'unsettled', 'Checking…'],
  ] as const)('a host that is %s gets an %s dot labelled %j', (state, tone, words) => {
    render(<HostStatusIcon hostId="h1" state={state} />)
    const dot = screen.getByRole('img', { name: words })

    expect(dot).toHaveAttribute('data-host-state', state)
    expect(dot).toHaveAttribute('data-host-tone', tone)
    expect(dot).toHaveAttribute('title', words)
  })

  // Where the host runs is told by its record, not drawn the same for all: this machine, or a cloud.
  it('draws this machine for the local host and a cloud for a paired one', () => {
    const { container, rerender } = render(<HostStatusIcon hostId="local" state="up" />)
    expect(container.querySelector('svg')).toHaveAttribute('data-host-place', 'local')

    rerender(<HostStatusIcon hostId="h1" state="up" />)
    expect(container.querySelector('svg')).toHaveAttribute('data-host-place', 'remote')
  })

  // The computer is decoration: the dot on its corner is the one thing read out.
  it('is read as its dot alone', () => {
    render(<HostStatusIcon hostId="h1" state="down" />)

    expect(screen.getAllByRole('img')).toEqual([screen.getByTestId('host-status-h1')])
  })

  // Inside a control that has its own tooltip, a second native one would stack on top of it.
  it('can leave its dot without a native tooltip, still named in words', () => {
    render(<HostStatusIcon hostId="h1" state="down" showTitle={false} />)

    expect(screen.getByRole('img', { name: 'Offline' })).not.toHaveAttribute('title')
  })
})

describe('HostStatusDot', () => {
  // The icon draws this same dot: a state can't read one way on a computer and another on a mark.
  it.each([
    ['up', 'online'],
    ['down', 'offline'],
    ['needs-repair', 'needs-attention'],
    ['probing', 'unsettled'],
  ] as const)('a host that is %s marks what it sits on with an %s dot', (state, tone) => {
    render(
      <HostStatusDot hostId="h1" state={state}>
        <span>mark</span>
      </HostStatusDot>,
    )
    const dot = screen.getByTestId('host-status-h1')

    expect(screen.getByText('mark')).toBeVisible()
    expect(dot).toHaveAttribute('data-host-tone', tone)
    render(<HostStatusIcon hostId="h2" state={state} />)
    expect(screen.getByTestId('host-status-h2')).toHaveAttribute('data-host-tone', tone)
  })

  // Where the surface's own label says the state, the dot is not read out a second time.
  it('is silent unless labelled', () => {
    render(
      <HostStatusDot hostId="h1" state="down">
        <span>mark</span>
      </HostStatusDot>,
    )

    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByTestId('host-status-h1')).toHaveAttribute('aria-hidden', 'true')
  })
})
