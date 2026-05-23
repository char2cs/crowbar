import { render, screen, fireEvent } from '@testing-library/react'
import { ChatInput } from '@/components/chat/ChatInput'

test('renders placeholder text', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByPlaceholderText('Message…')).toBeInTheDocument()
})

test('calls onSend with input value when send button clicked', () => {
  const onSend = vi.fn()
  render(<ChatInput placeholder="Message…" onSend={onSend} />)
  const textarea = screen.getByPlaceholderText('Message…')
  fireEvent.change(textarea, { target: { value: 'hello' } })
  fireEvent.click(screen.getByRole('button', { name: /send/i }))
  expect(onSend).toHaveBeenCalledWith('hello')
})

test('clears input after send', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  const textarea = screen.getByPlaceholderText('Message…')
  fireEvent.change(textarea, { target: { value: 'hello' } })
  fireEvent.click(screen.getByRole('button', { name: /send/i }))
  expect((textarea as HTMLTextAreaElement).value).toBe('')
})

test('shows model name', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} modelName="Sonnet 4.6" />)
  expect(screen.getByText('Sonnet 4.6')).toBeInTheDocument()
})
