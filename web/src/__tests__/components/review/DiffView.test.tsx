import { render, screen } from '@testing-library/react'
import { DiffView } from '@/components/review/DiffView'

test('renders workspace id and step name', () => {
  render(<DiffView workspaceId="ws1" step="ai_review" />)
  expect(screen.getByText(/ws1/)).toBeInTheDocument()
  expect(screen.getByText(/ai_review/)).toBeInTheDocument()
})
