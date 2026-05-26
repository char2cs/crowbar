import { render, screen } from '@testing-library/react'
import { vi } from 'vitest'
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

test('does not call scrollIntoView when sentinel is not intersecting (user scrolled up)', () => {
  const scrollIntoView = vi.fn()
  HTMLElement.prototype.scrollIntoView = scrollIntoView

  // IntersectionObserver reports sentinel NOT visible (user scrolled away)
  let observerCallback: ((entries: Partial<IntersectionObserverEntry>[]) => void) | null = null
  vi.stubGlobal('IntersectionObserver', vi.fn((cb: typeof observerCallback) => {
    observerCallback = cb
    return { observe: vi.fn(), disconnect: vi.fn() }
  }))

  const { rerender } = render(
    <ChatView
      messages={[{ id: '1', role: 'user', content: 'Hello', authorName: 'A', authorInitials: 'A', timestamp: 'now' }]}
      onSend={noop}
    />,
  )

  // Simulate user having scrolled up (sentinel not visible)
  observerCallback?.([{ isIntersecting: false }])

  // Clear calls from initial render (which fired before observer reported not-intersecting)
  scrollIntoView.mockClear()

  // Add a new message (streaming update)
  rerender(
    <ChatView
      messages={[
        { id: '1', role: 'user', content: 'Hello', authorName: 'A', authorInitials: 'A', timestamp: 'now' },
        { id: '2', role: 'assistant', content: 'Hi!', authorName: 'C', authorInitials: 'C', timestamp: 'now' },
      ]}
      onSend={noop}
    />,
  )

  expect(scrollIntoView).not.toHaveBeenCalled()

  vi.unstubAllGlobals()
  delete (HTMLElement.prototype as any).scrollIntoView
})
