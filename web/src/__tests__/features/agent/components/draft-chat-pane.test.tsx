import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-api'

const claude: AgentProvider = {
  id: 'claude',
  displayName: 'Claude',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: true,
  models: ['sonnet'],
  efforts: { sonnet: ['low', 'high'] },
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
}

vi.mock('@/features/agent/hooks/use-pane-providers', () => ({ usePaneProviders: () => [claude] }))
vi.mock('@/features/workspace/stores/workspace-context', () => ({ useWorkspaceStore: () => ({}) }))
// The document itself is the chat's own; only the controls the pane hands it matter here.
vi.mock('@/features/agent/chat/agent-empty-document', () => ({
  AgentEmptyDocument: ({ controls }: { controls: ReactNode }) => <div>{controls}</div>,
}))

const { DraftChatPane } = await import('@/features/agent/components/draft-chat-pane')

describe('DraftChatPane', () => {
  // The chip's layout lives under the pane scope; outside it the chip is an unstyled
  // button whose chevron drops to a second line.
  it('draws the model chip inside the scope the chat pane styles it under', () => {
    render(<DraftChatPane paneId="p1" wsId="w1" hostId="local" belowOverlayHeader={false} />)
    const chip = screen.getByRole('button', { name: /Agent:/ })
    expect(chip.closest('.agent-chat-pane')).not.toBeNull()
  })
})
