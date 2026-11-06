import type { ComponentProps } from 'react'
import { render } from '@testing-library/react'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'

/** Renders chat-1's transcript, live and idle unless `overrides` say otherwise. */
export function drawTranscript(overrides: Partial<ComponentProps<typeof AgentTranscript>> = {}) {
  const props: ComponentProps<typeof AgentTranscript> = {
    chatId: 'chat-1',
    providers: [],
    status: 'live',
    working: false,
    line: { working: false },
    onOpenTerminal: () => {},
    promptActions: noRowActions,
    ...overrides,
  }
  const view = render(<AgentTranscript {...props} />)
  return {
    ...view,
    redraw: (next: Partial<ComponentProps<typeof AgentTranscript>>) =>
      view.rerender(<AgentTranscript {...props} {...next} />),
  }
}
