import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { toast } from '@/features/window/stores/toast-store'
import { openExternalUrl } from '@/lib/external-open'
import { resolvedHostOf } from '@/lib/host-resolve'

const LOOPBACK_HOSTNAME =
  /^(?:localhost|.+\.localhost|127(?:\.\d{1,3}){3}|0\.0\.0\.0|\[::1?\]|\[::ffff:7f[0-9a-f]{2}:[0-9a-f]{1,4}\])\.?$/i

function isLoopbackUrl(url: string): boolean {
  try {
    return LOOPBACK_HOSTNAME.test(new URL(url, 'http://relative.invalid').hostname)
  } catch {
    return false
  }
}

function webUrl(link: string): string {
  return link.startsWith('//') ? `https:${link}` : link
}

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

export async function openWorkspaceLink(link: string, wsId: string | null): Promise<void> {
  if (refuseRemoteLoopback(link, wsId)) return
  await openExternalUrl(webUrl(link))
}
