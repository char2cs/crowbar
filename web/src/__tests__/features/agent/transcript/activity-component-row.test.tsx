import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { ActivityComponent } from '@/features/agent/lib/activity-components'
import { ActivityComponentRow } from '@/features/agent/transcript/activity-component-row'

function component(overrides: Partial<ActivityComponent> = {}): ActivityComponent {
  return {
    id: 'turn-1:plan',
    turnId: 'turn-1',
    parentId: 'turn-1',
    seq: 1,
    kind: 'plan',
    status: 'completed',
    createdAt: '2026-09-28T12:00:00Z',
    updatedAt: '2026-09-28T12:00:01Z',
    payload: {},
    ...overrides,
  }
}

describe('ActivityComponentRow', () => {
  it('renders a durable plan with the canonical lifecycle state', () => {
    render(
      <ActivityComponentRow
        component={component({
          payload: {
            steps: [
              { text: 'Inspect the protocol', status: 'done' },
              { text: 'Verify the UI', status: 'active' },
            ],
          },
        })}
      />,
    )

    expect(screen.getByTestId('agent-durable-plan')).toBeInTheDocument()
    expect(screen.getByText('Inspect the protocol')).toHaveAttribute('data-status', 'done')
    expect(screen.getByText('Verify the UI')).toHaveAttribute('data-status', 'active')
    expect(screen.getByText('completed')).toHaveAttribute('data-status', 'completed')
  })

  it('keeps a connection failure as an explicit timeline notice', () => {
    render(
      <ActivityComponentRow
        component={component({
          id: 'connection-1',
          parentId: undefined,
          kind: 'connection_notice',
          status: 'failed',
          payload: { detail: 'Connection lost. Retry is available.' },
        })}
      />,
    )

    const row = screen.getByText('Connection lost. Retry is available.').closest('article')
    expect(row).toHaveAttribute('data-component-id', 'connection-1')
    expect(row).toHaveAttribute('data-status', 'failed')
  })

  it('renders citations as inspectable links without provider checks', () => {
    render(
      <ActivityComponentRow
        component={component({
          id: 'citation-1',
          parentId: undefined,
          kind: 'citation',
          payload: { title: 'Protocol docs', url: 'https://example.com/protocol' },
        })}
      />,
    )

    expect(screen.getByRole('link', { name: 'Protocol docs' })).toHaveAttribute(
      'href',
      'https://example.com/protocol',
    )
  })

  it('does not turn an unsafe provider URL into a browser link', () => {
    render(
      <ActivityComponentRow
        component={component({
          id: 'citation-unsafe',
          parentId: undefined,
          kind: 'citation',
          payload: { title: 'Unsafe citation', url: 'javascript:alert(1)' },
        })}
      />,
    )

    expect(screen.getByText('Unsafe citation')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Unsafe citation' })).not.toBeInTheDocument()
  })

  it('renders historical reasoning with the quiet transcript treatment', () => {
    render(
      <ActivityComponentRow
        component={component({
          id: 'reasoning-1',
          kind: 'reasoning',
          payload: { text: '**Inspecting** the renderer' },
        })}
      />,
    )

    const reasoning = screen.getByTestId('agent-reasoning')
    expect(reasoning).toHaveClass('thinking')
    expect(reasoning).toHaveTextContent('Inspecting the renderer')
    expect(reasoning.closest('.activity-card')).toBeNull()
    expect(screen.queryByText('completed')).not.toBeInTheDocument()
  })

  it('renders a historical diff inline without a disclosure wrapper', () => {
    render(
      <ActivityComponentRow
        component={component({
          id: 'diff-1',
          kind: 'diff',
          payload: {
            unifiedDiff:
              'diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -1 +1 @@\n-old\n+new\n',
          },
        })}
      />,
    )

    expect(screen.getByTestId('agent-turn-diff').tagName).toBe('DIV')
    expect(screen.queryByText('Turn changes')).not.toBeInTheDocument()
    expect(screen.getByTestId('turn-diff-preview')).toBeInTheDocument()
  })

  it('does not indent a direct turn child, but keeps genuinely nested activity indented', () => {
    const { rerender } = render(
      <ActivityComponentRow component={component({ id: 'turn-1:reasoning', kind: 'reasoning' })} />,
    )

    expect(screen.getByTestId('agent-activity-component')).not.toHaveAttribute('data-nested')

    rerender(
      <ActivityComponentRow
        component={component({
          id: 'nested-reasoning',
          kind: 'reasoning',
          parentId: 'subagent-1',
        })}
      />,
    )
    expect(screen.getByTestId('agent-activity-component')).toHaveAttribute('data-nested')
  })
})
