import { render, screen } from '@testing-library/react'
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
  it('shows the message as plain text with a one-line note when its body throws', () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <MessageBoundary text="BOOM raw">
        <Body text="BOOM raw" />
      </MessageBoundary>,
    )
    quiet.mockRestore()
    expect(screen.getByText('Could not render this message')).toBeInTheDocument()
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
    expect(screen.queryByText('Could not render this message')).toBeNull()
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
