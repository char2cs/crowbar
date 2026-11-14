import { render } from '@testing-library/react'
import type { ComponentProps } from 'react'
import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import { ComposerChoice } from '@/features/agent/composer/composer-choice'

/** A pending, answerable Bash permission; override what a test is about. */
export function choice(overrides: Partial<AgentChoice> = {}): AgentChoice {
  return {
    id: 'k1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'tool_permission',
    toolName: 'Bash',
    options: [
      { id: 'allow', kind: 'allow', label: 'Allow' },
      { id: 'deny', kind: 'deny', label: 'Deny' },
    ],
    pending: true,
    answerable: true,
    at: '2026-08-18T12:00:00Z',
    ...overrides,
  }
}

export type ChoiceProps = Partial<ComponentProps<typeof ComposerChoice>>

export function showChoice(overrides: Partial<AgentChoice> = {}, props: ChoiceProps = {}) {
  return render(
    <ComposerChoice
      wsId="w1"
      chatId="c1"
      choice={choice(overrides)}
      providerLabel="Provider A"
      {...props}
    />,
  )
}
