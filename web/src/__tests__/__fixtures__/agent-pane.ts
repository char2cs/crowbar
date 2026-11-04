import { act, render } from '@testing-library/react'
import { createElement } from 'react'
import { vi } from 'vitest'
import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'
import * as dropdown from '@/features/agent/components/provider-switch-dropdown'
import * as keymap from '@/features/keymaps/hooks/use-effective-keymap'
import * as reconnect from '@/features/terminal/lib/terminal-reconnect-map'
import { toast } from '@/features/window/stores/toast-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/** The pane chrome a surface test does not look at: no chords, no reconnect
 *  writes, silent toasts, and a provider switcher reduced to a marker. */
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

/** Chat c1 on runner r1 in w1, as one pane; each visibility axis settable alone. */
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

export const renderChatPane = (store: WorkspaceStore, props: PaneProps = {}) =>
  act(async () => render(chatPane(store, props)))
