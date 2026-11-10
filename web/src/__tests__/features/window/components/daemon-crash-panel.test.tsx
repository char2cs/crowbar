import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as bridge from '@/lib/crowbar-bridge'
import { DaemonCrashPanel } from '@/features/window/components/daemon-crash-panel'
import { useDaemonCrashStore } from '@/features/window/stores/daemon-crash-store'
import { toast } from '@/features/window/stores/toast-store'

const REPORT = 'Crowbar 1.2.3 on linux x86_64\n[err] panic: boom'

describe('DaemonCrashPanel', () => {
  beforeEach(() => {
    useDaemonCrashStore.getState().clear()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('is absent while the supervisor still restarts the backend', () => {
    const { container } = render(<DaemonCrashPanel />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows why the backend keeps crashing', () => {
    useDaemonCrashStore.getState().crashed(REPORT)
    render(<DaemonCrashPanel />)
    expect(screen.getByRole('alert', { name: /keeps crashing/i })).toBeInTheDocument()
    expect(screen.getByText(/panic: boom/)).toBeInTheDocument()
  })

  it('restarts the backend, and says why when that fails', async () => {
    const restart = vi
      .spyOn(bridge, 'restartDaemon')
      .mockRejectedValueOnce(new Error('daemon did not become healthy in 30 attempts'))
      .mockResolvedValueOnce(undefined)
    useDaemonCrashStore.getState().crashed(REPORT)
    render(<DaemonCrashPanel />)

    await userEvent.click(screen.getByRole('button', { name: 'Restart backend' }))
    expect(await screen.findByText(/did not become healthy/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Restart backend' }))
    expect(restart).toHaveBeenCalledTimes(2)
    await waitFor(() => expect(screen.queryByText(/did not become healthy/)).toBeNull())
    // The supervisor's `daemon:restarted` is what clears the crash and takes the panel down.
    act(() => useDaemonCrashStore.getState().clear())
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('opens the daemon log', async () => {
    const reveal = vi.spyOn(bridge, 'revealDaemonLog').mockResolvedValue(undefined)
    useDaemonCrashStore.getState().crashed(REPORT)
    render(<DaemonCrashPanel />)
    await userEvent.click(screen.getByRole('button', { name: 'Open log' }))
    expect(reveal).toHaveBeenCalledTimes(1)
  })

  it('copies the report as diagnostics', async () => {
    const user = userEvent.setup()
    const success = vi.spyOn(toast, 'success')
    useDaemonCrashStore.getState().crashed(REPORT)
    render(<DaemonCrashPanel />)
    await user.click(screen.getByRole('button', { name: 'Copy diagnostics' }))
    expect(await navigator.clipboard.readText()).toBe(REPORT)
    await waitFor(() => expect(success).toHaveBeenCalledWith('Diagnostics copied'))
  })
})
