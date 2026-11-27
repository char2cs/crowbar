import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

if (typeof window.PointerEvent !== 'function') {
  class PointerEventShim extends MouseEvent {}
  // @ts-expect-error install the shim onto the jsdom window
  window.PointerEvent = PointerEventShim
}

import { ChatSplitViewSetting } from '@/features/settings/components/tabs/chat-split-view-setting'
import { SETTINGS_TAB_ITEMS } from '@/features/settings/components/settings-tab-items'
import {
  defaultSettings,
  getDefaultSettingsSnapshot,
} from '@/features/settings/config/default-settings'
import { useSplitPresentationEnabled } from '@/features/settings/lib/chat-presentation'
import {
  normalizeSettingValue,
  normalizeSettings,
} from '@/features/settings/lib/settings-normalization'
import type { Settings } from '@/features/settings/types/settings'
import { useSettingsStore } from '@/features/settings/store'

const toggle = () => screen.getByTestId('chat-split-view-toggle')

function SplitGate() {
  return <span data-testid="gate">{String(useSplitPresentationEnabled())}</span>
}
const gate = () => screen.getByTestId('gate').textContent

beforeEach(() => {
  localStorage.clear()
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
})

afterEach(() => {
  cleanup()
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  localStorage.clear()
})

describe('the split-view switch', () => {
  it('is off before anybody asks for it', () => {
    expect(defaultSettings.chatSplitPresentationEnabled).toBe(false)
    render(<ChatSplitViewSetting />)
    expect(toggle()).toHaveAttribute('aria-checked', 'false')
  })

  it('rides on the Developer tab, which only a development build has', () => {
    expect(SETTINGS_TAB_ITEMS.some((tab) => tab.id === 'developer')).toBe(
      import.meta.env.DEV === true,
    )
  })

  it('opens the surface only once the switch and the build agree', async () => {
    const user = userEvent.setup()
    render(
      <>
        <ChatSplitViewSetting />
        <SplitGate />
      </>,
    )

    expect(gate()).toBe('false')

    await user.click(toggle())

    expect(useSettingsStore.getState().settings.chatSplitPresentationEnabled).toBe(true)
    expect(gate()).toBe(String(import.meta.env.DEV === true))
  })

  it('says what it costs, so nobody leaves it on by accident', () => {
    render(<ChatSplitViewSetting />)
    expect(screen.getByText(/both surfaces render at once/i)).toBeInTheDocument()
  })

  it('offers a reset once it differs from the default, and not before', async () => {
    const user = userEvent.setup()
    render(<ChatSplitViewSetting />)

    const reset = () => screen.getByRole('button', { name: 'Reset Split view' })
    expect(reset()).toBeDisabled()

    await user.click(toggle())
    expect(reset()).toBeEnabled()

    await user.click(reset())
    expect(toggle()).toHaveAttribute('aria-checked', 'false')
    expect(useSettingsStore.getState().settings.chatSplitPresentationEnabled).toBe(false)
  })

  it.each([undefined, null, 'true', 1])(
    'treats a stored %p as no choice and stays off',
    (stored) => {
      const settings = {
        ...getDefaultSettingsSnapshot(),
        chatSplitPresentationEnabled: stored,
      } as unknown as Settings

      expect(normalizeSettings(settings).chatSplitPresentationEnabled).toBe(false)
      expect(
        normalizeSettingValue('chatSplitPresentationEnabled', stored as unknown as boolean),
      ).toBe(false)
    },
  )
})
