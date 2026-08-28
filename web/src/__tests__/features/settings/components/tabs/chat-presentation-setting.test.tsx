/**
 * The "Chat By Default" control (spec §11): the one visible thing that decides
 * which surface a chat lands on, sitting in the Agents tab beside the agents it
 * applies to.
 *
 * The interesting assertion is the one about turning it OFF. The row has to stay
 * exactly as present and as reversible with Chat de-selected as with it selected,
 * because the preference is a landing choice — an off switch that quietly removed
 * the control, or the Chat surface, would be a different feature.
 */
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-api', () => ({
  updateProviderPreferences: vi.fn(),
  listProviders: vi.fn().mockResolvedValue([]),
  getDefaultPermissionLevel: vi.fn().mockResolvedValue('guarded'),
  updateDefaultPermissionLevel: vi.fn(),
  PERMISSION_LEVEL_OPTIONS: [
    { value: 'guarded', label: 'Guarded' },
    { value: 'trusted', label: 'Trusted' },
    { value: 'full-auto', label: 'Full Auto' },
  ],
}))

// jsdom has no PointerEvent constructor; base-ui's Switch toggles by dispatching
// a PointerEvent('click') on its hidden checkbox. A MouseEvent-backed shim is
// enough — the checkbox's click activation still toggles and fires change.
if (typeof window.PointerEvent !== 'function') {
  class PointerEventShim extends MouseEvent {}
  // @ts-expect-error install the shim onto the jsdom window
  window.PointerEvent = PointerEventShim
}

import { ChatPresentationSetting } from '@/features/settings/components/tabs/chat-presentation-setting'
import { ProvidersSettings } from '@/features/settings/components/tabs/providers-settings'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { getDefaultChatPresentation } from '@/features/settings/lib/chat-presentation'
import { useSettingsStore } from '@/features/settings/store'

const toggle = () => screen.getByTestId('chat-default-presentation-toggle')

beforeEach(() => {
  localStorage.clear()
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
})

afterEach(() => {
  cleanup()
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  localStorage.clear()
})

describe('Chat By Default', () => {
  it('sits in the Agents tab, with the agents it applies to', () => {
    render(<ProvidersSettings />)
    expect(screen.getByRole('heading', { name: 'Agents' })).toBeInTheDocument()
    expect(toggle()).toBeInTheDocument()
  })

  it('is on before the user has touched it', () => {
    render(<ChatPresentationSetting />)
    expect(toggle()).toHaveAttribute('aria-checked', 'true')
  })

  it('says plainly that it only picks the surface you land on', () => {
    render(<ChatPresentationSetting />)
    expect(screen.getByText(/only picks the one you land on/i)).toBeInTheDocument()
    expect(screen.getByText(/both surfaces stay in every chat/i)).toBeInTheDocument()
  })

  it('turns off, and the chat then lands on Terminal', async () => {
    const user = userEvent.setup()
    render(<ChatPresentationSetting />)

    await user.click(toggle())

    expect(toggle()).toHaveAttribute('aria-checked', 'false')
    expect(useSettingsStore.getState().settings.chatIsDefaultPresentation).toBe(false)
    expect(getDefaultChatPresentation()).toBe('terminal')
  })

  // Off is a preference, not a removal: the control is still there, still says
  // what it does, and one more click puts Chat back in front.
  it('keeps Chat one click away once it is off', async () => {
    const user = userEvent.setup()
    render(<ChatPresentationSetting />)

    await user.click(toggle())
    expect(toggle()).toBeInTheDocument()
    expect(toggle()).toBeEnabled()
    expect(screen.getByText(/both surfaces stay in every chat/i)).toBeInTheDocument()

    await user.click(toggle())
    expect(toggle()).toHaveAttribute('aria-checked', 'true')
    expect(getDefaultChatPresentation()).toBe('chat')
  })

  it('offers a reset once it differs from the default, and not before', async () => {
    const user = userEvent.setup()
    render(<ChatPresentationSetting />)

    // Re-queried each time on purpose: SettingRow gives the reset button a
    // tooltip only while it can act, so the enabled and disabled buttons are
    // different DOM nodes rather than one node with an attribute flipped.
    const reset = () => screen.getByRole('button', { name: 'Reset Chat By Default' })
    expect(reset()).toBeDisabled()

    await user.click(toggle())
    expect(reset()).toBeEnabled()

    await user.click(reset())
    expect(toggle()).toHaveAttribute('aria-checked', 'true')
    expect(getDefaultChatPresentation()).toBe('chat')
  })
})
