import { act, render } from '@testing-library/react'
import { createElement } from 'react'
import { vi } from 'vitest'
import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'
import * as dropdown from '@/features/agent/components/provider-switch-dropdown'
import * as keymap from '@/features/keymaps/hooks/use-effective-keymap'
import * as reconnect from '@/features/terminal/lib/terminal-reconnect-map'
import { toast } from '@/features/window/stores/toast-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { cacheReadsSettled } from '@/__tests__/__fixtures__/feed-cache-reads'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

export function stubPaneChrome(): void {
  vi.spyOn(keymap, 'useEffectiveChordMap').mockImplementation(() => ({}))
  vi.spyOn(reconnect, 'saveReconnect').mockImplementation(() => {})
  vi.spyOn(toast, 'error').mockImplementation(() => '')
  vi.spyOn(dropdown, 'ProviderSwitchDropdown').mockImplementation(() =>
    createElement('button', { 'data-testid': 'provider-switch' }),
  )
}

interface PaneProps {
  isActivePane?: boolean
  isVisible?: boolean
  belowOverlayHeader?: boolean
}

export function chatPane(store: WorkspaceStore, props: PaneProps = {}) {
  return createElement(
    WorkspaceStoreContext.Provider,
    { value: store },
    createElement(AgentChatPaneGate, {
      chatId: 'c1',
      runnerId: 'r1',
      wsId: 'w1',
      paneId: 'b1',
      isActivePane: props.isActivePane ?? true,
      isVisible: props.isVisible ?? true,
      belowOverlayHeader: props.belowOverlayHeader ?? false,
    }),
  )
}

export const renderChatPane = async (store: WorkspaceStore, props: PaneProps = {}) => {
  const view = await act(async () => render(chatPane(store, props)))
  await cacheReadsSettled()
  return view
}
