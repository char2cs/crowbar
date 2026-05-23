import { render, screen } from '@testing-library/react'
import { ToolCallSeparator } from '@/components/chat/ToolCallSeparator'

test('renders tool call count and duration', () => {
  render(<ToolCallSeparator toolCalls={4} durationSec={18.3} />)
  expect(screen.getByText('4 tool calls · 18.3s')).toBeInTheDocument()
})

test('renders singular "tool call" for 1', () => {
  render(<ToolCallSeparator toolCalls={1} durationSec={2.1} />)
  expect(screen.getByText('1 tool call · 2.1s')).toBeInTheDocument()
})
