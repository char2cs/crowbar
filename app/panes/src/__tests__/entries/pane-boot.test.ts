import { describe, expect, it, vi } from 'vitest'

vi.mock('@/sdk/shell-channel', () => ({ shellChannel: () => null }))

const { bootPane } = await import('@/entries/pane-boot')

describe('bootPane', () => {
  it('refuses to run outside the Crowbar shell', () => {
    expect(() => bootPane('chat')).toThrow(/runs inside the Crowbar shell/)
  })
})
