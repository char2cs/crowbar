import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { MobileTopBar } from '@/components/layout/mobile/mobile-top-bar'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { applyEntityFrame } from '@/core/entities/entity-store'
import { resetChatStore } from '@/__tests__/__fixtures__/agent-chat'

const titleBox = () => screen.getByRole('banner').firstElementChild as HTMLElement

beforeEach(() => {
  resetWindowPaneStoreForTests()
  resetChatStore()
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
      applyEntityFrame('local', {
        t: 'entity',
        kind: 'telemetry',
        id: 'c1',
        scope: 'chat:c1',
        v: 1,
        data: {
          observedAt: '2026-08-24T12:00:00Z',
          source: 'callback',
          context: { usedPercent: 6 },
        },
      }),
    )
    expect(titleBox().style.paddingRight).toBe('148px')
  })
})
