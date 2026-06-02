import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ChatList } from '@/features/chats/components/chat-list'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('ChatList', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('renders a row for each chat', () => {
    useSidebarStore.setState({
      chats: [
        { id: '1', title: 'Architecture review', age: '5m' },
        { id: '2', title: 'Performance bottlenecks', age: '2h' },
      ],
    })
    render(<ChatList />)
    expect(screen.getByText('Architecture review')).toBeInTheDocument()
    expect(screen.getByText('Performance bottlenecks')).toBeInTheDocument()
    expect(screen.getByText('5m')).toBeInTheDocument()
  })

  it('renders a "+ New" button', () => {
    render(<ChatList />)
    expect(screen.getByRole('button', { name: /new/i })).toBeInTheDocument()
  })

  it('adds a chat when "+ New" is clicked', () => {
    render(<ChatList />)
    fireEvent.click(screen.getByRole('button', { name: /new/i }))
    expect(useSidebarStore.getState().chats).toHaveLength(1)
  })
})
