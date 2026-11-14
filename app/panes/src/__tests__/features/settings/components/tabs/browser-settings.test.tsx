import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BrowserSettings } from '@/features/settings/components/tabs/browser-settings'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'

const api = vi.hoisted(() => ({
  capabilities: vi.fn(),
  clearData: vi.fn(),
}))
vi.mock('@/lib/browser-bridge', () => ({ browserApi: api }))

const confirm = vi.hoisted(() => vi.fn())
vi.mock('@/components/ui/primitive-dialog-service', () => ({ primitiveConfirm: confirm }))

beforeEach(() => {
  api.capabilities.mockResolvedValue({ clearData: true })
  api.clearData.mockResolvedValue(undefined)
  act(() => useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() }))
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('BrowserSettings', () => {
  it('stores the homepage the user types', async () => {
    render(<BrowserSettings />)
    await userEvent.type(screen.getByLabelText('Homepage'), 'example.com')
    expect(useSettingsStore.getState().settings.browserHomepage).toBe('example.com')
  })

  it('shows the custom search url row only for the custom engine', () => {
    render(<BrowserSettings />)
    expect(screen.queryByText('Custom Search URL')).toBeNull()
    cleanup()
    act(() =>
      useSettingsStore.setState({
        settings: { ...getDefaultSettingsSnapshot(), browserSearchEngine: 'custom' },
      }),
    )
    render(<BrowserSettings />)
    expect(screen.getByText('Custom Search URL')).toBeTruthy()
  })

  it('clears browsing data only after the user confirms', async () => {
    confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true)
    render(<BrowserSettings />)
    const button = await screen.findByRole('button', { name: /clear browsing data/i })
    await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false))

    await userEvent.click(button)
    expect(api.clearData).not.toHaveBeenCalled()

    await userEvent.click(button)
    await waitFor(() => expect(api.clearData).toHaveBeenCalledTimes(1))
  })

  it('disables clearing and says why where the system cannot do it safely', async () => {
    api.capabilities.mockResolvedValue({ clearData: false })
    render(<BrowserSettings />)
    const button = await screen.findByRole('button', { name: /clear browsing data/i })
    expect((button as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByText(/would erase app data too/)).toBeTruthy()
    expect(api.clearData).not.toHaveBeenCalled()
  })
})
