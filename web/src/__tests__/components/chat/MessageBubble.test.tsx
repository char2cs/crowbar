import { render, screen } from '@testing-library/react'
import { MessageBubble } from '@/components/chat/MessageBubble'

test('renders user bubble with content and attribution', () => {
  render(
    <MessageBubble
      role="user"
      content="How should we handle auth?"
      authorName="Mateo"
      authorInitials="MU"
      timestamp="2h ago"
    />,
  )
  expect(screen.getByText('How should we handle auth?')).toBeInTheDocument()
  expect(screen.getByText('Mateo')).toBeInTheDocument()
  expect(screen.getByText('2h ago')).toBeInTheDocument()
})

test('renders assistant bubble with model name', () => {
  render(
    <MessageBubble
      role="assistant"
      content="A shared auth service works best."
      authorName="Claude"
      authorInitials="✦"
      modelName="Sonnet 4.6"
      timestamp="2h ago · 18.3s"
    />,
  )
  expect(screen.getByText('A shared auth service works best.')).toBeInTheDocument()
  expect(screen.getByText('Sonnet 4.6')).toBeInTheDocument()
})
