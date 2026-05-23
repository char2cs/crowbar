import { render, screen } from '@testing-library/react'
import { ChatInput } from '@/components/chat/ChatInput'
import { expect, test } from 'vitest'

test('shows model selector trigger', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByText('Sonnet 4.6')).toBeInTheDocument()
})

test('shows effort level toggles', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByText('Mid')).toBeInTheDocument()
})

test('send button exists', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByRole('button', { name: /send/i })).toBeInTheDocument()
})

test('renders placeholder text', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByPlaceholderText('Message…')).toBeInTheDocument()
})
