import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSidebarFocusMode } from '@/components/layout/use-sidebar-focus-mode'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { ChatTabItem } from '@/features/tabs/components/chat-tab-item'
import { resetChatStore, setChatWorking, writeChat } from '@/__tests__/__fixtures__/agent-chat'

function renderChatTab(overrides: { isActive?: boolean; onSelect?: () => void } = {}) {
  return render(
    <ChatTabItem
      chatId="chat-1"
      isActive={overrides.isActive ?? false}
      onSelect={overrides.onSelect ?? (() => {})}
    />,
  )
}

beforeEach(() => {
  resetChatStore()
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

// The chat's own entry in the IDE sector's tab strip: never closable or draggable.
describe('ChatTabItem', () => {
  it('renders the chat title', () => {
    writeChat({ id: 'chat-1', title: 'My Chat' })
    renderChatTab()
    expect(screen.getByText('My Chat')).toBeInTheDocument()
  })

  it('falls back to UNTITLED_CHAT_LABEL for an empty title', () => {
    writeChat({ id: 'chat-1', title: '' })
    renderChatTab()
    expect(screen.getByText(UNTITLED_CHAT_LABEL)).toBeInTheDocument()
  })

  it('shows no title, not UNTITLED_CHAT_LABEL, for a chat the store has no record of', () => {
    renderChatTab()
    const title = screen.getByTestId('chat-tab-item-title')
    expect(title).toHaveAttribute('data-title-pending', 'true')
    expect(title).toBeEmptyDOMElement()
    expect(screen.queryByText(UNTITLED_CHAT_LABEL)).not.toBeInTheDocument()
  })

  it("wears its provider's glyph and spins while the chat works", () => {
    useAgentProvidersStore.setState({
      providers: [
        { id: 'alpha', icon: '<svg data-p="alpha"></svg>' } as AgentProvider,
        { id: 'zeta', icon: '<svg data-p="zeta"></svg>' } as AgentProvider,
      ],
    })
    writeChat({ id: 'chat-1', title: 'My Chat', activeProviderId: 'zeta' })
    renderChatTab()
    const tab = screen.getByTestId('chat-tab-item')
    expect(tab.querySelector('[data-p="zeta"]')).not.toBeNull()
    expect(tab.querySelector('[data-p="alpha"]')).toBeNull()
    act(() => setChatWorking('chat-1', true))
    expect(tab.querySelector('[data-p="zeta"]')).toBeNull()
  })

  it('fires onSelect when clicked', () => {
    const onSelect = vi.fn()
    renderChatTab({ onSelect })
    fireEvent.click(screen.getByTestId('chat-tab-item'))
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('renders no close affordance — it is never closable', () => {
    renderChatTab()
    expect(screen.queryByRole('button', { name: /close/i })).not.toBeInTheDocument()
  })
})

describe('ChatTabItem sidebar surface', () => {
  // The mode follows focus only with the opt-in setting on.
  beforeEach(() => {
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), sidebarFollowsFocus: true },
    })
  })
  it('focusing the chat tab shows Chats', () => {
    treeUiStore.setState({ activeTab: 'files' })
    function FocusMode() {
      useSidebarFocusMode()
      return null
    }
    render(<FocusMode />)
    renderChatTab({ isActive: true })

    // Focus only moves the mode once the user has pressed something.
    fireEvent.pointerDown(document.body)
    act(() => screen.getByRole('tab').focus())

    expect(treeUiStore.getState().activeTab).toBe('chats')
  })
})
