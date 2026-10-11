import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'

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
