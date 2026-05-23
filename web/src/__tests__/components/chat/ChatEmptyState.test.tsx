import { render, screen } from '@testing-library/react'
import { ChatEmptyState } from '@/components/chat/ChatEmptyState'
import { expect, test } from 'vitest'

test('renders the empty state heading', () => {
  render(<ChatEmptyState />)
  expect(screen.getByText('Start a conversation')).toBeInTheDocument()
})
