/**
 * The switch behind the chat's Split surface: a DIAGNOSTIC, which is the whole
 * reason it lives in Developer and starts off.
 *
 * Two gates, and both have to be asserted separately because either one alone
 * would be a hole. The BUILD gate is the Developer tab itself, which
 * settings-tab-items only appends under `import.meta.env.DEV`. The PREFERENCE
 * gate is this switch, and its default is the thing a regression would quietly
 * flip — two live surfaces cost real work, and nobody should pay for that
 * without having asked.
 */
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

// jsdom has no PointerEvent constructor; base-ui's Switch toggles by dispatching
// a PointerEvent('click') on its hidden checkbox. A MouseEvent-backed shim is
// enough — the checkbox's click activation still toggles and fires change.
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
import {
  SPLIT_PRESENTATION_AVAILABLE,
  useSplitPresentationEnabled,
} from '@/features/settings/lib/chat-presentation'
import {
  normalizeSettingValue,
  normalizeSettings,
} from '@/features/settings/lib/settings-normalization'
import type { Settings } from '@/features/settings/types/settings'
import { useSettingsStore } from '@/features/settings/store'

const toggle = () => screen.getByTestId('chat-split-view-toggle')

/** Read the hook the pane's switcher is gated on, without standing up a pane. */
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

  // The build gate. The row is rendered by DeveloperSettings, and the Developer
  // tab only exists in a development build — so in production there is no
  // control at all, which is the point.
  it('rides on the Developer tab, which only a development build has', () => {
    expect(SETTINGS_TAB_ITEMS.some((tab) => tab.id === 'developer')).toBe(
      import.meta.env.DEV === true,
    )
    expect(SPLIT_PRESENTATION_AVAILABLE).toBe(import.meta.env.DEV === true)
  })

  // Both gates, together. Off, the surface is not on offer no matter what build
  // this is; on, it is on offer exactly when the build allows it.
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
    expect(gate()).toBe(String(SPLIT_PRESENTATION_AVAILABLE))
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

  // An older profile, or a hand-edited export, carries no choice here — which is
  // not the same as choosing to turn a diagnostic on.
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
