import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { subagentSessionRows } from '@/__tests__/__fixtures__/ledger-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'

const actions = noRowActions

const rows = subagentSessionRows()

beforeEach(() => {
  chatStore.setState({ feeds: {} })
  openFeed('id-1')
  for (const row of rows) applyFeedFrame('id-1', { type: 'row', row })
  applyFeedFrame('id-1', { type: 'ready', epoch: 'e1' })
})

describe('the recorded subagent session', () => {
  it('symptom 8: every tool row of the recorded subagent session has a non-empty label', () => {
    const feed = chatStore.getState().feeds['id-1']!
    const tools = Object.values(feed.items).filter(
      (item) => item.kind === 'tool' && item.family !== 'meta' && item.family !== 'plan',
    )
    expect(tools.length).toBeGreaterThanOrEqual(4)
    for (const tool of tools) {
      const view = render(
        <TranscriptRow chatId="id-1" id={tool.id} providers={[]} actions={actions} />,
      )
      const label = view.getByTestId('agent-tool-label').textContent?.trim() ?? ''
      expect(label, tool.id).not.toBe('')
      view.unmount()
    }
  })
})
