import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ChatViewSwitch } from '@/features/agent/controls/chat-view-switch'
import { ChatHeaderSlot, ChatHeaderSlotProvider } from '@/features/agent/hooks/chat-header-slot'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
afterEach(() => {
  platform.mobile = false
})

const props = {
  presentation: 'chat' as const,
  splitEnabled: false,
  hasTerminal: true,
  onSelect: vi.fn(),
}

describe('ChatViewSwitch', () => {
  it('sits in the pane header, where the terminal surface puts it', () => {
    render(
      <ChatHeaderSlotProvider>
        <ChatHeaderSlot />
        <ChatViewSwitch {...props} />
      </ChatHeaderSlotProvider>,
    )
    const header = screen.getByRole('tablist', { name: 'View' }).closest('div')
    expect(header).toHaveClass('flex')
    fireEvent.click(screen.getByRole('tab', { name: 'Terminal' }))
    expect(props.onSelect).toHaveBeenCalledWith('terminal')
  })

  it('stays on the chat surface when the pane has no header row', () => {
    render(<ChatViewSwitch {...props} />)
    expect(screen.getByRole('tablist', { name: 'View' })).toBeInTheDocument()
  })

  it('is absent from the header on the phone, whose switch is in the dock footer', () => {
    platform.mobile = true
    render(
      <ChatHeaderSlotProvider>
        <ChatHeaderSlot />
        <ChatViewSwitch {...props} />
      </ChatHeaderSlotProvider>,
    )
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
  })

  it('is absent on the terminal, which carries its own, and for a provider with no terminal', () => {
    const { rerender } = render(<ChatViewSwitch {...props} presentation="terminal" />)
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    rerender(<ChatViewSwitch {...props} hasTerminal={false} />)
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
  })
})
