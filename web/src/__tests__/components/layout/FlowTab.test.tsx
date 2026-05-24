// web/src/__tests__/components/layout/FlowTab.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { FlowTab } from '@/components/layout/FlowTab'

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet-content">Chat content</div>,
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/workspaces/ws1/brainstorming' } }),
}))
vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQuery: () => ({
      data: {
        branch: 'feat/payment-flow',
        currentState: 'brainstorming',
        flow: {
          states: [
            { name: 'brainstorming', label: 'Brainstorm', ui: 'chat' },
            { name: 'ai_review', label: 'AI Review', ui: 'diff' },
          ],
        },
      },
    }),
  }
})
vi.mock('@/components/layout/WorkspaceStepTabs', () => ({
  WorkspaceStepTabs: ({ states }: { states: unknown[] }) => (
    <div data-testid="step-tabs">{states.length} steps</div>
  ),
}))

describe('FlowTab', () => {
  it('renders outlet content', () => {
    render(<FlowTab workspaceId="ws1" />)
    expect(screen.getByTestId('outlet-content')).toBeInTheDocument()
  })

  it('renders WorkspaceStepTabs', () => {
    render(<FlowTab workspaceId="ws1" />)
    expect(screen.getByTestId('step-tabs')).toBeInTheDocument()
  })

  it('WorkspaceStepTabs comes after outlet in DOM', () => {
    const { container } = render(<FlowTab workspaceId="ws1" />)
    const outlet = container.querySelector('[data-testid="outlet-content"]')
    const tabs = container.querySelector('[data-testid="step-tabs"]')
    expect(outlet!.compareDocumentPosition(tabs!)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })
})
