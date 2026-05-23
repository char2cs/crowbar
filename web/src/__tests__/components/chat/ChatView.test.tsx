import { render, screen } from '@testing-library/react'
import { ChatView } from '@/components/chat/ChatView'
import type { ChatMessage } from '@/lib/types'

const noop = () => {}

test('renders messages', () => {
  const messages: ChatMessage[] = [
    { id: '1', role: 'user', content: 'Hello world', authorName: 'Mateo', authorInitials: 'MU', timestamp: 'now' },
  ]
  render(<ChatView messages={messages} onSend={noop} />)
  expect(screen.getByText('Hello world')).toBeInTheDocument()
})

test('renders input placeholder', () => {
  render(<ChatView messages={[]} onSend={noop} inputPlaceholder="Type here…" />)
  expect(screen.getByPlaceholderText('Type here…')).toBeInTheDocument()
})
