import { screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import { chatStore } from '@/features/agent/stores/chat-store'
import { measureRows } from '@/__tests__/__fixtures__/transcript-layout'
import { drawTranscript as draw } from '@/__tests__/__fixtures__/agent-transcript'

let restoreRows = () => {}
beforeEach(() => chatStore.setState({ feeds: {}, outbox: {} }))
afterEach(() => restoreRows())

describe('AgentTranscript row gaps', () => {
  it('stacks notice cards 10px apart and the row after them 18px below', () => {
    const l = ledger()
    const warn = (detail: string) => l.row('marker', { kind: 'warning', detail })
    seedFeed('chat-1', [warn('One. A'), warn('Two. B'), l.prompt('r1', 'hi')])
    restoreRows = measureRows({ row: 60, viewport: () => 600 })
    draw()
    const list = screen.getByTestId('agent-message-list')
    const rows = [...list.querySelectorAll<HTMLElement>('[data-index]')]
    expect(rows.slice(0, 2).map((row) => row.style.paddingBottom)).toEqual(['10px', '18px'])
  })
})
