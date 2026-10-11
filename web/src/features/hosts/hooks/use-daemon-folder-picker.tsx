import { useState, type ReactNode } from 'react'
import { RemoteFilePicker } from '@/features/hosts/components/remote-file-picker'
import { LOCAL_HOST } from '@/features/hosts/types'
import { openNativeDialog } from '@/lib/native-dialog'

interface DaemonFolderPicker {
  /** Opens the picker; does nothing while the host is not known. */
  pick: () => void
  /** The remote picker's dialog, to render; null for the local daemon. */
  picker: ReactNode
}

/**
 * Picks a folder on `hostId`'s own filesystem: the OS dialog for the local daemon, as it always
 * was, and the remote picker for a paired host — whose disk no OS dialog can reach. A `null`
 * host (not resolved yet) picks nothing rather than guessing Local.
 */
export function useDaemonFolderPicker(
  hostId: string | null,
  onPick: (path: string) => void,
): DaemonFolderPicker {
  const [openFor, setOpenFor] = useState<string | null>(null)
  const remote = hostId !== null && hostId !== LOCAL_HOST.id

  async function pickLocal() {
    const selected = await openNativeDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') onPick(selected)
  }

  function pick() {
    if (hostId === LOCAL_HOST.id) void pickLocal()
    else if (remote) setOpenFor(hostId)
  }

  const picker = remote ? (
    <RemoteFilePicker
      hostId={hostId}
      open={openFor === hostId}
      onOpenChange={(open) => setOpenFor(open ? hostId : null)}
      onSelect={onPick}
    />
  ) : null
  return { pick, picker }
}
