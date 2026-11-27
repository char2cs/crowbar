import { fireEvent, render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { MessageBoundary } from '@/features/agent/transcript/rows/message-boundary'

function Body({ text }: { text: string }) {
  if (text.includes('BOOM')) throw new Error('cannot render')
  return <p>{text}</p>
}

function Probe({ onMount }: { onMount: () => void }) {
  useEffect(onMount, [onMount])
  return <p>probe</p>
}

describe('MessageBoundary', () => {
  it('says the message could not be displayed and offers its plain text and another try', () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <MessageBoundary text="BOOM raw">
        <Body text="BOOM raw" />
      </MessageBoundary>,
    )
    quiet.mockRestore()
    expect(screen.getByText('This message could not be displayed.')).toBeInTheDocument()
    expect(screen.queryByText('BOOM raw')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Show as plain text' }))
    expect(screen.getByText('BOOM raw')).toBeInTheDocument()
  })

  it('draws the body again once the message text changes', () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { rerender } = render(
      <MessageBoundary text="BOOM">
        <Body text="BOOM" />
      </MessageBoundary>,
    )
    quiet.mockRestore()
    rerender(
      <MessageBoundary text="fixed">
        <Body text="fixed" />
      </MessageBoundary>,
    )
    expect(screen.queryByText('This message could not be displayed.')).toBeNull()
    expect(screen.getByText('fixed')).toBeInTheDocument()
  })

  it('keeps its child mounted while the text grows', () => {
    const onMount = vi.fn()
    const { rerender } = render(
      <MessageBoundary text="a">
        <Probe onMount={onMount} />
      </MessageBoundary>,
    )
    rerender(
      <MessageBoundary text="ab">
        <Probe onMount={onMount} />
      </MessageBoundary>,
    )
    expect(onMount).toHaveBeenCalledTimes(1)
  })
})
