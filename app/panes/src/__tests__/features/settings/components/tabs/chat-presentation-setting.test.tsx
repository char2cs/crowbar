import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-providers-api', () => ({
  updateProviderPreferences: vi.fn(),
  listProviders: vi.fn().mockResolvedValue([]),
  listGlobalProviders: vi.fn().mockResolvedValue([]),
  getDefaultPermissionLevel: vi.fn().mockResolvedValue('guarded'),
  updateDefaultPermissionLevel: vi.fn(),
  getDescriptorReports: vi.fn().mockResolvedValue([]),
  PERMISSION_LEVEL_OPTIONS: [
    { value: 'guarded', label: 'Guarded' },
    { value: 'trusted', label: 'Trusted' },
    { value: 'full-auto', label: 'Full Auto' },
  ],
}))

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

    const reset = () => screen.getByRole('button', { name: 'Reset Chat By Default' })
    expect(reset()).toBeDisabled()

    await user.click(toggle())
    expect(reset()).toBeEnabled()

    await user.click(reset())
    expect(toggle()).toHaveAttribute('aria-checked', 'true')
    expect(getDefaultChatPresentation()).toBe('chat')
  })
})
