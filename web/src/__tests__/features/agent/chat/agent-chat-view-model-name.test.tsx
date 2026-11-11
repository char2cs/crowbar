import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { seedFeed, spokenIn } from '@/__tests__/__fixtures__/ledger-rows'
import * as reads from '@/features/agent/api/agent-chat-reads'
import { chatStore } from '@/features/agent/stores/chat-store'
import { entityStore } from '@/core/entities/entity-store'
import { provider, slashCatalog } from '@/__tests__/__fixtures__/agent-chat'
import { drawChatView as draw } from '@/__tests__/__fixtures__/agent-chat-view'

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'))
  vi.spyOn(reads, 'getSlashCatalog').mockResolvedValue(slashCatalog())
})

// One chat, one name for the model: the blank handle and the underbar are the same
// control, and a live runner's raw launch id must not replace the reported name.
describe('AgentChatView model name', () => {
  const selectable = [
    provider('provider-b', 'Provider B', { modelSelect: true, models: ['model-five'] }),
  ]

  it('names the launch model on the blank document handle too', () => {
    seedFeed('c1', [])
    draw('c1', { providers: selectable, launchModel: 'model-luna' })

    expect(screen.getByTestId('agent-selection-picker')).toHaveAccessibleName(
      'Agent: Provider B, model model-luna, effort Default',
    )
  })

  it('keeps the reported display name when a live runner reports its raw launch id', () => {
    const report: reads.AgentTelemetry = {
      observedAt: '2026-09-23T00:00:00Z',
      source: 'callback',
      model: { id: 'model-x-4-5', displayName: 'Model X 4.5' },
    }
    entityStore.setState((s) => ({
      byKind: { ...s.byKind, telemetry: { ...s.byKind.telemetry, c1: { v: 1, data: report } } },
    }))
    seedFeed('c1', spokenIn('c1'))
    draw('c1', { providers: selectable, launchModel: 'model-x-4-5' })

    const picker = screen.getByTestId('agent-selection-picker')
    expect(picker).toHaveTextContent('Model X 4.5')
    expect(picker).toHaveAccessibleName(
      'Agent: Provider B, model Default (reported Model X 4.5), effort Default',
    )
  })
})
