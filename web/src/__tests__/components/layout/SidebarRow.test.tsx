import { render, screen } from '@testing-library/react'
import { ChatRow, NewRow } from '@/components/layout/SidebarRow'

test('ChatRow renders title and age', () => {
  render(<ChatRow title="Architecture decisions" age="2h" active />)
  expect(screen.getByText('Architecture decisions')).toBeInTheDocument()
  expect(screen.getByText('2h')).toBeInTheDocument()
})

test('NewRow renders label', () => {
  render(<NewRow label="New workspace" />)
  expect(screen.getByText('New workspace')).toBeInTheDocument()
})
