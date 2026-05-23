import { render, screen } from '@testing-library/react'
import { ChatView } from '@/components/chat/ChatView'

const noop = () => {}

test('renders all flow step labels', () => {
  render(
    <ChatView step="brainstorm" onStepChange={noop} messages={[]} onSend={noop} />,
  )
  expect(screen.getByText('Brainstorm')).toBeInTheDocument()
  expect(screen.getByText('Spec')).toBeInTheDocument()
  expect(screen.getByText('Builder')).toBeInTheDocument()
  expect(screen.getByText('AI Review')).toBeInTheDocument()
  expect(screen.getByText('Human Review')).toBeInTheDocument()
})

test('renders messages', () => {
  render(
    <ChatView
      step="brainstorm"
      onStepChange={noop}
      messages={[
        { id: '1', role: 'user', content: 'Hello world', authorName: 'Mateo', authorInitials: 'MU', timestamp: 'now' },
      ]}
      onSend={noop}
    />,
  )
  expect(screen.getByText('Hello world')).toBeInTheDocument()
})

test('renders input placeholder', () => {
  render(
    <ChatView
      step="spec"
      onStepChange={noop}
      messages={[]}
      onSend={noop}
      inputPlaceholder="Type here…"
    />,
  )
  expect(screen.getByPlaceholderText('Type here…')).toBeInTheDocument()
})
