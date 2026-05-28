import { render, screen, fireEvent } from '@testing-library/react'
import { WorkspaceStepTabs } from '@/components/layout/WorkspaceStepTabs'
import type { FlowStateDefinition } from '@/lib/types'

const STATES: FlowStateDefinition[] = [
  { name: 'brainstorming', label: 'Brainstorm', ui: 'chat' },
  { name: 'spec',          label: 'Spec',        ui: 'chat' },
  { name: 'ai_review',     label: 'AI Review',   ui: 'diff' },
]

test('renders all state labels', () => {
  render(<WorkspaceStepTabs states={STATES} currentStep="brainstorming" onStepChange={() => {}} />)
  expect(screen.getByText('Brainstorm')).toBeInTheDocument()
  expect(screen.getByText('Spec')).toBeInTheDocument()
  expect(screen.getByText('AI Review')).toBeInTheDocument()
})

test('calls onStepChange with state name when tab clicked', () => {
  const onStepChange = vi.fn()
  render(<WorkspaceStepTabs states={STATES} currentStep="brainstorming" onStepChange={onStepChange} />)
  // Base UI Tabs fires onValueChange via onClick
  fireEvent.click(screen.getByRole('tab', { name: /spec/i }))
  expect(onStepChange).toHaveBeenCalledWith('spec')
})

test('renders separator chevrons between tabs', () => {
  render(<WorkspaceStepTabs states={STATES} currentStep="brainstorming" onStepChange={() => {}} />)
  expect(screen.getAllByText('›')).toHaveLength(2)
})
