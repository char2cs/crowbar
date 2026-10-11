import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch: vi.fn(),
}))
vi.mock('@/lib/native-dialog', () => ({ openNativeDialog: vi.fn() }))

import { apiFetch } from '@/lib/api'
import { openNativeDialog } from '@/lib/native-dialog'
import { useDaemonFolderPicker } from '@/features/hosts/hooks/use-daemon-folder-picker'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

type OnPick = (path: string, pickedOn: string) => void

function Harness({ hostId, onPick }: { hostId: string | null; onPick: OnPick }) {
  const { pick, picker } = useDaemonFolderPicker(hostId, onPick)
  return (
    <>
      <button type="button" onClick={pick}>
        Browse
      </button>
      {picker}
    </>
  )
}

const host = (id: string, name: string) => ({
  id,
  name,
  endpoint: `${id}:3737`,
  pairedFingerprint: 'fp',
  clientId: 'c',
})

beforeEach(() => {
  vi.mocked(openNativeDialog).mockReset()
  vi.mocked(apiFetch).mockReset()
  vi.mocked(apiFetch).mockResolvedValue([{ name: 'srv', path: '/srv', isDir: true }] as never)
  useHostConfigStore.setState({ hosts: [host('h1', 'homelab-prox'), host('h2', 'office-vm')] })
  getOrCreateHostRuntime('h1').getState().setStatus('up')
  getOrCreateHostRuntime('h2').getState().setStatus('up')
})

describe('useDaemonFolderPicker', () => {
  it('the local daemon picks with the OS dialog, as it always did', async () => {
    vi.mocked(openNativeDialog).mockResolvedValue('/Users/me/code')
    const onPick = vi.fn()
    render(<Harness hostId="local" onPick={onPick} />)

    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))

    await waitFor(() => expect(onPick).toHaveBeenCalledWith('/Users/me/code', 'local'))
    expect(openNativeDialog).toHaveBeenCalledExactlyOnceWith({ directory: true, multiple: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('a dismissed OS dialog picks nothing', async () => {
    vi.mocked(openNativeDialog).mockResolvedValue(null)
    const onPick = vi.fn()
    render(<Harness hostId="local" onPick={onPick} />)

    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))

    await waitFor(() => expect(openNativeDialog).toHaveBeenCalled())
    expect(onPick).not.toHaveBeenCalled()
  })

  it('a remote host picks in the remote picker on that host, never the OS dialog', async () => {
    const onPick = vi.fn()
    render(<Harness hostId="h1" onPick={onPick} />)

    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    const picker = await screen.findByRole('dialog', { name: 'Choose a folder' })
    fireEvent.click(await within(picker).findByRole('treeitem', { name: 'srv' }))
    fireEvent.click(within(picker).getByRole('button', { name: 'Select "srv"' }))

    expect(onPick).toHaveBeenCalledWith('/srv', 'h1')
    expect(openNativeDialog).not.toHaveBeenCalled()
    expect(vi.mocked(apiFetch).mock.calls.every((call) => call[3] === 'h1')).toBe(true)
  })

  // The OS dialog can answer after the daemon changed: its answer is still a path on this machine.
  it('a late OS dialog answer names the local daemon, whatever is selected by then', async () => {
    let answer!: (path: string) => void
    vi.mocked(openNativeDialog).mockReturnValue(new Promise((resolve) => (answer = resolve)))
    const onPick = vi.fn()
    const { rerender } = render(<Harness hostId="local" onPick={onPick} />)
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))

    rerender(<Harness hostId="h1" onPick={onPick} />)
    await act(async () => answer('/Users/me/code'))

    expect(onPick).toHaveBeenCalledExactlyOnceWith('/Users/me/code', 'local')
  })

  it('a host not resolved yet picks nothing — neither Local nor a guess', () => {
    render(<Harness hostId={null} onPick={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))

    expect(openNativeDialog).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(apiFetch).not.toHaveBeenCalled()
  })

  // Whatever the host passes through on the way back — another host, or none resolved.
  it.each(['h2', null])(
    'a picker closed by a host change stays closed when that host comes back (via %s)',
    async (between) => {
      const { rerender } = render(<Harness hostId="h1" onPick={vi.fn()} />)
      fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
      await screen.findByRole('dialog', { name: 'Choose a folder' })

      rerender(<Harness hostId={between} onPick={vi.fn()} />)
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      rerender(<Harness hostId="h1" onPick={vi.fn()} />)

      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    },
  )
})
