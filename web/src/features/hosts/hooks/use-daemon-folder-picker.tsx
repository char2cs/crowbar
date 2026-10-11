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

/** Picks a folder on `hostId`'s own disk: the OS dialog for Local (as always), the remote picker
 *  for a paired host, nothing for `null` (unresolved — never a guess of Local). `onPick` names the
 *  host the path was picked on: a slow dialog can answer after the selection changed. */
export function useDaemonFolderPicker(
  hostId: string | null,
  onPick: (path: string, pickedOn: string) => void,
): DaemonFolderPicker {
  const [openFor, setOpenFor] = useState<string | null>(null)
  const remote = hostId !== null && hostId !== LOCAL_HOST.id

  async function pickLocal() {
    const selected = await openNativeDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') onPick(selected, LOCAL_HOST.id)
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
      onSelect={(path) => onPick(path, hostId)}
    />
  ) : null
  return { pick, picker }
}
