import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { toast } from '@/features/window/stores/toast-store'
import { openExternalUrl } from '@/lib/external-open'
import { resolvedHostOf } from '@/lib/host-resolve'

// localhost and its subdomains, 127.0.0.0/8, 0.0.0.0, [::1], [::] and IPv4-mapped 127.0.0.0/8,
// as URL parsing normalises them (`[::ffff:127.0.0.1]` reads `[::ffff:7f00:1]`).
const LOOPBACK_HOSTNAME =
  /^(?:localhost|.+\.localhost|127(?:\.\d{1,3}){3}|0\.0\.0\.0|\[::1?\]|\[::ffff:7f[0-9a-f]{2}:[0-9a-f]{1,4}\])\.?$/i

function isLoopbackUrl(url: string): boolean {
  try {
    return LOOPBACK_HOSTNAME.test(new URL(url, 'http://relative.invalid').hostname)
  } catch {
    return false
  }
}

// No page scheme for a protocol-relative `//host/…` to inherit here: it is a web link.
function webUrl(link: string): string {
  return link.startsWith('//') ? `https:${link}` : link
}

/**
 * True, once the user is told why, when a link found in workspace `wsId`'s content (null:
 * content of no workspace) must not open on this computer: a loopback address from a
 * workspace on another host — or one whose host is not known — is that host's.
 */
export function refuseRemoteLoopback(link: string, wsId: string | null): boolean {
  if (wsId === null || !isLoopbackUrl(webUrl(link))) return false
  const hostId = resolvedHostOf({ wsId })
  if (hostId === LOCAL_HOST.id) return false
  const name = useHostConfigStore.getState().hosts.find((host) => host.id === hostId)?.name
  toast.info(
    `This link is on ${name ?? 'its workspace’s host'}, not this computer — forward the port with ssh -L to open it here`,
  )
  return true
}

/** Opens a link found in workspace `wsId`'s content in the OS browser, unless it is refused. */
export async function openWorkspaceLink(link: string, wsId: string | null): Promise<void> {
  if (refuseRemoteLoopback(link, wsId)) return
  await openExternalUrl(webUrl(link))
}
