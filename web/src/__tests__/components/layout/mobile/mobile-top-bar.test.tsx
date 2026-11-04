import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { MobileTopBar } from '@/components/layout/mobile/mobile-top-bar'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { AgentTelemetry } from '@/features/agent/api/agent-api'

const titleBox = () => screen.getByRole('banner').firstElementChild as HTMLElement

beforeEach(() => {
  resetWindowPaneStoreForTests()
  act(() =>
    windowPaneStore.getState().paneActions.openChat('c1', { projectId: 'p1', workspaceId: 'ws-1' }),
  )
})
afterEach(() => resetWindowPaneStoreForTests())

describe('MobileTopBar', () => {
  // The title sits beside the chat-list button and stops short of the native context meter.
  it('keeps the title clear of the context meter while the focused chat reports usage', () => {
    render(<MobileTopBar />)
    expect(titleBox().style.paddingLeft).toBe('68px')
    expect(titleBox().style.paddingRight).toBe('68px')
    act(() =>
      getOrCreateWorkspaceStore('ws-1')
        .getState()
        .setAgentChatTelemetry('c1', { context: { usedPercent: 6 } } as AgentTelemetry),
    )
    expect(titleBox().style.paddingRight).toBe('148px')
  })
})
