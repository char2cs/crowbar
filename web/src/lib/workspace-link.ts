import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { toast } from '@/features/window/stores/toast-store'
import { openExternalUrl } from '@/lib/external-open'
import { resolvedHostOf } from '@/lib/host-resolve'

// localhost and its subdomains, 127.0.0.0/8, 0.0.0.0 and [::1], as URL parsing normalises them.
const LOOPBACK_HOSTNAME = /^(?:localhost|.+\.localhost|127(?:\.\d{1,3}){3}|0\.0\.0\.0|\[::1\])\.?$/i

function isLoopbackUrl(url: string): boolean {
  try {
    return LOOPBACK_HOSTNAME.test(new URL(url, 'http://relative.invalid').hostname)
  } catch {
    return false
  }
}

/**
 * Opens a link found in workspace `wsId`'s content (null: content of no workspace). A loopback
 * address from a workspace on another host — or one whose host is not known — is that host's,
 * so it is not opened on this computer.
 */
export async function openWorkspaceLink(url: string, wsId: string | null): Promise<void> {
  if (wsId !== null && isLoopbackUrl(url)) {
    const hostId = resolvedHostOf({ wsId })
    if (hostId !== LOCAL_HOST.id) {
      const name = useHostConfigStore.getState().hosts.find((host) => host.id === hostId)?.name
      toast.info(
        `This link is on ${name ?? 'its workspace’s host'}, not this computer — forward the port with ssh -L to open it here`,
      )
      return
    }
  }
  await openExternalUrl(url)
}
